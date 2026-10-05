package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/catalog"
	resolverpkg "github.com/vantt/mcp-skill-hub/internal/resolver"
	"github.com/vantt/mcp-skill-hub/internal/telemetry"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
)

// ResolverService is the transport-neutral application contract used by CLI and MCP adapters.
// It only recommends; it never loads or activates skill content.
type ResolverService struct {
	Policy    resolverpkg.Policy
	Cache     *resolverpkg.Cache
	Telemetry TelemetrySink

	// probe performs the non-executing setup checks for the post-ranking
	// setup annotation; the zero value uses the host.
	probe runtimeProbe
}

// TelemetrySink accepts already-minimized events. Implementations must return
// quickly; telemetry.Recorders satisfy this contract with bounded async writes.
type TelemetrySink interface {
	Record(telemetry.Event)
}

var sharedResolverCache = resolverpkg.NewCache(1024)

func (service ResolverService) Resolve(ctx context.Context, path string, request resolverpkg.Request) (response resolverpkg.Response, resultErr error) {
	startedAt := time.Now()
	stage := "workspace_discovery"
	catalogSnapshot, policyRevision := "", ""
	telemetryStarted := false
	defer func() {
		if !telemetryStarted {
			return
		}
		payload := resolutionTelemetryPayload(request, response, time.Since(startedAt))
		eventType := telemetry.EventResolutionCompleted
		if resultErr != nil {
			eventType = telemetry.EventResolutionFailed
			payload["status"] = "failed"
			payload["error_code"] = stage + "_failed"
		} else if response.Primary != nil {
			recommended := telemetryEvent(telemetry.EventResolutionRecommended, request, response, catalogSnapshot, policyRevision, payload)
			safeRecordTelemetry(service.Telemetry, recommended)
		}
		safeRecordTelemetry(service.Telemetry, telemetryEvent(eventType, request, response, catalogSnapshot, policyRevision, payload))
	}()

	root, err := workspace.Discover(path)
	if err != nil {
		return response, err
	}
	stage = "catalog_open"
	var handle *catalog.Handle
	if request.Prior != nil {
		handle, err = catalog.OpenCurrentLocked(ctx, root)
	} else {
		handle, err = catalog.OpenWithFallbackLocked(ctx, root)
	}
	if err != nil {
		return response, err
	}
	catalogSnapshot = handle.Pointer.CatalogSnapshot
	defer func() {
		if closeErr := handle.Close(); resultErr == nil && closeErr != nil {
			resultErr = fmt.Errorf("close catalog generation: %w", closeErr)
		}
	}()
	stage = "policy_load"
	policy := service.Policy
	if policy.Revision == "" {
		policy, err = resolverpkg.LoadPolicy(ctx, handle.DB)
		if err != nil {
			return response, fmt.Errorf("load recommendation policy: %w", err)
		}
	}
	policyRevision = policy.Revision
	telemetryStarted = true
	safeRecordTelemetry(service.Telemetry, telemetryEvent(telemetry.EventResolutionStarted, request, response, catalogSnapshot, policyRevision, resolutionStartPayload(request)))

	stage = "resolution"
	response, resultErr = service.resolveWithin(ctx, root, handle, request, nil)
	return response, resultErr
}

func (service ResolverService) resolveWithin(ctx context.Context, root string, handle *catalog.Handle, request resolverpkg.Request, decorate func(resolverpkg.Catalog) resolverpkg.Catalog) (resolverpkg.Response, error) {
	catalogSnapshot := handle.Pointer.CatalogSnapshot
	sqliteCatalog, err := resolverpkg.NewSQLiteCatalog(handle.DB, catalogSnapshot)
	if err != nil {
		return resolverpkg.Response{}, err
	}
	var view resolverpkg.Catalog = sqliteCatalog
	if decorate != nil {
		view = decorate(view)
	}
	policy := service.Policy
	if policy.Revision == "" {
		policy, err = resolverpkg.LoadPolicy(ctx, handle.DB)
		if err != nil {
			return resolverpkg.Response{}, fmt.Errorf("load recommendation policy: %w", err)
		}
	}
	baseCache := service.Cache
	if baseCache == nil {
		baseCache = sharedResolverCache
	}
	excluded := map[string]struct{}{}
	for {
		cache := baseCache
		if len(excluded) > 0 {
			cache = resolverpkg.NewCache(1)
		}
		engine, err := resolverpkg.New(excludingCatalog{Catalog: view, excluded: excluded}, policy, cache)
		if err != nil {
			return resolverpkg.Response{}, err
		}
		response, err := engine.Resolve(ctx, request)
		if response.Supporting == nil {
			response.Supporting = []resolverpkg.Supporting{}
		}
		if handle.Status.ServingMode == catalog.ServingFallback && handle.Status.Warning != "" {
			response.Warnings = append(response.Warnings, handle.Status.Warning)
		}
		if err != nil || response.Primary == nil {
			return response, err
		}
		primary, err := buildDistributedSkill(ctx, root, handle, response.Primary.ID)
		if errors.Is(err, catalog.ErrSkillNotServable) || errors.Is(err, ErrResourceContentUnavailable) {
			excluded[response.Primary.ID] = struct{}{}
			continue
		}
		if err != nil {
			return response, fmt.Errorf("build resolved skill manifest: %w", err)
		}
		response.Primary.URI = primary.URI
		response.Primary.Version = primary.Version
		response.Primary.Setup = service.setupStatus(ctx, root, handle, primary)
		served := response.Supporting[:0]
		for _, supporting := range response.Supporting {
			entry, entryErr := buildDistributedSkill(ctx, root, handle, supporting.ID)
			if errors.Is(entryErr, catalog.ErrSkillNotServable) || errors.Is(entryErr, ErrResourceContentUnavailable) {
				continue
			}
			if entryErr != nil {
				return response, fmt.Errorf("build supporting skill manifest: %w", entryErr)
			}
			supporting.URI = entry.URI
			supporting.Version = entry.Version
			supporting.Setup = service.setupStatus(ctx, root, handle, entry)
			served = append(served, supporting)
		}
		response.Supporting = served
		return response, nil
	}
}

