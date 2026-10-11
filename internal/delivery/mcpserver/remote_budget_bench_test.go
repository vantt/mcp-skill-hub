package mcpserver

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"text/tabwriter"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vantt/mcp-skill-hub/internal/app"
	"gopkg.in/yaml.v3"
)

// The budgets of the remote hub plan ("Success Criteria", part 2), checked by
// TestRemoteBudgetBench. A loopback HTTP link stands in for a LAN.
const (
	budgetResolveOverStdio  = 50 * time.Millisecond
	budgetFirstFetchLAN     = 500 * time.Millisecond
	budgetFirstFetchSlow    = 2 * time.Second
	budgetCachedRepeat      = 20 * time.Millisecond
	benchSlowLinkRoundTrips = 100 * time.Millisecond
)

// TestRemoteBudgetBench measures what an agent pays to resolve and get a
// skill over three transports and fails when a budget of the plan is missed:
//
//   - stdio: `skillhub mcp serve`, the hub on the agent's own machine;
//   - loopback HTTP: the agent host's `skillhub mcp proxy` talking to
//     `skillhub serve mcp --addr 127.0.0.1:0`, a stand-in for a LAN;
//   - a simulated link: the same, through a relay that adds a round-trip
//     time to every exchange and one more to every new connection (no root
//     needed, no bandwidth limit).
//
// Hub and proxy are real processes. Every run of a remote transport starts a
// new proxy with an empty cache, so each first fetch is a real download. The
// relay counts the HTTP requests and bytes between proxy and hub: a first
// fetch must be one request, a cached repeat none at all. At the end a
// metadata-only change and then a content change on the hub show which
// skills are fetched again.
//
// It is wall-clock and machine dependent, so it runs only with SKILLHUB_PERF=1;
// `make remote-bench` (scripts/remote-bench.sh) is the usual entry point.
// Tunables, all optional: SKILLHUB_REMOTE_HUB, SKILLHUB_REMOTE_SKILLS (see
// TestRemoteFetchBench), SKILLHUB_REMOTE_RUNS (sessions per transport,
// default 20), SKILLHUB_REMOTE_WARM (warm resolves per session, default 10),
// SKILLHUB_REMOTE_RTT (simulated round trip, default 100ms; the 2 s budget
// is set for 100 ms) and SKILLHUB_REMOTE_BENCH_REPORT (a file that receives
// the report).
func TestRemoteBudgetBench(t *testing.T) {
	requireMemoryPerf(t)
	runs := envInt(t, "SKILLHUB_REMOTE_RUNS", 20)
	warm := envInt(t, "SKILLHUB_REMOTE_WARM", 10)
	rtt := envDuration(t, "SKILLHUB_REMOTE_RTT", benchSlowLinkRoundTrips)
	if runs < 1 || warm < 1 {
		t.Fatal("SKILLHUB_REMOTE_RUNS and SKILLHUB_REMOTE_WARM must be at least 1")
	}
	skillIDs := remoteBenchSkillIDs()
	binary := buildSkillHub(t)
	var root, source string
	if hubRepo := os.Getenv("SKILLHUB_REMOTE_HUB"); hubRepo != "" {
		root = cloneRemoteBenchHub(t, binary, hubRepo, skillIDs)
		source = "read-only clone of " + hubRepo
	} else {
		root = newRemoteBenchFixture(t, binary)
		source = "fixture skills shaped like test-audit and herdr-cook-plan"
	}
	tasks := map[string]string{}
	for _, id := range skillIDs {
		tasks[id] = remoteBenchTaskText(t, root, id)
	}
	hub := startHubProcess(t, binary, root, remoteBenchEnv())
	target := strings.TrimPrefix(hub.url, "http://")

	report := &benchReport{}
	report.line("Skill Hub remote benchmark, %s", time.Now().Format("2006-01-02 15:04 MST"))
	report.line("Machine: %s/%s, %d CPUs. Skills: %s.", runtime.GOOS, runtime.GOARCH, runtime.NumCPU(), source)
	report.line("Each transport: %d agent sessions, each with a first and %d warm skill_resolve calls, then per skill a first and a repeated skill_get.", runs, warm)
	report.line("")

	stdio := measureStdio(t, binary, root, skillIDs, tasks, runs, warm)
	report.transport("stdio (hub on this machine, no network)", stdio, skillIDs)

	type remoteTransport struct {
		name  string
		link  *benchLink
		fetch time.Duration
	}
	transports := []remoteTransport{
		{"loopback HTTP (stands in for a LAN)", startLink(t, target, 0), budgetFirstFetchLAN},
		{fmt.Sprintf("simulated link, %s round trip", rtt), startLink(t, target, rtt), budgetFirstFetchSlow},
	}
	var remote []benchTransport
	for _, transport := range transports {
		measured := measureRemote(t, binary, hub, transport.link, skillIDs, tasks, runs, warm)
		remote = append(remote, measured)
		report.transport(transport.name, measured, skillIDs)
	}

	report.line("Changes on the hub (one proxy session per link, every skill fetched first)")
	var changes []changeRow
	for index, transport := range transports {
		changes = append(changes, checkHubChanges(t, binary, root, hub, transport.link, transport.name, skillIDs, tasks, index)...)
	}
	changeRows := make([][]string, 0, len(changes))
	for _, change := range changes {
		changeRows = append(changeRows, []string{change.link, change.change, change.skill, change.answer, fmt.Sprint(change.requests), fmt.Sprint(change.bytes)})
	}
	report.table([]string{"Link", "Hub change", "skill_get of", "Answer", "Requests", "Bytes"}, changeRows)

	report.line("Budgets")
	var budgets [][]string
	check := func(name, measured, limit string, ok bool) {
		verdict := "PASS"
		if !ok {
			verdict = "FAIL"
			t.Errorf("budget missed: %s: %s, budget %s", name, measured, limit)
		}
		budgets = append(budgets, []string{verdict, name, measured, limit})
	}
	lan := remote[0]
	check("skill_resolve on loopback HTTP, first of a session, p95", roundMS(lan.firstResolve.p95()), "stdio "+roundMS(stdio.firstResolve.p95())+" + 50 ms",
		lan.firstResolve.p95() <= stdio.firstResolve.p95()+budgetResolveOverStdio)
	check("skill_resolve on loopback HTTP, warm, p95", roundMS(lan.warmResolve.p95()), "stdio "+roundMS(stdio.warmResolve.p95())+" + 50 ms",
		lan.warmResolve.p95() <= stdio.warmResolve.p95()+budgetResolveOverStdio)
	for index, transport := range transports {
		measured := remote[index]
		for _, id := range skillIDs {
			skill := measured.skills[id]
			check(fmt.Sprintf("first skill_get %s, %s, p95", id, linkLabel(index)), roundMS(skill.first.p95()), roundMS(transport.fetch), skill.first.p95() <= transport.fetch)
			check(fmt.Sprintf("first skill_get %s, %s, requests per fetch", id, linkLabel(index)), requestRange(skill.firstRequests), "1", allEqual(skill.firstRequests, 1))
			check(fmt.Sprintf("cached skill_get %s, %s, p95", id, linkLabel(index)), roundMS(skill.repeat.p95()), roundMS(budgetCachedRepeat), skill.repeat.p95() <= budgetCachedRepeat)
			check(fmt.Sprintf("cached skill_get %s, %s, requests and bytes", id, linkLabel(index)), fmt.Sprintf("requests %s, bytes %s", requestRange(skill.repeatRequests), requestRange(skill.repeatBytes)), "requests 0, bytes 0",
				allEqual(skill.repeatRequests, 0) && allEqual(skill.repeatBytes, 0))
		}
	}
	for _, change := range changes {
		name := fmt.Sprintf("%s, %s: skill_get %s", change.link, change.change, change.skill)
		measured := fmt.Sprintf("%s, requests %d, bytes %d", change.answer, change.requests, change.bytes)
		if change.wantFetch {
			check(name, measured, "fetched, requests 1", change.answer == remoteCacheFetched && change.requests == 1)
		} else {
			check(name, measured, "hit, requests 0, bytes 0", change.answer == remoteCacheHit && change.requests == 0 && change.bytes == 0)
		}
	}
	report.table([]string{"Result", "Budget", "Measured", "Limit"}, budgets)
	if rtt != benchSlowLinkRoundTrips {
		report.line("Note: the simulated link used %s instead of %s; its 2 s budget is set for %s.", rtt, benchSlowLinkRoundTrips, benchSlowLinkRoundTrips)
	}

	text := report.String()
	for _, line := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		t.Log(line)
	}
	if path := os.Getenv("SKILLHUB_REMOTE_BENCH_REPORT"); path != "" {
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Errorf("write the report: %v", err)
		}
	}
}

