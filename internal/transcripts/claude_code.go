package transcripts

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"
)

const (
	maxLineSize    = 4 * 1024 * 1024 // 4 MiB line cap
	resolveWindow  = 30 * time.Minute
	gitExecTimeout = 5 * time.Second
)

// Observation describes one tool invocation observed in a transcript.
type Observation struct {
	ToolUseID      string    `json:"tool_use_id"`
	Tool           string    `json:"tool"`
	SkillID        string    `json:"skill_id,omitempty"`
	SessionID      string    `json:"session_id"`
	At             time.Time `json:"at"`
	ResolvedBefore bool      `json:"resolved_before,omitempty"`
}

// ScanResult holds all parsed observations and scan statistics.
type ScanResult struct {
	Observations []Observation
	FilesScanned int
	LinesSkipped int
	Malformed    int
}

// EncodeProjectDir replaces '/', '\', '.', and ':' with '-'.
func EncodeProjectDir(path string) string {
	return encodeProjectDir(path)
}

func encodeProjectDir(path string) string {
	var b strings.Builder
	b.Grow(len(path))
	for _, c := range path {
		if c == '/' || c == '\\' || c == '.' || c == ':' {
			b.WriteByte('-')
		} else {
			b.WriteRune(c)
		}
	}
	return b.String()
}

// ProjectDirs discovers matching project and worktree transcript directories under configDir/projects/.
func ProjectDirs(configDir, projectRoot string) ([]string, error) {
	if configDir == "" {
		if env := os.Getenv("CLAUDE_CONFIG_DIR"); env != "" {
			configDir = env
		} else if home, err := os.UserHomeDir(); err == nil {
			configDir = filepath.Join(home, ".claude")
		} else {
			return nil, errors.New("cannot determine claude config dir")
		}
	}

	projectsBase := filepath.Join(configDir, "projects")
	if info, err := os.Stat(projectsBase); err != nil || !info.IsDir() {
		return nil, nil
	}

	canonRoot, err := filepath.EvalSymlinks(projectRoot)
	if err != nil {
		canonRoot = projectRoot
	}
	canonRoot = filepath.Clean(canonRoot)

	validCwds := queryValidCwds(canonRoot)

	encRoot := encodeProjectDir(canonRoot)
	worktreePrefix := encRoot + "--claude-worktrees-"

	validEncNames := make(map[string]bool, len(validCwds))
	for _, cwd := range validCwds {
		validEncNames[encodeProjectDir(cwd)] = true
	}

	entries, err := os.ReadDir(projectsBase)
	if err != nil {
		return nil, err
	}

	var matchingDirs []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if validEncNames[name] || strings.HasPrefix(name, worktreePrefix) {
			matchingDirs = append(matchingDirs, filepath.Join(projectsBase, name))
		}
	}
	sort.Strings(matchingDirs)
	return matchingDirs, nil
}

// ValidCwds returns all canonical paths belonging to projectRoot (including git worktrees).
func ValidCwds(projectRoot string) []string {
	canonRoot, err := filepath.EvalSymlinks(projectRoot)
	if err != nil {
		canonRoot = projectRoot
	}
	return queryValidCwds(filepath.Clean(canonRoot))
}

func queryValidCwds(canonRoot string) []string {
	cwds := []string{canonRoot}

	ctx, cancel := context.WithTimeout(context.Background(), gitExecTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "-C", canonRoot, "worktree", "list", "--porcelain")
	output, err := cmd.Output()
	if err == nil {
		scanner := bufio.NewScanner(bytes.NewReader(output))
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "worktree ") {
				wtPath := strings.TrimPrefix(line, "worktree ")
				if canonWt, err := filepath.EvalSymlinks(wtPath); err == nil {
					canonWt = filepath.Clean(canonWt)
					if !slices.Contains(cwds, canonWt) {
						cwds = append(cwds, canonWt)
					}
				}
			}
		}
	}
	return cwds
}

