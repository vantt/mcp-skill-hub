package mcpserver

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/telemetry"
)

// These checks measure the resident memory and telemetry delivery of real
// `skillhub mcp serve` processes. They are wall-clock and machine dependent, so
// they run only with SKILLHUB_PERF=1. Every process they start is stopped by
// closing its stdin (and killed by the transport if it does not exit).
//
// Tunables (all optional):
//
//	SKILLHUB_MEMORY_SKILLS    skills in the fixture catalog (default 32)
//	SKILLHUB_MEMORY_RESOLVES  resolves in the "many" step (default 1000)
//	SKILLHUB_MEMORY_IDLE      idle wait after the resolves (default 60s)
//	SKILLHUB_MEMORY_ENV       extra environment for the served process,
//	                          space separated, for example "GOMEMLIMIT=16MiB"
//	SKILLHUB_MEMORY_BINARY    serve this binary instead of the freshly compiled
//	                          one; it is run as `<binary> mcp serve --profile
//	                          runtime --workspace <path>`
//	SKILLHUB_HEAP_PROFILE     file that receives the in-process heap profile
//	SKILLHUB_FANOUT_PROCS     processes in the fan-out (default 20)
//	SKILLHUB_FANOUT_RATE      calls per second per process; 0 is back-to-back (default 1)
//	SKILLHUB_FANOUT_DURATION  fan-out duration (default 30s)
//	SKILLHUB_FANOUT_IDLE      wait after the calls, then sample total memory again (default 0: skip)

const (
	// A working process measured about 32-42 MB before any slimming; this
	// ceiling only catches a gross regression, not normal machine noise.
	memoryBudgetCeilingKB = 96 << 10
	// A single resolve over stdio is a few milliseconds; this is a hang guard.
	memoryBudgetResolveHang = 2 * time.Second
)

func TestIdleMemoryReleaserReleasesOncePerQuietPeriod(t *testing.T) {
	t.Parallel()
	releases := 0
	releaser := &idleMemoryReleaser{release: func() { releases++ }}
	start := time.Unix(1_000, 0)
	releaser.noteActivity(start)

	if releaser.releaseIfIdle(start.Add(29*time.Second), 30*time.Second) {
		t.Fatal("released before the idle period elapsed")
	}
	if !releaser.releaseIfIdle(start.Add(30*time.Second), 30*time.Second) || releases != 1 {
		t.Fatalf("expected one release after the idle period, got %d", releases)
	}
	if releaser.releaseIfIdle(start.Add(90*time.Second), 30*time.Second) || releases != 1 {
		t.Fatalf("released again without new activity, got %d releases", releases)
	}
	releaser.noteActivity(start.Add(100 * time.Second))
	if !releaser.releaseIfIdle(start.Add(130*time.Second), 30*time.Second) || releases != 2 {
		t.Fatalf("expected a second release after a new burst went quiet, got %d", releases)
	}
}

func TestIdleMemoryReleaserWaitsForInFlightRequests(t *testing.T) {
	t.Parallel()
	releases := 0
	releaser := &idleMemoryReleaser{release: func() { releases++ }}
	entered, finish := make(chan struct{}), make(chan struct{})
	handler := releaser.middleware(func(context.Context, string, mcp.Request) (mcp.Result, error) {
		close(entered)
		<-finish
		return nil, nil
	})
	done := make(chan struct{})
	go func() {
		defer close(done)
		if _, err := handler(t.Context(), "tools/call", nil); err != nil {
			t.Errorf("handler: %v", err)
		}
	}()
	<-entered
	if releaser.releaseIfIdle(time.Now().Add(time.Hour), time.Second) || releases != 0 {
		t.Fatal("released memory while a request was in flight")
	}
	close(finish)
	<-done
	if releaser.releaseIfIdle(time.Now(), time.Second) {
		t.Fatal("released memory right after a request finished")
	}
	if !releaser.releaseIfIdle(time.Now().Add(2*time.Second), time.Second) || releases != 1 {
		t.Fatalf("expected one release once the server went quiet, got %d", releases)
	}
}