// setupStatus annotates a recommended skill after ranking. It is nil for
// trusted skills without a runtime block. The annotation is advisory: a manifest that cannot
// be read here yields no annotation rather than failing or reordering the
// resolution, and activation reports the full preflight anyway.
func (service ResolverService) setupStatus(ctx context.Context, root string, handle *catalog.Handle, entry DistributedSkill) *resolverpkg.SetupStatus {
	var contentJSON string
	if err := handle.DB.QueryRowContext(ctx, `SELECT content_json FROM canonical_entities WHERE id=?`, entry.SkillID).Scan(&contentJSON); err != nil {
		return nil
	}
	status, err := setupAnnotation(root, entry, []byte(contentJSON), service.probe)
	if err != nil {
		return nil
	}
	return status
}

// excludingCatalog hides skills that were found unservable during this request.
type excludingCatalog struct {
	resolverpkg.Catalog
	excluded map[string]struct{}
}

func (view excludingCatalog) Skills(ctx context.Context) ([]resolverpkg.Skill, error) {
	skills, err := view.Catalog.Skills(ctx)
	if err != nil || len(view.excluded) == 0 {
		return skills, err
	}
	kept := make([]resolverpkg.Skill, 0, len(skills))
	for _, item := range skills {
		if _, hidden := view.excluded[item.ID]; !hidden {
			kept = append(kept, item)
		}
	}
	return kept, nil
}

func resolutionStartPayload(request resolverpkg.Request) map[string]any {
	payload := map[string]any{
		"status":           "started",
		"constraint_count": len(request.Task.Constraints),
		"fact_keys":        telemetryFactKeys(request.Context.Facts),
	}
	addRequestTelemetry(payload, request)
	return payload
}

func resolutionTelemetryPayload(request resolverpkg.Request, response resolverpkg.Response, duration time.Duration) map[string]any {
	status := string(response.Status)
	if status == "" {
		status = "unknown"
	}
	recommendedSkillIDs := make([]string, 0, len(response.Supporting)+1)
	if response.Primary != nil {
		recommendedSkillIDs = append(recommendedSkillIDs, response.Primary.ID)
	}
	for _, supporting := range response.Supporting {
		recommendedSkillIDs = append(recommendedSkillIDs, supporting.ID)
	}
	payload := map[string]any{
		"status":                status,
		"constraint_count":      len(request.Task.Constraints),
		"fact_keys":             telemetryFactKeys(request.Context.Facts),
		"candidate_count":       len(response.Supporting),
		"recommended_skill_ids": recommendedSkillIDs,
		"reason_codes":          append([]string(nil), response.ReasonCodes...),
		"duration_ms":           duration.Milliseconds(),
	}
	if response.Primary != nil {
		payload["candidate_count"] = len(response.Supporting) + 1
		payload["top_skill_id"] = response.Primary.ID
		payload["skill_id"] = response.Primary.ID
		payload["confidence_band"] = response.Primary.Confidence
		if response.Primary.Setup != nil {
			payload["setup_state"] = response.Primary.Setup.State
		}
	}
	addRequestTelemetry(payload, request)
	return payload
}

func addRequestTelemetry(payload map[string]any, request resolverpkg.Request) {
	if safeTelemetryToken(request.Operation) {
		payload["operation"] = request.Operation
	}
	if request.Context.ActiveArtifact != nil && safeTelemetryToken(request.Context.ActiveArtifact.Kind) {
		payload["artifact_kind"] = request.Context.ActiveArtifact.Kind
	}
}

func telemetryFactKeys(facts []resolverpkg.Fact) []string {
	keys := make([]string, 0, len(facts))
	seen := make(map[string]struct{}, len(facts))
	for _, fact := range facts {
		if !safeTelemetryToken(fact.Key) {
			continue
		}
		if _, exists := seen[fact.Key]; exists {
			continue
		}
		seen[fact.Key] = struct{}{}
		keys = append(keys, fact.Key)
	}
	return keys
}

func telemetryEvent(eventType string, request resolverpkg.Request, response resolverpkg.Response, snapshot, policy string, payload map[string]any) telemetry.Event {
	event := telemetry.Event{
		Type: eventType, CatalogSnapshot: snapshot, PolicyRevision: policy,
		Client: telemetry.Client{Name: "skillhub"}, Payload: payload,
	}
	if safeTelemetryToken(request.RequestID) {
		event.RequestID = request.RequestID
	}
	if safeTelemetryToken(response.ResolutionID) {
		event.ResolutionID = response.ResolutionID
	}
	return event
}

func safeTelemetryToken(value string) bool {
	if len(value) == 0 || len(value) > 256 {
		return false
	}
	for index := 0; index < len(value); index++ {
		character := value[index]
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || character == '.' || character == '_' ||
			character == ':' || character == '@' || character == '+' || character == '-' {
			continue
		}
		return false
	}
	return true
}

func safeRecordTelemetry(sink TelemetrySink, event telemetry.Event) {
	if sink == nil {
		return
	}
	defer func() { _ = recover() }()
	sink.Record(event)
}
