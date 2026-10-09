package telemetry

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ErrCaseConflict indicates an event_id was already used for a case record.
var ErrCaseConflict = errors.New("event_id was already used for a case record")

// CaseRecord stores the rich context of a resolution disagreement.
type CaseRecord struct {
	CaseID          string         `json:"case_id"`
	OccurredAt      time.Time      `json:"occurred_at"`
	Kind            string         `json:"kind"`
	SessionHash     string         `json:"session_hash,omitempty"`
	ResolutionID    string         `json:"resolution_id,omitempty"`
	EventID         string         `json:"event_id,omitempty"`
	Client          Client         `json:"client"`
	CatalogSnapshot string         `json:"catalog_snapshot"`
	PriorVerified   bool           `json:"prior_verified"`
	Task            map[string]any `json:"task"`
	Operation       string         `json:"operation"`
	Request         map[string]any `json:"request"`
	Resolver        map[string]any `json:"resolver"`
	Chosen          string         `json:"chosen"`
	Followup        map[string]any `json:"followup,omitempty"`
}

func randomCaseID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return "case_" + hex.EncodeToString(raw[:]), nil
}

// IsCaseJournalEnabled reports whether the case journal opt-in is active.
func IsCaseJournalEnabled(workspaceRoot string) bool {
	if v := os.Getenv("SKILLHUB_CASE_JOURNAL"); v != "" {
		v = strings.ToLower(strings.TrimSpace(v))
		if v == "1" || v == "true" || v == "yes" || v == "on" {
			return true
		}
		if v == "0" || v == "false" || v == "no" || v == "off" {
			return false
		}
	}
	if workspaceRoot == "" {
		return false
	}
	path := filepath.Join(workspaceRoot, "runtime", "case_journal.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var state struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.Unmarshal(b, &state); err != nil {
		return false
	}
	return state.Enabled
}

// SetCaseJournalEnabled sets the runtime flag enabling or disabling case journal logging.
func SetCaseJournalEnabled(workspaceRoot string, enabled bool) error {
	if workspaceRoot == "" {
		return errors.New("workspace root is required")
	}
	runtimeDir := filepath.Join(workspaceRoot, "runtime")
	if err := os.MkdirAll(runtimeDir, 0755); err != nil {
		return err
	}
	path := filepath.Join(runtimeDir, "case_journal.json")
	state := struct {
		Enabled bool `json:"enabled"`
	}{Enabled: enabled}
	b, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0644)
}

// RecordCase enqueues one disagreement case onto the worker queue without blocking the request.
func (r *Recorder) RecordCase(ctx context.Context, record CaseRecord) error {
	if r == nil {
		return nil
	}
	if !r.config.CaseJournalEnabled && !IsCaseJournalEnabled(r.config.WorkspaceRoot) {
		return nil
	}

	if record.EventID != "" {
		if _, loaded := r.caseEventIDs.LoadOrStore(record.EventID, struct{}{}); loaded {
			return ErrCaseConflict
		}
	}

	r.gate.RLock()
	defer r.gate.RUnlock()
	if r.closed {
		return errors.New("telemetry recorder is closed")
	}

	req := request{
		op:         opRecordCase,
		ctx:        ctx,
		caseRecord: &record,
	}
	select {
	case r.queue <- req:
		r.accepted.Add(1)
		return nil
	default:
		r.dropped.Add(1)
		return nil
	}
}

// PurgeCases removes all stored cases through the worker.
func (r *Recorder) PurgeCases(ctx context.Context) error {
	if r == nil {
		return nil
	}
	result := r.admin(ctx, request{op: opPurgeCases})
	return result.err
}

// PruneCases removes cases older than 90 days through the worker.
func (r *Recorder) PruneCases(ctx context.Context) error {
	if r == nil {
		return nil
	}
	result := r.admin(ctx, request{op: opPruneCases})
	return result.err
}

// Cases lists stored cases matching the given query parameters through the worker.
func (r *Recorder) Cases(ctx context.Context, since time.Time, kind string) ([]CaseRecord, error) {
	if r == nil {
		return nil, nil
	}
	result := r.admin(ctx, request{op: opCases, caseSince: since, caseKind: kind})
	return result.cases, result.err
}