func TestIdleMemoryReleaserRunStopsWithContext(t *testing.T) {
	t.Parallel()
	released := make(chan struct{}, 1)
	releaser := &idleMemoryReleaser{release: func() {
		select {
		case released <- struct{}{}:
		default:
		}
	}}
	releaser.noteActivity(time.Now())
	ctx, cancel := context.WithCancel(t.Context())
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		releaser.run(ctx, 5*time.Millisecond, 20*time.Millisecond)
	}()
	select {
	case <-released:
	case <-time.After(5 * time.Second):
		t.Fatal("run never released memory after the idle period")
	}
	cancel()
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("run did not stop after its context was cancelled")
	}
}

func requireMemoryPerf(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("memory measurements are excluded from short tests")
	}
	if os.Getenv("SKILLHUB_PERF") != "1" {
		t.Skip("process memory measurements run only with SKILLHUB_PERF=1 on an otherwise idle machine")
	}
	if memoryRaceEnabled() {
		t.Skip("memory measurements are not meaningful under race instrumentation")
	}
}

func TestMCPMemoryBudgetPerformance(t *testing.T) {
	requireMemoryPerf(t)
	if _, err := readMemory(os.Getpid()); err != nil {
		t.Skipf("resident memory is not readable on %s: %v", runtime.GOOS, err)
	}
	skills := envInt(t, "SKILLHUB_MEMORY_SKILLS", 32)
	resolves := envInt(t, "SKILLHUB_MEMORY_RESOLVES", 1000)
	idle := envDuration(t, "SKILLHUB_MEMORY_IDLE", 60*time.Second)
	extraEnv := strings.Fields(os.Getenv("SKILLHUB_MEMORY_ENV"))

	binary := buildSkillHub(t)
	serveBinary := memoryServeBinary(binary)
	root := newMemoryWorkspace(t, binary, skills)
	proc := startMemoryServe(t, serveBinary, root, extraEnv)

	// Let start-up settle (catalog check, snapshot GC, telemetry open).
	time.Sleep(500 * time.Millisecond)
	atStart := proc.rss(t)

	var durations []time.Duration
	expectedEvents := 0
	call := func(index int) {
		elapsed, events, err := resolveFixtureSkill(t.Context(), proc.session, skills, fmt.Sprintf("memory-%d", index), index)
		if err != nil {
			t.Fatalf("skill_resolve %d: %v; stderr=%s", index, err, proc.stderr.String())
		}
		durations = append(durations, elapsed)
		expectedEvents += events
	}
	call(0)
	afterOne := proc.rss(t)
	for index := 1; index <= resolves; index++ {
		call(index)
	}
	afterMany := proc.rss(t)
	mappingsAfterMany := residentByMapping(proc.command.Process.Pid)
	time.Sleep(idle)
	afterIdle := proc.rss(t)
	mappingsAfterIdle := residentByMapping(proc.command.Process.Pid)
	proc.close(t)

	written := countTelemetryEvents(t, root)
	p50, p95 := durationPercentile(durations, 50), durationPercentile(durations, 95)
	if diagnostics := proc.stderr.String(); diagnostics != "" {
		// Normally empty; with SKILLHUB_MEMORY_ENV="GODEBUG=gctrace=1" it shows the Go heap.
		t.Logf("serve stderr (last 1500 bytes):\n%s", diagnostics[max(0, len(diagnostics)-1500):])
	}
	t.Logf("mcp memory skills=%d env=%q", skills, strings.Join(extraEnv, " "))
	t.Logf("memory columns are RSS / PSS / private (PSS and private need Linux smaps_rollup)")
	t.Logf("| start | after 1 resolve | after %d resolves | after %s idle | resolve p50 | resolve p95 | events written/expected |", resolves+1, idle)
	t.Logf("| %s | %s | %s | %s | %s | %s | %d/%d |", atStart, afterOne, afterMany, afterIdle, roundMS(p50), roundMS(p95), written, expectedEvents)
	if mappingsAfterMany != "" {
		t.Logf("resident memory by mapping after the resolves: %s", mappingsAfterMany)
		t.Logf("resident memory by mapping after idle: %s", mappingsAfterIdle)
	}

	if written != expectedEvents {
		t.Errorf("telemetry events written = %d, want %d", written, expectedEvents)
	}
	if p95 >= memoryBudgetResolveHang {
		t.Errorf("skill_resolve p95 %s exceeds hang ceiling %s", p95, memoryBudgetResolveHang)
	}
	for _, sample := range []memSample{atStart, afterOne, afterMany, afterIdle} {
		if sample.rss > memoryBudgetCeilingKB {
			t.Errorf("resident memory %s exceeds ceiling %s", formatKB(sample.rss), formatKB(memoryBudgetCeilingKB))
		}
	}
}

