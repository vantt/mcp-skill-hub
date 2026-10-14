package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"sort"
	"time"
	"unicode/utf8"

	"github.com/vantt/skillhub-mcp/internal/mutation"
	"github.com/vantt/skillhub-mcp/internal/skill"
	"github.com/vantt/skillhub-mcp/internal/workspace"
)

// Limits of a whole-skill edit from the web. Edited text stays within what the
// resource editor always allowed; an upload may be larger, and a request as a
// whole is capped by the server's body limit.
const (
	maxEditedTextBytes = skill.MaxEditableResourceBytes
	maxUploadedBytes   = 2 << 20
)

// Actions of one change in a changeset.
const (
	TreeActionAdd    = "add"
	TreeActionModify = "modify"
	TreeActionDelete = "delete"
	TreeActionRename = "rename"
)

// Encodings of the content of one change.
const (
	TreeEncodingUTF8   = "utf8"
	TreeEncodingBase64 = "base64"
)

// SkillFileEntry is one content file of a skill as the hub holds it.
type SkillFileEntry struct {
	Path       string `json:"path"`
	SizeBytes  int64  `json:"size_bytes"`
	Digest     string `json:"digest"`
	Text       bool   `json:"text"`
	Editable   bool   `json:"editable"`
	Executable bool   `json:"executable"`
}

// SkillFilesResult lists the content files of a skill, SKILL.md first. The
// base version names exactly this tree; a changeset is made against it.
type SkillFilesResult struct {
	Result
	SkillID     string           `json:"skill_id"`
	State       string           `json:"state"`
	BaseVersion string           `json:"base_version"`
	Files       []SkillFileEntry `json:"files"`
}

// SkillFileContent is one content file with its text. Content is omitted for a
// binary file.
type SkillFileContent struct {
	SkillFileEntry
	SkillID    string  `json:"skill_id"`
	LineEnding string  `json:"line_ending"`
	Content    *string `json:"content,omitempty"`
}

// SkillTreeChange is one pending change of a skill's files. Paths are relative
// to the skill folder.
type SkillTreeChange struct {
	Action string `json:"action"`
	Path   string `json:"path"`
	// From is the old path of a rename.
	From string `json:"from,omitempty"`
	// Content is the new content of an add or a modify, and an optional new
	// content for a rename. Encoding is "utf8" (the default) or "base64".
	Content  string `json:"content,omitempty"`
	Encoding string `json:"encoding,omitempty"`
	// ExpectedDigest pins the bytes a modify, delete or rename started from.
	ExpectedDigest string `json:"expected_digest,omitempty"`
}

// SkillChangesInput is a changeset against one version of a skill.
type SkillChangesInput struct {
	BaseVersion string            `json:"base_version"`
	Changes     []SkillTreeChange `json:"changes"`
}

// SkillChangeFile is one file of a changeset with its own diff.
type SkillChangeFile struct {
	SkillCommitFile
	// From is the old path when Change is "R".
	From   string `json:"from,omitempty"`
	Binary bool   `json:"binary"`
	Diff   string `json:"diff,omitempty"`
}

// SkillChangesPreview is the proposal a changeset makes, with what a person
// reviews before committing it.
type SkillChangesPreview struct {
	SkillProposal
	BaseVersion    string            `json:"base_version"`
	Files          []SkillChangeFile `json:"files"`
	RoutingLint    []SkillCommitLint `json:"routing_lint"`
	DefaultMessage string            `json:"default_message"`
	Trust          *SkillCommitTrust `json:"trust,omitempty"`
}

