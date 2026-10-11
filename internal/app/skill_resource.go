package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/skill"
)

// SkillResourceItem is one companion file of a skill as it is on disk now.
// Reason is empty when the file is editable, otherwise one of binary,
// too_large, missing or not_regular.
type SkillResourceItem struct {
	Path      string `json:"path"`
	Kind      string `json:"kind"`
	SizeBytes int64  `json:"size_bytes"`
	Digest    string `json:"digest"`
	Editable  bool   `json:"editable"`
	Reason    string `json:"reason"`
}

// SkillResourceList lists the companion files of a skill, without SKILL.md
// and hub metadata.
type SkillResourceList struct {
	SkillID string              `json:"skill_id"`
	Items   []SkillResourceItem `json:"items"`
}

// SkillResourceContent is one companion file with its text. Content is
// omitted for binary files and cut at skill.MaxEditableResourceBytes for
// larger text files, with Truncated set.
type SkillResourceContent struct {
	SkillID    string  `json:"skill_id"`
	Path       string  `json:"path"`
	SizeBytes  int64   `json:"size_bytes"`
	Digest     string  `json:"digest"`
	Editable   bool    `json:"editable"`
	Reason     string  `json:"reason"`
	LineEnding string  `json:"line_ending"`
	Truncated  bool    `json:"truncated"`
	Content    *string `json:"content,omitempty"`
}

// ListSkillResources classifies every companion file of a skill.
func (service SkillService) ListSkillResources(ctx context.Context, path, id string) (SkillResourceList, error) {
	root, err := skill.ResolveWorkspace(path)
	if err != nil {
		return SkillResourceList{}, err
	}
	files, err := skill.ListResourceFiles(ctx, root, id)
	if err != nil {
		return SkillResourceList{}, resourceError(err)
	}
	list := SkillResourceList{SkillID: id, Items: make([]SkillResourceItem, 0, len(files))}
	for _, file := range files {
		list.Items = append(list.Items, SkillResourceItem{
			Path: file.Path, Kind: file.Kind, SizeBytes: file.SizeBytes,
			Digest: file.Digest, Editable: file.Editable, Reason: file.Reason,
		})
	}
	return list, nil
}

// ReadSkillResource returns one companion file of a skill with its text when
// the file is text.
func (service SkillService) ReadSkillResource(ctx context.Context, path, id, resourcePath string) (SkillResourceContent, error) {
	root, err := skill.ResolveWorkspace(path)
	if err != nil {
		return SkillResourceContent{}, err
	}
	file, err := skill.ReadResourceFile(ctx, root, id, resourcePath)
	if err != nil {
		return SkillResourceContent{}, resourceError(err)
	}
	content := SkillResourceContent{
		SkillID: id, Path: file.Path, SizeBytes: file.SizeBytes, Digest: file.Digest,
		Editable: file.Editable, Reason: file.Reason, LineEnding: file.LineEnding, Truncated: file.Truncated,
	}
	if file.IsText {
		text := string(file.Content)
		content.Content = &text
	}
	return content, nil
}

// PreviewSkillResourceUpdate plans replacing the whole content of one
// companion file and stores the proposal for the generic skill confirm.
func (service SkillService) PreviewSkillResourceUpdate(ctx context.Context, path, id string, input skill.ResourceUpdateInput) (SkillProposal, error) {
	root, err := skill.ResolveWorkspace(path)
	if err != nil {
		return SkillProposal{}, err
	}
	proposal, err := service.Manager.PreviewResourceUpdate(ctx, root, id, input, true)
	if err != nil {
		var conflictErr *skill.EditConflictError
		if errors.As(err, &conflictErr) {
			why := fmt.Sprintf("%s changed on disk after it was opened: expected %s, found %s.", input.Path, conflictErr.ExpectedDigest, conflictErr.ActualDigest)
			return SkillProposal{}, NewEditConflictError(why, "Reload the file, then apply your edit again.")
		}
		return SkillProposal{}, resourceError(err)
	}
	if err := skill.StoreProposal(root, proposal, time.Now()); err != nil {
		return SkillProposal{}, err
	}
	return makeSkillProposal(proposal, "Resource edit is ready for review."), nil
}

// resourceError turns a refused resource request into an actionable invalid
// request and a catalog that cannot be opened into a reason the person can act
// on. skill.ErrNotFound passes through so callers can answer 404. The domain
// messages name the rule and the path, never file content.
func resourceError(err error) error {
	switch {
	case errors.Is(err, skill.ErrInvalidResourceEdit):
		why := strings.TrimPrefix(err.Error(), skill.ErrInvalidResourceEdit.Error()+": ")
		return NewInvalidRequestError(why, "Choose a text file listed in the skill's resources and send its full new content.")
	case errors.Is(err, skill.ErrSnapshotExpired):
		return &Error{Code: ErrorSnapshotExpired, Retryable: true, Render: ErrorRender{
			Error: "The skill catalog could not be opened.",
			Why:   "The catalog cannot be built from the current skill files; a symlink or another unsupported file in a skill folder blocks it.",
			Fix:   "Remove symlinks and unsupported files from the skill folders, run `skillhub maintain rebuild`, then reload.",
		}}
	}
	return err
}
