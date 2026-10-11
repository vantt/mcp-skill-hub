package mcpserver

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vantt/mcp-skill-hub/internal/app"
)

// TestRemoteFetchBench measures what a client pays today, over stdio, to get a
// whole skill folder: skill_resolve, skill_get, the skills/list and skills/get
// calls that reveal the version-pinned resource URIs, and one resources/read
// per file. It reports latency, round trips and response bytes per step, and
// checks every file it reads against the digest the hub announced, so the
// numbers describe a complete, byte-identical fetch.
//
// Each run is a new agent session: a new `skillhub mcp serve` process and a
// new client, because the Go SDK client caches resources/read results for
// their TTL and a second fetch in one session would read nothing. That
// in-session repeat is still measured once per run and reported apart.
//
// It is wall-clock and machine dependent, so it runs only with SKILLHUB_PERF=1;
// `make remote-baseline` is the usual entry point. Every served process is
// stopped by closing its stdin (and killed by the transport if it lingers).
//
// Tunables (all optional):
//
//	SKILLHUB_REMOTE_HUB     Git repository of a hub to measure. It is cloned
//	                        into a temp directory and only the clone is
//	                        touched; skills awaiting content review are
//	                        approved in the clone only. Unset: a temp
//	                        workspace with fixture skills of the same shape
//	                        (file count and sizes) as test-audit and
//	                        herdr-cook-plan.
//	SKILLHUB_REMOTE_SKILLS  comma-separated skill IDs (default
//	                        "test-audit,herdr-cook-plan")
//	SKILLHUB_REMOTE_RUNS    sessions, each fetching every skill (default 30)
//	SKILLHUB_REMOTE_RTT     network round-trip time used to project the same
//	                        call sequence onto a slower link (default 100ms)
func TestRemoteFetchBench(t *testing.T) {
	requireMemoryPerf(t)
	runs := envInt(t, "SKILLHUB_REMOTE_RUNS", 30)
	rtt := envDuration(t, "SKILLHUB_REMOTE_RTT", 100*time.Millisecond)
	if runs < 1 {
		t.Fatal("SKILLHUB_REMOTE_RUNS must be at least 1")
	}
	skillIDs := remoteBenchSkillIDs()

	binary := buildSkillHub(t)
	hub := os.Getenv("SKILLHUB_REMOTE_HUB")
	var root, source string
	if hub != "" {
		root = cloneRemoteBenchHub(t, binary, hub, skillIDs)
		source = "clone of " + hub
	} else {
		root = newRemoteBenchFixture(t, binary)
		source = "fixture skills shaped like test-audit and herdr-cook-plan"
	}

	descriptions := map[string]string{}
	for _, id := range skillIDs {
		descriptions[id] = remoteBenchTaskText(t, root, id)
	}

	fetches := map[string][]remoteFetch{}
	repeats := map[string][]remoteStep{}
	var startups []time.Duration
	for run := 0; run < runs; run++ {
		bench := startRemoteBenchSession(t, binary, root)
		startups = append(startups, bench.startup)
		for _, id := range skillIDs {
			for attempt := 0; attempt < 2; attempt++ {
				fetch, err := fetchWholeSkill(t.Context(), bench.session, bench.counter, id, descriptions[id], fmt.Sprintf("remote-bench-%s-%d-%d", id, run, attempt))
				if err != nil {
					bench.close(t)
					t.Fatalf("fetch %s run %d: %v; stderr=%s", id, run, err, bench.stderr.String())
				}
				if attempt == 0 {
					fetches[id] = append(fetches[id], fetch)
				} else {
					repeats[id] = append(repeats[id], fetch.steps[remoteFolderStep])
				}
			}
		}
		bench.close(t)
	}

	t.Logf("remote fetch baseline over stdio: %s; runs=%d; projected link RTT=%s", source, runs, rtt)
	t.Logf("process start and initialize: p50 %s, p95 %s", roundMS(durationPercentile(startups, 50)), roundMS(durationPercentile(startups, 95)))
	t.Logf("first = the first fetch ever (the hub exports the skill snapshot); p50/p95 = first fetch in each later session; KB = JSON-RPC response bytes")
	t.Logf("| skill | served files | content KB | round trips | step | first | p50 | p95 | response KB |")
	t.Logf("|---|---|---|---|---|---|---|---|---|")
	for _, id := range skillIDs {
		reportRemoteFetches(t, id, fetches[id], repeats[id], rtt)
	}
}