// ListSkillFiles lists the content files of a skill in any state.
func (SkillService) ListSkillFiles(ctx context.Context, path, id string) (SkillFilesResult, error) {
	root, err := skill.ResolveWorkspace(path)
	if err != nil {
		return SkillFilesResult{}, err
	}
	hub, err := loadHubSkill(root, id)
	if err != nil {
		return SkillFilesResult{}, err
	}
	if hub == nil {
		return SkillFilesResult{}, skill.ErrNotFound
	}
	files := make([]SkillFileEntry, 0, len(hub.tree))
	for rel, file := range hub.tree {
		files = append(files, fileEntry(rel, file))
	}
	sortSkillFiles(files)
	result := SkillFilesResult{
		Result:      NewResult(StatusOK, fmt.Sprintf("%s: %s.", id, pluralize(len(files), "file"))),
		SkillID:     id,
		State:       parseSkillReviewMeta(hub.meta).Status,
		BaseVersion: hub.version,
		Files:       files,
	}
	return result, ctx.Err()
}

// ReadSkillFile returns one content file of a skill as the hub holds it now.
func (SkillService) ReadSkillFile(ctx context.Context, path, id, rel string) (SkillFileContent, error) {
	root, err := skill.ResolveWorkspace(path)
	if err != nil {
		return SkillFileContent{}, err
	}
	if !safeTreePath(rel) {
		return SkillFileContent{}, NewInvalidRequestError("The file path must stay inside the skill folder.", "Pick a file from the skill's file list.")
	}
	hub, err := loadHubSkill(root, id)
	if err != nil {
		return SkillFileContent{}, err
	}
	if hub == nil {
		return SkillFileContent{}, skill.ErrNotFound
	}
	file, ok := hub.tree[rel]
	if !ok {
		return SkillFileContent{}, skill.ErrNotFound
	}
	content := SkillFileContent{SkillFileEntry: fileEntry(rel, file), SkillID: id, LineEnding: lineEndingOf(file.Data)}
	if content.Text {
		text := string(file.Data)
		content.Content = &text
	}
	return content, ctx.Err()
}

func fileEntry(rel string, file treeFile) SkillFileEntry {
	text := isPlainText(file.Data)
	return SkillFileEntry{
		Path: rel, SizeBytes: int64(len(file.Data)), Digest: digestOf(file.Data),
		Text: text, Editable: text && len(file.Data) <= maxEditedTextBytes, Executable: file.Executable,
	}
}

// isPlainText reports whether data is text a person can read and edit.
func isPlainText(data []byte) bool {
	return !bytes.Contains(data, []byte{0}) && utf8.Valid(data)
}

// sortSkillFiles puts SKILL.md first and the rest in path order.
func sortSkillFiles(files []SkillFileEntry) {
	sort.Slice(files, func(i, j int) bool {
		if (files[i].Path == "SKILL.md") != (files[j].Path == "SKILL.md") {
			return files[i].Path == "SKILL.md"
		}
		return files[i].Path < files[j].Path
	})
}

// lineEndingOf names the line ending style of a text file: lf, crlf, mixed or
// none when it has no line break.
func lineEndingOf(data []byte) string {
	total := bytes.Count(data, []byte("\n"))
	if total == 0 {
		return "none"
	}
	switch crlf := bytes.Count(data, []byte("\r\n")); crlf {
	case 0:
		return "lf"
	case total:
		return "crlf"
	}
	return "mixed"
}

