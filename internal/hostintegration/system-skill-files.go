package hostintegration

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/vantt/mcp-skill-hub/internal/systemskills"
)

// System skills are written once, into the shared Agent Skills folder
// <root>/.agents/skills/<id>/, which Codex and Gemini CLI read directly.
// Claude Code reads only .claude/skills, so it gets a generated wrapper
// SKILL.md there that points at the shared folder. Older releases wrote a
// full curator copy for every host; those copies are migrated: the Claude
// copy becomes the wrapper and the Gemini copy is removed.
//
// A file at one of these paths that Skill Hub did not write is never
// overwritten or deleted. It blocks only itself: the plan leaves it out and
// lists it in PlanResult.Blocked, and every other change still runs.

const (
	skillEntryFile = "SKILL.md"

	blockedUnknownReason = "not written by Skill Hub; Skill Hub left it in place and will not point agents at it. Review it, delete it, and run the command again."

	// NoticeGeminiLegacy is the Notice code of a plan that removes an old
	// Gemini CLI curator copy.
	NoticeGeminiLegacy  = "gemini_legacy_curator_removed"
	geminiLegacySummary = "Gemini CLI now reads system-curator from .agents/skills. A Gemini CLI older than v0.28.0 reads only .gemini/skills, and Skill Hub does not serve system-curator to Gemini over MCP, so such a version no longer sees the curator; update Gemini CLI."

	// NoticeSharedSkillKept is the Notice code of a remote or removal plan
	// that keeps a shared skill because another agent in the folder needs it.
	NoticeSharedSkillKept = "shared_skill_kept"
)

// hubMarkerRelativePath marks a hub folder. Removal plans consider hub-only
// system skills only in such a folder, so a code repository that tracks the
// same files as a source is never touched.
var hubMarkerRelativePath = filepath.Join(".skillhub", "schema-version")

// plannedSystemSkills returns the system skills a plan covers. A local plan
// installs them: every skill, except that a hub-only skill is installed only
// in the hub folder itself. A remote or removal plan installs none, and these
// are the skills whose files it may remove.
func plannedSystemSkills(mode planMode, scope Scope, root, workspace string) []systemskills.SystemSkill {
	var planned []systemskills.SystemSkill
	for _, skill := range systemskills.SystemSkills() {
		if skill.HubOnly {
			if mode.local() && (scope != ScopeProject || filepath.Clean(root) != filepath.Clean(workspace)) {
				continue
			}
			if !mode.local() && !hasHubMarker(root) {
				continue
			}
		}
		planned = append(planned, skill)
	}
	return planned
}

func hasHubMarker(root string) bool {
	info, err := os.Lstat(filepath.Join(root, hubMarkerRelativePath))
	return err == nil && info.Mode().IsRegular()
}

// systemSkillFiles is what a plan does with the system skill files.
type systemSkillFiles struct {
	shared  []preparedFile
	hosts   map[Host][]preparedFile
	notices []Notice
}

func (files *systemSkillFiles) notice(code, summary string) {
	if !slices.ContainsFunc(files.notices, func(notice Notice) bool { return notice.Summary == summary }) {
		files.notices = append(files.notices, Notice{Code: code, Summary: summary})
	}
}

