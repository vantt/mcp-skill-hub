package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/delivery/cli/termui"
	"github.com/vantt/mcp-skill-hub/internal/skill"
)

// editorSessionResult holds the outcome of an external editor session.
type editorSessionResult struct {
	updated        []byte
	expectedDigest string
	recoveryID     string
	recoveryPath   string
}

// openEditorSession seeds a private temporary file with canonical content,
// invokes $VISUAL or $EDITOR, captures modified bytes, and persists an immutable
// recovery artifact before any preview or mutation.
func openEditorSession(ctx context.Context, service app.SkillService, workspace, id string) (editorSessionResult, error) {
	editable, err := service.ReadEditableSkill(ctx, workspace, id)
	if err != nil {
		return editorSessionResult{}, err
	}

	temporary, err := os.CreateTemp("", "skillhub-edit-*.md")
	if err != nil {
		return editorSessionResult{}, err
	}
	name := temporary.Name()
	defer os.Remove(name)

	if err := temporary.Chmod(0o600); err == nil {
		_, err = temporary.Write(editable.Content)
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return editorSessionResult{}, err
	}

	editor := strings.TrimSpace(os.Getenv("VISUAL"))
	if editor == "" {
		editor = strings.TrimSpace(os.Getenv("EDITOR"))
	}
	fields := strings.Fields(editor)
	if len(fields) == 0 {
		return editorSessionResult{}, errors.New("$VISUAL or $EDITOR must be set for --editor")
	}

	command := exec.CommandContext(ctx, fields[0], append(fields[1:], name)...)
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stderr, os.Stderr
	if err := command.Run(); err != nil {
		return editorSessionResult{}, fmt.Errorf("external editor failed: %w", err)
	}

	temporaryRoot, err := os.OpenRoot(filepath.Dir(name))
	if err != nil {
		return editorSessionResult{}, err
	}
	defer temporaryRoot.Close()

	base := filepath.Base(name)
	info, err := temporaryRoot.Lstat(base)
	if err != nil {
		return editorSessionResult{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return editorSessionResult{}, errors.New("external editor replaced the temporary file with an unsafe object")
	}

	updated, err := temporaryRoot.ReadFile(base)
	if err != nil {
		return editorSessionResult{}, err
	}
	if bytes.Equal(editable.Content, updated) {
		return editorSessionResult{}, errors.New("external editor made no changes")
	}

	// Persist recovery artifact before preview (BUG-03).
	recoveryID, recErr := service.SaveEditorRecovery(ctx, workspace, id, updated)
	if recErr != nil {
		return editorSessionResult{}, fmt.Errorf("save editor recovery: %w", recErr)
	}
	recPath := filepath.Join("runtime/edits", recoveryID+".md")

	return editorSessionResult{
		updated:        updated,
		expectedDigest: editable.Digest,
		recoveryID:     recoveryID,
		recoveryPath:   recPath,
	}, nil
}

// writeEditorSkillPreview formats preview output for an interactive editor change.
// It renders the actual diff and the exact short confirm command without advising --yes (BUG-12).
func writeEditorSkillPreview(stdout io.Writer, preview app.SkillProposal) {
	p := termui.New(stdout)
	p.Line(preview.Summary)
	p.Bullets(fmt.Sprintf("%d added, %d modified, %d deleted file(s).",
		len(preview.Diff.Added), len(preview.Diff.Modified), len(preview.Diff.Deleted)))
	pins := preview.Confirmation.Confirmation.Pins
	p.Fields(
		termui.Field{Label: "Proposal", Value: pins.ProposalID},
		termui.Field{Label: "Digest", Value: pins.ProposalDigest},
		termui.Field{Label: "Base version", Value: pins.BaseVersion},
	)
	if preview.RoutingImpact != nil {
		p.Fields(termui.Field{Label: "Routing impact", Value: preview.RoutingImpact.Summary})
		for _, warning := range preview.RoutingImpact.Warnings {
			p.Warning(warning)
		}
	}
	if preview.FullDiff != "" {
		p.Raw(preview.FullDiff)
		if !strings.HasSuffix(preview.FullDiff, "\n") {
			p.Raw("\n")
		}
	}
	p.Line(fmt.Sprintf("No files changed. Confirm with:\n  skillhub skill confirm %s", pins.ProposalID))
}

// cleanupProposalRecovery removes any recovery artifacts associated with proposalID.
func cleanupProposalRecovery(workspace, proposalID string) error {
	rootHandle, err := os.OpenRoot(workspace)
	if err != nil {
		return err
	}
	defer rootHandle.Close()

	editsHandle, err := rootHandle.Open("runtime/edits")
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	defer editsHandle.Close()

	entries, err := editsHandle.Readdirnames(-1)
	if err != nil {
		return err
	}

	prefix := fmt.Sprintf("REC-%s-", proposalID)
	for _, entry := range entries {
		if strings.HasPrefix(entry, prefix) && strings.HasSuffix(entry, ".md") {
			_ = rootHandle.Remove(filepath.Join("runtime/edits", entry))
		}
	}

	// Also perform TTL expiry cleanup on other artifacts
	_ = skill.CleanupExpiredRecoveries(workspace, time.Now())
	return nil
}