// PreviewSkillChanges plans a changeset of one skill as a single proposal that
// is confirmed through the generic skill confirm, and so becomes one hub commit.
func (service SkillService) PreviewSkillChanges(ctx context.Context, path, id string, input SkillChangesInput) (SkillChangesPreview, error) {
	root, err := skill.ResolveWorkspace(path)
	if err != nil {
		return SkillChangesPreview{}, err
	}
	hub, err := loadHubSkill(root, id)
	if err != nil {
		return SkillChangesPreview{}, err
	}
	if hub == nil {
		return SkillChangesPreview{}, skill.ErrNotFound
	}
	if input.BaseVersion != hub.version {
		return SkillChangesPreview{}, NewStaleProposalError(
			"The skill changed on the hub after these changes were started.",
			"Reload the skill, then apply your changes again.")
	}
	final, renames, refusal := applyTreeChanges(id, hub, input.Changes)
	if refusal != nil {
		return SkillChangesPreview{}, refusal
	}
	changes := changesBetween(hub.relDir, hub.tree, final)
	if len(changes) == 0 {
		return SkillChangesPreview{}, NewInvalidRequestError("These changes leave every file as it is.", "Change a file, then review again.")
	}
	proposal, err := service.Manager.PreviewChanges(ctx, root, id, changes, false)
	if err != nil {
		var conflict *skill.EditConflictError
		if errors.As(err, &conflict) {
			return SkillChangesPreview{}, NewEditConflictError(
				fmt.Sprintf("%s changed on the hub after it was opened.", conflict.Path), "Reload the skill, then apply your changes again.")
		}
		return SkillChangesPreview{}, err
	}
	if err := skill.StoreProposal(root, proposal, time.Now()); err != nil {
		return SkillChangesPreview{}, err
	}
	files := describeChangeFiles(hub.tree, final, renames)
	preview := SkillChangesPreview{
		SkillProposal:  makeSkillProposal(proposal, "Changes are ready for review."),
		BaseVersion:    hub.version,
		Files:          files,
		RoutingLint:    changesLint(ctx, root, id, hub.meta, final),
		DefaultMessage: fmt.Sprintf("Commit %s: %s", id, pluralize(len(files), "file")),
		Trust:          changesTrust(hub, final),
	}
	return preview, nil
}

// applyTreeChanges applies a changeset to a copy of the hub's tree. It returns
// the resulting tree and, for each renamed file, its new path and old path.
func applyTreeChanges(id string, hub *hubSkill, changes []SkillTreeChange) (map[string]treeFile, map[string]string, *Error) {
	final := copyTree(hub.tree)
	renames := map[string]string{}
	touched := map[string]bool{}
	for _, change := range changes {
		if refusal := checkTreeChangePath(change.Path); refusal != nil {
			return nil, nil, refusal
		}
		current, exists := final[change.Path]
		if touched[change.Path] {
			return nil, nil, NewInvalidRequestError(fmt.Sprintf("%s is changed twice in one set.", change.Path), "Send one change for each file.")
		}
		touched[change.Path] = true
		data, hasContent, refusal := decodeTreeContent(change)
		if refusal != nil {
			return nil, nil, refusal
		}
		switch change.Action {
		case TreeActionAdd:
			if exists {
				return nil, nil, NewInvalidRequestError(fmt.Sprintf("%s is already in the skill.", change.Path), "Choose a name that is not used, or modify the file.")
			}
			if !hasContent {
				return nil, nil, NewInvalidRequestError(fmt.Sprintf("%s has no content.", change.Path), "Send the content of a new file, empty text for an empty file.")
			}
			final[change.Path] = treeFile{Data: data}
		case TreeActionModify:
			if !exists || !hasContent {
				return nil, nil, NewInvalidRequestError(fmt.Sprintf("%s cannot be modified.", change.Path), "Modify a file the skill has, and send its full new content.")
			}
			if refusal := checkExpectedDigest(hub, change); refusal != nil {
				return nil, nil, refusal
			}
			final[change.Path] = treeFile{Data: matchLineEnding(current.Data, data, change.Encoding), Executable: current.Executable}
		case TreeActionDelete:
			if !exists || change.Path == "SKILL.md" {
				return nil, nil, NewInvalidRequestError(fmt.Sprintf("%s cannot be deleted.", change.Path), "Delete a file the skill has; SKILL.md stays.")
			}
			if refusal := checkExpectedDigest(hub, change); refusal != nil {
				return nil, nil, refusal
			}
			delete(final, change.Path)
		case TreeActionRename:
			if refusal := renameTree(final, hub, change, data, hasContent); refusal != nil {
				return nil, nil, refusal
			}
			touched[change.From] = true
			renames[change.Path] = change.From
		default:
			return nil, nil, NewInvalidRequestError(fmt.Sprintf("Unknown action %q.", change.Action), "Use add, modify, delete or rename.")
		}
	}
	if skillMD, ok := final["SKILL.md"]; !ok || len(bytes.TrimSpace(skillMD.Data)) == 0 {
		return nil, nil, NewInvalidRequestError("SKILL.md must not be empty.", "Keep the skill's instructions in SKILL.md.")
	}
	if skillMD := final["SKILL.md"]; !bytes.Equal(skillMD.Data, hub.tree["SKILL.md"].Data) {
		if refusal := normalizeProjectSkillMD(final, id, hub); refusal != nil {
			return nil, nil, refusal
		}
	}
	return final, renames, nil
}