// TestMCPHeapProfilePerformance serves in-process over an in-memory transport,
// drives the same resolves and writes a heap profile, so `go tool pprof` can
// show where a working server keeps its memory.
func TestMCPHeapProfilePerformance(t *testing.T) {
	requireMemoryPerf(t)
	skills := envInt(t, "SKILLHUB_MEMORY_SKILLS", 32)
	resolves := envInt(t, "SKILLHUB_MEMORY_RESOLVES", 1000)
	// Sample every 4 KiB allocated so small long-lived structures show up.
	previousRate := runtime.MemProfileRate
	runtime.MemProfileRate = 4096
	t.Cleanup(func() { runtime.MemProfileRate = previousRate })
	profilePath := os.Getenv("SKILLHUB_HEAP_PROFILE")
	if profilePath == "" {
		profilePath = filepath.Join(t.TempDir(), "heap.pprof")
	}

	binary := buildSkillHub(t)
	root := newMemoryWorkspace(t, binary, skills)
	var before runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)

	adapter, server, err := New(root, nil, ProfileRuntime)
	if err != nil {
		t.Fatal(err)
	}
	recorder, err := (app.TelemetryService{}).Open(adapter.workspace)
	if err != nil {
		t.Fatal(err)
	}
	adapter.telemetry = recorder
	adapter.resolver.Telemetry = recorder
	adapter.feedback.Recorder = recorder
	adapter.source.Telemetry = recorder
	adapter.curationUX.Recorder = recorder
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(t.Context(), serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "memory-profile", Version: "1"}, nil)
	session, err := client.Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index <= resolves; index++ {
		if _, _, err := resolveFixtureSkill(t.Context(), session, skills, fmt.Sprintf("profile-%d", index), index); err != nil {
			t.Fatalf("skill_resolve %d: %v", index, err)
		}
	}
	if err := recorder.Flush(t.Context()); err != nil {
		t.Fatal(err)
	}
	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	file, err := os.Create(profilePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := pprof.Lookup("heap").WriteTo(file, 0); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	t.Logf("heap profile: %s (inspect with: go tool pprof -sample_index=inuse_space -top %s)", profilePath, profilePath)
	t.Logf("go runtime after %d resolves: heap_inuse=%s (before %s) heap_idle=%s heap_released=%s stack_inuse=%s sys=%s (before %s) num_gc=%d",
		resolves+1, formatBytes(after.HeapInuse), formatBytes(before.HeapInuse), formatBytes(after.HeapIdle), formatBytes(after.HeapReleased),
		formatBytes(after.StackInuse), formatBytes(after.Sys), formatBytes(before.Sys), after.NumGC-before.NumGC)

	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	if err := serverSession.Wait(); err != nil {
		t.Logf("server session ended: %v", err)
	}
	closeContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := recorder.Close(closeContext); err != nil {
		t.Fatal(err)
	}
}