// prepareSystemSkills plans the shared files, the Claude Code wrappers and
// the cleanup of old copies for the selected hosts.
func prepareSystemSkills(selected []Adapter, scope Scope, root, workspace string, mode planMode) (systemSkillFiles, error) {
	files := systemSkillFiles{hosts: map[Host][]preparedFile{}}
	for _, skill := range plannedSystemSkills(mode, scope, root, workspace) {
		canonicalBlocked := false
		if mode.local() {
			for _, file := range skill.Files {
				prepared, err := prepareSharedWrite(skill, file, root)
				if err != nil {
					return systemSkillFiles{}, err
				}
				canonicalBlocked = canonicalBlocked || prepared.state.Blocked != ""
				files.shared = append(files.shared, prepared)
			}
		}
		for _, adapter := range selected {
			if adapter.Wrapper {
				// A wrapper is withdrawn when its target is not Skill Hub's,
				// so Claude Code never follows foreign content under a Skill
				// Hub name, and for a client that gets the skill over MCP.
				remove := !mode.local() || canonicalBlocked || (skill.ServedOverMCP && HostSupportsSkillsExtension(adapter.Host))
				prepared, err := prepareWrapper(adapter, skill, root, remove)
				if err != nil {
					return systemSkillFiles{}, err
				}
				files.hosts[adapter.Host] = append(files.hosts[adapter.Host], prepared)
			}
			if adapter.LegacySkillsDirRelativePath != "" && skill.ID == systemskills.CuratorSkillID {
				prepared, err := prepareLegacyCopy(adapter, skill, root)
				if err != nil {
					return systemSkillFiles{}, err
				}
				if prepared.remove && prepared.state.Exists {
					files.notice(NoticeGeminiLegacy, geminiLegacySummary)
				}
				files.hosts[adapter.Host] = append(files.hosts[adapter.Host], prepared)
			}
		}
		if mode.local() {
			continue
		}
		if keeper, keep := canonicalKeeper(skill, selected, scope, root); keep {
			entry := sharedSkillPath(root, skill.ID, skillEntryFile)
			if _, err := os.Lstat(entry); err == nil {
				for _, adapter := range selected {
					if adapter.NativeSkill && !adapter.Wrapper {
						files.notice(NoticeSharedSkillKept, fmt.Sprintf("%s still sees %s in %s while %s in this folder uses a local hub.", hostLabel(adapter.Host), skill.ID, SharedSkillsDirRelativePath, hostLabel(keeper)))
					}
				}
			}
			continue
		}
		for _, file := range skill.Files {
			prepared, err := prepareSharedRemoval(skill, file, root)
			if err != nil {
				return systemSkillFiles{}, err
			}
			files.shared = append(files.shared, prepared)
		}
	}
	return files, nil
}

// canonicalKeeper reports a host that still needs the shared files of a skill
// after a remote or removal plan: one that is not part of the plan, has a
// local registration in this root and does not get the skill over MCP.
func canonicalKeeper(skill systemskills.SystemSkill, selected []Adapter, scope Scope, root string) (Host, bool) {
	for _, adapter := range adapters {
		if slices.ContainsFunc(selected, func(chosen Adapter) bool { return chosen.Host == adapter.Host }) {
			continue
		}
		registration, err := scopeRegistration(adapter, scope, root)
		if err != nil {
			// A config file that cannot be read may hold a local entry;
			// keeping the shared files is the safe answer.
			return adapter.Host, true
		}
		servedOverMCP := skill.ServedOverMCP && HostSupportsSkillsExtension(adapter.Host)
		if registration.Present && !registration.Remote() && !servedOverMCP {
			return adapter.Host, true
		}
	}
	return "", false
}

func sharedSkillPath(root, id, file string) string {
	return filepath.Join(root, filepath.FromSlash(SharedSkillsDirRelativePath), id, filepath.FromSlash(file))
}

func wrapperPath(adapter Adapter, root, id string) string {
	return filepath.Join(root, filepath.FromSlash(adapter.SkillsDirRelativePath), id, skillEntryFile)
}

func legacyCopyPath(adapter Adapter, root, id string) string {
	return filepath.Join(root, filepath.FromSlash(adapter.LegacySkillsDirRelativePath), id, skillEntryFile)
}

// skillFile is a system skill file as planning read it.
type skillFile struct {
	host    Host
	kind    ChangeKind
	path    string
	raw     []byte
	mode    os.FileMode
	exists  bool
	blocked string
}

// readSkillFile reads a system skill file. A symbolic link on the way is not
// an error here: the file is reported as blocked and the rest of the plan
// proceeds.
func readSkillFile(host Host, kind ChangeKind, path, root string) (skillFile, error) {
	file := skillFile{host: host, kind: kind, path: path}
	var err error
	file.raw, file.mode, file.exists, err = readManagedFile(path, root)
	if errors.Is(err, ErrSymlink) {
		link := strings.TrimPrefix(err.Error(), ErrSymlink.Error()+": ")
		return skillFile{host: host, kind: kind, path: path, mode: 0o644, blocked: "Skill Hub does not write through the symbolic link " + link + "; system skill files were skipped"}, nil
	}
	return file, err
}

// unchanged plans no change for the file.
func (file skillFile) unchanged() preparedFile {
	return newPreparedFile(file.host, file.kind, file.path, file.raw, file.raw, file.mode, file.exists, "")
}

