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
	TopKSkillIDs    []string
	TopKMatched     []string
	TopKChannels    []string
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
	if len(load.TopKSkillIDs) > 0 {
		payload["topk_skill_ids"] = append([]string(nil), load.TopKSkillIDs...)
		payload["topk_matched"] = append([]string(nil), load.TopKMatched...)
		payload["topk_channels"] = append([]string(nil), load.TopKChannels...)
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

type ServerMetric struct {
	Name    string
	Value   float64
	SkillID string
	Client  telemetry.Client
}

func RecordServerMetric(ctx context.Context, sink TelemetrySink, workspacePath string, metric ServerMetric) {
	if sink == nil {
		return
	}
	payload := map[string]any{
		"metric_name":  metric.Name,
		"metric_value": metric.Value,
	}
	if metric.SkillID != "" {
		payload["skill_id"] = metric.SkillID
	}
	event := curationTelemetryEvent(telemetry.EventServerMetric, payload)
	if metric.Client.Name != "" {
		event.Client = metric.Client
	} else {
		event.Client = telemetry.Client{Name: "other"}
	}
	recordCurationTelemetry(ctx, sink, workspacePath, event)
}
