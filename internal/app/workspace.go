package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/vantt/mcp-skill-hub/internal/canonical"
	catalogpkg "github.com/vantt/mcp-skill-hub/internal/catalog"
	"github.com/vantt/mcp-skill-hub/internal/hostintegration"
	"github.com/vantt/mcp-skill-hub/internal/mutation"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
)

// WorkspaceService is the shared application service used by init and doctor --fix.
type WorkspaceService struct {
	// hosts limits the repair of the hub's own agent connection to these
	// hosts; nil repairs every host. The hub checks set it when a host entry
	// in the hub folder points at another hub on purpose.
	hosts []hostintegration.Host
}

// Doctor reports a read-only remediation plan and canonical validation findings.
func (service WorkspaceService) Doctor(path string) (Result, error) {
	return service.doctorInternal(path, false)
}

func (service WorkspaceService) doctorInternal(path string, isInit bool) (Result, error) {
	root, err := workspace.Discover(path)
	if err != nil {
		return Result{}, err
	}
	if _, statErr := os.Lstat(root); errors.Is(statErr, os.ErrNotExist) {
		if !isInit {
			return Result{}, fmt.Errorf("no Skill Hub workspace at %s", root)
		}
		// A missing workspace has no lock file yet. Keeping this preview purely
		// read-only is the documented new-workspace bootstrap exception.
		plan, inspectErr := workspace.Inspect(root)
		if inspectErr != nil {
			return Result{}, inspectErr
		}
		return doctorResult(root, plan, nil, nil)
	} else if statErr != nil {
		return Result{}, statErr
	}
	if isInit {
		if _, markerErr := os.Lstat(filepath.Join(root, ".skillhub")); errors.Is(markerErr, os.ErrNotExist) {
			// An uninitialized target has no workspace lock to coordinate yet.
			// Inspect it without creating runtime/locks so preview remains read-only.
			plan, inspectErr := workspace.Inspect(root)
			if inspectErr != nil {
				return Result{}, inspectErr
			}
			return doctorResult(root, plan, nil, nil)
		} else if markerErr != nil {
			return Result{}, markerErr
		}
	}
	lock, err := mutation.AcquireSharedLock(context.Background(), root, mutation.DefaultLockTimeout)
	if err != nil {
		return Result{}, err
	}
	defer lock.Unlock()
	state, err := inspectHubWhileLocked(context.Background(), root, nil)
	if err != nil {
		return Result{}, err
	}
	result, err := doctorResult(root, state.plan, state.issues, state.recoveries)
	if err != nil || len(state.recoveries) != 0 || len(state.plan.Findings) != 0 || len(state.issues) != 0 {
		return result, err
	}
	if state.catalog != nil && state.catalog.State != catalogpkg.StateHealthy {
		result.Status = StatusActionRequired
		result.Summary = "Workspace canonical files are valid, but the derived catalog needs rebuilding."
		result.Items = append(result.Items, Item{ID: "catalog_" + string(state.catalog.State), Summary: "Catalog is " + string(state.catalog.State) + ": " + state.catalog.Detail, Impact: "Run an offline full rebuild before applying host integration changes."})
		result.SuggestedActions = append(result.SuggestedActions, Action{Label: "Rebuild catalog", Command: "skillhub maintain rebuild --workspace " + root, RequiresConfirmation: false})
		return result, nil
	}
	return hostIntegrationResult(root, result, state.host), nil
}

