package app

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/vantt/mcp-skill-hub/internal/catalog"
	resolverpkg "github.com/vantt/mcp-skill-hub/internal/resolver"
	"github.com/vantt/mcp-skill-hub/internal/telemetry"
	"github.com/vantt/mcp-skill-hub/internal/transcripts"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
)

var telemetryTokenPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:@+\-]{0,255}$`)

func isValidTelemetryToken(s string) bool {
	return telemetryTokenPattern.MatchString(strings.TrimSpace(s)) && s == strings.TrimSpace(s)
}

// ImportInput specifies project location, time window, and optional config directory.
type ImportInput struct {
	Project   string
	Since     time.Time
	Until     time.Time
	ConfigDir string
}

// ImportResult summarizes transcript scanning and insertion metrics.
type ImportResult struct {
	FilesScanned int            `json:"files_scanned"`
	LinesSkipped int            `json:"lines_skipped"`
	Observations int            `json:"observations"`
	Inserted     int            `json:"inserted"`
	Duplicates   int            `json:"duplicates"`
	PerTool      map[string]int `json:"per_tool"`
	Summary      string         `json:"summary,omitempty"`
}

// TranscriptImportService imports tool use observations from local Claude Code transcripts.
type TranscriptImportService struct {
	Telemetry TelemetryService
	Now       func() time.Time
}

func transcriptEventID(toolUseID string) string {
	sum := sha256.Sum256([]byte(toolUseID))
	return "transcript-" + hex.EncodeToString(sum[:])[:32]
}

func sessionIDHash(sessionID string) string {
	sum := sha256.Sum256([]byte("claude-code:" + sessionID))
	return hex.EncodeToString(sum[:])[:32]
}

func queryExistingEventIDs(dbPath string, ids []string) (map[string]bool, error) {
	if _, err := os.Stat(dbPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return make(map[string]bool), nil
		}
		return nil, err
	}

	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath)+"?mode=ro")
	if err != nil {
		return nil, err
	}
	defer func() { _ = db.Close() }()

	existing := make(map[string]bool)
	stmt, err := db.Prepare(`SELECT id FROM telemetry_events WHERE id = ?`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = stmt.Close() }()

	for _, id := range ids {
		var found string
		if err := stmt.QueryRow(id).Scan(&found); err == nil && found != "" {
			existing[found] = true
		}
	}
	return existing, nil
}

func normalizeImportWindow(since, until, now time.Time) (time.Time, time.Time) {
	rawCutoff := now.Add(-time.Duration(FunnelRawRetentionDays) * 24 * time.Hour)
	if since.IsZero() || since.Before(rawCutoff) {
		since = rawCutoff
	}
	if until.IsZero() {
		until = now
	}
	return since, until
}

func fetchCatalogMetadata(ctx context.Context, root string) (string, string) {
	handle, err := catalog.OpenCurrent(ctx, root)
	if err != nil {
		return "", ""
	}
	defer func() { _ = handle.Close() }()
	snapshot := handle.Pointer.CatalogSnapshot
	policy, err := resolverpkg.LoadPolicy(ctx, handle.DB)
	if err != nil {
		return snapshot, ""
	}
	return snapshot, policy.Revision
}

func recordTranscriptEvent(recorder *telemetry.Recorder, obs transcripts.Observation, eventID, sessionHash, snapshot, policyRev string) {
	payload := map[string]any{
		"tool":            obs.Tool,
		"source":          telemetry.TranscriptSourceClaude,
		"basis":           telemetry.TranscriptBasis,
		"resolved_before": obs.ResolvedBefore,
	}
	if obs.SkillID != "" && isValidTelemetryToken(obs.SkillID) {
		payload["skill_id"] = obs.SkillID
	}

	event := telemetry.Event{
		Version:         telemetry.EventVersion,
		ID:              eventID,
		Type:            telemetry.EventTranscriptToolObserved,
		OccurredAt:      obs.At,
		SessionIDHash:   sessionHash,
		CatalogSnapshot: snapshot,
		PolicyRevision:  policyRev,
		Client:          telemetry.Client{Name: "skillhub"},
		Payload:         payload,
	}
	recorder.Record(event)
}

// Import reads transcripts for the specified project, extracts skill tool observations, and records them.
func (service TranscriptImportService) Import(ctx context.Context, path string, in ImportInput) (ImportResult, error) {
	if in.Project == "" {
		return ImportResult{}, NewInvalidRequestError("project path is required", "Pass `--project <dir>`.")
	}

	projectRoot, err := filepath.Abs(in.Project)
	if err != nil {
		return ImportResult{}, err
	}

	root, err := workspace.Discover(path)
	if err != nil {
		return ImportResult{}, err
	}

	now := time.Now().UTC()
	if service.Now != nil {
		now = service.Now().UTC()
	}

	since, until := normalizeImportWindow(in.Since, in.Until, now)

	dirs, err := transcripts.ProjectDirs(in.ConfigDir, projectRoot)
	if err != nil {
		return ImportResult{}, err
	}
	validCwds := transcripts.ValidCwds(projectRoot)

	scanResult, err := transcripts.Scan(dirs, since, until, validCwds)
	if err != nil {
		return ImportResult{}, err
	}

	perTool := make(map[string]int)
	for _, obs := range scanResult.Observations {
		perTool[obs.Tool]++
	}

	res := ImportResult{
		FilesScanned: scanResult.FilesScanned,
		LinesSkipped: scanResult.LinesSkipped,
		Observations: len(scanResult.Observations),
		PerTool:      perTool,
	}

	if len(scanResult.Observations) == 0 {
		res.Summary = "0 transcript observations imported."
		return res, nil
	}

	eventIDs := make([]string, len(scanResult.Observations))
	for i, obs := range scanResult.Observations {
		eventIDs[i] = transcriptEventID(obs.ToolUseID)
	}

	dbPath := filepath.Join(root, "runtime", "telemetry.db")
	existingIDs, err := queryExistingEventIDs(dbPath, eventIDs)
	if err != nil {
		return ImportResult{}, err
	}

	recorder, err := service.Telemetry.Open(root)
	if err != nil {
		return ImportResult{}, err
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = recorder.Close(closeCtx)
	}()

	snapshot, policyRev := fetchCatalogMetadata(ctx, root)

	for i, obs := range scanResult.Observations {
		eID := eventIDs[i]
		if existingIDs[eID] {
			res.Duplicates++
			continue
		}
		recordTranscriptEvent(recorder, obs, eID, sessionIDHash(obs.SessionID), snapshot, policyRev)
		res.Inserted++
	}

	if err := recorder.Flush(ctx); err != nil {
		return ImportResult{}, err
	}

	res.Summary = fmt.Sprintf("Imported %d transcript observations (%d new, %d duplicate).",
		res.Observations, res.Inserted, res.Duplicates)
	return res, nil
}
