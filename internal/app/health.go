package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"

	"github.com/vantt/skillhub-mcp/internal/hostintegration"
	"github.com/vantt/skillhub-mcp/internal/shellquote"
)

// The health check engine behind `skillhub status` and `skillhub doctor`.
//
// One set of checks runs for both commands: status runs them at DepthLight
// (no wait for the workspace lock, at most one short remote probe) and doctor
// at DepthFull. Each check reports Findings; a Finding that doctor --fix can
// repair carries a FixAction. Run groups the findings into Sections (the hub
// section first, then one section per connection of this project or of the
// user account), and ApplyFixes runs the fixes of a Run result.
//
// Other packages extend the hub section by registering a CheckProvider from
// an init function:
//
//	func init() { app.RegisterProvider(nodeCheck{}) }
//
//	type nodeCheck struct{}
//
//	func (nodeCheck) Name() string { return "node" }
//	func (nodeCheck) Applies(report app.RoleReport) bool { return report.HealthHub() != "" }
//	func (nodeCheck) Check(ctx context.Context, report app.RoleReport, depth app.Depth) []app.Finding {
//		if _, err := exec.LookPath("node"); err == nil {
//			return nil
//		}
//		return []app.Finding{{Code: "hub_node_missing", Label: "Node.js", Severity: app.SeverityNotice,
//			Message: "Node.js is not installed; the distill-lab scripts need it.", Manual: "..."}}
//	}

// Depth selects how much the checks do.
type Depth int

const (
	// DepthLight is what status runs: it never waits for the workspace lock
	// and probes a remote hub at most once, with a one second timeout.
	DepthLight Depth = iota
	// DepthFull is what doctor runs: every check, waiting up to the lock
	// timeout for the workspace lock.
	DepthFull
)

// Severity says how a Finding counts.
type Severity string

const (
	// SeverityOK is a check that passed; it is shown so the table is complete.
	SeverityOK Severity = "ok"
	// SeverityNotice is worth knowing but not a problem: doctor still exits 0.
	SeverityNotice Severity = "notice"
	// SeverityProblem makes doctor exit 1 until it is fixed.
	SeverityProblem Severity = "problem"
	// SeveritySkipped is a check that could not run; Message says why.
	SeveritySkipped Severity = "skipped"
)

// Changed lists the files a fix wrote, as absolute paths.
type Changed []string

// FixAction is what `doctor --fix` does for a Finding. Findings that share
// one *FixAction value (for example several hub findings that one hub repair
// fixes) run it once.
type FixAction struct {
	// Summary says in one line what the fix will do, for example
	// "rebuild the search index".
	Summary string
	// Apply makes the change and returns the files it wrote. It must be safe
	// to run when the problem is already gone.
	Apply func(ctx context.Context) (Changed, error)
	// Notes may be set by Apply to sentences the person should read about
	// what the fix did, for example that an old file was removed and what
	// that means for an agent.
	Notes []string
}

// Finding is the result of one check.
type Finding struct {
	// Code is a stable identifier, for example "hub_index_stale"; tests and
	// `doctor --json` consumers match on it.
	Code string
	// Label names the check in the table, for example "Search index".
	Label    string
	Severity Severity
	// Message says in plain language what the check found.
	Message string
	// Fix is nil when doctor --fix cannot fix the finding.
	Fix *FixAction
	// Manual is the one command a person runs when Fix is nil, for example
	// "skillhub maintain migrate"; empty when there is no single command.
	Manual string
	// Reason says why doctor --fix leaves the finding to a person; it is
	// printed under "Not fixed:". Empty uses a generic sentence.
	Reason string
	// Detail is optional longer evidence, for example the managed blocks a
	// fix would rewrite; doctor prints it below the table, status leaves it
	// out.
	Detail string
	// untrack lists the hub files, relative to the hub, that a tracked-file
	// finding asks the person to stop tracking.
	untrack []string
}