func doctorResult(root string, plan workspace.Plan, issues []canonical.Issue, recoveries []mutation.Recovery) (Result, error) {
	result := NewResult(StatusReady, "Workspace is healthy.")
	if len(recoveries) > 0 {
		result.Status = StatusRecoveryRequired
		result.Summary = "Workspace recovery is required before normal work can continue."
		result.Error = NewRecoveryRequiredError(fmt.Sprintf("%d interrupted mutation(s) need safe recovery.", len(recoveries)))
		fixable := true
		for _, recovery := range recoveries {
			impact := "Doctor will roll this approved mutation forward."
			switch recovery.Action {
			case mutation.RecoveryAbort:
				impact = "Doctor will remove this incomplete journal; canonical files were not changed."
			case mutation.RecoveryConflict:
				fixable = false
				impact = "Automatic recovery is blocked: " + recovery.Detail
			}
			result.Items = append(result.Items, Item{ID: recovery.OperationID, Summary: "Interrupted mutation " + recovery.OperationID + " (" + string(recovery.Action) + ")", Impact: impact})
		}
		if fixable {
			result.SuggestedActions = []Action{{Label: "Recover interrupted mutations", Command: "skillhub doctor --fix --workspace " + root, RequiresConfirmation: true}}
		}
		return result, nil
	}
	hasCanonicalMigration := false
	for _, finding := range plan.Findings {
		if finding.ID == "workspace_missing" {
			// The preview describes the planned action; the same finding is
			// reported as completed after apply.
			finding.Summary = "Workspace directory will be created."
		}
		impact := "Mechanical workspace remediation is available."
		if finding.ID == "machine_file_tracked" {
			impact = "Stop tracking it with `git rm --cached " + finding.Path + "`; the file stays on disk."
		} else if !finding.Fixable {
			impact = "Doctor will not change canonical schema files; review an explicit migration preview."
			if finding.ID == "canonical_schema_incompatible" {
				hasCanonicalMigration = true
			}
		}
		result.Items = append(result.Items, Item{ID: finding.ID, Summary: finding.Path + ": " + finding.Summary, Impact: impact})
	}
	for _, issue := range issues {
		if !containsFinding(plan, issue) {
			result.Items = append(result.Items, Item{ID: "validation_issue", Summary: issue.Path + ": " + issue.Message, Impact: "Canonical data must be corrected before it can be used."})
		}
	}
	if len(result.Items) > 0 {
		result.Status = StatusActionRequired
		result.Summary = "Workspace needs attention; doctor is read-only."
		if hasCanonicalMigration {
			result.SuggestedActions = append(result.SuggestedActions, Action{Label: "Preview canonical migration", Command: "skillhub maintain migrate --workspace " + root, RequiresConfirmation: false})
		} else if len(issues) > 0 {
			result.SuggestedActions = append(result.SuggestedActions, Action{Label: "Correct content errors", Command: "skillhub maintain validate --workspace " + root, RequiresConfirmation: false})
		} else {
			result.SuggestedActions = append(result.SuggestedActions, Action{Label: "Apply workspace remediation", Command: "skillhub doctor --fix --workspace " + root, RequiresConfirmation: true})
		}
	}
	return result, nil
}

// DoctorFix applies the mechanical hub repair: it finishes interrupted
// changes, adds missing layout and .gitignore entries, rebuilds the search
// index and refreshes the hub's own agent connection. A tracked connection
// file does not stop it; the hub checks report that file with its
// `git rm --cached` command. With dryRun it only reports the repairs it would
// make. The connections of projects and of the user account are repaired by
// the client checks of the health engine, not here.
func (service WorkspaceService) DoctorFix(path string, dryRun bool) (Result, error) {
	if dryRun {
		result, err := service.doctorInternal(path, false)
		if err != nil {
			return Result{}, err
		}
		result.Status = StatusActionRequired
		result.Summary = "Dry run: workspace remediation was not applied."
		return result, nil
	}
	root, err := workspace.Discover(path)
	if err != nil {
		return Result{}, err
	}
	if _, statErr := os.Lstat(root); errors.Is(statErr, os.ErrNotExist) {
		return Result{}, fmt.Errorf("no Skill Hub workspace at %s", root)
	} else if statErr != nil {
		return Result{}, statErr
	}
	if err := mutation.RollForwardWithOptions(root, mutation.Options{PostCanonical: func(expected string) (mutation.Publication, error) {
		built, err := catalogpkg.BuildCatalogGenerationWhileLocked(context.Background(), root, expected, catalogpkg.BuildOptions{})
		if err != nil {
			return mutation.Publication{}, err
		}
		return mutation.Publication{CatalogSnapshot: built.Pointer.CatalogSnapshot, Generation: built.Pointer.Generation}, nil
	}}); err != nil {
		return Result{}, err
	}
	return service.apply(path, false)
}