// durations is a set of timings of one step.
type durations []time.Duration

func (values durations) p50() time.Duration { return durationPercentile(values, 50) }
func (values durations) p95() time.Duration { return durationPercentile(values, 95) }

type benchSkill struct {
	first, repeat               durations
	firstRequests, firstBytes   []int64
	repeatRequests, repeatBytes []int64
}

type benchTransport struct {
	startup, firstResolve, warmResolve durations
	skills                             map[string]*benchSkill
	local                              bool
}

func newBenchTransport(skillIDs []string) benchTransport {
	measured := benchTransport{skills: map[string]*benchSkill{}}
	for _, id := range skillIDs {
		measured.skills[id] = &benchSkill{}
	}
	return measured
}

func timedResolve(t *testing.T, session *mcp.ClientSession, task, requestID string) time.Duration {
	t.Helper()
	started := time.Now()
	resolveFor(t, session, task, requestID)
	return time.Since(started)
}

// measureStdio times the local hub: a new `skillhub mcp serve` per session.
func measureStdio(t *testing.T, binary, root string, skillIDs []string, tasks map[string]string, runs, warm int) benchTransport {
	t.Helper()
	measured := newBenchTransport(skillIDs)
	measured.local = true
	for run := range runs {
		bench := startRemoteBenchSession(t, binary, root)
		measured.startup = append(measured.startup, bench.startup)
		measured.firstResolve = append(measured.firstResolve, timedResolve(t, bench.session, tasks[skillIDs[0]], fmt.Sprintf("stdio-%d-first", run)))
		for call := range warm {
			id := skillIDs[call%len(skillIDs)]
			measured.warmResolve = append(measured.warmResolve, timedResolve(t, bench.session, tasks[id], fmt.Sprintf("stdio-%d-%d", run, call)))
		}
		for _, id := range skillIDs {
			skill := measured.skills[id]
			for attempt := range 2 {
				started := time.Now()
				result, encoded := toolCall(t, bench.session, "skill_get", map[string]any{"skill_id": id})
				elapsed := time.Since(started)
				if result.IsError || !strings.Contains(string(encoded), `"path"`) {
					t.Fatalf("local skill_get %s = %s", id, encoded)
				}
				if attempt == 0 {
					skill.first = append(skill.first, elapsed)
				} else {
					skill.repeat = append(skill.repeat, elapsed)
				}
			}
		}
		bench.close(t)
	}
	return measured
}