// findingJSON is the JSON form of a Finding: the fix is its summary.
type findingJSON struct {
	Code     string   `json:"code"`
	Label    string   `json:"label"`
	Severity Severity `json:"severity"`
	Message  string   `json:"message"`
	Fix      string   `json:"fix,omitempty"`
	Manual   string   `json:"manual,omitempty"`
	Reason   string   `json:"reason,omitempty"`
	Detail   string   `json:"detail,omitempty"`
}

// MarshalJSON writes the fix as its summary, since a function has no JSON form.
func (finding Finding) MarshalJSON() ([]byte, error) {
	value := findingJSON{Code: finding.Code, Label: finding.Label, Severity: finding.Severity, Message: finding.Message, Manual: finding.Manual, Reason: finding.Reason, Detail: finding.Detail}
	if finding.Fix != nil {
		value.Fix = finding.Fix.Summary
	}
	return json.Marshal(value)
}

// UnmarshalJSON reads what MarshalJSON writes. A decoded fix has only its
// summary; it cannot be applied.
func (finding *Finding) UnmarshalJSON(data []byte) error {
	var value findingJSON
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*finding = Finding{Code: value.Code, Label: value.Label, Severity: value.Severity, Message: value.Message, Manual: value.Manual, Reason: value.Reason, Detail: value.Detail}
	if value.Fix != "" {
		finding.Fix = &FixAction{Summary: value.Fix}
	}
	return nil
}

// Section kinds.
const (
	SectionHub    = "hub"
	SectionClient = "client"
)

// Section groups the findings about one hub folder or one connection.
type Section struct {
	// Kind is SectionHub or SectionClient.
	Kind string `json:"kind"`
	// Root is the hub folder, the project folder or the home folder whose
	// files the section is about.
	Root string `json:"root"`
	// Title is the section header, for example "Hub at /home/ann/skill-hub".
	Title    string    `json:"title"`
	Findings []Finding `json:"findings"`
}

// CheckProvider is one group of checks. The engine calls Applies first and
// Check only when it returns true. Findings of a registered provider belong to
// the hub section, the section of report.HealthHub(); Applies should return
// false when that is empty. Check must not write anything and must not send
// the hub token anywhere.
type CheckProvider interface {
	// Name identifies the provider; it must be unique.
	Name() string
	Applies(report RoleReport) bool
	Check(ctx context.Context, report RoleReport, depth Depth) []Finding
}

// sectionProvider is a built-in provider whose findings belong to sections of
// its own (the connections of a project or of the user account).
type sectionProvider interface {
	CheckProvider
	sections(ctx context.Context, report RoleReport, depth Depth) []Section
}

var providers struct {
	sync.Mutex
	list []CheckProvider
}

// RegisterProvider adds checks to the hub section of status and doctor. Call
// it from an init function. It panics on a nil provider or a name that is
// already registered, because both are programming errors.
func RegisterProvider(provider CheckProvider) {
	if provider == nil {
		panic("app.RegisterProvider: nil provider")
	}
	providers.Lock()
	defer providers.Unlock()
	for _, existing := range append(builtinProviders(), providers.list...) {
		if existing.Name() == provider.Name() {
			panic("app.RegisterProvider: a provider named " + provider.Name() + " is already registered")
		}
	}
	providers.list = append(providers.list, provider)
}

func registeredProviders() []CheckProvider {
	providers.Lock()
	defer providers.Unlock()
	return slices.Clone(providers.list)
}

func builtinProviders() []CheckProvider {
	return []CheckProvider{hubCheck{}, clientLocalCheck{}, clientRemoteCheck{}}
}

