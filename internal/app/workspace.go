package app

import (
	"bytes"
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
type WorkspaceService struct{}

// Init previews the same mechanical remediation used by DoctorFix until explicitly confirmed.
func (service WorkspaceService) Init(path string, yes bool, force ...bool) (Result, error) {
	isForce := len(force) > 0 && force[0]
	if !isForce {
		if root, err := workspace.Discover(path); err == nil {
			if info, err := os.Stat(root); err == nil && info.IsDir() {
				entries, readErr := os.ReadDir(root)
				if readErr == nil && len(entries) > 0 {
					skillhubPath := filepath.Join(root, ".skillhub")
					if _, err := os.Lstat(skillhubPath); os.IsNotExist(err) {
						return ErrorResult(NewInvalidRequestError(
							fmt.Sprintf("Directory is not empty and is not a Skill Hub workspace: %s", root),
							"Choose a dedicated directory like `~/skillhub`, or add `--force` to initialize here anyway. If you meant to connect this project to a workspace, run `skillhub connect --yes` instead.",
						)), nil
					}
				}
			}
		}
	}
	if !yes {
		result, err := service.doctorInternal(path, true)
		if err != nil {
			return Result{}, err
		}
		result.Status = StatusActionRequired
		result.Summary = "Preview only: workspace initialization was not applied without --yes."
		result.Items = append([]Item{
			{ID: "workspace_structure", Summary: "Directory structure (skills, sources, distill, history, config, registry, evals)", Impact: "Will be created upon confirmation."},
			{ID: "git_repository", Summary: "Git repository (.git)", Impact: "Will be initialized upon confirmation."},
			{ID: "catalog_generation", Summary: "Search index database and schema", Impact: "Will be built upon confirmation."},
		}, result.Items...)
		result.SuggestedActions = []Action{{
			Label:                "Initialize workspace",
			Command:              "skillhub init " + path + " --yes",
			RequiresConfirmation: true,
		}}
		return result, nil
	}
	return service.apply(path, true)
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
			return Result{}, fmt.Errorf("No Skill Hub workspace at %s.", root)
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
	plan, err := workspace.Inspect(root)
	if err != nil {
		return Result{}, err
	}
	issues, err := canonical.Validate(root)
	if err != nil {
		return Result{}, err
	}
	recoveries, err := mutation.InspectRecoveryWhileLocked(root)
	if err != nil {
		return Result{}, err
	}
	result, err := doctorResult(root, plan, issues, recoveries)
	if err != nil || len(recoveries) != 0 || len(plan.Findings) != 0 || len(issues) != 0 {
		return result, err
	}
	catalogStatus, err := catalogpkg.InspectWhileLocked(context.Background(), root)
	if err != nil {
		return Result{}, err
	}
	if catalogStatus.State != catalogpkg.StateHealthy {
		result.Status = StatusActionRequired
		result.Summary = "Workspace canonical files are valid, but the derived catalog needs rebuilding."
		result.Items = append(result.Items, Item{ID: "catalog_" + string(catalogStatus.State), Summary: "Catalog is " + string(catalogStatus.State) + ": " + catalogStatus.Detail, Impact: "Run an offline full rebuild before applying host integration changes."})
		result.SuggestedActions = append(result.SuggestedActions, Action{Label: "Rebuild catalog", Command: "skillhub rebuild --workspace " + root, RequiresConfirmation: false})
		return result, nil
	}
	inspected, err := inspectHostIntegration(context.Background(), root, result)
	if err != nil {
		return Result{}, err
	}
	return checkProjectAndGlobalConnections(context.Background(), root, inspected), nil
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
			result.SuggestedActions = []Action{{Label: "Recover interrupted mutations", Command: "skillhub doctor --fix --workspace " + root + " --yes", RequiresConfirmation: true}}
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
		if !finding.Fixable {
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
			result.SuggestedActions = append(result.SuggestedActions, Action{Label: "Preview canonical migration", Command: "skillhub migrate --workspace " + root, RequiresConfirmation: false})
		} else if len(issues) > 0 {
			result.SuggestedActions = append(result.SuggestedActions, Action{Label: "Correct content errors", Command: "skillhub validate --workspace " + root, RequiresConfirmation: false})
		} else {
			result.SuggestedActions = append(result.SuggestedActions, Action{Label: "Apply workspace remediation", Command: "skillhub doctor --fix --workspace " + root + " --yes", RequiresConfirmation: true})
		}
	}
	return result, nil
}

