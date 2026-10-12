package app

import (
	"sync"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/catalog"
	"github.com/vantt/mcp-skill-hub/internal/telemetry"
)

type collectingSink struct {
	mu     sync.Mutex
	events []telemetry.Event
}

func (sink *collectingSink) Record(event telemetry.Event) {
	sink.mu.Lock()
	defer sink.mu.Unlock()
	sink.events = append(sink.events, event)
}

func (sink *collectingSink) last(t *testing.T) telemetry.Event {
	t.Helper()
	sink.mu.Lock()
	defer sink.mu.Unlock()
	if len(sink.events) == 0 {
		t.Fatal("no event recorded")
	}
	return sink.events[len(sink.events)-1]
}

// Events without a pin carry the snapshot of the current generation, and keep
// doing so after a rebuild publishes a new one, without a stale cached pin.
func TestTelemetryPinsFollowThePublishedGeneration(t *testing.T) {
	t.Parallel()
	root := newResolverWorkspace(t)
	sink := &collectingSink{}

	RecordServerMetric(t.Context(), sink, root, ServerMetric{Name: "tools_list_bytes", Value: 1})
	first := sink.last(t)
	pointer, err := catalog.CurrentPointer(root)
	if err != nil {
		t.Fatal(err)
	}
	if first.CatalogSnapshot != pointer.CatalogSnapshot || first.PolicyRevision == "" {
		t.Fatalf("first event pins = %q / %q, want snapshot %q and a policy revision", first.CatalogSnapshot, first.PolicyRevision, pointer.CatalogSnapshot)
	}

	writeResolveBenchmarkFile(t, root, "skills/pins/pins-skill/skill.meta.yaml", "schema_version: 1\nid: pins-skill\nname: Pins Skill\nstatus: active\ndescription: Check telemetry pins.\nquality:\n  reviewed: true\nrouting:\n  triggers: [check pins]\n  operations: [review]\n  not_for: [write prose]\n  min_scope: multi_step\n")
	writeResolveBenchmarkFile(t, root, "skills/pins/pins-skill/SKILL.md", "---\nname: pins-skill\ndescription: Check telemetry pins.\n---\n\n# Pins\n")
	if _, err := catalog.BuildCatalogGeneration(t.Context(), root, catalog.BuildOptions{}); err != nil {
		t.Fatal(err)
	}
	RecordServerMetric(t.Context(), sink, root, ServerMetric{Name: "tools_list_bytes", Value: 1})
	second := sink.last(t)
	if second.CatalogSnapshot == first.CatalogSnapshot {
		t.Fatalf("event after a rebuild still carries the old snapshot %q", second.CatalogSnapshot)
	}
	if pointer, err = catalog.CurrentPointer(root); err != nil || second.CatalogSnapshot != pointer.CatalogSnapshot {
		t.Fatalf("second event snapshot = %q, want %q (%v)", second.CatalogSnapshot, pointer.CatalogSnapshot, err)
	}
}
