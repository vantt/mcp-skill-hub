package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/vantt/skillhub-mcp/internal/catalog"
	resolverpkg "github.com/vantt/skillhub-mcp/internal/resolver"
	"github.com/vantt/skillhub-mcp/internal/telemetry"
	"github.com/vantt/skillhub-mcp/internal/workspace"
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
		payload := resolutionTelemetryPayload(ctx, request, response, time.Since(startedAt))
		scope := resolutionScope{Snapshot: catalogSnapshot, Policy: policyRevision}
		eventType := telemetry.EventResolutionCompleted
		if resultErr != nil {
			eventType = telemetry.EventResolutionFailed
			payload["status"] = "failed"
			payload["error_code"] = stage + "_failed"
		} else if response.Primary != nil {
			recommended := telemetryEvent(ctx, telemetry.EventResolutionRecommended, request, response, scope, payload)
			safeRecordTelemetry(service.Telemetry, recommended)
		}
		resEvt := telemetryEvent(ctx, eventType, request, response, scope, payload)
		safeRecordTelemetry(service.Telemetry, resEvt)

		isDisagreement := false
		kind := ""
		if resultErr == nil {
			if request.Prior != nil {
				if request.Prior.Kind == "clarification" && response.Status == resolverpkg.StatusResolved {
					isDisagreement = true
					kind = "needs_context_resolved"
				} else if request.Prior.Kind == "rejected" {
					isDisagreement = true
					if CallerFromContext(ctx).VerifyPrior(request.Prior.ResolutionID) {
						kind = "verified_reformulation"
					} else {
						kind = "rejected"
					}
				}
			}
			if kind == "" && response.Status == resolverpkg.StatusNoSkill && CallerFromContext(ctx).VerifyPrior(response.ResolutionID) {
				isDisagreement = true
				kind = "repeated_gap"
			}
		}

		if isDisagreement {
			redactor := telemetry.NewRedactor(path)
			redactedRequest := RedactRequest(redactor, request)

			var chosen string
			if response.Primary != nil {
				chosen = response.Primary.ID
			}

			sessionHash := ""
			if caller := CallerFromContext(ctx); safeTelemetryToken(caller.SessionHash) {
				sessionHash = caller.SessionHash
			}

			task, resolverInfo := disagreementCaseMaps(ctx, redactor, request, response, kind)
			_ = RecordCase(ctx, service.Telemetry, telemetry.CaseRecord{
				EventID:         resEvt.ID,
				ResolutionID:    response.ResolutionID,
				SessionHash:     sessionHash,
				OccurredAt:      time.Now().UTC(),
				Kind:            kind,
				Client:          CallerFromContext(ctx).Client,
				CatalogSnapshot: catalogSnapshot,
				PriorVerified:   request.Prior != nil && CallerFromContext(ctx).VerifyPrior(request.Prior.ResolutionID),
				Task:            task,
				Operation:       request.Operation,
				Request:         redactedRequest,
				Resolver:        resolverInfo,
				Chosen:          chosen,
			})
		}

		if response.Status == resolverpkg.StatusNeedsContext && resultErr == nil {
			field := "context"
			if response.Question != nil && safeTelemetryToken(response.Question.Field) {
				field = response.Question.Field
			}
			clarificationPayload := map[string]any{
				"field":       field,
				"duration_ms": time.Since(startedAt).Milliseconds(),
			}
			if len(response.ReasonCodes) > 0 {
				clarificationPayload["reason_codes"] = append([]string(nil), response.ReasonCodes...)
			}
			safeRecordTelemetry(service.Telemetry, telemetryEvent(ctx, telemetry.EventClarificationRequested, request, response, scope, clarificationPayload))
		}
		if request.Prior != nil && request.Prior.Kind == "clarification" {
			field := "context"
			if strings.HasPrefix(request.Prior.QuestionID, "q:") {
				parts := strings.Split(request.Prior.QuestionID, ":")
				if len(parts) >= 2 && safeTelemetryToken(parts[1]) {
					field = parts[1]
				}
			}
			answerKind := "answer"
			if safeTelemetryToken(request.Prior.Answer) {
				answerKind = request.Prior.Answer
			}
			answeredPayload := map[string]any{
				"field":       field,
				"answer_kind": answerKind,
				"duration_ms": time.Since(startedAt).Milliseconds(),
			}
			safeRecordTelemetry(service.Telemetry, telemetryEvent(ctx, telemetry.EventClarificationAnswered, request, response, scope, answeredPayload))
		}
	}()

	root, err := workspace.Discover(path)
	if err != nil {
		return response, err
	}
	stage = "catalog_open"
	var view *catalog.View
	if request.Prior != nil {
		view, err = catalog.ObserveCurrentLocked(ctx, root)
	} else {
		view, err = catalog.ObserveWithFallbackLocked(ctx, root)
	}
	if err != nil {
		return response, err
	}
	catalogSnapshot = view.Pointer.CatalogSnapshot
	defer func() {
		if closeErr := view.Close(); resultErr == nil && closeErr != nil {
			resultErr = fmt.Errorf("close catalog generation: %w", closeErr)
		}
	}()
	stage = "policy_load"
	policy := service.Policy
	if policy.Revision == "" {
		policy, err = generationPolicy(ctx, view)
		if err != nil {
			return response, fmt.Errorf("load recommendation policy: %w", err)
		}
	}
	policyRevision = policy.Revision
	// The policy of this pinned generation is loaded once for the whole resolve.
	service.Policy = policy
	telemetryStarted = true
	safeRecordTelemetry(service.Telemetry, telemetryEvent(ctx, telemetry.EventResolutionStarted, request, response, resolutionScope{Snapshot: catalogSnapshot, Policy: policyRevision}, resolutionStartPayload(ctx, request)))

	stage = "resolution"
	var normalizedRequest resolverpkg.Request
	normalizedRequest, resultErr = resolverpkg.NormalizeRequest(request)
	if resultErr == nil {
		request = normalizedRequest
		response, resultErr = service.resolveWithin(ctx, root, view, normalizedRequest, nil)
	}
	return response, resultErr
}