// remoteBenchSession is one served process and the client connected to it.
type remoteBenchSession struct {
	session *mcp.ClientSession
	counter *responseByteCounter
	stderr  *bytes.Buffer
	startup time.Duration
	closed  bool
}

func startRemoteBenchSession(t *testing.T, binary, root string) *remoteBenchSession {
	t.Helper()
	bench := &remoteBenchSession{counter: &responseByteCounter{}, stderr: &bytes.Buffer{}}
	command := exec.Command(binary, "mcp", "serve", "--profile", "runtime", "--workspace", root)
	command.Env = remoteBenchEnv()
	command.Stderr = &lockedWriter{target: bench.stderr}
	client := mcp.NewClient(&mcp.Implementation{Name: "remote-bench", Version: "1"}, nil)
	if err := mcp.AddSendingCustomMethod[*listSkillsParams, *listSkillsResult](client, "skills/list"); err != nil {
		t.Fatal(err)
	}
	if err := mcp.AddSendingCustomMethod[*getSkillParams, *getSkillResult](client, "skills/get"); err != nil {
		t.Fatal(err)
	}
	transport := &mcp.LoggingTransport{Transport: &mcp.CommandTransport{Command: command, TerminateDuration: 15 * time.Second}, Writer: bench.counter}
	started := time.Now()
	session, err := client.Connect(t.Context(), transport, nil)
	if err != nil {
		t.Fatalf("connect: %v; stderr=%s", err, bench.stderr.String())
	}
	bench.startup = time.Since(started)
	bench.session = session
	t.Cleanup(func() { bench.close(t) })
	return bench
}

// close ends stdin so the server flushes telemetry and exits.
func (bench *remoteBenchSession) close(t *testing.T) {
	t.Helper()
	if bench.closed {
		return
	}
	bench.closed = true
	if err := bench.session.Close(); err != nil {
		t.Logf("close serve: %v; stderr=%s", err, bench.stderr.String())
	}
}

// remoteFetch is one complete fetch of a skill folder by a client that cannot
// read the hub's disk, so it must pull every file over MCP.
type remoteFetch struct {
	files, contentBytes, roundTrips int
	primaryMatched                  bool
	steps                           map[string]remoteStep
}

type remoteStep struct {
	elapsed time.Duration
	bytes   int64
	calls   int
}

const remoteFolderStep = "folder (all but resolve)"

var remoteFetchSteps = []string{"skill_resolve", "skill_get", "skills/list", "skills/get", "resources/read (all files)", remoteFolderStep}