func (service WorkspaceService) apply(path string, isInit bool) (Result, error) {
	root, err := workspace.Discover(path)
	if err != nil {
		return Result{}, err
	}
	var workspaceResult Result
	_, statErr := os.Lstat(root)
	if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return Result{}, statErr
	}
	missing := errors.Is(statErr, os.ErrNotExist)
	if missing && !isInit {
		return Result{}, fmt.Errorf("no Skill Hub workspace at %s", root)
	}
	if missing || (isInit && !IsHubRoot(root)) {
		// A folder without the hub marker is created from scratch: bootstrap
		// creates the lock and canonical skeleton itself. Every repair after
		// this point uses the mutation service.
		if _, err := workspace.Apply(root); err != nil {
			return Result{}, err
		}
		workspaceResult, err = remediationResult(root, workspace.Plan{Root: root, Findings: []workspace.Finding{{ID: "workspace_missing", Path: ".", Summary: "Hub folders created.", Fixable: true}}})
		if err != nil {
			return Result{}, err
		}
		return applyHostIntegration(context.Background(), root, workspaceResult, service.hosts)
	}

	before, err := workspace.Inspect(root)
	if err != nil {
		return Result{}, err
	}
	issues, err := canonical.Validate(root)
	if err != nil {
		return Result{}, err
	}
	for _, finding := range before.Findings {
		if blocksRemediation(finding) {
			return doctorResult(root, before, issues, nil)
		}
	}
	// A tracked connection file is reported (init and the hub checks print
	// its `git rm --cached` command); only the fixable findings are remediated.
	before = fixableFindings(before)
	if len(before.Findings) == 0 && len(issues) == 0 {
		// Runtime and host repair are derived/external maintenance, not canonical
		// mutations, and therefore must never create an operation receipt.
		workspaceResult, err = (CatalogService{}).EnsureCatalog(context.Background(), root)
		if err != nil {
			return Result{}, err
		}
		return applyHostIntegration(context.Background(), root, workspaceResult, service.hosts)
	}

	lock, err := mutation.AcquireExclusiveLock(context.Background(), root, mutation.DefaultLockTimeout)
	if err != nil {
		return Result{}, err
	}
	before, err = workspace.Inspect(root)
	if err == nil {
		// The state may have changed between the unlocked inspection and the
		// lock. Findings that need migration must never be remediated here.
		for _, finding := range before.Findings {
			if blocksRemediation(finding) {
				unlockErr := lock.Unlock()
				if unlockErr != nil {
					return Result{}, unlockErr
				}
				lockedIssues, validateErr := canonical.Validate(root)
				if validateErr != nil {
					return Result{}, validateErr
				}
				return doctorResult(root, before, lockedIssues, nil)
			}
		}
	}
	if err == nil {
		before = fixableFindings(before)
	}
	if err == nil {
		var recoveries []mutation.Recovery
		recoveries, err = mutation.InspectRecoveryWhileLocked(root)
		if err == nil && len(recoveries) != 0 {
			err = mutation.ErrRecoveryRequired
		}
	}
	if err == nil {
		_, err = workspace.PrepareLayout(root)
	}
	unlockErr := lock.Unlock()
	if err != nil {
		return Result{}, err
	}
	if unlockErr != nil {
		return Result{}, unlockErr
	}
	files, err := workspace.RemediationFiles(root)
	if err != nil {
		return Result{}, err
	}
	changes := make([]mutation.Change, 0, len(files))
	for _, file := range files {
		changes = append(changes, mutation.Change{Path: file.Path, Contents: file.Contents})
	}
	proposal, err := mutation.PlanMutation(root, mutation.WriteSet{Command: "workspace_remediation", Changes: changes})
	if err != nil {
		return Result{}, err
	}
	if _, err := mutation.ConfirmMutationWithOptions(root, proposal, mutation.Confirmation{ProposalID: proposal.ID, ProposalDigest: proposal.Digest, BaseCatalogSnapshot: proposal.BaseCatalogSnapshot}, mutation.Options{PostCanonical: func(expected string) (mutation.Publication, error) {
		built, err := catalogpkg.BuildCatalogGenerationWhileLocked(context.Background(), root, expected, catalogpkg.BuildOptions{})
		if err != nil {
			return mutation.Publication{}, err
		}
		return mutation.Publication{CatalogSnapshot: built.Pointer.CatalogSnapshot, Generation: built.Pointer.Generation}, nil
	}}); err != nil {
		return Result{}, err
	}
	workspaceResult, err = remediationResult(root, before)
	if err != nil {
		return Result{}, err
	}
	return applyHostIntegration(context.Background(), root, workspaceResult, service.hosts)
}

