package app

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"sort"
)

// openCheckoutPriority sorts open checkouts below the work that blocks others
// and above reminders about uncommitted Git changes.
const openCheckoutPriority = 30

// openCheckoutActions lists the checkouts of this hub that the project around
// projectDir still holds, each with how many files changed since the checkout
// and whether the hub changed meanwhile. A forgotten checkout is a second copy
// that drifts, so status names it.
func openCheckoutActions(ctx context.Context, root, projectDir string) []ActionItem {
	projectRoot := projectRootOf(ctx, projectDir)
	markers, err := filepath.Glob(filepath.Join(projectRoot, filepath.FromSlash(projectSkillsDir), "*", CheckoutMarkerName))
	if err != nil {
		return nil
	}
	sort.Strings(markers)
	var actions []ActionItem
	for _, markerPath := range markers {
		folder := filepath.Dir(markerPath)
		marker, found, err := ReadCheckoutMarker(folder)
		if err != nil || !found || !sameHub(marker.Hub, root) {
			continue
		}
		if action, ok := describeOpenCheckout(ctx, root, folder, marker); ok {
			actions = append(actions, action)
		}
	}
	return actions
}

func sameHub(recorded, root string) bool {
	if recorded == root {
		return true
	}
	a, errA := filepath.EvalSymlinks(recorded)
	b, errB := filepath.EvalSymlinks(root)
	return errA == nil && errB == nil && a == b
}

func describeOpenCheckout(ctx context.Context, root, folder string, marker CheckoutMarker) (ActionItem, bool) {
	tree, _, refusal, err := readProjectFolder(folder)
	if err != nil || refusal != nil {
		return ActionItem{}, false
	}
	action := ActionItem{Kind: "open_checkout", ID: marker.SkillID, Priority: openCheckoutPriority, Command: "skillhub skill commit " + marker.SkillID}
	hub, err := loadHubSkill(root, marker.SkillID)
	if err != nil {
		return ActionItem{}, false
	}
	if hub == nil {
		action.Count = len(tree)
		action.Summary = fmt.Sprintf("%s: the hub no longer has this skill; committing adds it as a new skill", marker.SkillID)
		return action, true
	}
	_ = normalizeProjectSkillMD(tree, marker.SkillID, hub)
	base, hubChanged, baseKnown := treeBytes(hub.tree), hub.version != marker.BaseVersion, true
	if hubChanged {
		base, baseKnown = findCheckoutBase(ctx, root, hub.relDir, marker.SkillID, marker)
	}
	action.Count = countTreeDifferences(base, tree)
	action.Summary = checkoutProgress(marker.SkillID, action.Count, hubChanged, baseKnown)
	return action, true
}

// countTreeDifferences counts the files added, changed or removed from base to tree.
func countTreeDifferences(base map[string][]byte, tree map[string]treeFile) int {
	count := 0
	for rel, file := range tree {
		if before, ok := base[rel]; !ok || !bytes.Equal(before, file.Data) {
			count++
		}
	}
	for rel := range base {
		if _, ok := tree[rel]; !ok {
			count++
		}
	}
	return count
}

func checkoutProgress(id string, changed int, hubChanged, baseKnown bool) string {
	summary := fmt.Sprintf("%s: no changes since checkout", id)
	if changed > 0 {
		summary = fmt.Sprintf("%s: %s changed since checkout", id, pluralize(changed, "file"))
	}
	switch {
	case hubChanged && !baseKnown:
		summary += "; the hub changed too and the starting point is gone, so a commit will ask about every differing file"
	case hubChanged:
		summary += "; the hub changed too"
	}
	return summary
}