// renameTree moves a file inside the tree, with new content when the change
// carries it. The old path leaves the tree and the new one must be free.
func renameTree(final map[string]treeFile, hub *hubSkill, change SkillTreeChange, data []byte, hasContent bool) *Error {
	if refusal := checkTreeChangePath(change.From); refusal != nil {
		return refusal
	}
	moved, exists := final[change.From]
	if !exists || change.From == "SKILL.md" || change.Path == "SKILL.md" {
		return NewInvalidRequestError(fmt.Sprintf("%s cannot be renamed.", change.From), "Rename a file the skill has; SKILL.md keeps its name.")
	}
	if _, taken := final[change.Path]; taken {
		return NewInvalidRequestError(fmt.Sprintf("%s is already in the skill.", change.Path), "Choose a name that is not used.")
	}
	if refusal := checkExpectedDigest(hub, SkillTreeChange{Path: change.From, ExpectedDigest: change.ExpectedDigest}); refusal != nil {
		return refusal
	}
	if hasContent {
		moved.Data = matchLineEnding(moved.Data, data, change.Encoding)
	}
	delete(final, change.From)
	final[change.Path] = moved
	return nil
}

// checkTreeChangePath refuses a path that leaves the skill folder, names hub
// metadata, or is one an import never copies.
func checkTreeChangePath(rel string) *Error {
	switch {
	case rel == "" || !safeTreePath(rel):
		return NewInvalidRequestError(fmt.Sprintf("%q is not a path inside the skill folder.", rel), "Use a relative path without '..' or backslashes.")
	case workspace.IsHubMeta(rel):
		return NewInvalidRequestError(fmt.Sprintf("%s is managed by the hub.", rel), "Change routing and metadata from the Routing tab.")
	}
	if reason, excluded := folderExclusion(rel); excluded {
		return NewInvalidRequestError(fmt.Sprintf("%s is not part of a skill (%s).", rel, reason), "Keep secrets and build output out of the skill.")
	}
	return nil
}

// checkExpectedDigest refuses a change made from bytes the hub no longer has.
func checkExpectedDigest(hub *hubSkill, change SkillTreeChange) *Error {
	current, ok := hub.tree[change.Path]
	if !ok || change.ExpectedDigest == "" || change.ExpectedDigest == digestOf(current.Data) {
		return nil
	}
	return NewEditConflictError(
		fmt.Sprintf("%s changed on the hub after it was opened.", change.Path),
		"Reload the skill, then apply your changes again.")
}

// decodeTreeContent turns the content of a change into bytes and enforces the
// size limits. Edited text stays valid UTF-8 without NUL bytes.
func decodeTreeContent(change SkillTreeChange) ([]byte, bool, *Error) {
	if change.Action == TreeActionDelete || (change.Content == "" && change.Encoding == "" && change.Action != TreeActionAdd) {
		return nil, false, nil
	}
	switch change.Encoding {
	case "", TreeEncodingUTF8:
		data := []byte(change.Content)
		switch {
		case len(data) > maxEditedTextBytes:
			return nil, false, NewResourceLimitsExceededError(
				fmt.Sprintf("%s is %d bytes of text; the limit is %d.", change.Path, len(data), maxEditedTextBytes),
				"Upload a file this large instead of typing it.")
		case !isPlainText(data):
			return nil, false, NewInvalidRequestError(fmt.Sprintf("%s is not valid text.", change.Path), "Send binary files as base64.")
		}
		return data, true, nil
	case TreeEncodingBase64:
		data, err := base64.StdEncoding.DecodeString(change.Content)
		if err != nil {
			return nil, false, NewInvalidRequestError(fmt.Sprintf("The content of %s is not valid base64.", change.Path), "Upload the file again.")
		}
		if len(data) > maxUploadedBytes {
			return nil, false, NewResourceLimitsExceededError(
				fmt.Sprintf("%s is %d bytes; an upload is limited to %d.", change.Path, len(data), maxUploadedBytes),
				"Keep large files out of the skill, or add them with `skillhub skill commit`.")
		}
		return data, true, nil
	}
	return nil, false, NewInvalidRequestError(fmt.Sprintf("Unknown encoding %q.", change.Encoding), "Use utf8 or base64.")
}

