package hostintegration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/vantt/skillhub-mcp/internal/systemskills"
)

func systemSkill(t *testing.T, id string) systemskills.SystemSkill {
	t.Helper()
	for _, skill := range systemskills.SystemSkills() {
		if skill.ID == id {
			return skill
		}
	}
	t.Fatalf("no system skill %s", id)
	return systemskills.SystemSkill{}
}

func currentWrapper(t *testing.T, id string) []byte {
	t.Helper()
	wrapper, err := skillWrapper(systemSkill(t, id))
	if err != nil {
		t.Fatal(err)
	}
	return wrapper
}

// assertSystemSkillLayout checks that root holds exactly the given system
// skills: every bundled file under .agents/skills/<id>/, the Claude Code
// wrapper under .claude/skills/<id>/, and nothing under .gemini/skills.
func assertSystemSkillLayout(t *testing.T, root string, ids ...string) {
	t.Helper()
	for _, skill := range systemskills.SystemSkills() {
		wanted := slices.Contains(ids, skill.ID)
		for _, file := range skill.Files {
			path := sharedSkillPath(root, skill.ID, file.Path)
			content, err := os.ReadFile(path)
			switch {
			case wanted && err != nil:
				t.Fatalf("missing %s: %v", path, err)
			case wanted && !bytes.Equal(content, file.Content):
				t.Fatalf("%s is not the bundled file", path)
			case !wanted && !errors.Is(err, os.ErrNotExist):
				t.Fatalf("%s should not exist: %v", path, err)
			}
		}
		wrapper := filepath.Join(root, ".claude", "skills", skill.ID, "SKILL.md")
		content, err := os.ReadFile(wrapper)
		switch {
		case wanted && err != nil:
			t.Fatalf("missing wrapper %s: %v", wrapper, err)
		case wanted && !bytes.Equal(content, currentWrapper(t, skill.ID)):
			t.Fatalf("%s is not the current wrapper:\n%s", wrapper, content)
		case !wanted && !errors.Is(err, os.ErrNotExist):
			t.Fatalf("wrapper %s should not exist: %v", wrapper, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, ".gemini", "skills")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf(".gemini/skills exists: %v", err)
	}
}

func relPath(t *testing.T, root, path string) string {
	t.Helper()
	relative, err := filepath.Rel(root, path)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.ToSlash(relative)
}

func planFor(t *testing.T, request Request) PlanResult {
	t.Helper()
	plan, err := Plan(context.Background(), request)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	return plan
}

func applyPlan(t *testing.T, plan PlanResult) {
	t.Helper()
	if _, err := Apply(context.Background(), plan, ApplyOptions{Confirmed: true}); err != nil {
		t.Fatalf("apply: %v", err)
	}
}

// writeOldLayout writes the three full curator copies that earlier releases
// installed in a project.
func writeOldLayout(t *testing.T, root string, content []byte) {
	t.Helper()
	for _, dir := range []string{".claude", ".agents", ".gemini"} {
		writeTestFile(t, filepath.Join(root, dir, "skills", "system-curator", "SKILL.md"), content, 0o644)
	}
}

func TestFreshHubPlanWritesSharedFilesBeforeClaudeWrappers(t *testing.T) {
	hub := t.TempDir()
	plan := planFor(t, Request{Workspace: hub, Binary: absoluteBinary(t)})
	lastShared, firstWrapper := -1, len(plan.Changes)
	wrappers := map[string]bool{}
	shared := map[string]bool{}
	for index, change := range plan.Changes {
		relative := relPath(t, hub, change.Path)
		if strings.HasPrefix(relative, ".gemini/skills") {
			t.Fatalf("plan writes under .gemini/skills: %s", relative)
		}
		switch change.Kind {
		case ChangeSharedSkill:
			if change.Host != "" || change.Remove {
				t.Fatalf("shared change %+v", change)
			}
			shared[strings.Split(relative, "/")[2]] = true
			lastShared = index
		case ChangeNativeSkill:
			if change.Host != HostClaude {
				t.Fatalf("native skill change for %s: %s", change.Host, relative)
			}
			wrappers[relative] = true
			firstWrapper = min(firstWrapper, index)
		}
	}
	if lastShared > firstWrapper {
		t.Fatal("a wrapper is written before a shared file")
	}
	if !shared["system-curator"] || !shared["distill-lab"] {
		t.Fatalf("shared skills = %v", shared)
	}
	if !wrappers[".claude/skills/system-curator/SKILL.md"] || !wrappers[".claude/skills/distill-lab/SKILL.md"] || len(wrappers) != 2 {
		t.Fatalf("wrappers = %v", wrappers)
	}
	applyPlan(t, plan)
	assertSystemSkillLayout(t, hub, systemskills.CuratorSkillID, systemskills.DistillSkillID)
	for _, file := range systemSkill(t, systemskills.DistillSkillID).Files {
		assertMode(t, sharedSkillPath(hub, systemskills.DistillSkillID, file.Path), 0o644)
	}
	if again := planFor(t, Request{Workspace: hub, Binary: absoluteBinary(t)}); len(again.Blocked) != 0 {
		t.Fatalf("second plan blocks: %+v", again.Blocked)
	}
}

func TestProjectAndUserPlansInstallOnlyTheCurator(t *testing.T) {
	hub := t.TempDir()
	project := t.TempDir()
	home := t.TempDir()
	binary := absoluteBinary(t)
	for _, request := range []Request{
		{Workspace: hub, Root: project, Binary: binary},
		{Workspace: hub, Root: home, Scope: ScopeUser, Binary: binary},
	} {
		plan := planFor(t, request)
		for _, change := range plan.Changes {
			if strings.Contains(change.Path, "distill-lab") {
				t.Fatalf("%s plan installs distill-lab: %s", request.Scope, change.Path)
			}
		}
		applyPlan(t, plan)
		assertSystemSkillLayout(t, request.Root, systemskills.CuratorSkillID)
	}
	projectWrapper := readTestFile(t, filepath.Join(project, ".claude", "skills", "system-curator", "SKILL.md"))
	userWrapper := readTestFile(t, filepath.Join(home, ".claude", "skills", "system-curator", "SKILL.md"))
	if !bytes.Equal(projectWrapper, userWrapper) {
		t.Fatal("project and user wrappers differ")
	}
}

func TestRemoteAndRemovePlansNeverWriteSystemSkills(t *testing.T) {
	for _, mode := range []string{"remote", "remove"} {
		t.Run(mode, func(t *testing.T) {
			hub := t.TempDir()
			writeTestFile(t, filepath.Join(hub, ".skillhub", "schema-version"), []byte("3\n"), 0o644)
			binary := absoluteBinary(t)
			planAndApply(t, Request{Workspace: hub, Binary: binary})
			request := Request{Root: hub, Binary: binary, Remove: true}
			if mode == "remote" {
				request = Request{Root: hub, Binary: binary, Hub: &RemoteHub{URL: testHubURL}}
			}
			plan := planFor(t, request)
			lastNative, firstSharedRemoval := -1, len(plan.Changes)
			for index, change := range plan.Changes {
				if (change.Kind == ChangeSharedSkill || change.Kind == ChangeNativeSkill) && !change.Remove {
					t.Fatalf("%s plan writes a system skill file: %s", mode, change.Path)
				}
				if change.Kind == ChangeNativeSkill {
					lastNative = index
				}
				if change.Kind == ChangeSharedSkill {
					firstSharedRemoval = min(firstSharedRemoval, index)
				}
			}
			if firstSharedRemoval == len(plan.Changes) || lastNative > firstSharedRemoval {
				t.Fatalf("wrappers must be removed before shared files: %+v", plan.Changes)
			}
			applyPlan(t, plan)
			assertSystemSkillLayout(t, hub)
		})
	}
}

func TestRemovePlanSkipsHubOnlySkillsOutsideAHub(t *testing.T) {
	root := t.TempDir()
	binary := absoluteBinary(t)
	planAndApply(t, Request{Workspace: root, Root: root, Binary: binary})
	distill := systemSkill(t, systemskills.DistillSkillID)
	if _, err := os.Stat(sharedSkillPath(root, distill.ID, "SKILL.md")); err != nil {
		t.Fatalf("a local plan in its own workspace installs distill-lab: %v", err)
	}
	// Without the hub marker the folder looks like a code repository that
	// tracks the distill-lab source and wrapper.
	plan := planFor(t, Request{Root: root, Binary: binary, Remove: true})
	for _, change := range plan.Changes {
		if strings.Contains(change.Path, "distill-lab") {
			t.Fatalf("removal outside a hub touches distill-lab: %s", change.Path)
		}
	}
	for _, blocked := range plan.Blocked {
		if strings.Contains(blocked.Path, "distill-lab") {
			t.Fatalf("removal outside a hub reports distill-lab: %s", blocked.Path)
		}
	}
	applyPlan(t, plan)
	if _, err := os.Stat(filepath.Join(root, ".claude", "skills", distill.ID, "SKILL.md")); err != nil {
		t.Fatalf("distill-lab wrapper removed outside a hub: %v", err)
	}
}

func TestRemoteForEveryHostRemovesTheSharedCurator(t *testing.T) {
	hub, project, binary := t.TempDir(), t.TempDir(), absoluteBinary(t)
	planAndApply(t, Request{Workspace: hub, Root: project, Binary: binary})
	planAndApply(t, Request{Root: project, Binary: binary, Hub: &RemoteHub{URL: testHubURL}})
	assertSystemSkillLayout(t, project)
	if _, err := os.Stat(filepath.Join(project, ".agents")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("empty .agents folder left: %v", err)
	}
}

func TestRemoteForCodexKeepsTheSharedCuratorWhileClaudeIsLocal(t *testing.T) {
	hub, project, binary := t.TempDir(), t.TempDir(), absoluteBinary(t)
	// Only Claude Code is registered locally; Codex is not part of that plan.
	planAndApply(t, Request{Workspace: hub, Root: project, Binary: binary, Hosts: []Host{HostClaude}})
	plan := planAndApply(t, Request{Root: project, Binary: binary, Hosts: []Host{HostCodex}, Hub: &RemoteHub{URL: testHubURL}})
	assertSystemSkillLayout(t, project, systemskills.CuratorSkillID)
	want := "Codex still sees system-curator in .agents/skills while Claude Code in this folder uses a local hub."
	if !slices.ContainsFunc(plan.Notices, func(notice Notice) bool { return notice.Summary == want }) {
		t.Fatalf("notices = %+v", plan.Notices)
	}
}

func TestOldLayoutMigratesToTheSharedFolderAndAWrapper(t *testing.T) {
	versions := map[string][]byte{"current": []byte(systemCuratorInstructions())}
	if previous := previousCuratorVersion(t); previous != nil {
		versions["previous"] = previous
	}
	for name, content := range versions {
		t.Run(name, func(t *testing.T) {
			hub, project, binary := t.TempDir(), t.TempDir(), absoluteBinary(t)
			writeOldLayout(t, project, content)
			request := Request{Workspace: hub, Root: project, Binary: binary}
			plan := planFor(t, request)
			if len(plan.Blocked) != 0 {
				t.Fatalf("blocked: %+v", plan.Blocked)
			}
			var gemini *Change
			for index, change := range plan.Changes {
				if strings.HasSuffix(filepath.ToSlash(change.Path), ".gemini/skills/system-curator/SKILL.md") {
					gemini = &plan.Changes[index]
				}
			}
			if gemini == nil || !gemini.Remove || !strings.Contains(gemini.Preview, "v0.28.0") {
				t.Fatalf("old Gemini copy is not removed with the notice: %+v", gemini)
			}
			if !slices.ContainsFunc(plan.Notices, func(notice Notice) bool { return notice.Code == NoticeGeminiLegacy }) {
				t.Fatalf("notices = %+v", plan.Notices)
			}
			planAndApply(t, request)
			assertSystemSkillLayout(t, project, systemskills.CuratorSkillID)
		})
	}
}

// previousCuratorVersion returns a curator SKILL.md an earlier release
// shipped, when this checkout has the tag.
func previousCuratorVersion(t *testing.T) []byte {
	t.Helper()
	content, err := gitOutput("show", "v0.1.0:internal/systemskills/curator/SKILL.md")
	if err != nil {
		return nil
	}
	if !fullCopyOwned(systemskills.CuratorSkillID, []byte(content)) {
		t.Fatal("the v0.1.0 curator is not a known version")
	}
	return []byte(content)
}

func TestEditedOldCopyIsBlockedWhileTheOtherChangesApply(t *testing.T) {
	for _, dir := range []string{".claude", ".gemini"} {
		t.Run(dir, func(t *testing.T) {
			hub, project, binary := t.TempDir(), t.TempDir(), absoluteBinary(t)
			writeOldLayout(t, project, []byte(systemCuratorInstructions()))
			edited := filepath.Join(project, dir, "skills", "system-curator", "SKILL.md")
			writeTestFile(t, edited, []byte("my own curator\n"), 0o644)
			request := Request{Workspace: hub, Root: project, Binary: binary}
			plan := planFor(t, request)
			if len(plan.Blocked) != 1 || plan.Blocked[0].Path != edited || !strings.Contains(plan.Blocked[0].Blocked, "not written by Skill Hub") {
				t.Fatalf("blocked = %+v", plan.Blocked)
			}
			applyPlan(t, plan)
			if got := readTestFile(t, edited); string(got) != "my own curator\n" {
				t.Fatalf("edited copy changed: %q", got)
			}
			if registration := ReadRegistration(HostClaude, readTestFile(t, filepath.Join(project, ".mcp.json"))); !registration.Present {
				t.Fatal("MCP entry not written next to a blocked skill file")
			}
			if finding := MissingFinding(planFor(t, request)); finding == nil || finding.ID != FindingHostIntegrationBlocked || !strings.Contains(finding.Summary, edited) {
				t.Fatalf("finding = %+v", finding)
			}
		})
	}
}

func TestEditedSharedFileBlocksItselfAndWithdrawsTheWrapper(t *testing.T) {
	hub, project, binary := t.TempDir(), t.TempDir(), absoluteBinary(t)
	request := Request{Workspace: hub, Root: project, Binary: binary}
	planAndApply(t, request)
	canonical := sharedSkillPath(project, systemskills.CuratorSkillID, "SKILL.md")
	writeTestFile(t, canonical, []byte("---\nname: system-curator\n---\nsomething else\n"), 0o644)
	plan := planFor(t, request)
	if len(plan.Blocked) != 1 || plan.Blocked[0].Path != canonical {
		t.Fatalf("blocked = %+v", plan.Blocked)
	}
	applyPlan(t, plan)
	if _, err := os.Stat(filepath.Join(project, ".claude", "skills", "system-curator", "SKILL.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("wrapper kept next to a foreign shared file: %v", err)
	}
	if got := readTestFile(t, canonical); !strings.Contains(string(got), "something else") {
		t.Fatal("foreign shared file changed")
	}
}

func TestDanglingWrapperGetsItsSharedFilesBack(t *testing.T) {
	hub, project, binary := t.TempDir(), t.TempDir(), absoluteBinary(t)
	request := Request{Workspace: hub, Root: project, Binary: binary}
	planAndApply(t, request)
	if err := os.RemoveAll(filepath.Join(project, ".agents")); err != nil {
		t.Fatal(err)
	}
	if finding := MissingFinding(planFor(t, request)); finding == nil || finding.ID != FindingHostIntegrationMissing {
		t.Fatalf("finding = %+v", finding)
	}
	planAndApply(t, request)
	assertSystemSkillLayout(t, project, systemskills.CuratorSkillID)
}

// swapKinds exchanges the first change of kind a with the first of kind b.
func swapKinds(t *testing.T, plan PlanResult, a, b ChangeKind) PlanResult {
	t.Helper()
	forged := plan
	forged.Changes = slices.Clone(plan.Changes)
	first := slices.IndexFunc(forged.Changes, func(change Change) bool { return change.Kind == a })
	second := slices.IndexFunc(forged.Changes, func(change Change) bool { return change.Kind == b })
	if first < 0 || second < 0 {
		t.Fatalf("plan lacks %s or %s: %+v", a, b, plan.Changes)
	}
	forged.Changes[first], forged.Changes[second] = forged.Changes[second], forged.Changes[first]
	return forged
}

func TestApplyChecksTheRankOfSkillChanges(t *testing.T) {
	hub, project, binary := t.TempDir(), t.TempDir(), absoluteBinary(t)
	writeOldLayout(t, project, []byte(systemCuratorInstructions()))
	if err := os.Remove(sharedSkillPath(project, systemskills.CuratorSkillID, "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	// Migration: a shared write, a wrapper update and the Gemini removal.
	migration := planFor(t, Request{Workspace: hub, Root: project, Binary: binary})
	if _, err := Apply(context.Background(), swapKinds(t, migration, ChangeSharedSkill, ChangeNativeSkill), ApplyOptions{Confirmed: true}); err == nil || !strings.Contains(err.Error(), "dependency ordered") {
		t.Fatalf("shared write after a wrapper write: %v", err)
	}
	applyPlan(t, migration)

	// Blocked shared file: the wrapper removal sits next to the MCP writes.
	writeTestFile(t, sharedSkillPath(project, systemskills.CuratorSkillID, "SKILL.md"), []byte("mine\n"), 0o644)
	applyPlan(t, planFor(t, Request{Workspace: hub, Root: project, Binary: binary}))
	if err := os.Remove(sharedSkillPath(project, systemskills.CuratorSkillID, "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	applyPlan(t, planFor(t, Request{Workspace: hub, Root: project, Binary: binary}))

	// Remote: wrapper removal before the shared removal.
	remote := planFor(t, Request{Root: project, Binary: binary, Hub: &RemoteHub{URL: testHubURL}})
	if _, err := Apply(context.Background(), swapKinds(t, remote, ChangeSharedSkill, ChangeNativeSkill), ApplyOptions{Confirmed: true}); err == nil || !strings.Contains(err.Error(), "dependency ordered") {
		t.Fatalf("shared removal before a wrapper removal: %v", err)
	}
	applyPlan(t, remote)
	assertSystemSkillLayout(t, project)
}

func TestApplyRejectsForgedSkillPaths(t *testing.T) {
	hub, project, binary := t.TempDir(), t.TempDir(), absoluteBinary(t)
	plan := planFor(t, Request{Workspace: hub, Root: project, Binary: binary})
	forge := func(edit func(change *Change) bool) PlanResult {
		forged := plan
		forged.Changes = slices.Clone(plan.Changes)
		for index := range forged.Changes {
			if edit(&forged.Changes[index]) {
				return forged
			}
		}
		t.Fatal("nothing to forge")
		return forged
	}
	cases := map[string]PlanResult{
		"shared file outside the skill folder": forge(func(change *Change) bool {
			if change.Kind != ChangeSharedSkill {
				return false
			}
			change.Path = filepath.Join(project, ".agents", "skills", "other", "SKILL.md")
			return true
		}),
		"native skill written under .gemini": forge(func(change *Change) bool {
			if change.Kind != ChangeNativeSkill {
				return false
			}
			change.Host = HostGemini
			change.Path = filepath.Join(project, ".gemini", "skills", "system-curator", "SKILL.md")
			return true
		}),
		"shared file with a host": forge(func(change *Change) bool {
			if change.Kind != ChangeSharedSkill {
				return false
			}
			change.Host = HostCodex
			return true
		}),
		"hub-only skill in a project": forge(func(change *Change) bool {
			if change.Kind != ChangeSharedSkill {
				return false
			}
			change.Path = sharedSkillPath(project, systemskills.DistillSkillID, "SKILL.md")
			return true
		}),
	}
	for name, forged := range cases {
		if _, err := Apply(context.Background(), forged, ApplyOptions{Confirmed: true}); err == nil || (!strings.Contains(err.Error(), "contract") && !strings.Contains(err.Error(), "no host")) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestSkillChangePathsUsePlatformSeparators(t *testing.T) {
	root := t.TempDir()
	covered := systemskills.SystemSkills()
	path := func(slash string) string { return filepath.Join(root, filepath.FromSlash(slash)) }
	cases := []struct {
		change Change
		ok     bool
	}{
		{Change{Kind: ChangeSharedSkill, Path: path(".agents/skills/distill-lab/scripts/distill.mjs")}, true},
		{Change{Kind: ChangeSharedSkill, Path: path(".agents/skills/distill-lab/scripts/other.mjs")}, false},
		{Change{Kind: ChangeNativeSkill, Host: HostClaude, Path: path(".claude/skills/distill-lab/SKILL.md")}, true},
		{Change{Kind: ChangeNativeSkill, Host: HostCodex, Path: path(".agents/skills/system-curator/SKILL.md")}, false},
		{Change{Kind: ChangeNativeSkill, Host: HostGemini, Path: path(".gemini/skills/system-curator/SKILL.md"), Remove: true}, true},
		{Change{Kind: ChangeNativeSkill, Host: HostGemini, Path: path(".gemini/skills/system-curator/SKILL.md")}, false},
	}
	for _, test := range cases {
		err := validateChangePath(root, ScopeProject, covered, test.change)
		if (err == nil) != test.ok {
			t.Errorf("%s %s: %v", test.change.Kind, test.change.Path, err)
		}
	}
}

func TestWrapperOwnership(t *testing.T) {
	hub, project, binary := t.TempDir(), t.TempDir(), absoluteBinary(t)
	request := Request{Workspace: hub, Root: project, Binary: binary, Hosts: []Host{HostClaude}}
	planAndApply(t, request)
	wrapper := filepath.Join(project, ".claude", "skills", "system-curator", "SKILL.md")

	older := []byte("---\nname: system-curator\ndescription: an older render\n---\n<!-- Written by Skill Hub (wrapper v0). -->\n")
	original := knownWrappers
	knownWrappers = map[string][]string{systemskills.CuratorSkillID: {systemskills.FileDigest(older)}}
	t.Cleanup(func() { knownWrappers = original })
	writeTestFile(t, wrapper, older, 0o644)
	planAndApply(t, request)
	if !bytes.Equal(readTestFile(t, wrapper), currentWrapper(t, systemskills.CuratorSkillID)) {
		t.Fatal("a shipped older wrapper was not updated")
	}

	for name, edit := range map[string]func([]byte) []byte{
		"description": func(raw []byte) []byte {
			return bytes.Replace(raw, []byte("description: "), []byte("description: My own words. "), 1)
		},
		"extra text": func(raw []byte) []byte { return append(raw, []byte("\nAlso do this.\n")...) },
	} {
		edited := edit(currentWrapper(t, systemskills.CuratorSkillID))
		writeTestFile(t, wrapper, edited, 0o644)
		plan := planFor(t, request)
		if len(plan.Blocked) != 1 || plan.Blocked[0].Path != wrapper {
			t.Fatalf("%s: blocked = %+v", name, plan.Blocked)
		}
		applyPlan(t, plan)
		if !bytes.Equal(readTestFile(t, wrapper), edited) {
			t.Fatalf("%s: edited wrapper overwritten", name)
		}
	}
}

func TestWrapperRendersMatchTheGoldenFiles(t *testing.T) {
	for _, skill := range systemskills.SystemSkills() {
		golden := filepath.Join("testdata", "wrappers", skill.ID+".golden")
		got := currentWrapper(t, skill.ID)
		if os.Getenv("SKILLHUB_UPDATE_GOLDEN") == "1" {
			writeTestFile(t, golden, got, 0o644)
		}
		want, err := os.ReadFile(golden)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("%s render differs from %s; rerun with SKILLHUB_UPDATE_GOLDEN=1 and, once released, add the digest to known-wrappers.go:\n%s", skill.ID, golden, got)
		}
		if !strings.Contains(string(got), "(wrapper v1)") || !strings.Contains(string(got), "${CLAUDE_SKILL_DIR}/../../../.agents/skills/"+skill.ID+"/SKILL.md") {
			t.Fatalf("%s wrapper body:\n%s", skill.ID, got)
		}
		description, err := frontmatterDescription(got)
		if err != nil {
			t.Fatal(err)
		}
		if canonical, _ := frontmatterDescription(skill.Files[0].Content); description != canonical {
			t.Fatalf("%s wrapper description %q, want %q", skill.ID, description, canonical)
		}
	}
}

func TestSymlinkedSkillFolderIsBlockedPerItem(t *testing.T) {
	for _, scope := range []Scope{ScopeUser, ScopeProject} {
		t.Run(string(scope), func(t *testing.T) {
			hub, root, binary := t.TempDir(), t.TempDir(), absoluteBinary(t)
			target := t.TempDir()
			if err := os.Symlink(target, filepath.Join(root, ".agents")); err != nil {
				t.Skipf("symbolic links unavailable: %v", err)
			}
			host, config := HostClaude, filepath.Join(root, ".claude.json")
			if scope == ScopeProject {
				host, config = HostCodex, filepath.Join(root, ".codex", "config.toml")
			}
			plan := planFor(t, Request{Workspace: hub, Root: root, Scope: scope, Binary: binary, Hosts: []Host{host}})
			if len(plan.Blocked) == 0 {
				t.Fatal("no blocked skill file")
			}
			for _, blocked := range plan.Blocked {
				if !strings.Contains(blocked.Blocked, "symbolic link") {
					t.Fatalf("blocked = %+v", blocked)
				}
			}
			applyPlan(t, plan)
			if registration := ReadRegistration(host, readTestFile(t, config)); !registration.Present {
				t.Fatal("MCP entry not written")
			}
			if entries, _ := os.ReadDir(target); len(entries) != 0 {
				t.Fatalf("wrote through the symbolic link: %v", entries)
			}
		})
	}
}

func TestVerifiedSkillsClientDropsOnlyTheCuratorWrapper(t *testing.T) {
	original := bytes.Clone(matrixData)
	t.Cleanup(func() {
		matrixData = original
		parsedMatrix = newParsedMatrix()
	})
	hub, binary := t.TempDir(), absoluteBinary(t)
	request := Request{Workspace: hub, Binary: binary, Hosts: []Host{HostClaude}}
	planAndApply(t, request)
	var doc map[string]any
	if err := json.Unmarshal(original, &doc); err != nil {
		t.Fatal(err)
	}
	for _, value := range doc["stock_clients"].([]any) {
		entry := value.(map[string]any)
		if entry["client"] == "Claude Code" {
			entry["skills_extension"] = map[string]any{"status": "verified", "evidence": "test fixture"}
		}
	}
	matrixData, _ = json.Marshal(doc)
	parsedMatrix = newParsedMatrix()
	wrapper := filepath.Join(hub, ".claude", "skills", "system-curator", "SKILL.md")
	edited := append(currentWrapper(t, systemskills.CuratorSkillID), []byte("\nmine\n")...)
	writeTestFile(t, wrapper, edited, 0o644)
	if plan := planFor(t, request); len(plan.Blocked) != 1 {
		t.Fatalf("edited wrapper of a verified client: %+v", plan.Blocked)
	}
	writeTestFile(t, wrapper, currentWrapper(t, systemskills.CuratorSkillID), 0o644)
	planAndApply(t, request)
	if _, err := os.Stat(wrapper); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("curator wrapper kept for a verified client: %v", err)
	}
	if _, err := os.Stat(filepath.Join(hub, ".claude", "skills", "distill-lab", "SKILL.md")); err != nil {
		t.Fatalf("distill-lab wrapper removed: %v", err)
	}
	if _, err := os.Stat(sharedSkillPath(hub, systemskills.CuratorSkillID, "SKILL.md")); err != nil {
		t.Fatalf("shared curator removed by a local plan: %v", err)
	}
}

func TestSystemSkillFileOwned(t *testing.T) {
	curator := []byte(systemCuratorInstructions())
	distill := systemSkill(t, systemskills.DistillSkillID)
	cases := []struct {
		path  string
		raw   []byte
		owned bool
	}{
		{".agents/skills/system-curator/SKILL.md", curator, true},
		{".agents/skills/system-curator/SKILL.md", []byte("x"), false},
		{".agents/skills/distill-lab/" + distill.Files[1].Path, distill.Files[1].Content, true},
		{".claude/skills/system-curator/SKILL.md", currentWrapper(t, systemskills.CuratorSkillID), true},
		{".claude/skills/system-curator/SKILL.md", curator, true},
		{".claude/skills/distill-lab/SKILL.md", currentWrapper(t, systemskills.CuratorSkillID), false},
		{".gemini/skills/system-curator/SKILL.md", curator, true},
		{".gemini/skills/distill-lab/SKILL.md", distill.Files[0].Content, false},
		{".claude/skills/my-skill/SKILL.md", curator, false},
	}
	for _, test := range cases {
		if got := SystemSkillFileOwned(filepath.FromSlash(test.path), test.raw); got != test.owned {
			t.Errorf("%s: owned = %v, want %v", test.path, got, test.owned)
		}
	}
}

// crlf turns every LF line ending into CRLF, as a Git checkout with
// core.autocrlf=true does on Windows.
func crlf(content []byte) []byte {
	return bytes.ReplaceAll(content, []byte("\n"), []byte("\r\n"))
}

func TestCRLFCheckoutOfSystemSkillFilesIsStillOwned(t *testing.T) {
	t.Run("hub", func(t *testing.T) {
		hub, binary := t.TempDir(), absoluteBinary(t)
		for _, skill := range systemskills.SystemSkills() {
			for _, file := range skill.Files {
				writeTestFile(t, sharedSkillPath(hub, skill.ID, file.Path), crlf(file.Content), 0o644)
			}
			writeTestFile(t, filepath.Join(hub, ".claude", "skills", skill.ID, "SKILL.md"), crlf(currentWrapper(t, skill.ID)), 0o644)
		}
		request := Request{Workspace: hub, Binary: binary}
		if plan := planFor(t, request); len(plan.Blocked) != 0 {
			t.Fatalf("CRLF copies are blocked: %+v", plan.Blocked)
		}
		planAndApply(t, request)
		assertSystemSkillLayout(t, hub, systemskills.CuratorSkillID, systemskills.DistillSkillID)
	})
	t.Run("old layout", func(t *testing.T) {
		hub, project, binary := t.TempDir(), t.TempDir(), absoluteBinary(t)
		writeOldLayout(t, project, crlf([]byte(systemCuratorInstructions())))
		request := Request{Workspace: hub, Root: project, Binary: binary}
		if plan := planFor(t, request); len(plan.Blocked) != 0 {
			t.Fatalf("CRLF copies are blocked: %+v", plan.Blocked)
		}
		planAndApply(t, request)
		assertSystemSkillLayout(t, project, systemskills.CuratorSkillID)
	})
	t.Run("ownership", func(t *testing.T) {
		curator := crlf([]byte(systemCuratorInstructions()))
		for path, raw := range map[string][]byte{
			".agents/skills/system-curator/SKILL.md": curator,
			".claude/skills/system-curator/SKILL.md": crlf(currentWrapper(t, systemskills.CuratorSkillID)),
			".gemini/skills/system-curator/SKILL.md": curator,
		} {
			if !SystemSkillFileOwned(filepath.FromSlash(path), raw) {
				t.Errorf("%s with CRLF line endings is not owned", path)
			}
		}
		edited := crlf([]byte(systemCuratorInstructions() + "mine\n"))
		if SystemSkillFileOwned(filepath.FromSlash(".agents/skills/system-curator/SKILL.md"), edited) {
			t.Error("an edited CRLF copy is owned")
		}
	})
}

// TestKnownWrappersCoverReleasedRenders checks that every wrapper a release
// tag shipped is recognized, so an unedited wrapper written by any released
// binary is upgraded instead of reported as the user's. It needs Git and the
// release tags; without them it skips, unless SKILLHUB_REQUIRE_TAGS=1.
func TestKnownWrappersCoverReleasedRenders(t *testing.T) {
	require := os.Getenv("SKILLHUB_REQUIRE_TAGS") == "1"
	unavailable := func(format string, args ...any) {
		t.Helper()
		if require {
			t.Fatalf(format, args...)
		}
		t.Skipf(format, args...)
	}
	if _, err := exec.LookPath("git"); err != nil {
		unavailable("git is not available: %v", err)
	}
	tagList, err := gitOutput("tag", "-l", "v*")
	if err != nil {
		unavailable("list release tags: %v", err)
	}
	tags := strings.Fields(tagList)
	if len(tags) == 0 {
		unavailable("this checkout has no release tags")
	}
	for _, tag := range tags {
		listing, err := gitOutput("ls-tree", "--full-tree", "-r", "--name-only", tag, "--", "internal/hostintegration/testdata/wrappers/")
		if err != nil {
			t.Fatalf("list wrappers in %s: %v", tag, err)
		}
		for _, file := range strings.Fields(listing) {
			id := strings.TrimSuffix(filepath.Base(file), ".golden")
			content, err := gitOutput("show", tag+":"+file)
			if err != nil {
				t.Fatalf("read %s in %s: %v", file, tag, err)
			}
			if !wrapperOwned(systemSkill(t, id), []byte(content)) {
				t.Errorf("%s shipped the %s wrapper with a digest missing from known-wrappers.go", tag, id)
			}
		}
	}
}

func gitOutput(args ...string) (string, error) {
	command := exec.Command("git", args...)
	command.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0")
	output, err := command.Output()
	return string(output), err
}