// Run runs every check that applies to the report: the hub section first
// (the built-in hub checks, then the registered providers), then the
// sections of the local and remote connections.
func Run(ctx context.Context, report RoleReport, depth Depth) []Section {
	var sections []Section
	if hub := report.HealthHub(); hub != "" {
		section := Section{Kind: SectionHub, Root: hub, Title: "Hub at " + hub}
		for _, provider := range append([]CheckProvider{hubCheck{}}, registeredProviders()...) {
			if provider.Applies(report) {
				section.Findings = append(section.Findings, checkSafely(ctx, provider, report, depth)...)
			}
		}
		shareUntrackCommand(hub, section.Findings)
		sections = append(sections, section)
	}
	for _, provider := range []sectionProvider{clientLocalCheck{}, clientRemoteCheck{}} {
		if provider.Applies(report) {
			sections = append(sections, provider.sections(ctx, report, depth)...)
		}
	}
	return sections
}

// checkSafely runs a provider and turns a panic into a problem finding, so one
// broken check cannot stop status or doctor.
func checkSafely(ctx context.Context, provider CheckProvider, report RoleReport, depth Depth) (findings []Finding) {
	defer func() {
		if recovered := recover(); recovered != nil {
			findings = []Finding{{Code: provider.Name() + "_check_failed", Label: provider.Name(), Severity: SeverityProblem,
				Message: fmt.Sprintf("The %s check stopped with an internal error: %v.", provider.Name(), recovered),
				Reason:  "This is a bug in Skill Hub; report it with this message."}}
		}
	}()
	return provider.Check(ctx, report, depth)
}

// sectionFindings flattens the sections of a section provider, for Check.
func sectionFindings(sections []Section) []Finding {
	var findings []Finding
	for _, section := range sections {
		findings = append(findings, section.Findings...)
	}
	return findings
}

// FixReport is what ApplyFixes did.
type FixReport struct {
	Fixed    []FixedItem    `json:"fixed"`
	NotFixed []NotFixedItem `json:"not_fixed"`
}

// FixedItem is one fix that ran, with the codes of the findings it fixed and
// the files it wrote.
type FixedItem struct {
	Codes   []string `json:"codes"`
	Summary string   `json:"summary"`
	Changed Changed  `json:"changed"`
	Notes   []string `json:"notes,omitempty"`
}

// NotFixedItem is a problem doctor --fix left alone, or a fix that failed.
type NotFixedItem struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Reason  string `json:"reason"`
	Manual  string `json:"manual,omitempty"`
}

// ApplyFixes runs the fix of every problem and notice that has one, in section
// order, each FixAction once. Problems without a fix and failed fixes are
// listed as not fixed, with the reason and the command a person runs instead.
// Run the checks again afterwards to see what remains.
func ApplyFixes(ctx context.Context, sections []Section) FixReport {
	report := FixReport{Fixed: []FixedItem{}, NotFixed: []NotFixedItem{}}
	type outcome struct {
		fixed  int
		failed int
	}
	done := map[*FixAction]outcome{}
	for _, section := range sections {
		for _, finding := range section.Findings {
			if finding.Severity != SeverityProblem && finding.Severity != SeverityNotice {
				continue
			}
			if finding.Fix == nil || finding.Fix.Apply == nil {
				if finding.Severity == SeverityProblem {
					report.NotFixed = append(report.NotFixed, NotFixedItem{Code: finding.Code, Message: finding.Message, Reason: notFixedReason(finding), Manual: finding.Manual})
				}
				continue
			}
			if previous, seen := done[finding.Fix]; seen {
				if previous.fixed >= 0 && !slices.Contains(report.Fixed[previous.fixed].Codes, finding.Code) {
					report.Fixed[previous.fixed].Codes = append(report.Fixed[previous.fixed].Codes, finding.Code)
				}
				continue
			}
			if err := ctx.Err(); err != nil {
				report.NotFixed = append(report.NotFixed, NotFixedItem{Code: finding.Code, Message: finding.Message, Reason: "doctor --fix was stopped before it could " + finding.Fix.Summary + "."})
				done[finding.Fix] = outcome{fixed: -1, failed: len(report.NotFixed) - 1}
				continue
			}
			changed, err := finding.Fix.Apply(ctx)
			if err != nil {
				report.NotFixed = append(report.NotFixed, NotFixedItem{Code: finding.Code, Message: finding.Message, Reason: "Could not " + finding.Fix.Summary + ": " + err.Error(), Manual: finding.Manual})
				done[finding.Fix] = outcome{fixed: -1, failed: len(report.NotFixed) - 1}
				continue
			}
			if changed == nil {
				changed = Changed{}
			}
			report.Fixed = append(report.Fixed, FixedItem{Codes: []string{finding.Code}, Summary: finding.Fix.Summary, Changed: changed, Notes: finding.Fix.Notes})
			done[finding.Fix] = outcome{fixed: len(report.Fixed) - 1, failed: -1}
		}
	}
	return report
}

