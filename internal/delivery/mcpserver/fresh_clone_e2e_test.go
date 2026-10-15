package mcpserver

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	freshCloneCLISkill    = "fresh-clone-cli"
	freshCloneNotesSkill  = "fresh-clone-notes"
	freshCloneBrokenSkill = "fresh-clone-broken"

	freshCloneCLITrigger    = "audit the fresh clone cli skill"
	freshCloneNotesTrigger  = "summarize the fresh clone notes skill"
	freshCloneBrokenTrigger = "repair the fresh clone broken skill"

	freshCloneBrokenBody = "Body of the skill that cannot be served."

	userGuideDoc    = "docs/user-guide.md"
	userGuideMove   = "## Move to another machine"
	troubleshootDoc = "docs/troubleshooting.md"
	troubleshootFix = "### Fresh clone on a new machine"
	readmeDoc       = "README.md"
	readmeCheck     = "### 3. Check"
	// The README gives no sample output for the check; the closing line of the
	// doctor sample in the remote hub guide is the documented all-clear.
	doctorHealthyMsg = "Healthy."
	doctorSampleDoc  = "docs/remote-hub-guide.md"
)

// sampleHub is the hub of the old machine, committed and pushed to a bare
// repository that stands in for the remote the user owns.
type sampleHub struct {
	root, hub, bare string
	head, refs      string
}

// freshCloneEnv is the machine's environment with the test binary first on
// PATH, so a documented `skillhub ...` line runs the binary under test.
func freshCloneEnv(m machine, binary string) []string {
	return append(m.env(), "PATH="+filepath.Dir(binary)+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// runFreshClone runs one command with the machine's environment and fails with
// the command and its output when it exits non-zero.
func runFreshClone(t *testing.T, env []string, dir, what string, name string, args ...string) string {
	t.Helper()
	command := exec.Command(name, args...)
	command.Dir = dir
	command.Env = env
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("%s: %s %s: %v\n%s", what, name, strings.Join(args, " "), err, output)
	}
	return string(output)
}

// runDocLine runs one command line of a doc the way the user pastes it, in the
// folder the doc assumes (the user's home unless the line starts with cd).
func runDocLine(t *testing.T, env []string, dir, doc, section, line string) string {
	t.Helper()
	command := exec.Command("sh", "-c", line)
	command.Dir = dir
	command.Env = env
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %q line %q: %v\n%s", doc, section, line, err, output)
	}
	return string(output)
}

func writeFreshCloneSkill(t *testing.T, hub, id, frontmatter, body string, metadata string) {
	t.Helper()
	directory := filepath.Join(hub, "skills", "default", id)
	writeFixtureFile(t, filepath.Join(directory, ".meta", "skill.yaml"), []byte(metadata))
	writeFixtureFile(t, filepath.Join(directory, "SKILL.md"), []byte("---\n"+frontmatter+"---\n\n"+body))
}

func freshCloneMetadata(id, trigger string) string {
	return "schema_version: 1\nid: " + id + "\nstatus: active\nquality:\n  reviewed: true\nrouting:\n  triggers: [" + trigger +
		"]\n  not_for: [write marketing prose]\n  min_scope: multi_step\n"
}