// matchLineEnding keeps a CRLF file CRLF when its text is edited, as the
// resource editor always did. Uploaded bytes are kept exactly.
func matchLineEnding(before, after []byte, encoding string) []byte {
	if encoding == TreeEncodingBase64 || lineEndingOf(before) != "crlf" {
		return after
	}
	after = bytes.ReplaceAll(after, []byte("\r\n"), []byte("\n"))
	return bytes.ReplaceAll(after, []byte("\n"), []byte("\r\n"))
}

// describeChangeFiles lists every file a changeset touches. A renamed file is
// one "R" row with the lines its edit changed, not a delete and an add.
func describeChangeFiles(hub, final map[string]treeFile, renames map[string]string) []SkillChangeFile {
	moved := map[string]bool{}
	for _, from := range renames {
		moved[from] = true
	}
	files := []SkillChangeFile{}
	for _, row := range describeFiles(hub, final, nil) {
		if moved[row.Path] && row.Change == "D" {
			continue
		}
		file := SkillChangeFile{SkillCommitFile: row}
		before, after := hub[row.Path], final[row.Path]
		if from, ok := renames[row.Path]; ok {
			file.Change, file.From, before = "R", from, hub[from]
			file.Added, file.Removed = 0, 0
			if mergeableText(before.Data) && mergeableText(after.Data) {
				file.Added, file.Removed = skill.CountLineChanges(before.Data, after.Data)
			}
		}
		file.Binary = (file.Change != "A" && !mergeableText(before.Data)) || (file.Change != "D" && !mergeableText(after.Data))
		if !file.Binary {
			file.Diff = fileDiff(row.Path, file.Change, before.Data, after.Data)
		}
		files = append(files, file)
	}
	return files
}

// fileDiff renders the unified diff of one file.
func fileDiff(rel, change string, before, after []byte) string {
	change2 := mutation.Change{Path: rel, Contents: after, Delete: change == "D"}
	return skill.RenderFullDiff([]mutation.Change{change2}, map[string][]byte{rel: before})
}

// changesLint lints the proposed skill the way review does.
func changesLint(ctx context.Context, root, id string, meta []byte, final map[string]treeFile) []SkillCommitLint {
	findings := []SkillCommitLint{}
	active, ok := activeSkillsForLint(ctx, root)
	if !ok {
		return findings
	}
	lint, err := lintReviewedSkill(id, meta, final["SKILL.md"].Data, active)
	if err != nil {
		return append(findings, SkillCommitLint{Code: "lint_skipped", Message: "Routing lint skipped: " + err.Error()})
	}
	for _, finding := range lint {
		findings = append(findings, SkillCommitLint{Code: finding.Code, Message: finding.Message, Fix: finding.Fix})
	}
	return findings
}

// changesTrust says whether the changed content needs an approval again.
func changesTrust(hub *hubSkill, final map[string]treeFile) *SkillCommitTrust {
	plan := &commitPlan{metaBefore: hub.meta, hub: hub}
	trust := trustOfProposal(plan, final, hub.meta)
	if !trust.ThirdParty {
		return nil
	}
	return trust
}
