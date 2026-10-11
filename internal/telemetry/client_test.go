package telemetry

import (
	"context"
	"testing"
	"time"
)

func TestRemoteClientLabel(t *testing.T) {
	cases := []struct {
		in   Client
		want Client
	}{
		{Client{Name: "claude-code", Version: "2.1"}, Client{Name: "remote:claude-code", Version: "2.1"}},
		{Client{Name: "other"}, Client{Name: "remote:other"}},
		{Client{}, Client{Name: "remote:other"}},
		{Client{Name: "remote:codex", Version: "1"}, Client{Name: "remote:codex", Version: "1"}},
	}
	for _, tc := range cases {
		if got := RemoteClient(tc.in); got != tc.want {
			t.Errorf("RemoteClient(%+v) = %+v, want %+v", tc.in, got, tc.want)
		}
	}
}

// A remote label must pass event validation and keep per-client rollups
// separate from the local client of the same host.
func TestRemoteClientEventsAreStoredAndRolledUpPerClient(t *testing.T) {
	recorder := newTestRecorder(t, Config{})
	now := time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)
	local := validEvent(EventServerMetric)
	local.ID, local.OccurredAt, local.Client = "evt_local", now, Client{Name: "claude-code", Version: "2.1"}
	local.Payload = map[string]any{"metric_name": "tools_list_bytes", "metric_value": float64(1000)}
	remote := validEvent(EventServerMetric)
	remote.ID, remote.OccurredAt, remote.Client = "evt_remote", now, RemoteClient(Client{Name: "claude-code", Version: "2.1"})
	remote.Payload = map[string]any{"metric_name": "tools_list_bytes", "metric_value": float64(2000)}
	resolution := validEvent(EventResolutionCompleted)
	resolution.ID, resolution.OccurredAt, resolution.Client = "evt_remote_resolution", now, RemoteClient(Client{Name: "codex"})
	for _, event := range []Event{local, remote, resolution} {
		recorder.Record(event)
	}
	mustFlush(t, recorder)
	if health := recorder.Health(); health.Rejected != 0 {
		t.Fatalf("Rejected = %d, want 0: a remote client label must be a valid client name", health.Rejected)
	}

	events, err := recorder.RawEvents(context.Background(), "2026-10-11", "2026-10-11")
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]string{}
	for _, event := range events {
		names[event.ID] = event.Client.Name
	}
	want := map[string]string{"evt_local": "claude-code", "evt_remote": "remote:claude-code", "evt_remote_resolution": "remote:codex"}
	for id, name := range want {
		if names[id] != name {
			t.Errorf("stored client of %s = %q, want %q", id, names[id], name)
		}
	}

	counts := rollupCounts(t, recorder, "", "")
	if got := counts["2026-10-11||tools_list_bytes:claude-code"]; got != 1000 {
		t.Errorf("local tools_list_bytes = %d, want 1000 (all: %v)", got, counts)
	}
	if got := counts["2026-10-11||tools_list_bytes:remote:claude-code"]; got != 2000 {
		t.Errorf("remote tools_list_bytes = %d, want 2000 (all: %v)", got, counts)
	}
}