// commitSampleHub builds the old machine's hub with two skills that can be
// served and one that cannot, commits everything and pushes it to a bare repository.
func commitSampleHub(t *testing.T, binary string, old machine) sampleHub {
	t.Helper()
	env := freshCloneEnv(old, binary)
	hub := filepath.Join(old.home(), "skill-hub")
	if err := os.MkdirAll(hub, 0o755); err != nil {
		t.Fatal(err)
	}
	runFreshClone(t, env, hub, "old machine: create the hub", binary, "init")
	// The first skill is written in a project and committed to the hub.
	authoring := filepath.Join(old.home(), "authoring")
	if err := os.MkdirAll(authoring, 0o755); err != nil {
		t.Fatal(err)
	}
	runFreshClone(t, env, authoring, "old machine: create the first skill in a project", binary, "skill", "create", freshCloneCLISkill,
		"--description", "Made with the skill commands.", "--trigger", freshCloneCLITrigger, "--not-for", "write marketing prose")
	writeFixtureFile(t, filepath.Join(authoring, ".claude", "skills", freshCloneCLISkill, "SKILL.md"),
		[]byte("---\nname: "+freshCloneCLISkill+"\ndescription: Made with the skill commands. Use when "+freshCloneCLITrigger+". Do not use for write marketing prose.\n---\n\n# Fresh clone CLI\n\nSteps made with the CLI.\n"))
	runFreshClone(t, env, authoring, "old machine: commit the first skill to the hub", binary, "skill", "commit", freshCloneCLISkill,
		"--workspace", hub, "--operation", "review", "--min-scope", "multi_step", "--yes")
	runFreshClone(t, env, hub, "old machine: activate the first skill", binary, "skill", "activate", freshCloneCLISkill,
		"--idempotency-key", "fresh-clone-activate", "--yes")

	notes := filepath.Join(hub, "skills", "default", freshCloneNotesSkill)
	writeFreshCloneSkill(t, hub, freshCloneNotesSkill, "name: "+freshCloneNotesSkill+"\ndescription: Notes kept in a reference file.\n",
		"# Fresh clone notes\n\nRead references/notes.md.\n", freshCloneMetadata(freshCloneNotesSkill, freshCloneNotesTrigger))
	writeFixtureFile(t, filepath.Join(notes, "references", "notes.md"), []byte("# Notes\n\nA reference file that must arrive on the new machine.\n"))
	// One curation file the hub keeps next to the skill; init must accept it silently.
	writeFixtureFile(t, filepath.Join(notes, ".meta", "distill.yaml"), []byte("goal:\n  purpose: Keep the notes skill current.\n  status: draft\n"+
		"  in_scope:\n    - the notes it tells\n  out_of_scope:\n    - other skills\n  failures_it_prevents:\n    - stale notes\n"))

	// No frontmatter name: the hub accepts the folder, but it cannot be served.
	writeFreshCloneSkill(t, hub, freshCloneBrokenSkill, "description: A skill without a name.\n", "# Fresh clone broken\n\n"+freshCloneBrokenBody+"\n",
		freshCloneMetadata(freshCloneBrokenSkill, freshCloneBrokenTrigger))

	runFreshClone(t, env, hub, "old machine: stage the hub", "git", "add", "-A")
	runFreshClone(t, env, hub, "old machine: commit the hub", "git", "commit", "-q", "-m", "add sample skills")
	if status := runFreshClone(t, env, hub, "old machine: git status", "git", "status", "--porcelain"); status != "" {
		t.Fatalf("the old hub keeps files Git does not hold after the commit, so they would not reach another machine:\n%s", status)
	}
	bare := filepath.Join(old.root, "hub.git")
	runFreshClone(t, env, old.root, "old machine: publish the hub", "git", "clone", "--bare", "--quiet", hub, bare)
	return sampleHub{
		root: old.root, hub: hub, bare: bare,
		head: strings.TrimSpace(runFreshClone(t, env, hub, "old machine: HEAD", "git", "rev-parse", "HEAD")),
		refs: runFreshClone(t, env, bare, "old machine: remote refs", "git", "for-each-ref"),
	}
}

// TestFreshCloneOnANewMachineServesTheHubsSkills runs the lines the docs give
// for moving a hub to a new machine, in order and as shell lines, and then asks
// the server the agent host would start for the skills of the old hub.
func TestFreshCloneOnANewMachineServesTheHubsSkills(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the documented commands are POSIX shell lines")
	}
	binary := buildSkillHub(t)
	old := newMachine(t, "old")
	sample := commitSampleHub(t, binary, old)

	t.Run("init from the Git URL", func(t *testing.T) {
		m := newMachine(t, "new")
		env := freshCloneEnv(m, binary)
		project := newFreshCloneProject(t, m)
		move := docSectionCommands(t, userGuideDoc, userGuideMove)
		if len(move) != 2 {
			t.Fatalf("%s %q has %d command lines, want the fetch and the connect: %q", userGuideDoc, userGuideMove, len(move), move)
		}
		values := map[string]string{"git-url": sample.bare, "your-project": project}
		initOutput := runDocLine(t, env, m.home(), userGuideDoc, userGuideMove, fillDocPlaceholders(t, move[0], values))
		assertInitQuiet(t, userGuideDoc, move[0], initOutput)
		runDocLine(t, env, m.home(), userGuideDoc, userGuideMove, fillDocPlaceholders(t, move[1], values))
		assertNewMachineServesSampleHub(t, binary, m, sample, project)
	})

	t.Run("git clone then init", func(t *testing.T) {
		m := newMachine(t, "new")
		env := freshCloneEnv(m, binary)
		project := newFreshCloneProject(t, m)
		move := docSectionCommands(t, userGuideDoc, userGuideMove)
		fix := docSectionCommands(t, troubleshootDoc, troubleshootFix)
		if len(move) != 2 || len(fix) != 1 {
			t.Fatalf("%s has %d lines and %s has %d lines, want 2 and 1", userGuideDoc, len(move), troubleshootDoc, len(fix))
		}
		// The user's own step: the troubleshooting section starts from a clone made with Git.
		runDocLine(t, env, m.home(), "the clone the troubleshooting section presupposes", troubleshootFix, "git clone --quiet "+shellQuote(sample.bare)+" ~/skill-hub")
		initOutput := runDocLine(t, env, m.home(), troubleshootDoc, troubleshootFix, fix[0])
		assertInitQuiet(t, troubleshootDoc, fix[0], initOutput)
		runDocLine(t, env, m.home(), userGuideDoc, userGuideMove, fillDocPlaceholders(t, move[1], map[string]string{"your-project": project}))
		assertNewMachineServesSampleHub(t, binary, m, sample, project)
	})

	// What moved is only what Git holds: nothing on the old machine changed.
	env := freshCloneEnv(old, binary)
	if head := strings.TrimSpace(runFreshClone(t, env, sample.hub, "old machine: HEAD", "git", "rev-parse", "HEAD")); head != sample.head {
		t.Errorf("the old hub moved from %s to %s", sample.head, head)
	}
	if status := runFreshClone(t, env, sample.hub, "old machine: git status", "git", "status", "--porcelain"); status != "" {
		t.Errorf("the old hub changed:\n%s", status)
	}
	if refs := runFreshClone(t, env, sample.bare, "old machine: remote refs", "git", "for-each-ref"); refs != sample.refs {
		t.Errorf("the remote changed:\nbefore %s\nafter  %s", sample.refs, refs)
	}
}

