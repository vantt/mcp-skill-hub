package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestClassifyInitSource(t *testing.T) {
	t.Parallel()
	repo := t.TempDir()
	if output, err := exec.Command("git", "init", "-q", "-b", "main", repo).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	bare := filepath.Join(t.TempDir(), "hub.git")
	if output, err := exec.Command("git", "init", "-q", "--bare", "-b", "main", bare).CombinedOutput(); err != nil {
		t.Fatalf("git init --bare: %v: %s", err, output)
	}
	plain := t.TempDir()
	insideRepo := filepath.Join(repo, "sub")
	if err := os.MkdirAll(insideRepo, 0o755); err != nil {
		t.Fatal(err)
	}
	accepted := map[string]string{
		"https://github.com/me/hub.git":   "url",
		"ssh://git@github.com/me/hub.git": "url",
		"file:///srv/git/hub.git":         "url",
		"git@github.com:me/hub.git":       "scp",
		"github.com:me/hub.git":           "scp",
		repo:                              "path",
		bare:                              "path",
	}
	for arg, kind := range accepted {
		source, problem := ClassifyInitSource(arg)
		if problem != nil || source.Kind != kind {
			t.Errorf("ClassifyInitSource(%q) = %+v, %v; want kind %s", arg, source, problem, kind)
		}
	}
	refused := []string{
		"", "-oProxyCommand=x", "--upload-pack=x", "ext::sh -c x", "fd::3", "http://example.com/hub.git",
		"git://example.com/hub.git", "https://user:secret@example.com/hub.git", "https://token@example.com/hub.git",
		"ssh://git:secret@example.com/hub.git", "git:secret@example.com:hub.git", `C:\x`, "C:/x",
		plain, insideRepo, filepath.Join(t.TempDir(), "missing"),
	}
	for _, arg := range refused {
		source, problem := ClassifyInitSource(arg)
		if problem == nil {
			t.Errorf("ClassifyInitSource(%q) = %+v, want a refusal", arg, source)
			continue
		}
		if strings.Contains(problem.Render.Error+problem.Render.Why+problem.Render.Fix, "secret") {
			t.Errorf("refusal of %q prints the password: %+v", arg, problem.Render)
		}
	}
}

func TestRedactUserinfoRemovesPasswordsFromGitOutput(t *testing.T) {
	t.Parallel()
	got := redactUserinfo("fatal: unable to access 'https://me:hunter2@example.com/hub.git/': denied\nssh://git:pw@host/x")
	if strings.Contains(got, "hunter2") || strings.Contains(got, ":pw@") || !strings.Contains(got, "https://***@example.com") {
		t.Fatalf("redacted = %q", got)
	}
}

func TestRemoteGitCommandKeepsPromptsOnlyForAPerson(t *testing.T) {
	t.Parallel()
	for _, interactive := range []bool{true, false} {
		command := remoteGitCommandFor(context.Background(), interactive, "/hub", "ls-remote", "--symref", "--", "https://example.com/x")
		for _, want := range []string{"protocol.ext.allow=never", "protocol.fd.allow=never", "core.fsmonitor=false"} {
			if !slices.Contains(command.Args, want) {
				t.Errorf("interactive=%v: args %v lack %s", interactive, command.Args, want)
			}
		}
		disabled := slices.Contains(command.Env, "GIT_TERMINAL_PROMPT=0")
		if disabled == interactive {
			t.Errorf("interactive=%v: GIT_TERMINAL_PROMPT=0 set = %v", interactive, disabled)
		}
		if interactive && command.Stdin == nil {
			t.Error("an interactive run must pass the terminal to Git for credential prompts")
		}
		for _, value := range command.Env {
			if value == "GIT_ASKPASS=" || value == "SSH_ASKPASS=" {
				t.Errorf("interactive=%v: %s disables credential helpers", interactive, value)
			}
		}
	}
}

func TestParseRemoteRefsOrdersCandidateBranches(t *testing.T) {
	t.Parallel()
	refs := parseRemoteRefs("ref: refs/heads/master\tHEAD\n" +
		"1111111111111111111111111111111111111111\tHEAD\n" +
		"2222222222222222222222222222222222222222\trefs/heads/zeta\n" +
		"3333333333333333333333333333333333333333\trefs/heads/main\n" +
		"4444444444444444444444444444444444444444\trefs/heads/alpha\n" +
		"5555555555555555555555555555555555555555\trefs/tags/v1\n")
	// HEAD names master, which does not exist: main comes first, then the rest by name.
	if got, want := refs.candidates(), []string{"main", "alpha", "zeta"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("candidates = %v, want %v", got, want)
	}
	if !refs.any || refs.head != "master" {
		t.Fatalf("refs = %+v", refs)
	}
	if empty := parseRemoteRefs(""); empty.any || len(empty.candidates()) != 0 {
		t.Fatalf("empty remote = %+v", empty)
	}
	if tags := parseRemoteRefs("5555555555555555555555555555555555555555\trefs/tags/v1\n"); !tags.any || len(tags.candidates()) != 0 {
		t.Fatalf("tags only = %+v", tags)
	}
}
