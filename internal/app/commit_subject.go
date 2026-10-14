package app

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/vantt/skillhub-mcp/internal/mutation"
)

var commitVerbs = map[string]string{
	"skill_add":                "Add",
	"skill_upstream_update":    "Update",
	"source_watch_confirm":     "Watch",
	"source_onboard":           "Onboard",
	"source_import":            "Import",
	"source_attach":            "Attach",
	"source_detach":            "Detach",
	"source_unwatch":           "Unwatch",
	"source_backfill":          "Backfill",
	"source_candidate_capture": "Capture",
	"source_candidate_triage":  "Triage",
	"source_revision_update":   "Record a new revision of",
}

// commitSubject writes the Git subject of a confirmed write set as
// "<Verb> <id>", taking the id from the paths it changes: a skill folder or a
// source or candidate file.
func commitSubject(set mutation.WriteSet) string {
	verb := commitVerbs[set.Command]
	if verb == "" {
		verb = "Update"
	}
	ids := map[string]bool{}
	for _, change := range set.Changes {
		parts := strings.Split(change.Path, "/")
		switch {
		case len(parts) >= 3 && parts[0] == "skills":
			ids[parts[2]] = true
		case len(parts) == 3 && parts[0] == "sources":
			ids[strings.TrimSuffix(path.Base(change.Path), ".yaml")] = true
		}
	}
	names := make([]string, 0, len(ids))
	for id := range ids {
		names = append(names, id)
	}
	sort.Strings(names)
	switch len(names) {
	case 0:
		return verb + " the hub"
	case 1:
		return verb + " " + names[0]
	case 2, 3:
		return verb + " " + strings.Join(names, ", ")
	default:
		return fmt.Sprintf("%s %d items", verb, len(names))
	}
}