func newFreshCloneProject(t *testing.T, m machine) string {
	t.Helper()
	project := filepath.Join(m.home(), "project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	return project
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

// assertInitQuiet checks that init from a hub with a curation file in it names no error.
func assertInitQuiet(t *testing.T, doc, line, output string) {
	t.Helper()
	if strings.Contains(strings.ToLower(output), "error") || strings.Contains(output, "distill") {
		t.Fatalf("%s line %q names an error for the hub's files:\n%s", doc, line, output)
	}
}

// assertNewMachineServesSampleHub runs the README check from the connected
// project, starts the server the project's connection file names and asks it for
// the skills of the old hub.
func assertNewMachineServesSampleHub(t *testing.T, binary string, m machine, sample sampleHub, project string) {
	t.Helper()
	env := freshCloneEnv(m, binary)
	newHub, err := filepath.EvalSymlinks(filepath.Join(m.home(), "skill-hub"))
	if err != nil {
		t.Fatal(err)
	}

	check := docSectionCommands(t, readmeDoc, readmeCheck)
	if len(check) != 1 {
		t.Fatalf("%s %q has %d command lines, want 1: %q", readmeDoc, readmeCheck, len(check), check)
	}
	doctor := runDocLine(t, env, project, readmeDoc, readmeCheck, check[0])
	if !strings.Contains(doctor, doctorHealthyMsg) {
		t.Fatalf("%s %q line %q does not report %q, the all-clear line of the doctor sample in %s (change that doc too if the wording changed on purpose):\n%s",
			readmeDoc, readmeCheck, check[0], doctorHealthyMsg, doctorSampleDoc, doctor)
	}

	var config struct {
		Servers map[string]struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"mcpServers"`
	}
	raw, err := os.ReadFile(filepath.Join(project, ".mcp.json"))
	if err != nil || json.Unmarshal(raw, &config) != nil {
		t.Fatalf("read the connection file connect wrote: %v\n%s", err, raw)
	}
	entry, ok := config.Servers["skillhub"]
	if !ok {
		t.Fatalf("the connection file has no skillhub entry:\n%s", raw)
	}
	wantBinary, err := filepath.EvalSymlinks(binary)
	if err != nil {
		t.Fatal(err)
	}
	gotBinary, err := filepath.EvalSymlinks(entry.Command)
	gotHub := ""
	if len(entry.Args) > 0 {
		gotHub, _ = filepath.EvalSymlinks(entry.Args[len(entry.Args)-1])
	}
	if err != nil || gotBinary != wantBinary || len(entry.Args) != 6 || !slices.Equal(entry.Args[:5], []string{"mcp", "serve", "--profile", "runtime", "--workspace"}) || gotHub != newHub {
		t.Fatalf("connection file entry = %+v, want %s mcp serve --profile runtime --workspace %s", entry, wantBinary, newHub)
	}

	session, stderr := startFreshCloneServer(t, entry.Command, entry.Args, project, env)
	assertServesSampleHub(t, session, stderr, newHub, sample)
}

func startFreshCloneServer(t *testing.T, command string, args []string, dir string, env []string) (*mcp.ClientSession, *lockedBuffer) {
	t.Helper()
	stderr := &lockedBuffer{}
	process := exec.Command(command, args...)
	process.Dir, process.Env, process.Stderr = dir, env, stderr
	client := mcp.NewClient(&mcp.Implementation{Name: "claude-code", Version: "1"}, nil)
	if err := mcp.AddSendingCustomMethod[*listSkillsParams, *listSkillsResult](client, "skills/list"); err != nil {
		t.Fatal(err)
	}
	session, err := client.Connect(t.Context(), &mcp.CommandTransport{Command: process, TerminateDuration: 15 * time.Second}, nil)
	if err != nil {
		t.Fatalf("start the server the connection file names: %v; stderr=%s", err, stderr.String())
	}
	t.Cleanup(func() { _ = session.Close() })
	return session, stderr
}

func assertServesSampleHub(t *testing.T, session *mcp.ClientSession, stderr *lockedBuffer, newHub string, sample sampleHub) {
	t.Helper()
	fail := func(format string, args ...any) {
		t.Helper()
		t.Fatalf(format+"\nserver stderr: %s", append(args, stderr.String())...)
	}
	for _, skill := range []struct{ id, trigger string }{{freshCloneCLISkill, freshCloneCLITrigger}, {freshCloneNotesSkill, freshCloneNotesTrigger}} {
		resolved, encoded := resolveFor(t, session, skill.trigger, "REQ-fresh-clone-"+skill.id)
		if resolved.Resolution.Primary == nil || resolved.Resolution.Primary.ID != skill.id {
			fail("skill_resolve %q chose %s, want %s", skill.trigger, encoded, skill.id)
		}
		response, outcome := skillGetOutcome(t, session, skill.id)
		if response.IsError || outcome.Result == nil || outcome.Result.Local == nil {
			fail("skill_get %s = %#v", skill.id, outcome)
		}
		local := outcome.Result.Local
		// The server names the folder under the workspace path it was given,
		// which may pass through a symlink (macOS /var is /private/var), so
		// compare resolved paths as newHub is.
		gotPath, err := filepath.EvalSymlinks(local.Path)
		if err != nil {
			fail("skill_get %s: the folder %q it names cannot be resolved: %v", skill.id, local.Path, err)
		}
		oldRoot, err := filepath.EvalSymlinks(sample.root)
		if err != nil {
			t.Fatal(err)
		}
		runtimeDir := filepath.Join(newHub, "runtime") + string(os.PathSeparator)
		if local.Status != "ready" || !strings.HasPrefix(gotPath, runtimeDir) || strings.HasPrefix(gotPath, oldRoot+string(os.PathSeparator)) {
			fail("skill_get %s: status %q at %q, want ready inside %s and outside the old machine %s", skill.id, local.Status, local.Path, runtimeDir, sample.root)
		}
		files := []string{"SKILL.md"}
		if skill.id == freshCloneNotesSkill {
			files = append(files, filepath.Join("references", "notes.md"))
		}
		for _, file := range files {
			want, err := os.ReadFile(filepath.Join(sample.hub, "skills", "default", skill.id, file))
			if err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(filepath.Join(local.Path, file))
			if err != nil || !bytes.Equal(got, want) {
				fail("%s of %s on the new machine differs from the old hub's: %v\nwant %q\ngot  %q", file, skill.id, err, want, got)
			}
		}
	}

	resolved, encoded := resolveFor(t, session, freshCloneBrokenTrigger, "REQ-fresh-clone-broken")
	if resolved.Resolution.Primary != nil && resolved.Resolution.Primary.ID == freshCloneBrokenSkill {
		fail("skill_resolve recommends the skill that cannot be served: %s", encoded)
	}
	for _, supporting := range resolved.Resolution.Supporting {
		if supporting.ID == freshCloneBrokenSkill {
			fail("skill_resolve lists the skill that cannot be served as supporting: %s", encoded)
		}
	}
	response, outcome := skillGetOutcome(t, session, freshCloneBrokenSkill)
	requireRefused(t, response, outcome, "not_servable", freshCloneBrokenBody)

	var listed []string
	cursor := ""
	for {
		page, err := mcp.CallCustomMethod[*listSkillsParams, *listSkillsResult](t.Context(), session, "skills/list", &listSkillsParams{Cursor: cursor})
		if err != nil {
			fail("skills/list: %v", err)
		}
		for _, entry := range page.Skills {
			listed = append(listed, entry.URI)
		}
		if cursor = page.NextCursor; cursor == "" {
			break
		}
	}
	has := func(id string) bool {
		return slices.ContainsFunc(listed, func(uri string) bool { return strings.Contains(uri, "/"+id+"/") })
	}
	if !has(freshCloneCLISkill) || !has(freshCloneNotesSkill) || has(freshCloneBrokenSkill) {
		fail("skills/list = %v, want %s and %s listed and %s absent", listed, freshCloneCLISkill, freshCloneNotesSkill, freshCloneBrokenSkill)
	}
}