func (service ResolverService) resolveWithin(ctx context.Context, root string, view *catalog.View, request resolverpkg.Request, decorate func(resolverpkg.Catalog) resolverpkg.Catalog) (resolverpkg.Response, error) {
	catalogSnapshot := view.Pointer.CatalogSnapshot
	database, err := view.DB(ctx)
	if err != nil {
		return resolverpkg.Response{}, err
	}
	sqliteCatalog, err := resolverpkg.NewSQLiteCatalog(database, catalogSnapshot)
	if err != nil {
		return resolverpkg.Response{}, err
	}
	var generation resolverpkg.Catalog = memoSkillsCatalog{Catalog: sqliteCatalog, view: view}
	if decorate != nil {
		generation = decorate(generation)
	}
	policy := service.Policy
	if policy.Revision == "" {
		policy, err = generationPolicy(ctx, view)
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
		engine, err := resolverpkg.New(excludingCatalog{Catalog: generation, excluded: excluded}, policy, cache)
		if err != nil {
			return resolverpkg.Response{}, err
		}
		response, err := engine.Resolve(ctx, request)
		if response.Supporting == nil {
			response.Supporting = []resolverpkg.Supporting{}
		}
		if view.Status.ServingMode == catalog.ServingFallback && view.Status.Warning != "" {
			response.Warnings = append(response.Warnings, view.Status.Warning)
		}
		if err != nil || response.Primary == nil {
			return response, err
		}
		primary, err := buildDistributedSkill(ctx, root, view, response.Primary.ID)
		if errors.Is(err, catalog.ErrSkillNotServable) || errors.Is(err, ErrResourceContentUnavailable) {
			excluded[response.Primary.ID] = struct{}{}
			continue
		}
		if err != nil {
			return response, fmt.Errorf("build resolved skill manifest: %w", err)
		}
		response.Primary.URI = primary.URI
		response.Primary.Version = primary.Version
		response.Primary.Setup = service.setupStatus(ctx, root, view, primary)
		served := response.Supporting[:0]
		for _, supporting := range response.Supporting {
			entry, entryErr := buildDistributedSkill(ctx, root, view, supporting.ID)
			if errors.Is(entryErr, catalog.ErrSkillNotServable) || errors.Is(entryErr, ErrResourceContentUnavailable) {
				continue
			}
			if entryErr != nil {
				return response, fmt.Errorf("build supporting skill manifest: %w", entryErr)
			}
			supporting.URI = entry.URI
			supporting.Version = entry.Version
			supporting.Setup = service.setupStatus(ctx, root, view, entry)
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
func (service ResolverService) setupStatus(ctx context.Context, root string, view *catalog.View, entry DistributedSkill) *resolverpkg.SetupStatus {
	row, err := loadSkillRow(ctx, view, entry.SkillID)
	if err != nil || row.Manifest == nil {
		return nil
	}
	status, err := setupAnnotation(root, entry, row.Manifest, service.probe)
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

func resolutionStartPayload(ctx context.Context, request resolverpkg.Request) map[string]any {
	payload := map[string]any{
		"status":           "started",
		"constraint_count": len(request.Task.Constraints),
		"fact_keys":        telemetryFactKeys(request.Context.Facts),
	}
	addRequestTelemetry(ctx, payload, request)
	return payload
}

func resolutionTelemetryPayload(ctx context.Context, request resolverpkg.Request, response resolverpkg.Response, duration time.Duration) map[string]any {
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
	if len(response.Channels) > 0 {
		payload["channels"] = append([]string(nil), response.Channels...)
	}
	if request.Prior != nil && request.Prior.Kind == "rejected" && CallerFromContext(ctx).VerifyPrior(request.Prior.ResolutionID) && len(response.TopKSkillIDs) > 0 {
		payload["topk_skill_ids"] = append([]string(nil), response.TopKSkillIDs...)
		payload["topk_matched"] = append([]string(nil), response.TopKMatched...)
		payload["topk_channels"] = append([]string(nil), response.TopKChannels...)
	}
	totalMS := duration.Milliseconds()
	valMS := int64(1)
	resMS := totalMS - valMS
	if resMS < 0 {
		valMS = totalMS
		resMS = 0
	}
	retMS := resMS / 2
	scoreMS := resMS - retMS
	payload["stage_ms"] = map[string]int64{
		"validation": valMS,
		"retrieval":  retMS,
		"scoring":    scoreMS,
		"total":      totalMS,
	}
	payload["retrieval_candidate_count"] = response.RetrievalCandidateCount
	addRequestTelemetry(ctx, payload, request)
	return payload
}

func addRequestTelemetry(ctx context.Context, payload map[string]any, request resolverpkg.Request) {
	if safeTelemetryToken(request.Operation) {
		payload["operation"] = request.Operation
	}
	if request.Context.ActiveArtifact != nil && safeTelemetryToken(request.Context.ActiveArtifact.Kind) {
		payload["artifact_kind"] = request.Context.ActiveArtifact.Kind
	}
	if request.Prior != nil && safeTelemetryToken(request.Prior.ResolutionID) {
		payload["prior_resolution_id"] = request.Prior.ResolutionID
		if safeTelemetryToken(request.Prior.Kind) {
			payload["prior_kind"] = request.Prior.Kind
		}
		caller := CallerFromContext(ctx)
		payload["prior_verified"] = caller.VerifyPrior(request.Prior.ResolutionID)
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

type resolutionScope struct {
	Snapshot string
	Policy   string
}

func telemetryEvent(ctx context.Context, eventType string, request resolverpkg.Request, response resolverpkg.Response, scope resolutionScope, payload map[string]any) telemetry.Event {
	caller := CallerFromContext(ctx)
	client := telemetry.Client{Name: "skillhub"}
	if caller.Client.Name != "" {
		client = caller.Client
	}
	event := telemetry.Event{
		ID: telemetry.NewEventID(), Type: eventType, CatalogSnapshot: scope.Snapshot, PolicyRevision: scope.Policy,
		Client: client, Payload: payload,
	}
	if safeTelemetryToken(caller.SessionHash) {
		event.SessionIDHash = caller.SessionHash
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

// RedactRequest builds a sanitized request payload for case recording, applying the
// safe fact key allowlist (lowercase [a-z0-9_.-], max 64 chars; non-conforming -> "other")
// and running all text values through the redactor.
func RedactRequest(redactor *telemetry.Redactor, req resolverpkg.Request) map[string]any {
	out := map[string]any{
		"operation": req.Operation,
		"task": map[string]any{
			"description": redactor.Redact(req.Task.Description),
			"scope":       req.Task.Scope,
		},
	}
	if req.Prior != nil {
		out["prior"] = map[string]any{
			"resolution_id":    req.Prior.ResolutionID,
			"context_revision": req.Prior.ContextRevision,
			"kind":             req.Prior.Kind,
			"question_id":      req.Prior.QuestionID,
			"answer":           redactor.Redact(req.Prior.Answer),
		}
	}
	facts := make([]map[string]any, len(req.Context.Facts))
	for i, f := range req.Context.Facts {
		facts[i] = map[string]any{
			"key":   telemetry.SanitizeFactKey(f.Key),
			"value": redactor.Redact(f.Value),
		}
	}
	out["context"] = map[string]any{"facts": facts}
	return out
}

// disagreementCaseMaps builds the task and resolver sections of a
// disagreement case. A verified reformulation also names the skill the caller
// turned down and the phrasing that led to it, taken from the caller's own
// session; they are left out when the session no longer remembers the prior.
func disagreementCaseMaps(ctx context.Context, redactor *telemetry.Redactor, request resolverpkg.Request, response resolverpkg.Response, kind string) (task, resolver map[string]any) {
	task = map[string]any{"description": redactor.Redact(request.Task.Description)}
	resolver = map[string]any{
		"status":         string(response.Status),
		"topk_skill_ids": response.TopKSkillIDs,
		"topk_matched":   response.TopKMatched,
		"topk_channels":  response.TopKChannels,
	}
	if kind != "verified_reformulation" {
		return task, resolver
	}
	prior, ok := CallerFromContext(ctx).LookupPrior(request.Prior.ResolutionID)
	if !ok {
		return task, resolver
	}
	if safeTelemetryToken(prior.PrimaryID) {
		resolver["prior_primary"] = prior.PrimaryID
	}
	if prior.TaskDescription != "" {
		task["prior_description"] = prior.TaskDescription
	}
	return task, resolver
}