// fetchWholeSkill follows what a remote client must do today: resolve with the
// task text, then pull the skill and every file it lists.
func fetchWholeSkill(ctx context.Context, session *mcp.ClientSession, counter *responseByteCounter, id, description, requestID string) (remoteFetch, error) {
	fetch := remoteFetch{steps: map[string]remoteStep{}}
	step := func(name string, call func() error) error {
		before, started := counter.total(), time.Now()
		err := call()
		current := fetch.steps[name]
		current.elapsed += time.Since(started)
		current.bytes += counter.total() - before
		current.calls++
		fetch.steps[name] = current
		fetch.roundTrips++
		return err
	}

	var resolved resolveResult
	if err := step("skill_resolve", func() error {
		return remoteBenchCallTool(ctx, session, "skill_resolve", map[string]any{
			"schema_version": SchemaVersion, "request_id": requestID,
			"task": map[string]any{"description": description, "scope": "multi_step"},
		}, &resolved)
	}); err != nil {
		return fetch, err
	}
	var detail skillGetResult
	if err := step("skill_get", func() error {
		return remoteBenchCallTool(ctx, session, "skill_get", map[string]any{"skill_id": id}, &detail)
	}); err != nil {
		return fetch, err
	}
	if detail.Local != nil && detail.Local.Status == app.LocalStatusReviewRequired {
		return fetch, fmt.Errorf("skill %s awaits content review; its content is withheld", id)
	}
	fetch.primaryMatched = resolved.Resolution.Primary != nil && resolved.Resolution.Primary.ID == id

	var entry *skillEntry
	cursor := ""
	for entry == nil {
		var listed *listSkillsResult
		if err := step("skills/list", func() error {
			var err error
			listed, err = mcp.CallCustomMethod[*listSkillsParams, *listSkillsResult](ctx, session, "skills/list", &listSkillsParams{Cursor: cursor})
			return err
		}); err != nil {
			return fetch, err
		}
		for index := range listed.Skills {
			if remoteEntryID(listed.Skills[index].URI) == id {
				entry = &listed.Skills[index]
				break
			}
		}
		if entry == nil && listed.NextCursor == "" {
			return fetch, fmt.Errorf("skill %s is not listed by skills/list", id)
		}
		cursor = listed.NextCursor
	}
	var got *getSkillResult
	if err := step("skills/get", func() error {
		var err error
		got, err = mcp.CallCustomMethod[*getSkillParams, *getSkillResult](ctx, session, "skills/get", &getSkillParams{URI: entry.URI})
		return err
	}); err != nil {
		return fetch, err
	}
	for _, resource := range got.Skill.Resources {
		var read *mcp.ReadResourceResult
		if err := step("resources/read (all files)", func() error {
			var err error
			read, err = session.ReadResource(ctx, &mcp.ReadResourceParams{URI: resource.URI})
			return err
		}); err != nil {
			return fetch, fmt.Errorf("read %s: %w", resource.URI, err)
		}
		if len(read.Contents) != 1 {
			return fetch, fmt.Errorf("read %s returned %d contents", resource.URI, len(read.Contents))
		}
		data := read.Contents[0].Blob
		if data == nil {
			data = []byte(read.Contents[0].Text)
		}
		sum := sha256.Sum256(data)
		if digest := "sha256:" + hex.EncodeToString(sum[:]); digest != resource.Digest || int64(len(data)) != resource.Size {
			return fetch, fmt.Errorf("read %s: got %d bytes %s, announced %d bytes %s", resource.URI, len(data), digest, resource.Size, resource.Digest)
		}
		fetch.files++
		fetch.contentBytes += len(data)
	}
	folder := remoteStep{}
	for _, name := range []string{"skill_get", "skills/list", "skills/get", "resources/read (all files)"} {
		folder.elapsed += fetch.steps[name].elapsed
		folder.bytes += fetch.steps[name].bytes
		folder.calls += fetch.steps[name].calls
	}
	fetch.steps[remoteFolderStep] = folder
	return fetch, nil
}

// remoteBenchTaskText is the task a client would send to reach the skill: its
// first routing trigger, or its description when it has none.
func remoteBenchTaskText(t *testing.T, root, id string) string {
	t.Helper()
	detail, err := (app.SkillService{}).GetSkillDetail(t.Context(), root, id)
	if err != nil {
		t.Fatalf("skill %s in %s: %v", id, root, err)
	}
	if len(detail.Routing.Triggers) > 0 {
		return detail.Routing.Triggers[0]
	}
	return detail.Description
}

func remoteBenchCallTool(ctx context.Context, session *mcp.ClientSession, name string, arguments map[string]any, target any) error {
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: arguments})
	if err != nil {
		return err
	}
	if result.IsError {
		return fmt.Errorf("%s tool error: %v", name, result.StructuredContent)
	}
	encoded, err := json.Marshal(result.StructuredContent)
	if err != nil {
		return err
	}
	var outcome struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(encoded, &outcome); err != nil {
		return err
	}
	if len(outcome.Result) == 0 {
		return fmt.Errorf("%s returned no result: %s", name, encoded)
	}
	return json.Unmarshal(outcome.Result, target)
}

// remoteEntryID extracts the skill name from
// skill://skillhub/{manifest}/{name}/SKILL.md.
func remoteEntryID(uri string) string {
	parts := strings.Split(strings.TrimPrefix(uri, "skill://"), "/")
	if len(parts) < 4 {
		return ""
	}
	return parts[2]
}