// measureRemote times the proxy against the hub through link, with a new
// proxy and an empty cache per session.
func measureRemote(t *testing.T, binary string, hub *hubProcess, link *benchLink, skillIDs []string, tasks map[string]string, runs, warm int) benchTransport {
	t.Helper()
	measured := newBenchTransport(skillIDs)
	for run := range runs {
		proxied := startProxyProcess(t, proxyCommand(binary, link.url, hub.tokenFile, append(remoteBenchEnv(), "XDG_STATE_HOME="+t.TempDir())))
		measured.startup = append(measured.startup, proxied.startup)
		measured.firstResolve = append(measured.firstResolve, timedResolve(t, proxied.session, tasks[skillIDs[0]], fmt.Sprintf("remote-%d-first", run)))
		for call := range warm {
			id := skillIDs[call%len(skillIDs)]
			measured.warmResolve = append(measured.warmResolve, timedResolve(t, proxied.session, tasks[id], fmt.Sprintf("remote-%d-%d", run, call)))
		}
		for _, id := range skillIDs {
			skill := measured.skills[id]
			// The usual pair: the resolution names the version, then the get.
			resolveFor(t, proxied.session, tasks[id], fmt.Sprintf("remote-%d-%s", run, id))
			for attempt := range 2 {
				requests, bytes := link.counts()
				started := time.Now()
				result, encoded := toolCall(t, proxied.session, "skill_get", map[string]any{"skill_id": id})
				elapsed := time.Since(started)
				afterRequests, afterBytes := link.counts()
				got := decodeSkillGet(t, encoded)
				want := remoteCacheFetched
				if attempt == 1 {
					want = remoteCacheHit
				}
				if result.IsError || got.Remote == nil || got.Remote.Cache != want {
					t.Fatalf("skill_get %s attempt %d = %s", id, attempt, encoded)
				}
				if attempt == 0 {
					skill.first = append(skill.first, elapsed)
					skill.firstRequests = append(skill.firstRequests, afterRequests-requests)
					skill.firstBytes = append(skill.firstBytes, afterBytes-bytes)
				} else {
					skill.repeat = append(skill.repeat, elapsed)
					skill.repeatRequests = append(skill.repeatRequests, afterRequests-requests)
					skill.repeatBytes = append(skill.repeatBytes, afterBytes-bytes)
				}
			}
		}
		proxied.close(t)
	}
	return measured
}