// TestMCPFanoutBench starts N processes on one workspace, each calling
// skill_resolve at a fixed rate, and reports total resident memory, latency and
// telemetry delivery. Run it through scripts/mcp-fanout-bench.sh.
func TestMCPFanoutBench(t *testing.T) {
	requireMemoryPerf(t)
	if _, err := readMemory(os.Getpid()); err != nil {
		t.Skipf("resident memory is not readable on %s: %v", runtime.GOOS, err)
	}
	procs := envInt(t, "SKILLHUB_FANOUT_PROCS", 20)
	rate := envInt(t, "SKILLHUB_FANOUT_RATE", 1)
	duration := envDuration(t, "SKILLHUB_FANOUT_DURATION", 30*time.Second)
	quiet := envDuration(t, "SKILLHUB_FANOUT_IDLE", 0)
	skills := envInt(t, "SKILLHUB_MEMORY_SKILLS", 32)
	extraEnv := strings.Fields(os.Getenv("SKILLHUB_MEMORY_ENV"))
	if procs < 1 || rate < 0 {
		t.Fatalf("SKILLHUB_FANOUT_PROCS must be >= 1 and SKILLHUB_FANOUT_RATE >= 0, got %d and %d", procs, rate)
	}

	binary := buildSkillHub(t)
	serveBinary := memoryServeBinary(binary)
	root := newMemoryWorkspace(t, binary, skills)
	servers := make([]*memoryServe, procs)
	for index := range servers {
		servers[index] = startMemoryServe(t, serveBinary, root, extraEnv)
	}
	time.Sleep(500 * time.Millisecond)
	idleTotal := totalRSS(t, servers)

	type workerResult struct {
		durations []time.Duration
		events    int
		failures  int
	}
	results := make([]workerResult, procs)
	deadline := time.Now().Add(duration)
	var peakMu sync.Mutex
	peak := idleTotal
	stopSampling := make(chan struct{})
	samplerDone := make(chan struct{})
	go func() {
		defer close(samplerDone)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stopSampling:
				return
			case <-ticker.C:
				sum, err := sumMemory(servers)
				if err != nil {
					continue
				}
				peakMu.Lock()
				peak = peak.max(sum)
				peakMu.Unlock()
			}
		}
	}()
	var workers sync.WaitGroup
	for index, server := range servers {
		workers.Add(1)
		go func() {
			defer workers.Done()
			var ticker *time.Ticker
			if rate > 0 {
				ticker = time.NewTicker(time.Second / time.Duration(rate))
				defer ticker.Stop()
			}
			result := &results[index]
			for call := 0; time.Now().Before(deadline); call++ {
				if ticker != nil {
					<-ticker.C
				}
				elapsed, events, err := resolveFixtureSkill(t.Context(), server.session, skills, fmt.Sprintf("fanout-%d-%d", index, call), index*7919+call)
				if err != nil {
					result.failures++
					continue
				}
				result.durations = append(result.durations, elapsed)
				result.events += events
			}
		}()
	}
	workers.Wait()
	workingTotal := totalRSS(t, servers)
	close(stopSampling)
	<-samplerDone
	quietLabel, quietTotal := "not measured", ""
	if quiet > 0 {
		time.Sleep(quiet)
		quietLabel, quietTotal = quiet.String(), totalRSS(t, servers).String()
	}
	for _, server := range servers {
		server.close(t)
	}

	var durations []time.Duration
	expected, failures := 0, 0
	for _, result := range results {
		durations = append(durations, result.durations...)
		expected += result.events
		failures += result.failures
	}
	if len(durations) == 0 {
		t.Fatal("no skill_resolve call succeeded")
	}
	written := countTelemetryEvents(t, root)
	lost := expected - written
	t.Logf("mcp fanout procs=%d rate=%d/s/process duration=%s skills=%d env=%q", procs, rate, duration, skills, strings.Join(extraEnv, " "))
	t.Logf("memory columns are RSS / PSS / private summed over all processes (PSS and private need Linux smaps_rollup)")
	t.Logf("| processes | rate per process | calls | failed calls | events written/expected | lost | total at start | total working | peak while working | per process working | total after %s idle | resolve p50 | resolve p95 |", quietLabel)
	t.Logf("| %d | %s | %d | %d | %d/%d | %d | %s | %s | %s | %s | %s | %s | %s |", procs, rateLabel(rate), len(durations), failures, written, expected, lost,
		idleTotal, workingTotal, peak, workingTotal.div(int64(procs)), quietTotal,
		roundMS(durationPercentile(durations, 50)), roundMS(durationPercentile(durations, 95)))
	if lost != 0 || failures != 0 {
		for index, server := range servers {
			if diagnostics := server.stderr.String(); diagnostics != "" {
				t.Logf("serve %d stderr (last 1500 bytes):\n%s", index, diagnostics[max(0, len(diagnostics)-1500):])
			}
		}
	}
	if rate > 0 && (lost != 0 || failures != 0) {
		t.Errorf("at %d calls per second per process telemetry lost %d events and %d calls failed", rate, lost, failures)
	}
}