// blocksRemediation reports whether a finding stops mechanical remediation.
// A tracked connection file does not: it is reported with its
// `git rm --cached` command, and every other repair still runs.
func blocksRemediation(finding workspace.Finding) bool {
	return !finding.Fixable && finding.ID != "machine_file_tracked"
}

func fixableFindings(plan workspace.Plan) workspace.Plan {
	kept := workspace.Plan{Root: plan.Root}
	for _, finding := range plan.Findings {
		if finding.Fixable {
			kept.Findings = append(kept.Findings, finding)
		}
	}
	return kept
}

func remediationResult(root string, before workspace.Plan) (Result, error) {
	issues, err := canonical.Validate(root)
	if err != nil {
		return Result{}, err
	}
	if len(issues) > 0 {
		return Result{}, fmt.Errorf("workspace remediation completed but validation failed: %s", renderIssues(issues))
	}
	built, err := (CatalogService{}).EnsureCatalog(context.Background(), root)
	if err != nil {
		return Result{}, err
	}
	result := NewResult(StatusApplied, "Workspace remediation and catalog rebuild are complete.")
	for _, finding := range before.Findings {
		result.Items = append(result.Items, Item{ID: finding.ID, Summary: finding.Path + ": " + finding.Summary, Impact: "Remediated."})
	}
	result.Items = append(result.Items, built.Items...)
	return result, nil
}

// hostIntegrationResult adds the state of the hub's own agent connection to
// doctor's report.
func hostIntegrationResult(root string, result Result, host *hubHostState) Result {
	if host == nil {
		return result
	}
	levels := make(map[hostintegration.Host]hostintegration.IntegrationLevel, len(host.inspection.Hosts))
	for _, inspected := range host.inspection.Hosts {
		levels[inspected.Host] = inspected.Level
	}
	if len(host.conflicts) > 0 {
		appendHostInspectionItems(&result, host.inspection)
		result.Status = StatusActionRequired
		result.Summary = "Host integration has conflicts; doctor made no changes."
		return result
	}
	missing := hostintegration.MissingFinding(host.plan)
	impact := "Remediation is available after explicit confirmation."
	if missing != nil {
		impact = "Fixable: " + missing.Note + "."
	}
	for _, change := range host.plan.Changes {
		item := hostChangeItem(change.Host, levels[change.Host], change.Kind, change.Path, impact)
		if change.Preview != "" {
			item.Summary += "\nManaged diff preview:\n" + change.Preview
		}
		result.Items = append(result.Items, item)
	}
	if len(host.plan.Changes) > 0 {
		result.Status = StatusActionRequired
		if missing != nil {
			result.Summary = "Workspace and catalog are healthy; host integration missing. Note: " + missing.Note + "."
		} else {
			result.Summary = "Workspace and catalog are healthy; host integration needs attention."
		}
		result.SuggestedActions = append(result.SuggestedActions, Action{Label: "Apply host integration remediation", Command: "skillhub doctor --fix --workspace " + root, RequiresConfirmation: true})
	}
	return result
}