// reportRemoteFetches logs one table block per skill. The first fetch ever is
// shown apart because the hub exports the skill snapshot then; p50 and p95
// cover the first fetch of every later session.
func reportRemoteFetches(t *testing.T, id string, fetches []remoteFetch, repeats []remoteStep, rtt time.Duration) {
	t.Helper()
	first := fetches[0]
	matched := 0
	for _, fetch := range fetches {
		if fetch.files != first.files || fetch.contentBytes != first.contentBytes || fetch.roundTrips != first.roundTrips {
			t.Errorf("%s: fetches differ in shape: %d files, %d bytes, %d round trips; the first had %d, %d, %d",
				id, fetch.files, fetch.contentBytes, fetch.roundTrips, first.files, first.contentBytes, first.roundTrips)
		}
		if fetch.primaryMatched {
			matched++
		}
	}
	steady := fetches[min(1, len(fetches)-1):]
	for _, name := range remoteFetchSteps {
		durations := make([]time.Duration, 0, len(steady))
		for _, fetch := range steady {
			durations = append(durations, fetch.steps[name].elapsed)
		}
		label := name
		if calls := first.steps[name].calls; calls > 1 {
			label = fmt.Sprintf("%s (%d calls)", name, calls)
		}
		t.Logf("| %s | %d | %.1f | %d | %s | %s | %s | %s | %.1f |", id, first.files, float64(first.contentBytes)/1024, first.roundTrips, label,
			roundMS(first.steps[name].elapsed), roundMS(durationPercentile(durations, 50)), roundMS(durationPercentile(durations, 95)),
			float64(first.steps[name].bytes)/1024)
	}
	folderCalls := first.steps[remoteFolderStep].calls
	folderDurations := make([]time.Duration, 0, len(steady))
	for _, fetch := range steady {
		folderDurations = append(folderDurations, fetch.steps[remoteFolderStep].elapsed)
	}
	repeatDurations := make([]time.Duration, 0, len(repeats))
	var repeatBytes int64
	for _, repeat := range repeats {
		repeatDurations = append(repeatDurations, repeat.elapsed)
		repeatBytes = max(repeatBytes, repeat.bytes)
	}
	t.Logf("%s: %d served files, %d content bytes, every file digest-checked; skill_resolve chose it as primary in %d/%d fetches",
		id, first.files, first.contentBytes, matched, len(fetches))
	t.Logf("%s: the folder takes %d sequential round trips; at %s RTT they alone add %s, so the projected folder p50 is %s",
		id, folderCalls, rtt, roundMS(time.Duration(folderCalls)*rtt),
		roundMS(durationPercentile(folderDurations, 50)+time.Duration(folderCalls)*rtt))
	t.Logf("%s: a second fetch in the same session, where the Go SDK client serves resources/read from its cache: folder p50 %s, at most %.1f KB of responses",
		id, roundMS(durationPercentile(repeatDurations, 50)), float64(repeatBytes)/1024)
}

// responseByteCounter counts the bytes of messages the client reads from the
// server, as logged by mcp.LoggingTransport ("read: <json>\n").
type responseByteCounter struct {
	mu    sync.Mutex
	bytes int64
}

func (counter *responseByteCounter) Write(data []byte) (int, error) {
	counter.mu.Lock()
	defer counter.mu.Unlock()
	if payload, ok := bytes.CutPrefix(data, []byte("read: ")); ok {
		counter.bytes += int64(len(payload))
	}
	return len(data), nil
}

func (counter *responseByteCounter) total() int64 {
	counter.mu.Lock()
	defer counter.mu.Unlock()
	return counter.bytes
}

func remoteBenchSkillIDs() []string {
	raw := os.Getenv("SKILLHUB_REMOTE_SKILLS")
	if strings.TrimSpace(raw) == "" {
		return []string{"test-audit", "herdr-cook-plan"}
	}
	var ids []string
	for _, id := range strings.Split(raw, ",") {
		if id = strings.TrimSpace(id); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

// remoteBenchEnv is the test process environment (HOME and XDG_* are already
// isolated by TestMain) plus a commit identity, because approving content in
// a clone commits there and the isolated home has no Git configuration.
func remoteBenchEnv() []string {
	return append(os.Environ(),
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=remote-bench", "GIT_AUTHOR_EMAIL=remote-bench@example.invalid",
		"GIT_COMMITTER_NAME=remote-bench", "GIT_COMMITTER_EMAIL=remote-bench@example.invalid",
	)
}

func runRemoteBench(t *testing.T, binary string, args ...string) {
	t.Helper()
	command := exec.Command(binary, args...)
	command.Env = remoteBenchEnv()
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("skillhub %s: %v\n%s", strings.Join(args, " "), err, output)
	}
}

// cloneRemoteBenchHub clones hub into a temp directory, compiles its catalog
// and approves the content of the measured skills in the clone when they
// await review. The source repository is only read.
func cloneRemoteBenchHub(t *testing.T, binary, hub string, skillIDs []string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "hub")
	clone := exec.Command("git", "clone", "--quiet", "--no-hardlinks", hub, root)
	clone.Env = remoteBenchEnv()
	if output, err := clone.CombinedOutput(); err != nil {
		t.Fatalf("clone %s: %v\n%s", hub, err, output)
	}
	runRemoteBench(t, binary, "rebuild", "--workspace", root)
	for _, id := range skillIDs {
		trust, err := (app.SkillService{}).ContentTrustFor(t.Context(), root, id)
		if err != nil {
			t.Fatalf("content trust of %s in the clone: %v", id, err)
		}
		if !trust.RequiresReview() {
			continue
		}
		t.Logf("approving %s in the clone only (reasons %v)", id, trust.ReasonCodes)
		runRemoteBench(t, binary, "skill", "edit", id, "--workspace", root, "--approve-content", trust.ContentDigest, "--yes")
	}
	return root
}