// DoctorFix applies mechanical remediation only after explicit confirmation.
func (service WorkspaceService) DoctorFix(path string, yes bool) (Result, error) {
	if !yes {
		result, err := service.doctorInternal(path, false)
		if err != nil {
			return Result{}, err
		}
		result.Status = StatusActionRequired
		result.Summary = "Preview only: workspace remediation was not applied without --yes."
		return result, nil
	}
	root, err := workspace.Discover(path)
	if err != nil {
		return Result{}, err
	}
	if _, statErr := os.Lstat(root); errors.Is(statErr, os.ErrNotExist) {
		return Result{}, fmt.Errorf("No Skill Hub workspace at %s.", root)
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
	if _, statErr := os.Lstat(root); errors.Is(statErr, os.ErrNotExist) {
		if !isInit {
			return Result{}, fmt.Errorf("No Skill Hub workspace at %s.", root)
		}
		// Bootstrap creates the lock and canonical skeleton itself. Every repair
		// after this point uses the mutation service.
		if _, err := workspace.Apply(root); err != nil {
			return Result{}, err
		}
		workspaceResult, err = remediationResult(root, workspace.Plan{Root: root, Findings: []workspace.Finding{{ID: "workspace_missing", Path: ".", Summary: "Workspace directory created.", Fixable: true}}})
		if err != nil {
			return Result{}, err
		}
		return applyHostIntegration(context.Background(), root, workspaceResult)
	} else if statErr != nil {
		return Result{}, statErr
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
		if !finding.Fixable {
			return doctorResult(root, before, issues, nil)
		}
	}
	if len(before.Findings) == 0 && len(issues) == 0 {
		// Runtime and host repair are derived/external maintenance, not canonical
		// mutations, and therefore must never create an operation receipt.
		workspaceResult, err = (CatalogService{}).EnsureCatalog(context.Background(), root)
		if err != nil {
			return Result{}, err
		}
		return applyHostIntegration(context.Background(), root, workspaceResult)
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
			if !finding.Fixable {
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
	return applyHostIntegration(context.Background(), root, workspaceResult)
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

func inspectHostIntegration(ctx context.Context, root string, result Result) (Result, error) {
	request, err := hostIntegrationRequest(root)
	if err != nil {
		return Result{}, err
	}
	inspection, err := hostintegration.Inspect(ctx, request)
	if err != nil {
		return Result{}, err
	}
	levels := make(map[hostintegration.Host]hostintegration.IntegrationLevel, len(inspection.Hosts))
	hasConflict := false
	for _, host := range inspection.Hosts {
		levels[host.Host] = host.Level
		for _, file := range host.Files {
			if file.Conflict != "" {
				hasConflict = true
			}
		}
	}
	if hasConflict {
		appendHostInspectionItems(&result, inspection)
		result.Status = StatusActionRequired
		result.Summary = "Host integration has conflicts; doctor made no changes."
		return result, nil
	}
	plan, err := hostintegration.Plan(ctx, request)
	if err != nil {
		return Result{}, err
	}
	for _, change := range plan.Changes {
		item := hostChangeItem(change.Host, levels[change.Host], change.Kind, change.Path, "Remediation is available after explicit confirmation.")
		if change.Preview != "" {
			item.Summary += "\nManaged diff preview:\n" + change.Preview
		}
		result.Items = append(result.Items, item)
	}
	if len(plan.Changes) > 0 {
		result.Status = StatusActionRequired
		result.Summary = "Workspace and catalog are healthy; host integration needs attention."
		result.SuggestedActions = append(result.SuggestedActions, Action{Label: "Apply host integration remediation", Command: "skillhub doctor --fix --workspace " + root + " --yes", RequiresConfirmation: true})
	}
	return result, nil
}

func applyHostIntegration(ctx context.Context, root string, result Result) (Result, error) {
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

func checkProjectAndGlobalConnections(ctx context.Context, root string, result Result) Result {
	binary, err := currentBinary()
	if err != nil {
		return result
	}
	if cwd, err := os.Getwd(); err == nil && cwd != root {
		hasProjectConnection := false
		for _, rel := range []string{".mcp.json", ".codex/config.toml", ".gemini/settings.json"} {
			if data, err := os.ReadFile(filepath.Join(cwd, rel)); err == nil && bytes.Contains(data, []byte("skillhub")) {
				hasProjectConnection = true
				break
			}
		}
		if hasProjectConnection {
			plan, planErr := hostintegration.Plan(ctx, hostintegration.Request{
				Workspace: root,
				Root:      cwd,
				Scope:     hostintegration.ScopeProject,
				Binary:    binary,
			})
			if planErr != nil || len(plan.Changes) > 0 {
				result.Status = StatusActionRequired
				result.Items = append(result.Items, Item{
					ID:      "project_connection_outdated",
					Summary: fmt.Sprintf("Current project connection at %s is outdated or needs repair.", cwd),
					Impact:  fmt.Sprintf("Run `skillhub connect --project %s --yes` to update.", cwd),
				})
				result.SuggestedActions = append(result.SuggestedActions, Action{
					Label:                "Update project connection",
					Command:              fmt.Sprintf("skillhub connect --project %s --yes", cwd),
					RequiresConfirmation: true,
				})
			}
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		hasGlobalConnection := false
		for _, rel := range []string{".claude.json", ".codex/config.toml", ".gemini/settings.json"} {
			if data, err := os.ReadFile(filepath.Join(home, rel)); err == nil && bytes.Contains(data, []byte("skillhub")) {
				hasGlobalConnection = true
				break
			}
		}
		if hasGlobalConnection {
			plan, planErr := hostintegration.Plan(ctx, hostintegration.Request{
				Workspace: root,
				Root:      home,
				Scope:     hostintegration.ScopeUser,
				Binary:    binary,
			})
			if planErr != nil || len(plan.Changes) > 0 {
				result.Status = StatusActionRequired
				result.Items = append(result.Items, Item{
					ID:      "global_connection_outdated",
					Summary: "Global agent connection is outdated or needs repair.",
					Impact:  "Run `skillhub connect -g --yes` to update.",
				})
				result.SuggestedActions = append(result.SuggestedActions, Action{
					Label:                "Update global connection",
					Command:              "skillhub connect -g --yes",
					RequiresConfirmation: true,
				})
			}
		}
	}
	return result
}

// NewWorkspaceInvalidError formats workspace failures consistently for every delivery adapter.
func NewWorkspaceInvalidError(reason string) *Error {
	return &Error{Code: ErrorWorkspaceInvalid, Render: ErrorRender{Error: "The workspace is invalid.", Why: reason, Fix: "Run `skillhub doctor --workspace <path>` to inspect safe remediation, then use `--fix --yes` only after review."}}
}