func recordCaseStore(ctx context.Context, config Config, record CaseRecord) error {
	database, err := openDatabase(ctx, config)
	if err != nil {
		return err
	}
	defer database.Close()

	caseRetention := config.CaseRetention
	if caseRetention <= 0 {
		caseRetention = DefaultCaseRetention
	}
	caseDailyLimit := config.CaseDailyLimit
	if caseDailyLimit <= 0 {
		caseDailyLimit = DefaultCaseDailyLimit
	}
	caseTotalLimit := config.CaseTotalLimit
	if caseTotalLimit <= 0 {
		caseTotalLimit = DefaultCaseTotalLimit
	}

	conn, err := database.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = conn.ExecContext(ctx, "ROLLBACK")
		}
	}()

	// 1. Enforce retention on record
	cutoff := time.Now().UTC().Add(-caseRetention).Format(time.RFC3339)
	if _, err := conn.ExecContext(ctx, "DELETE FROM telemetry_cases WHERE occurred_at < ?", cutoff); err != nil {
		return err
	}

	// 2. Check limits: daily cap
	if record.OccurredAt.IsZero() {
		record.OccurredAt = time.Now().UTC()
	}
	day := record.OccurredAt.UTC().Format("2006-01-02")
	var dayCount int
	err = conn.QueryRowContext(ctx, "SELECT count(*) FROM telemetry_cases WHERE substr(occurred_at, 1, 10) = ?", day).Scan(&dayCount)
	if err != nil {
		return fmt.Errorf("query daily case count: %w", err)
	}
	if dayCount >= caseDailyLimit {
		if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
			return err
		}
		committed = true
		return nil // daily cap reached
	}

	// 3. Check limits: total cap - eviction removes count - limit + 1 rows
	var totalCount int
	err = conn.QueryRowContext(ctx, "SELECT count(*) FROM telemetry_cases").Scan(&totalCount)
	if err != nil {
		return fmt.Errorf("query total case count: %w", err)
	}
	if totalCount >= caseTotalLimit {
		toRemove := totalCount - caseTotalLimit + 1
		_, err = conn.ExecContext(ctx, "DELETE FROM telemetry_cases WHERE case_id IN (SELECT case_id FROM telemetry_cases ORDER BY occurred_at ASC LIMIT ?)", toRemove)
		if err != nil {
			return fmt.Errorf("evict oldest cases: %w", err)
		}
	}

	if record.EventID != "" {
		var exists int
		err = conn.QueryRowContext(ctx, "SELECT 1 FROM telemetry_cases WHERE event_id = ?", record.EventID).Scan(&exists)
		if err == nil && exists == 1 {
			return ErrCaseConflict
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	} else {
		record.EventID = NewEventID()
	}

	if record.CaseID == "" {
		id, err := randomCaseID()
		if err != nil {
			return err
		}
		record.CaseID = id
	}

	encoded, err := json.Marshal(record)
	if err != nil {
		return err
	}

	_, err = conn.ExecContext(ctx, `INSERT INTO telemetry_cases(case_id, occurred_at, kind, session_hash, resolution_id, event_id, payload_json)
VALUES(?, ?, ?, ?, ?, ?, ?)`,
		record.CaseID, record.OccurredAt.UTC().Format(time.RFC3339Nano), record.Kind,
		record.SessionHash, record.ResolutionID, record.EventID, string(encoded))
	if err != nil {
		return err
	}

	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return err
	}
	committed = true
	return nil
}

func purgeCasesStore(ctx context.Context, config Config) error {
	database, err := openDatabase(ctx, config)
	if err != nil {
		return err
	}
	defer database.Close()
	_, err = database.ExecContext(ctx, "DELETE FROM telemetry_cases")
	return err
}

func pruneCasesStore(ctx context.Context, config Config) error {
	database, err := openDatabase(ctx, config)
	if err != nil {
		return err
	}
	defer database.Close()
	caseRetention := config.CaseRetention
	if caseRetention <= 0 {
		caseRetention = DefaultCaseRetention
	}
	cutoff := time.Now().UTC().Add(-caseRetention).Format(time.RFC3339)
	_, err = database.ExecContext(ctx, "DELETE FROM telemetry_cases WHERE occurred_at < ?", cutoff)
	return err
}

func casesStore(ctx context.Context, config Config, since time.Time, kind string) ([]CaseRecord, error) {
	database, err := openDatabase(ctx, config)
	if err != nil {
		return nil, err
	}
	defer database.Close()

	query := "SELECT case_id, occurred_at, kind, session_hash, resolution_id, event_id, payload_json FROM telemetry_cases WHERE 1=1"
	var args []any
	if !since.IsZero() {
		query += " AND occurred_at >= ?"
		args = append(args, since.UTC().Format(time.RFC3339Nano))
	}
	if kind != "" {
		query += " AND kind = ?"
		args = append(args, kind)
	}
	query += " ORDER BY occurred_at ASC"

	rows, err := database.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cases []CaseRecord
	for rows.Next() {
		var caseID, occurredAtStr, rowKind string
		var sessionHash, resolutionID, eventID *string
		var payload string
		if err := rows.Scan(&caseID, &occurredAtStr, &rowKind, &sessionHash, &resolutionID, &eventID, &payload); err != nil {
			return nil, err
		}
		var record CaseRecord
		if err := json.Unmarshal([]byte(payload), &record); err != nil {
			return nil, err
		}
		record.CaseID = caseID
		if parsed, err := time.Parse(time.RFC3339Nano, occurredAtStr); err == nil {
			record.OccurredAt = parsed
		}
		record.Kind = rowKind
		if sessionHash != nil {
			record.SessionHash = *sessionHash
		}
		if resolutionID != nil {
			record.ResolutionID = *resolutionID
		}
		if eventID != nil {
			record.EventID = *eventID
		}
		cases = append(cases, record)
	}
	return cases, rows.Err()
}