// applyHostIntegration writes the hub's own agent connection for hosts (nil:
// every host; empty: none).
func applyHostIntegration(ctx context.Context, root string, result Result, hosts []hostintegration.Host) (Result, error) {
	if hosts != nil && len(hosts) == 0 {
		return result, nil
	}
	status, err := catalogpkg.Inspect(ctx, root)
	if err != nil {
		return Result{}, err
	}
	if status.State != catalogpkg.StateHealthy {
		return Result{}, fmt.Errorf("host integration requires a healthy catalog; catalog is %s: %s", status.State, status.Detail)
	}
	request, err := hostIntegrationRequest(root)
	if err != nil {
		return Result{}, err
	}
	request.Hosts = hosts
	inspection, err := hostintegration.Inspect(ctx, request)
	if err != nil {
		return Result{}, err
	}
	levels := make(map[hostintegration.Host]hostintegration.IntegrationLevel, len(inspection.Hosts))
	for _, host := range inspection.Hosts {
		levels[host.Host] = host.Level
	}
	plan, err := hostintegration.Plan(ctx, request)
	if err != nil {
		return Result{}, err
	}
	applied, err := hostintegration.Apply(ctx, plan, hostintegration.ApplyOptions{Confirmed: true})
	if err != nil {
		return Result{}, err
	}
	for _, change := range applied.Changed {
		result.Items = append(result.Items, hostChangeItem(change.Host, levels[change.Host], change.Kind, change.Path, "Remediated."))
	}
	if len(applied.Changed) > 0 {
		result.Warnings = append(result.Warnings, hostBestEffortWarning())
		result.Status = StatusApplied
		result.Summary = "Workspace, catalog, and host integration remediation are complete."
	}
	return result, nil
}

func hostIntegrationRequest(root string) (hostintegration.Request, error) {
	binary, err := currentBinary()
	if err != nil {
		return hostintegration.Request{}, err
	}
	return hostintegration.Request{Workspace: root, Binary: binary}, nil
}

// currentBinary returns the absolute path of the running skillhub executable,
// which host registrations must reference.
func currentBinary() (string, error) {
	binary, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve current executable: %w", err)
	}
	if !filepath.IsAbs(binary) {
		binary, err = filepath.Abs(binary)
		if err != nil {
			return "", fmt.Errorf("make current executable path absolute: %w", err)
		}
	}
	return filepath.Clean(binary), nil
}

func appendHostInspectionItems(result *Result, inspection hostintegration.Inspection) {
	for _, kind := range []hostintegration.ChangeKind{hostintegration.ChangeMCP, hostintegration.ChangeHostPermissions, hostintegration.ChangeNativeSkill, hostintegration.ChangeBootstrap} {
		for _, host := range inspection.Hosts {
			for _, file := range host.Files {
				if file.Kind != kind || file.Current {
					continue
				}
				impact := "Remediation is available after explicit confirmation."
				if file.Conflict != "" {
					impact = "Automatic remediation is blocked: " + file.Conflict
				}
				result.Items = append(result.Items, hostChangeItem(host.Host, host.Level, file.Kind, file.Path, impact))
			}
		}
	}
}

func hostChangeItem(host hostintegration.Host, level hostintegration.IntegrationLevel, kind hostintegration.ChangeKind, path, impact string) Item {
	return Item{
		ID:      "host_" + string(host) + "_" + string(kind),
		Summary: fmt.Sprintf("%s (%s) %s: %s", host, level, kind, path),
		Impact:  impact,
	}
}

func hostBestEffortWarning() Warning {
	return Warning{Code: "host_activation_best_effort", Summary: "Host activation coordination is instruction-only and best effort; each Agent Host remains responsible for permissions and activation."}
}

func containsFinding(plan workspace.Plan, issue canonical.Issue) bool {
	for _, finding := range plan.Findings {
		if finding.Path == issue.Path && strings.Contains(issue.Message, strings.TrimSuffix(finding.Summary, ".")) {
			return true
		}
	}
	return false
}

func renderIssues(issues []canonical.Issue) string {
	values := make([]string, 0, len(issues))
	for _, issue := range issues {
		values = append(values, issue.Path+": "+issue.Message)
	}
	return strings.Join(values, "; ")
}

// NewWorkspaceInvalidError formats workspace failures consistently for every delivery adapter.
func NewWorkspaceInvalidError(reason string) *Error {
	return &Error{Code: ErrorWorkspaceInvalid, Render: ErrorRender{Error: "The workspace is invalid.", Why: reason, Fix: "Run `skillhub doctor` in the hub to see what needs repair, then `skillhub doctor --fix` to repair what it can."}}
}
