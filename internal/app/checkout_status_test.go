package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func openCheckoutOf(t *testing.T, home CurationHome, id string) (ActionItem, bool) {
	t.Helper()
	for _, action := range home.Actions {
		if action.Kind == "open_checkout" && action.ID == id {
			return action, true
		}
	}
	return ActionItem{}, false
}

func TestStatusListsOpenCheckoutsOfThisHubWithChangedFileCounts(t *testing.T) {
	t.Parallel()
	hub, project, folder := checkedOut(t, fixtureSkill{id: "review-checklist", files: map[string]string{"notes.md": "n\n"}})
	other := newGitHub(t)
	seedHubSkill(t, other, fixtureSkill{id: "other-hub-skill"})
	commitHubAll(t, other, "add")
	if result, err := (SkillCheckoutService{}).Checkout(context.Background(), other, SkillCheckoutInput{SkillID: "other-hub-skill", ProjectDir: project}); err != nil || result.Error != nil {
		t.Fatalf("checkout from another hub: %v %#v", err, result.Error)
	}
	replaceInProject(t, folder, "line one", "line 1")
	writeProjectFile(t, folder, "scripts/new.sh", "#!/bin/sh\n", 0o755)

	home, err := (CurationService{ProjectDir: project}).GetCurationHome(context.Background(), hub)
	if err != nil {
		t.Fatal(err)
	}
	action, found := openCheckoutOf(t, home, "review-checklist")
	if !found || action.Count != 2 || action.Summary != "review-checklist: 2 files changed since checkout" || action.Command != "skillhub skill commit review-checklist" {
		t.Fatalf("action = %+v found=%v", action, found)
	}
	if _, found := openCheckoutOf(t, home, "other-hub-skill"); found {
		t.Fatalf("a checkout of another hub must not be listed")
	}
	replaceInHub(t, hub, "review-checklist", "line three", "hub three")
	home, err = (CurationService{ProjectDir: project}).GetCurationHome(context.Background(), hub)
	if err != nil {
		t.Fatal(err)
	}
	if action, _ := openCheckoutOf(t, home, "review-checklist"); !strings.HasSuffix(action.Summary, "; the hub changed too") || action.Count != 2 {
		t.Fatalf("after a hub change = %+v", action)
	}
	plain, err := (CurationService{}).GetCurationHome(context.Background(), hub)
	if err != nil {
		t.Fatal(err)
	}
	if _, found := openCheckoutOf(t, plain, "review-checklist"); found {
		t.Fatalf("without a project directory no checkout is listed")
	}
}

func TestDirectEditOfAHubFileIsANoticeNotAFailure(t *testing.T) {
	t.Parallel()
	hub := newGitHub(t)
	dir := seedHubSkill(t, hub, fixtureSkill{id: "alpha"})
	commitHubAll(t, hub, "add alpha")
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(strings.Replace(hubSkillMD(t, hub, "alpha"), "line one", "edited by hand", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	sections := Run(context.Background(), RoleReport{Cwd: hub, Hub: &HubRef{Root: hub}}, DepthLight)
	finding := findingByCode(t, sections, "hub_index_stale")
	if finding.Severity != SeverityNotice || finding.Message != "Search index is behind file edits; it rebuilds on the next read." {
		t.Fatalf("finding = %+v", finding)
	}
	if problems := codesOf(sections, SeverityProblem); len(problems) != 0 {
		t.Fatalf("a direct edit must not be a problem, got %v", problems)
	}
	// A generation that is missing is still a fault.
	if err := os.RemoveAll(filepath.Join(hub, "runtime", "catalog")); err != nil {
		t.Fatal(err)
	}
	missing := Run(context.Background(), RoleReport{Cwd: hub, Hub: &HubRef{Root: hub}}, DepthLight)
	if f := findingByCode(t, missing, "hub_index_stale"); f.Severity != SeverityProblem {
		t.Fatalf("a missing generation = %+v", f)
	}
}