type transcriptLine struct {
	Type      string          `json:"type"`
	SessionID string          `json:"sessionId"`
	Cwd       string          `json:"cwd"`
	Timestamp string          `json:"timestamp"`
	Message   *messagePayload `json:"message"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     map[string]any  `json:"input"`
}

type messagePayload struct {
	ID      string         `json:"id"`
	Content []contentBlock `json:"content"`
}

type contentBlock struct {
	Type  string         `json:"type"`
	ID    string         `json:"id"`
	Name  string         `json:"name"`
	Input map[string]any `json:"input"`
}

func isInsideAnyCwd(targetCwd string, validCwds []string) bool {
	if targetCwd == "" || len(validCwds) == 0 {
		return true
	}
	canonTarget, err := filepath.EvalSymlinks(targetCwd)
	if err != nil {
		canonTarget = targetCwd
	}
	canonTarget = filepath.Clean(canonTarget)

	for _, valid := range validCwds {
		if canonTarget == valid || strings.HasPrefix(canonTarget, valid+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func parseSkillIDFromResourceURI(rawURI string) string {
	parsed, err := url.Parse(rawURI)
	if err != nil || parsed.Scheme != "skill" || parsed.Host != "skillhub" {
		return ""
	}
	parts := strings.Split(strings.TrimPrefix(parsed.Path, "/"), "/")
	if len(parts) >= 2 {
		return parts[1]
	}
	return ""
}

func findTranscriptFiles(dirs []string) []string {
	var files []string
	for _, dir := range dirs {
		if primaryFiles, err := filepath.Glob(filepath.Join(dir, "*.jsonl")); err == nil {
			files = append(files, primaryFiles...)
		}
		if subagentFiles, err := filepath.Glob(filepath.Join(dir, "*", "subagents", "*.jsonl")); err == nil {
			files = append(files, subagentFiles...)
		}
	}
	sort.Strings(files)
	return files
}

func readTranscriptLine(reader *bufio.Reader) ([]byte, bool, error) {
	lineBytes, isPrefix, readErr := reader.ReadLine()
	if readErr != nil {
		return nil, false, readErr
	}
	if !isPrefix {
		return lineBytes, false, nil
	}
	var fullLine bytes.Buffer
	fullLine.Write(lineBytes)
	exceeded := false
	for isPrefix && readErr == nil {
		lineBytes, isPrefix, readErr = reader.ReadLine()
		if fullLine.Len()+len(lineBytes) > maxLineSize {
			exceeded = true
		}
		if !exceeded {
			fullLine.Write(lineBytes)
		}
	}
	if exceeded {
		return nil, true, nil
	}
	return fullLine.Bytes(), false, nil
}

func parseToolBlock(b contentBlock, sessionID string, t time.Time) (Observation, bool) {
	var tool, skillID string
	switch {
	case strings.HasPrefix(b.Name, "mcp__skillhub__"):
		tool = strings.TrimPrefix(b.Name, "mcp__skillhub__")
		if sID, ok := b.Input["skill_id"].(string); ok {
			skillID = sID
		}
	case b.Name == "Skill":
		tool = "Skill"
		if sID, ok := b.Input["skill"].(string); ok {
			skillID = sID
		}
	case b.Name == "ReadMcpResourceTool":
		if server, _ := b.Input["server"].(string); server == "skillhub" {
			tool = "resources_read"
			if uri, ok := b.Input["uri"].(string); ok {
				skillID = parseSkillIDFromResourceURI(uri)
			}
		}
	default:
		return Observation{}, false
	}

	return Observation{
		ToolUseID: b.ID,
		Tool:      tool,
		SkillID:   skillID,
		SessionID: sessionID,
		At:        t,
	}, true
}

func annotateResolvedBefore(observations []Observation, sessionResolves map[string][]time.Time) {
	for i := range observations {
		obs := &observations[i]
		if obs.Tool == "Skill" && obs.SessionID != "" {
			for _, rTime := range sessionResolves[obs.SessionID] {
				diff := obs.At.Sub(rTime)
				if diff >= 0 && diff <= resolveWindow {
					obs.ResolvedBefore = true
					break
				}
			}
		}
	}
}

// Scan reads JSONL transcript files in the given project directories, extracts relevant skill tool
// observations within the given time window, and resolves preceding recommendation context.
func Scan(dirs []string, since, until time.Time, validCwds []string) (ScanResult, error) {
	result := ScanResult{}
	files := findTranscriptFiles(dirs)

	seenToolUseIDs := make(map[string]bool)
	var rawObservations []Observation
	sessionResolves := make(map[string][]time.Time)

	for _, filePath := range files {
		f, err := os.Open(filePath)
		if err != nil {
			continue
		}
		result.FilesScanned++

		reader := bufio.NewReaderSize(f, 64*1024)
		for {
			lineBytes, exceeded, readErr := readTranscriptLine(reader)
			if readErr != nil {
				if errors.Is(readErr, io.EOF) {
					break
				}
				result.Malformed++
				break
			}
			if exceeded {
				result.Malformed++
				continue
			}
			if !bytes.Contains(lineBytes, []byte(`"tool_use"`)) {
				result.LinesSkipped++
				continue
			}

			var rec transcriptLine
			if err := json.Unmarshal(lineBytes, &rec); err != nil {
				result.Malformed++
				continue
			}

			if !isInsideAnyCwd(rec.Cwd, validCwds) {
				result.LinesSkipped++
				continue
			}

			var t time.Time
			if rec.Timestamp != "" {
				parsedT, err := time.Parse(time.RFC3339Nano, rec.Timestamp)
				if err != nil {
					parsedT, err = time.Parse(time.RFC3339, rec.Timestamp)
				}
				if err == nil {
					t = parsedT.UTC()
				}
			}
			if (!since.IsZero() && t.Before(since)) || (!until.IsZero() && t.After(until)) {
				result.LinesSkipped++
				continue
			}

			var blocks []contentBlock
			if rec.Message != nil && len(rec.Message.Content) > 0 {
				for _, block := range rec.Message.Content {
					if block.Type == "tool_use" {
						blocks = append(blocks, block)
					}
				}
			} else if rec.Type == "tool_use" {
				blocks = append(blocks, contentBlock{
					Type:  rec.Type,
					ID:    rec.ID,
					Name:  rec.Name,
					Input: rec.Input,
				})
			}

			if len(blocks) == 0 {
				result.LinesSkipped++
				continue
			}

			for _, b := range blocks {
				if b.ID == "" || seenToolUseIDs[b.ID] {
					continue
				}
				obs, ok := parseToolBlock(b, rec.SessionID, t)
				if !ok {
					continue
				}
				seenToolUseIDs[b.ID] = true
				rawObservations = append(rawObservations, obs)
				if obs.Tool == "skill_resolve" && rec.SessionID != "" {
					sessionResolves[rec.SessionID] = append(sessionResolves[rec.SessionID], t)
				}
			}
		}
		_ = f.Close()
	}

	annotateResolvedBefore(rawObservations, sessionResolves)
	result.Observations = rawObservations
	return result, nil
}