// memoryServeBinary returns SKILLHUB_MEMORY_BINARY when set, so two binaries
// can be compared on the same fixture; otherwise the freshly compiled one.
func memoryServeBinary(fallback string) string {
	if override := os.Getenv("SKILLHUB_MEMORY_BINARY"); override != "" {
		return override
	}
	return fallback
}

type memoryServe struct {
	session *mcp.ClientSession
	command *exec.Cmd
	stderr  *bytes.Buffer
	closed  bool
}

func startMemoryServe(t *testing.T, binary, root string, extraEnv []string) *memoryServe {
	t.Helper()
	stderr := &bytes.Buffer{}
	command := exec.Command(binary, "mcp", "serve", "--profile", "runtime", "--workspace", root)
	command.Env = append(os.Environ(), extraEnv...)
	command.Stderr = &lockedWriter{target: stderr}
	client := mcp.NewClient(&mcp.Implementation{Name: "memory-budget", Version: "1"}, nil)
	session, err := client.Connect(t.Context(), &mcp.CommandTransport{Command: command, TerminateDuration: 15 * time.Second}, nil)
	if err != nil {
		t.Fatalf("connect: %v; stderr=%s", err, stderr.String())
	}
	served := &memoryServe{session: session, command: command, stderr: stderr}
	t.Cleanup(func() { served.close(t) })
	return served
}

func (served *memoryServe) rss(t *testing.T) memSample {
	t.Helper()
	kb, err := readMemory(served.command.Process.Pid)
	if err != nil {
		t.Fatalf("read resident memory of pid %d: %v", served.command.Process.Pid, err)
	}
	return kb
}

// close ends stdin so the server flushes telemetry and exits; the transport
// kills the process if it does not exit within its terminate duration.
func (served *memoryServe) close(t *testing.T) {
	t.Helper()
	if served.closed {
		return
	}
	served.closed = true
	if err := served.session.Close(); err != nil {
		t.Logf("close serve pid %d: %v; stderr=%s", served.command.Process.Pid, err, served.stderr.String())
	}
}

type lockedWriter struct {
	mu     sync.Mutex
	target *bytes.Buffer
}

func (writer *lockedWriter) Write(data []byte) (int, error) {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	return writer.target.Write(data)
}

func totalRSS(t *testing.T, servers []*memoryServe) memSample {
	t.Helper()
	sum, err := sumMemory(servers)
	if err != nil {
		t.Fatal(err)
	}
	return sum
}

func sumMemory(servers []*memoryServe) (memSample, error) {
	var sum memSample
	for _, server := range servers {
		sample, err := readMemory(server.command.Process.Pid)
		if err != nil {
			return memSample{}, err
		}
		sum = sum.add(sample)
	}
	return sum, nil
}