// changeRow is what one skill_get cost after a change on the hub.
type changeRow struct {
	link, change, skill, answer string
	requests, bytes             int64
	// wantFetch is set for the skill whose content changed.
	wantFetch bool
}

// checkHubChanges fetches every skill through one proxy session, then makes a
// metadata-only change to every skill on the hub, and then a content change
// to the last skill, and records what each skill_get costs after a fresh
// resolution.
func checkHubChanges(t *testing.T, binary, root string, hub *hubProcess, link *benchLink, linkName string, skillIDs []string, tasks map[string]string, round int) []changeRow {
	t.Helper()
	label := linkLabel(round)
	proxied := startProxyProcess(t, proxyCommand(binary, link.url, hub.tokenFile, append(remoteBenchEnv(), "XDG_STATE_HOME="+t.TempDir())))
	defer proxied.close(t)
	get := func(id, requestID string) (string, int64, int64) {
		resolveFor(t, proxied.session, tasks[id], requestID)
		requests, bytes := link.counts()
		result, encoded := toolCall(t, proxied.session, "skill_get", map[string]any{"skill_id": id})
		afterRequests, afterBytes := link.counts()
		got := decodeSkillGet(t, encoded)
		if result.IsError || got.Remote == nil {
			t.Fatalf("%s: skill_get %s = %s", linkName, id, encoded)
		}
		if digest, _ := folderDigest(t, got.Local.Path); digest != hubDigest(t, root, id) {
			t.Fatalf("%s: the copy of %s has digest %s, the hub pins %s", linkName, id, digest, hubDigest(t, root, id))
		}
		return got.Remote.Cache, afterRequests - requests, afterBytes - bytes
	}
	for _, id := range skillIDs {
		get(id, fmt.Sprintf("change-%d-first-%s", round, id))
	}
	var rows []changeRow
	before := map[string]string{}
	for _, id := range skillIDs {
		before[id] = hubDigest(t, root, id)
		editBenchSkillMeta(t, root, id, fmt.Sprintf("remote bench routing change %d", round))
	}
	runRemoteBench(t, binary, "rebuild", "--workspace", root)
	for _, id := range skillIDs {
		if hubDigest(t, root, id) != before[id] {
			t.Fatalf("a routing change altered the digest of %s", id)
		}
		answer, requests, bytes := get(id, fmt.Sprintf("change-%d-meta-%s", round, id))
		rows = append(rows, changeRow{link: label, change: "metadata only, every skill", skill: id, answer: answer, requests: requests, bytes: bytes})
	}
	changed := skillIDs[len(skillIDs)-1]
	editBenchSkillContent(t, binary, root, changed, fmt.Sprintf("Changed on the hub in round %d.", round))
	if hubDigest(t, root, changed) == before[changed] {
		t.Fatalf("a content change kept the digest of %s", changed)
	}
	for _, id := range skillIDs {
		answer, requests, bytes := get(id, fmt.Sprintf("change-%d-content-%s", round, id))
		rows = append(rows, changeRow{link: label, change: "content of " + changed, skill: id, answer: answer, requests: requests, bytes: bytes, wantFetch: id == changed})
	}
	return rows
}

func linkLabel(index int) string {
	if index == 0 {
		return "loopback"
	}
	return "slow link"
}

// skillDirectory finds a skill's folder in any collection of the workspace.
func skillDirectory(t *testing.T, root, id string) string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(root, "skills", "*", id, ".meta", "skill.yaml"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("skill %s in %s: %v, %v", id, root, matches, err)
	}
	return filepath.Dir(filepath.Dir(matches[0]))
}

