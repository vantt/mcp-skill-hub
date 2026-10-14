package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/vantt/skillhub-mcp/internal/catalog"
	"github.com/vantt/skillhub-mcp/internal/skill"
	"github.com/vantt/skillhub-mcp/internal/workspace"
)

// Baseline of a submission's files: the hub's copy of the skill today.
const reviewBaselineHub = "hub"

const submissionExcludedReason = "left out by the submitting machine"

// PreviewSubmission plans what approving a waiting submission would change, as
// a folder commit does: the files come from the hub's queue and the base is the
// version the submitting machine checked out. Nothing is written.
func (service SkillCommitService) PreviewSubmission(ctx context.Context, path, id string, input SkillCommitInput) (SkillCommitPreview, error) {
	if service.Clock == nil {
		service.Clock = SystemClock{}
	}
	root, err := workspace.Discover(path)
	if err != nil {
		return SkillCommitPreview{}, err
	}
	submission, err := ReadSubmission(root, id)
	if err != nil {
		return SkillCommitPreview{}, err
	}
	tree, err := readSubmissionTree(root, submission)
	if err != nil {
		return commitPreviewError(NewInvalidRequestError(err.Error(), fmt.Sprintf("Discard it with `skillhub skill review %s --discard` and ask for it to be sent again.", id))), nil
	}
	target := commitTarget{dir: SubmissionFilesDir(root, id), id: submission.SkillID}
	if submission.BaseVersion != "" {
		target.marker = &CheckoutMarker{SkillID: submission.SkillID, Hub: root, BaseVersion: submission.BaseVersion}
	}
	excluded := make([]ExcludedFile, 0, len(submission.Excluded))
	for _, file := range submission.Excluded {
		if file.Reason == "" {
			file.Reason = submissionExcludedReason
		}
		excluded = append(excluded, file)
	}
	return service.previewTree(ctx, root, target, folderContents{tree: tree, excluded: excluded}, input, &submission)
}

// ApplySubmission writes a previewed submission to the hub as one commit and
// drops it from the queue. The content is always approved in the same change:
// without options.Approve the apply is refused, so nothing from another machine
// is served unread.
func (service SkillCommitService) ApplySubmission(ctx context.Context, path string, preview SkillCommitPreview, options SkillCommitApplyOptions) (SkillCommitResult, error) {
	plan := preview.plan
	if plan == nil || plan.submission == nil {
		return commitResultError(NewInvalidRequestError("there is no previewed submission to apply", "Run `skillhub skill review SUB-…` again.")), nil
	}
	submission := *plan.submission
	result, err := service.Apply(ctx, path, preview, options)
	if err != nil || result.Error != nil || result.OperationID == "" {
		return result, err
	}
	if err := DiscardSubmission(plan.root, submission.ID); err != nil && !errors.Is(err, ErrSubmissionNotFound) {
		result.Warnings = append(result.Warnings, Warning{Code: "submission_not_dropped", Summary: fmt.Sprintf("%s was applied but could not be removed from the queue: %v", submission.ID, err)})
	}
	return result, nil
}

// SubmissionReviewContent is the full review of a waiting submission: the
// shape of a skill's review content, with where it came from.
type SubmissionReviewContent struct {
	SkillReviewContent
	SubmissionID string `json:"submission_id"`
	ClientLabel  string `json:"client_label"`
	SubmittedAt  string `json:"submitted_at"`
	// Case is "new", "update", "merge" or "replace".
	Case string `json:"case"`
	// Conflicts lists files the hub and the submitter both changed and that
	// need a decision on the hub machine before the submission can be applied.
	Conflicts []string       `json:"conflicts"`
	Excluded  []ExcludedFile `json:"excluded"`
}