// newMemoryWorkspace creates a temp workspace with a catalog that already
// matches its files, as on a machine where the hub has been served before.
func newMemoryWorkspace(t *testing.T, binary string, skills int) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "workspace")
	if output, err := exec.Command(binary, "init", root, "--yes").CombinedOutput(); err != nil {
		t.Fatalf("skillhub init: %v\n%s", err, output)
	}
	for index := 0; index < skills; index++ {
		id := fmt.Sprintf("memory-skill-%03d", index)
		directory := filepath.Join(root, "skills", "memory", id)
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
		metadata := fmt.Sprintf("schema_version: 1\nid: %s\nname: Memory Skill %03d\nstatus: active\ndescription: Review memory workspace target %03d.\nquality:\n  reviewed: true\nrouting:\n  triggers: [review memory target %03d]\n  operations: [review]\n  not_for: [write marketing prose]\n  min_scope: multi_step\n", id, index, index, index)
		body := fmt.Sprintf("---\nname: %s\ndescription: Review memory workspace target %03d.\n---\n\n# Memory Skill %03d\n\nReview target %03d with evidence.\n", id, index, index, index)
		if err := os.WriteFile(filepath.Join(directory, "skill.meta.yaml"), []byte(metadata), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, "SKILL.md"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := (app.CatalogService{}).EnsureCatalog(context.Background(), root); err != nil {
		t.Fatalf("ensure catalog: %v", err)
	}
	return root
}

// resolveFixtureSkill calls skill_resolve for one fixture skill and returns the
// round-trip time and the telemetry events the call must produce: one for the
// completed resolution, plus one when a primary skill was recommended.
func resolveFixtureSkill(ctx context.Context, session *mcp.ClientSession, skills int, requestID string, target int) (time.Duration, int, error) {
	description := "review memory target"
	if skills > 0 {
		description = fmt.Sprintf("review memory target %03d", target%skills)
	}
	started := time.Now()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "skill_resolve", Arguments: map[string]any{
		"schema_version": SchemaVersion, "request_id": requestID,
		"task": map[string]any{"description": description, "scope": "multi_step"}, "operation": "review",
	}})
	elapsed := time.Since(started)
	if err != nil {
		return elapsed, 0, err
	}
	if result.IsError {
		return elapsed, 0, fmt.Errorf("tool error: %v", result.Content)
	}
	encoded, err := json.Marshal(result.StructuredContent)
	if err != nil {
		return elapsed, 0, err
	}
	var outcome toolOutcome[resolveResult]
	if err := json.Unmarshal(encoded, &outcome); err != nil {
		return elapsed, 0, err
	}
	if outcome.Result == nil {
		return elapsed, 0, fmt.Errorf("skill_resolve returned no result: %s", encoded)
	}
	if outcome.Result.Resolution.Primary != nil {
		return elapsed, 2, nil
	}
	return elapsed, 1, nil
}