// write plans the file with the desired content.
func (file skillFile) write(desired []byte) preparedFile {
	return newPreparedFile(file.host, file.kind, file.path, file.raw, desired, file.mode, file.exists, "")
}

// block leaves the file unchanged and reports it with reason.
func (file skillFile) block(reason string) preparedFile {
	prepared := file.unchanged()
	prepared.state.Current = false
	prepared.state.Blocked = reason
	prepared.preview = "left unchanged: " + reason
	return prepared
}

// removal plans the deletion of the file.
func (file skillFile) removal(preview string) preparedFile {
	prepared := newPreparedFile(file.host, file.kind, file.path, file.raw, nil, file.mode, file.exists, "")
	markRemoval(&prepared, file.exists)
	prepared.preview = boundPreview(preview)
	return prepared
}

func prepareSharedWrite(skill systemskills.SystemSkill, bundled systemskills.SkillFile, root string) (preparedFile, error) {
	file, err := readSkillFile("", ChangeSharedSkill, sharedSkillPath(root, skill.ID, bundled.Path), root)
	switch {
	case err != nil:
		return preparedFile{}, err
	case file.blocked != "":
		return file.block(file.blocked), nil
	case file.exists && !systemskills.KnownFileContent(skill.ID, bundled.Path, file.raw):
		return file.block(blockedUnknownReason), nil
	}
	return file.write(bundled.Content), nil
}

func prepareSharedRemoval(skill systemskills.SystemSkill, bundled systemskills.SkillFile, root string) (preparedFile, error) {
	file, err := readSkillFile("", ChangeSharedSkill, sharedSkillPath(root, skill.ID, bundled.Path), root)
	switch {
	case err != nil:
		return preparedFile{}, err
	case file.blocked != "":
		return file.block(file.blocked), nil
	case !file.exists:
		return file.unchanged(), nil
	case !systemskills.KnownFileContent(skill.ID, bundled.Path, file.raw):
		return file.block(blockedUnknownReason), nil
	}
	return file.removal("remove the bundled " + skill.ID + " file: no agent in this folder uses a local hub any more"), nil
}

// fullCopyOwned reports whether raw is a full SKILL.md that Skill Hub wrote
// for the skill under the layout of earlier releases.
func fullCopyOwned(id string, raw []byte) bool {
	return systemskills.KnownFileContent(id, skillEntryFile, raw)
}

func prepareWrapper(adapter Adapter, skill systemskills.SystemSkill, root string, remove bool) (preparedFile, error) {
	file, err := readSkillFile(adapter.Host, ChangeNativeSkill, wrapperPath(adapter, root, skill.ID), root)
	if err != nil {
		return preparedFile{}, err
	}
	if file.blocked != "" {
		return file.block(file.blocked), nil
	}
	owned := file.exists && (wrapperOwned(skill, file.raw) || fullCopyOwned(skill.ID, file.raw))
	if remove {
		switch {
		case !file.exists:
			return file.unchanged(), nil
		case !owned:
			return file.block(blockedUnknownReason), nil
		}
		return file.removal("remove the Skill Hub wrapper of " + skill.ID), nil
	}
	desired, err := skillWrapper(skill)
	if err != nil {
		return preparedFile{}, err
	}
	if file.exists && !owned {
		return file.block(blockedUnknownReason), nil
	}
	return file.write(desired), nil
}

func prepareLegacyCopy(adapter Adapter, skill systemskills.SystemSkill, root string) (preparedFile, error) {
	file, err := readSkillFile(adapter.Host, ChangeNativeSkill, legacyCopyPath(adapter, root, skill.ID), root)
	switch {
	case err != nil:
		return preparedFile{}, err
	case file.blocked != "":
		return file.block(file.blocked), nil
	case !file.exists:
		return file.unchanged(), nil
	case !fullCopyOwned(skill.ID, file.raw):
		return file.block(blockedUnknownReason), nil
	}
	return file.removal("remove the old " + skill.ID + " copy; " + geminiLegacySummary), nil
}

// systemSkillTarget is what a system skill path in a change names.
type systemSkillTarget struct {
	skill  systemskills.SystemSkill
	file   string
	legacy bool
}