// ReviewSubmissionContent reads a waiting submission for the full review
// screen. Its files are compared with the hub's copy of the skill, or with
// nothing for a skill the hub does not have.
func (service SkillService) ReviewSubmissionContent(ctx context.Context, path, id string) (SubmissionReviewContent, error) {
	preview, submission, err := service.previewSubmission(ctx, path, id)
	if err != nil {
		return SubmissionReviewContent{}, err
	}
	plan := preview.plan
	var baseline map[string][]byte
	kind := reviewBaselineNone
	state := "new"
	meta := parseSkillReviewMeta(plan.metaAfter)
	if plan.hub != nil {
		baseline, kind = treeBytes(plan.hub.tree), reviewBaselineHub
		state = parseSkillReviewMeta(plan.hub.meta).Status
	}
	skillMD := plan.final["SKILL.md"].Data
	meta.Name, _ = catalog.DeriveSkillNameAndDesc(skillMD, meta.Name, meta.Description, submission.SkillID)
	_, _, license := parseSkillMDFrontmatter(skillMD)
	review := SubmissionReviewContent{
		SkillReviewContent: SkillReviewContent{
			SkillID: submission.SkillID, State: state, Title: meta.Name, License: reviewLicense(license, plan.final),
			Origin: "Submitted from " + submission.ClientLabel, ThirdParty: true, ContentDigest: preview.Trust.ContentDigest,
			Baseline: kind, Files: reviewFiles(plan.final, baseline, plan.hub == nil),
		},
		SubmissionID: submission.ID, ClientLabel: submission.ClientLabel, SubmittedAt: submission.SubmittedAt,
		Case: preview.Case, Conflicts: append([]string{}, preview.Conflicts...), Excluded: append([]ExcludedFile{}, submission.Excluded...),
	}
	review.Result = NewResult(StatusOK, fmt.Sprintf("%s from %s: %s, %d changed.", submission.SkillID, submission.ClientLabel, pluralize(len(review.Files), "file"), changedCount(review.Files)))
	return review, ctx.Err()
}

// previewSubmission previews a submission and turns a refusal into an error.
func (service SkillService) previewSubmission(ctx context.Context, path, id string) (SkillCommitPreview, Submission, error) {
	preview, err := (SkillCommitService{}).PreviewSubmission(ctx, path, id, SkillCommitInput{})
	if err != nil {
		return SkillCommitPreview{}, Submission{}, err
	}
	if preview.Error != nil {
		return SkillCommitPreview{}, Submission{}, preview.Error
	}
	return preview, *preview.plan.submission, nil
}

// SubmissionApprovalPreview is the proposal that applies a submission and
// approves its content as one change, with the commit subject to confirm it with.
type SubmissionApprovalPreview struct {
	SkillProposal
	SubmissionID   string            `json:"submission_id"`
	BaseVersion    string            `json:"base_version"`
	Files          []SkillCommitFile `json:"files"`
	DefaultMessage string            `json:"default_message"`
}

// PreviewSubmissionApproval plans applying a submission together with the
// approval of its content, which the person has just read. The digest is the
// one the review screen showed; content that changed since is refused, and so
// is a submission that still needs a decision on the hub machine.
func (service SkillService) PreviewSubmissionApproval(ctx context.Context, path, id, contentDigest string) (SubmissionApprovalPreview, error) {
	preview, submission, err := service.previewSubmission(ctx, path, id)
	if err != nil {
		return SubmissionApprovalPreview{}, err
	}
	plan := preview.plan
	if preview.Trust.ContentDigest != contentDigest {
		return SubmissionApprovalPreview{}, NewStaleProposalError("The submission changed after the review screen was opened.", "Open the review again and read the new content.")
	}
	if len(preview.Conflicts) > 0 {
		return SubmissionApprovalPreview{}, NewInvalidRequestError(
			fmt.Sprintf("%s changed both on the hub and in the submission since it was checked out", pluralize(len(preview.Conflicts), "file")),
			fmt.Sprintf("Decide each in a terminal on the hub machine with `skillhub skill review %s --accept <path>=hub` (or `=project`).", id))
	}
	if preview.NothingToCommit {
		return SubmissionApprovalPreview{}, NewInvalidRequestError(fmt.Sprintf("%s already matches the hub", plan.id), "Discard the submission; there is nothing to apply.")
	}
	changes, _, err := approvedChanges(plan, true)
	if err != nil {
		return SubmissionApprovalPreview{}, err
	}
	proposal, err := service.Manager.PreviewChanges(ctx, plan.root, plan.id, changes, false)
	if err != nil {
		return SubmissionApprovalPreview{}, err
	}
	if err := skill.StoreProposal(plan.root, proposal, time.Now()); err != nil {
		return SubmissionApprovalPreview{}, err
	}
	if err := BindSubmissionProposal(plan.root, submission.ID, proposal.ID); err != nil {
		return SubmissionApprovalPreview{}, err
	}
	return SubmissionApprovalPreview{
		SkillProposal:  makeSkillProposal(proposal, "The submission is ready to apply and approve."),
		SubmissionID:   submission.ID,
		BaseVersion:    submission.BaseVersion,
		Files:          preview.Files,
		DefaultMessage: submissionCommitMessage(preview, submission.ClientLabel, true, false),
	}, nil
}

// SubmissionIDOf reports whether a command-line argument names a submission.
func SubmissionIDOf(argument string) (string, bool) {
	argument = strings.TrimSpace(argument)
	return argument, submissionIDPattern.MatchString(argument)
}