func notFixedReason(finding Finding) string {
	if finding.Reason != "" {
		return finding.Reason
	}
	if finding.Manual != "" {
		return "doctor --fix cannot change this; run the command below."
	}
	return "doctor --fix cannot change this."
}

// HasProblems reports whether any finding is a problem.
func HasProblems(sections []Section) bool {
	return len(ProblemCodes(sections)) > 0
}

// ProblemCodes lists the codes of the problems in the sections, sorted and
// each once.
func ProblemCodes(sections []Section) []string {
	var codes []string
	for _, section := range sections {
		for _, finding := range section.Findings {
			if finding.Severity == SeverityProblem && !slices.Contains(codes, finding.Code) {
				codes = append(codes, finding.Code)
			}
		}
	}
	slices.Sort(codes)
	return codes
}

// describeChange names a host change in one line: host, kind and the file,
// relative to root when it is inside it, with forward slashes.
func describeChange(root string, change hostintegration.Change) string {
	host := string(change.Host)
	if host == "" {
		host = SharedSkillsItemKey
	}
	return host + " " + string(change.Kind) + " " + displayPath(root, change.Path)
}

// blockedSkillFinding reports the system skill files a plan leaves unchanged
// because Skill Hub did not write them. doctor --fix never changes them.
func blockedSkillFinding(root string, blocked []hostintegration.FileState) Finding {
	paths := make([]string, 0, len(blocked))
	for _, file := range blocked {
		paths = append(paths, displayPath(root, file.Path))
	}
	return Finding{Code: "system_skill_blocked", Label: "System skill files", Severity: SeverityProblem,
		Message: "Skill Hub left " + strings.Join(paths, ", ") + " unchanged: " + blocked[0].Blocked,
		Reason:  "doctor --fix never overwrites or deletes a file Skill Hub did not write; review it, delete it, then run doctor --fix again."}
}

// changePreviews joins the managed-block previews of host changes, one block
// per file, for Finding.Detail.
func changePreviews(changes []hostintegration.Change) string {
	var blocks []string
	for _, change := range changes {
		if change.Preview != "" {
			blocks = append(blocks, change.Path+":\n"+strings.TrimRight(change.Preview, "\n"))
		}
	}
	return strings.Join(blocks, "\n")
}

// findFinding returns the first finding with one of the codes and the given
// severity, in section order.
func findFinding(sections []Section, severity Severity, codes ...string) (Section, Finding, bool) {
	for _, section := range sections {
		for _, finding := range section.Findings {
			if finding.Severity == severity && slices.Contains(codes, finding.Code) {
				return section, finding, true
			}
		}
	}
	return Section{}, Finding{}, false
}

// NextStep is the one command status ends with.
type NextStep struct {
	// Label introduces the command, for example "fix what doctor can repair".
	Label   string `json:"label"`
	Command string `json:"command"`
	// Elsewhere is set when the command is not a skillhub command for this
	// folder: a Git line, or a command for the hub machine.
	Elsewhere bool `json:"elsewhere,omitempty"`
}

// HubInventory is what status knows about the skills of the hub, for the
// next-step rules; Known is false when the counts could not be read.
type HubInventory struct {
	Known  bool
	Skills int
}