// changeSkillTarget checks a shared or native skill change against the
// skills the plan covers and returns the file it names.
func changeSkillTarget(root string, covered []systemskills.SystemSkill, change Change) (systemSkillTarget, error) {
	invalid := fmt.Errorf("change path does not match host contract: %s", change.Path)
	clean := filepath.Clean(change.Path)
	var adapter Adapter
	if change.Kind != ChangeSharedSkill {
		index := slices.IndexFunc(adapters, func(candidate Adapter) bool { return candidate.Host == change.Host })
		if index < 0 {
			return systemSkillTarget{}, fmt.Errorf("unsupported host %q", change.Host)
		}
		adapter = adapters[index]
	}
	for _, skill := range covered {
		if change.Kind == ChangeSharedSkill {
			for _, file := range skill.Files {
				if clean == sharedSkillPath(root, skill.ID, file.Path) {
					return systemSkillTarget{skill: skill, file: file.Path}, nil
				}
			}
			continue
		}
		if adapter.Wrapper && clean == wrapperPath(adapter, root, skill.ID) {
			return systemSkillTarget{skill: skill, file: skillEntryFile}, nil
		}
		if adapter.LegacySkillsDirRelativePath != "" && skill.ID == systemskills.CuratorSkillID && clean == legacyCopyPath(adapter, root, skill.ID) {
			if !change.Remove {
				return systemSkillTarget{}, invalid
			}
			return systemSkillTarget{skill: skill, file: skillEntryFile, legacy: true}, nil
		}
	}
	return systemSkillTarget{}, invalid
}

// expectedSkillChange recomputes a system skill change from the file's
// current content: a removal only of bytes Skill Hub owns, a write only of
// the bundled file or the current wrapper.
func expectedSkillChange(root string, covered []systemskills.SystemSkill, change Change, raw []byte, mode planMode) ([]byte, bool, error) {
	target, err := changeSkillTarget(root, covered, change)
	if err != nil {
		return nil, false, err
	}
	if change.Remove {
		var owned bool
		switch {
		case change.Kind == ChangeSharedSkill:
			owned = systemskills.KnownFileContent(target.skill.ID, target.file, raw)
		case target.legacy:
			owned = fullCopyOwned(target.skill.ID, raw)
		default:
			owned = wrapperOwned(target.skill, raw) || fullCopyOwned(target.skill.ID, raw)
		}
		if !owned {
			return nil, false, &ConflictError{Path: change.Path, Reason: "the " + target.skill.ID + " skill file changed after the preview; it is left in place"}
		}
		return nil, true, nil
	}
	if !mode.local() {
		return nil, false, fmt.Errorf("a remote or removal plan never writes system skill files: %s", change.Path)
	}
	if change.Kind == ChangeSharedSkill {
		for _, file := range target.skill.Files {
			if file.Path == target.file {
				return file.Content, false, nil
			}
		}
	}
	wrapper, err := skillWrapper(target.skill)
	return wrapper, false, err
}

// SystemSkillFileOwned reports whether raw holds bytes that Skill Hub wrote
// at relPath, a path relative to a scope root: a current or previously
// shipped file of a system skill in the shared Agent Skills folder, a Claude
// Code wrapper, or a full copy of an earlier layout.
func SystemSkillFileOwned(relPath string, raw []byte) bool {
	rel := path.Clean(filepath.ToSlash(relPath))
	for _, skill := range systemskills.SystemSkills() {
		if file, found := strings.CutPrefix(rel, SharedSkillsDirRelativePath+"/"+skill.ID+"/"); found {
			return systemskills.KnownFileContent(skill.ID, file, raw)
		}
		for _, adapter := range adapters {
			if adapter.Wrapper && rel == adapter.SkillsDirRelativePath+"/"+skill.ID+"/"+skillEntryFile {
				return wrapperOwned(skill, raw) || fullCopyOwned(skill.ID, raw)
			}
			if adapter.LegacySkillsDirRelativePath != "" && skill.ID == systemskills.CuratorSkillID && rel == adapter.LegacySkillsDirRelativePath+"/"+skill.ID+"/"+skillEntryFile {
				return fullCopyOwned(skill.ID, raw)
			}
		}
	}
	return false
}

// hostLabel names a host in a sentence.
func hostLabel(host Host) string {
	switch host {
	case HostClaude:
		return "Claude Code"
	case HostCodex:
		return "Codex"
	case HostGemini:
		return "Gemini CLI"
	}
	return string(host)
}