func countTelemetryEvents(t *testing.T, root string) int {
	t.Helper()
	database, err := sql.Open("sqlite", filepath.Join(root, "runtime", "telemetry.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	var count int
	query := fmt.Sprintf("SELECT COUNT(*) FROM telemetry_events WHERE kind IN ('%s', '%s', '%s')",
		telemetry.EventResolutionCompleted, telemetry.EventResolutionRecommended, telemetry.EventResolutionFailed)
	if err := database.QueryRow(query).Scan(&count); err != nil {
		t.Fatalf("count telemetry events: %v", err)
	}
	return count
}

// memSample is the memory of one process (or a sum) in KiB. RSS counts
// shared pages, such as the mapped binary, in every process; PSS splits shared
// pages between the processes that map them, so PSS sums to the real cost of a
// fan-out; private is memory no other process shares. PSS and private are -1
// when the platform does not expose them.
type memSample struct {
	rss, pss, private int64
}

func (sample memSample) add(other memSample) memSample {
	sum := memSample{rss: sample.rss + other.rss, pss: sample.pss + other.pss, private: sample.private + other.private}
	if sample.pss < 0 || other.pss < 0 {
		sum.pss, sum.private = -1, -1
	}
	return sum
}

func (sample memSample) max(other memSample) memSample {
	return memSample{rss: max(sample.rss, other.rss), pss: max(sample.pss, other.pss), private: max(sample.private, other.private)}
}

func (sample memSample) div(count int64) memSample {
	if sample.pss < 0 {
		return memSample{rss: sample.rss / count, pss: -1, private: -1}
	}
	return memSample{rss: sample.rss / count, pss: sample.pss / count, private: sample.private / count}
}

func (sample memSample) String() string {
	if sample.pss < 0 {
		return formatKB(sample.rss) + " / n/a / n/a"
	}
	return fmt.Sprintf("%s / %s / %s", formatKB(sample.rss), formatKB(sample.pss), formatKB(sample.private))
}

// readMemory samples a process. Linux reads /proc/<pid>/smaps_rollup; other
// Unix systems ask ps for RSS only. Windows is not supported.
func readMemory(pid int) (memSample, error) {
	switch runtime.GOOS {
	case "linux":
		data, err := os.ReadFile(fmt.Sprintf("/proc/%d/smaps_rollup", pid))
		if err != nil {
			return memSample{}, err
		}
		fields := map[string]int64{}
		for _, line := range strings.Split(string(data), "\n") {
			name, value, ok := strings.Cut(line, ":")
			if !ok {
				continue
			}
			kb, err := strconv.ParseInt(strings.TrimSuffix(strings.TrimSpace(value), " kB"), 10, 64)
			if err == nil {
				fields[name] = kb
			}
		}
		rss, ok := fields["Rss"]
		if !ok {
			return memSample{}, fmt.Errorf("Rss missing from smaps_rollup of pid %d", pid)
		}
		return memSample{rss: rss, pss: fields["Pss"], private: fields["Private_Clean"] + fields["Private_Dirty"]}, nil
	case "windows":
		return memSample{}, fmt.Errorf("resident memory sampling is not implemented on windows")
	default:
		output, err := exec.Command("ps", "-o", "rss=", "-p", strconv.Itoa(pid)).Output()
		if err != nil {
			return memSample{}, err
		}
		rss, err := strconv.ParseInt(strings.TrimSpace(string(output)), 10, 64)
		if err != nil {
			return memSample{}, err
		}
		return memSample{rss: rss, pss: -1, private: -1}, nil
	}
}

// residentByMapping summarises a Linux process's resident memory by mapping
// name ("binary" for the executable, "[anon]" for unnamed anonymous memory such
// as the SQLite allocator, "[anon: Go: heap]" for the Go heap). It returns ""
// where /proc/<pid>/smaps is unavailable.
func residentByMapping(pid int) string {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/smaps", pid))
	if err != nil {
		return ""
	}
	executable, _ := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
	totals := map[string]int64{}
	counts := map[string]int{}
	name := ""
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 5 && strings.Contains(fields[0], "-") && !strings.HasSuffix(fields[0], ":") {
			name = strings.Join(fields[5:], " ")
			switch name {
			case "":
				name = "[anon]"
			case executable:
				name = "binary"
			}
			counts[name]++
			continue
		}
		if len(fields) == 3 && fields[0] == "Rss:" {
			if kb, err := strconv.ParseInt(fields[1], 10, 64); err == nil {
				totals[name] += kb
			}
		}
	}
	names := make([]string, 0, len(totals))
	for key := range totals {
		names = append(names, key)
	}
	sort.Slice(names, func(i, j int) bool { return totals[names[i]] > totals[names[j]] })
	parts := make([]string, 0, 6)
	for _, key := range names[:min(6, len(names))] {
		parts = append(parts, fmt.Sprintf("%s %s in %d maps", key, formatKB(totals[key]), counts[key]))
	}
	return strings.Join(parts, "; ")
}

func envInt(t *testing.T, key string, fallback int) int {
	t.Helper()
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		t.Fatalf("%s must be a non-negative integer, got %q", key, raw)
	}
	return value
}

func envDuration(t *testing.T, key string, fallback time.Duration) time.Duration {
	t.Helper()
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	value, err := time.ParseDuration(raw)
	if err != nil || value < 0 {
		t.Fatalf("%s must be a non-negative duration such as 60s, got %q", key, raw)
	}
	return value
}

func durationPercentile(samples []time.Duration, percentile int) time.Duration {
	if len(samples) == 0 {
		return 0
	}
	ordered := append([]time.Duration(nil), samples...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	index := (percentile*len(ordered)+99)/100 - 1
	return ordered[max(index, 0)]
}

func roundMS(value time.Duration) string {
	return fmt.Sprintf("%.1f ms", float64(value)/float64(time.Millisecond))
}

func formatKB(kb int64) string {
	return fmt.Sprintf("%.1f MB", float64(kb)/1024)
}

func formatBytes(value uint64) string {
	return fmt.Sprintf("%.1f MB", float64(value)/(1<<20))
}

func rateLabel(rate int) string {
	if rate == 0 {
		return "back-to-back"
	}
	return fmt.Sprintf("%d/s", rate)
}

func memoryRaceEnabled() bool {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return false
	}
	for _, setting := range info.Settings {
		if setting.Key == "-race" && setting.Value == "true" {
			return true
		}
	}
	return false
}