// editBenchSkillMeta adds a routing trigger, a change that leaves the files a
// hub serves (and so the skill digest) unchanged.
func editBenchSkillMeta(t *testing.T, root, id, trigger string) {
	t.Helper()
	path := filepath.Join(skillDirectory(t, root, id), ".meta", "skill.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := yaml.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	routing, _ := document["routing"].(map[string]any)
	if routing == nil {
		routing = map[string]any{}
		document["routing"] = routing
	}
	triggers, _ := routing["triggers"].([]any)
	routing["triggers"] = append(triggers, trigger)
	encoded, err := yaml.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, encoded, 0o644); err != nil {
		t.Fatal(err)
	}
}

// editBenchSkillContent appends a line to SKILL.md, rebuilds the catalog and,
// in a clone whose content needs approval, approves the new content there.
func editBenchSkillContent(t *testing.T, binary, root, id, line string) {
	t.Helper()
	path := filepath.Join(skillDirectory(t, root, id), "SKILL.md")
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("\n" + line + "\n"); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	runRemoteBench(t, binary, "rebuild", "--workspace", root)
	trust, err := (app.SkillService{}).ContentTrustFor(t.Context(), root, id)
	if err != nil {
		t.Fatal(err)
	}
	if trust.RequiresReview() {
		runRemoteBench(t, binary, "skill", "edit", id, "--workspace", root, "--approve-content", trust.ContentDigest, "--yes")
	}
}

func allEqual(values []int64, want int64) bool {
	for _, value := range values {
		if value != want {
			return false
		}
	}
	return len(values) > 0
}

// requestRange prints one number when every value is the same, else min-max.
func requestRange(values []int64) string {
	if len(values) == 0 {
		return "none"
	}
	low, high := slices.Min(values), slices.Max(values)
	if low == high {
		return fmt.Sprint(low)
	}
	return fmt.Sprintf("%d-%d", low, high)
}

// benchReport collects the plain-text report with aligned columns.
type benchReport struct {
	strings.Builder
}

func (report *benchReport) line(format string, arguments ...any) {
	fmt.Fprintf(report, format+"\n", arguments...)
}

func (report *benchReport) table(header []string, rows [][]string) {
	writer := tabwriter.NewWriter(report, 0, 0, 2, ' ', 0)
	fmt.Fprintln(writer, "  "+strings.Join(header, "\t"))
	for _, row := range rows {
		fmt.Fprintln(writer, "  "+strings.Join(row, "\t"))
	}
	_ = writer.Flush()
	report.line("")
}

func (report *benchReport) transport(name string, measured benchTransport, skillIDs []string) {
	report.line("Transport: %s", name)
	rows := [][]string{
		{"session start (process and initialize)", roundMS(measured.startup.p50()), roundMS(measured.startup.p95()), ""},
		{"skill_resolve, first of a session", roundMS(measured.firstResolve.p50()), roundMS(measured.firstResolve.p95()), ""},
		{"skill_resolve, warm", roundMS(measured.warmResolve.p50()), roundMS(measured.warmResolve.p95()), ""},
	}
	for _, id := range skillIDs {
		skill := measured.skills[id]
		if measured.local {
			rows = append(rows,
				[]string{"skill_get " + id + ", first", roundMS(skill.first.p50()), roundMS(skill.first.p95()), "local snapshot, no transfer"},
				[]string{"skill_get " + id + ", repeat", roundMS(skill.repeat.p50()), roundMS(skill.repeat.p95()), "local snapshot, no transfer"})
			continue
		}
		rows = append(rows,
			[]string{"skill_get " + id + ", first fetch", roundMS(skill.first.p50()), roundMS(skill.first.p95()),
				fmt.Sprintf("requests %s, %.1f KB on the wire", requestRange(skill.firstRequests), float64(slices.Max(skill.firstBytes))/1024)},
			[]string{"skill_get " + id + ", cached repeat", roundMS(skill.repeat.p50()), roundMS(skill.repeat.p95()),
				fmt.Sprintf("requests %s, bytes %s", requestRange(skill.repeatRequests), requestRange(skill.repeatBytes))})
	}
	report.table([]string{"Step", "p50", "p95", "Traffic"}, rows)
}
