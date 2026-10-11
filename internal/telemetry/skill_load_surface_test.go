package telemetry

import (
	"testing"
	"time"
)

// A whole-skill bundle is a load surface of its own; the surfaces that
// existed before stay accepted, unknown ones stay refused, and the event
// version does not change for an added value.
func TestSkillLoadAcceptsTheBundleSurface(t *testing.T) {
	if EventVersion != "1" {
		t.Fatalf("EventVersion = %q; adding an allowed value must not change it", EventVersion)
	}
	now := time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)
	load := func(surface string) error {
		_, err := validateAndBuild(Event{
			Type: EventSkillLoaded, CatalogSnapshot: "sha256:catalog", PolicyRevision: "sha256:policy",
			Client: Client{Name: "remote:claude-code", Version: "1"},
			Payload: map[string]any{
				"skill_id": "test-audit", "basis": LoadBasisServerObserved, "resource_kind": "entrypoint",
				"surface": surface, "attribution": "recommended", "first_activation": true,
			},
		}, ContentModeNone, now, "evt_surface_"+surface)
		return err
	}
	for _, surface := range []string{"skill_bundle", "skill_get", "skills_get", "resources_read"} {
		if err := load(surface); err != nil {
			t.Errorf("surface %s refused: %v", surface, err)
		}
	}
	if err := load("skill_cache"); err == nil {
		t.Error("an unknown surface was accepted")
	}
}
