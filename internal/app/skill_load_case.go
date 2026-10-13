package app

import (
	"context"
	"time"

	"github.com/vantt/skillhub-mcp/internal/telemetry"
)

// LoadResolution is what a delivery adapter remembers about the resolution a
// skill load follows: enough to describe the case when the host reached the
// skill without taking the recommendation.
type LoadResolution struct {
	ResolutionID    string
	Status          string
	CatalogSnapshot string
	Client          telemetry.Client
	PriorVerified   bool
	TaskDescription string
	Operation       string
	Request         map[string]any
	TopKSkillIDs    []string
	TopKMatched     []string
	TopKChannels    []string
}

// RecordLoadCase records the case for the first activation of a skill that the
// resolver did not recommend (an override, or a load after "no skill"). The
// load carries the event ID, attribution kind and session hash.
func RecordLoadCase(ctx context.Context, sink TelemetrySink, load SkillLoad, resolution LoadResolution) error {
	return RecordCase(ctx, sink, telemetry.CaseRecord{
		EventID:         load.EventID,
		ResolutionID:    resolution.ResolutionID,
		SessionHash:     load.SessionIDHash,
		OccurredAt:      time.Now().UTC(),
		Kind:            load.Attribution,
		Client:          resolution.Client,
		CatalogSnapshot: resolution.CatalogSnapshot,
		PriorVerified:   resolution.PriorVerified,
		Task:            map[string]any{"description": resolution.TaskDescription},
		Operation:       resolution.Operation,
		Request:         resolution.Request,
		Resolver: map[string]any{
			"status":         resolution.Status,
			"topk_skill_ids": resolution.TopKSkillIDs,
			"topk_matched":   resolution.TopKMatched,
			"topk_channels":  resolution.TopKChannels,
		},
		Chosen: load.SkillID,
	})
}
