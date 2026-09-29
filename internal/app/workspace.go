package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/vantt/mcp-skill-hub/internal/canonical"
	catalogpkg "github.com/vantt/mcp-skill-hub/internal/catalog"
	"github.com/vantt/mcp-skill-hub/internal/mutation"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
)

// WorkspaceService is the shared application service used by init and doctor --fix.
type WorkspaceService struct{}

// Init previews the same mechanical remediation used by DoctorFix until explicitly confirmed.
func (WorkspaceService) Init(path string, yes bool) (Result, error) {
	if !yes {
		result, err := WorkspaceService{}.Doctor(path)
		if err != nil {
			return Result{}, err
		}
		result.Status = StatusActionRequired
		result.Summary = "Preview only: workspace initialization was not applied without --yes."
		result.SuggestedActions = []Action{{
			Label:                "Initialize workspace",
			Command:              "skillhub init " + path + " --yes",
			RequiresConfirmation: true,
		}}
		return result, nil
	}
	return WorkspaceService{}.apply(path)
}

// Doctor reports a read-only remediation plan and canonical validation findings.
func (WorkspaceService) Doctor(path string) (Result, error) {
	root, err := workspace.Discover(path)
	if err != nil {
		return Result{}, err
	}
	if _, statErr := os.Lstat(root); errors.Is(statErr, os.ErrNotExist) {
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
		result.Items = append(result.Items, Item{ID: "catalog_" + string(catalogStatus.State), Summary: "Catalog is " + string(catalogStatus.State) + ": " + catalogStatus.Detail, Impact: "Run an offline full rebuild before serving current canonical state."})
		result.SuggestedActions = append(result.SuggestedActions, Action{Label: "Rebuild catalog", Command: "skillhub rebuild --workspace " + root, RequiresConfirmation: false})
	}
	return result, nil
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
	for _, finding := range plan.Findings {
		result.Items = append(result.Items, Item{ID: finding.ID, Summary: finding.Path + ": " + finding.Summary, Impact: "Mechanical workspace remediation is available."})
	}
	for _, issue := range issues {
		if !containsFinding(plan, issue) {
			result.Items = append(result.Items, Item{ID: "validation_issue", Summary: issue.Path + ": " + issue.Message, Impact: "Canonical data must be corrected before it can be used."})
		}
	}
	if len(result.Items) > 0 {
		result.Status = StatusActionRequired
		result.Summary = "Workspace needs attention; doctor is read-only."
		result.SuggestedActions = append(result.SuggestedActions, Action{Label: "Apply workspace remediation", Command: "skillhub doctor --fix --workspace " + root + " --yes", RequiresConfirmation: true})
	}
	return result, nil
}

// DoctorFix applies mechanical remediation only after explicit confirmation.
func (WorkspaceService) DoctorFix(path string, yes bool) (Result, error) {
	if !yes {
		result, err := WorkspaceService{}.Doctor(path)
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
	if _, statErr := os.Lstat(root); statErr == nil {
		if err := mutation.RollForwardWithOptions(root, mutation.Options{PostCanonical: func(expected string) (mutation.Publication, error) {
			built, err := catalogpkg.BuildCatalogGenerationWhileLocked(context.Background(), root, expected, catalogpkg.BuildOptions{})
			if err != nil {
				return mutation.Publication{}, err
			}
			return mutation.Publication{CatalogSnapshot: built.Pointer.CatalogSnapshot, Generation: built.Pointer.Generation}, nil
		}}); err != nil {
			return Result{}, err
		}
		plan, err := workspace.Inspect(root)
		if err != nil {
			return Result{}, err
		}
		issues, err := canonical.Validate(root)
		if err != nil {
			return Result{}, err
		}
		if len(plan.Findings) == 0 && len(issues) == 0 {
			// Runtime repair is derived-state maintenance, not a canonical mutation,
			// and therefore must never create an operation receipt.
			return (CatalogService{}).EnsureCatalog(context.Background(), root)
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return Result{}, statErr
	}
	return WorkspaceService{}.apply(path)
}

func (WorkspaceService) apply(path string) (Result, error) {
	root, err := workspace.Discover(path)
	if err != nil {
		return Result{}, err
	}
	if _, statErr := os.Lstat(root); errors.Is(statErr, os.ErrNotExist) {
		// Bootstrap creates the lock and canonical skeleton itself. Every repair
		// after this point uses the mutation service.
		if _, err := workspace.Apply(root); err != nil {
			return Result{}, err
		}
		return remediationResult(root, workspace.Plan{Root: root, Findings: []workspace.Finding{{ID: "workspace_missing", Path: ".", Summary: "Workspace directory does not exist.", Fixable: true}}})
	} else if statErr != nil {
		return Result{}, statErr
	}

	lock, err := mutation.AcquireExclusiveLock(context.Background(), root, mutation.DefaultLockTimeout)
	if err != nil {
		return Result{}, err
	}
	before, err := workspace.Inspect(root)
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
	return remediationResult(root, before)
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
	result.Warnings = append(result.Warnings, Warning{Code: "host_bootstrap_out_of_scope", Summary: "Only workspace remediation is currently available; host bootstrap and MCP registration are out of scope."})
	return result, nil
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
	return &Error{Code: ErrorWorkspaceInvalid, Render: ErrorRender{Error: "The workspace is invalid.", Why: reason, Fix: "Run `skillhub doctor --workspace <path>` to inspect safe remediation, then use `--fix --yes` only after review."}}
}