// remoteBenchFile is one file of a fixture skill: its path in the skill folder
// and its size in bytes, copied from the real skill it stands in for.
type remoteBenchFile struct {
	path string
	size int
}

// remoteBenchShapes mirror test-audit (4 files) and herdr-cook-plan (14
// files, about 186 KB) as they were on the hub on 2026-10-11, so the fixture
// run moves the same number of files and bytes as the real skills.
var remoteBenchShapes = map[string][]remoteBenchFile{
	"test-audit": {
		{"SKILL.md", 7754}, {"CAMPAIGN.md", 5463}, {".meta/distill.yaml", 46161},
	},
	"herdr-cook-plan": {
		{"SKILL.md", 20094}, {"agents/openai.yaml", 352}, {"extensions/context-guard.mjs", 15501},
		{"references/dispatch.md", 13540}, {"references/mailbox.md", 5490}, {"references/omp.md", 10610},
		{"references/recovery.md", 20711}, {"references/runtimes.md", 8095}, {"references/selection.md", 4733},
		{"references/supervision.md", 9377}, {"scripts/check-run-closed.py", 6477},
		{"tests/context-guard.test.mjs", 20964}, {"tests/instruction-contract.test.py", 50399},
	},
}

// newRemoteBenchFixture creates a temp workspace holding one active, reviewed
// skill per shape; .meta/skill.yaml is the fourth or fourteenth file.
func newRemoteBenchFixture(t *testing.T, binary string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	initCommand := exec.Command(binary, "init")
	initCommand.Dir = root
	initCommand.Env = remoteBenchEnv()
	if output, err := initCommand.CombinedOutput(); err != nil {
		t.Fatalf("skillhub init: %v\n%s", err, output)
	}
	for id, files := range remoteBenchShapes {
		directory := filepath.Join(root, "skills", "default", id)
		metadata := fmt.Sprintf("schema_version: 1\nid: %s\nstatus: active\nquality:\n  reviewed: true\nrouting:\n  triggers: [fetch the %s fixture skill]\n  not_for: [write marketing prose]\n  min_scope: multi_step\n", id, id)
		if err := writeRemoteBenchFile(filepath.Join(directory, ".meta", "skill.yaml"), []byte(metadata)); err != nil {
			t.Fatal(err)
		}
		for _, file := range files {
			content := remoteBenchFiller(file.size)
			if file.path == "SKILL.md" {
				header := fmt.Sprintf("---\nname: %s\ndescription: Fixture skill shaped like %s.\n---\n\n# %s\n\n", id, id, id)
				content = append([]byte(header), content[:max(0, file.size-len(header))]...)
			}
			if err := writeRemoteBenchFile(filepath.Join(directory, filepath.FromSlash(file.path)), content); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := (app.CatalogService{}).EnsureCatalog(t.Context(), root); err != nil {
		t.Fatalf("ensure catalog: %v", err)
	}
	return root
}

func writeRemoteBenchFile(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, content, 0o644)
}

// remoteBenchFiller returns size bytes of varied plain text lines.
func remoteBenchFiller(size int) []byte {
	var buffer bytes.Buffer
	for line := 0; buffer.Len() < size; line++ {
		fmt.Fprintf(&buffer, "Line %d of fixture text that stands in for real skill content.\n", line)
	}
	return buffer.Bytes()[:size]
}
