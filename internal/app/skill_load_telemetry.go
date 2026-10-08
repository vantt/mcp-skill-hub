package app

import (
	"context"

	"github.com/vantt/mcp-skill-hub/internal/telemetry"
)

// SkillLoad describes one server-observed load event. It contains only bounded
// tokens and identifiers; paths, URIs, and user tasks are never included.
type SkillLoad struct {
	SkillID         string
	ResourceKind    string
	Surface         string
	Attribution     string
	ResolutionID    string
	SessionIDHash   string
	CatalogSnapshot string
	PolicyRevision  string
	Client          telemetry.Client
	FirstActivation bool
	Blocked         bool
	ReasonCodes     []string
}

// RecordSkillLoad builds and asynchronously records one content-free skill.loaded event.
func RecordSkillLoad(ctx context.Context, sink TelemetrySink, workspacePath string, load SkillLoad) {
	if sink == nil || load.SkillID == "" {
		return
	}
	payload := map[string]any{
		"skill_id":      load.SkillID,
		"resource_kind": load.ResourceKind,
		"surface":       load.Surface,
		"basis":         telemetry.LoadBasisServerObserved,
	}
	if load.Attribution != "" {
		payload["attribution"] = load.Attribution
	}
	if load.Blocked {
		payload["status"] = "review_required"
		if len(load.ReasonCodes) > 0 {
			payload["reason_codes"] = append([]string(nil), load.ReasonCodes...)
		}
		payload["first_activation"] = false
	} else {
		payload["first_activation"] = load.FirstActivation
	}

	event := curationTelemetryEvent(telemetry.EventSkillLoaded, payload)
	event.ResolutionID = load.ResolutionID
	event.SessionIDHash = load.SessionIDHash
	event.CatalogSnapshot = load.CatalogSnapshot
	event.PolicyRevision = load.PolicyRevision
	if load.Client.Name != "" {
		event.Client = load.Client
	}

	recordCurationTelemetry(ctx, sink, workspacePath, event)
}