// StatusNext picks the one runnable command status ends with: the first rule
// of the table in the status/doctor design that matches.
func StatusNext(report RoleReport, sections []Section, inventory HubInventory) NextStep {
	if len(sections) == 0 {
		switch {
		case IsHubRoot(report.StoredHub):
			return NextStep{Label: "connect this project to the hub at " + report.StoredHub, Command: "skillhub connect"}
		case report.StoredHub != "":
			return NextStep{Label: "the hub stored at " + report.StoredHub + " is gone; if you moved it, record its new location", Command: shellquote.Chain("cd <new-location>", "skillhub init"), Elsewhere: true}
		}
		return NextStep{Label: "create a hub", Command: NewHubCommand()}
	}
	if _, _, busy := findFinding(sections, SeverityNotice, "hub_busy"); busy {
		return NextStep{Label: "the hub is busy with another change; check again in a moment", Command: report.hubCommand("skillhub status")}
	}
	for _, section := range sections {
		for _, finding := range section.Findings {
			if finding.Severity == SeverityProblem && finding.Fix != nil {
				return NextStep{Label: "fix what doctor can repair", Command: report.hubCommand("skillhub doctor --fix")}
			}
		}
	}
	if _, finding, found := findFinding(sections, SeverityProblem, "hub_schema"); found {
		return NextStep{Label: "migrate the hub to the schema of this skillhub", Command: finding.Manual}
	}
	if _, _, found := findFinding(sections, SeverityProblem, "hub_content_invalid"); found {
		return NextStep{Label: "list each content problem", Command: report.hubCommand("skillhub doctor")}
	}
	if _, finding, found := findFinding(sections, SeverityProblem, "remote_reachable"); found && finding.Manual != "" {
		return NextStep{Label: "on the hub machine, start the hub server (this machine cannot start it)", Command: finding.Manual, Elsewhere: true}
	}
	if _, finding, found := findFinding(sections, SeverityProblem, "remote_reconnect"); found {
		return NextStep{Label: "reconnect this project to the hub, if you trust it", Command: finding.Manual}
	}
	for _, section := range sections {
		for _, finding := range section.Findings {
			if finding.Severity == SeverityProblem {
				if finding.Manual != "" {
					return NextStep{Label: finding.Label + ": " + strings.TrimSuffix(firstSentence(finding.Message), "."), Command: finding.Manual, Elsewhere: !strings.HasPrefix(finding.Manual, "skillhub ")}
				}
				return NextStep{Label: "see what needs attention", Command: report.hubCommand("skillhub doctor")}
			}
		}
	}
	if _, finding, found := findFinding(sections, SeverityNotice, "hub_uncommitted"); found {
		return NextStep{Label: "commit the changes of the hub", Command: finding.Manual, Elsewhere: true}
	}
	_, hubSection := sectionOfKind(sections, SectionHub)
	if hubSection && inventory.Known && inventory.Skills == 0 {
		return NextStep{Label: "create your first skill (you will be asked a few questions)", Command: report.hubCommand("skillhub skill create")}
	}
	if hubSection {
		return NextStep{Label: "list the skills of the hub", Command: report.hubCommand("skillhub skill list")}
	}
	return NextStep{Label: "check the connection to the remote hub in full", Command: "skillhub doctor"}
}

func sectionOfKind(sections []Section, kind string) (Section, bool) {
	for _, section := range sections {
		if section.Kind == kind {
			return section, true
		}
	}
	return Section{}, false
}

func firstSentence(text string) string {
	if index := strings.Index(text, ". "); index >= 0 {
		return text[:index+1]
	}
	return text
}

// healthHTTPClientKey carries the HTTP client the remote checks use.
type healthHTTPClientKey struct{}

// WithProbeClient makes the remote checks of Run use client for their probes,
// for example a test client that dials a local test server. Without it they
// use a client that follows no redirect and ignores proxy settings.
func WithProbeClient(ctx context.Context, client *http.Client) context.Context {
	return context.WithValue(ctx, healthHTTPClientKey{}, client)
}

func probeClientFrom(ctx context.Context) *http.Client {
	client, _ := ctx.Value(healthHTTPClientKey{}).(*http.Client)
	return client
}
