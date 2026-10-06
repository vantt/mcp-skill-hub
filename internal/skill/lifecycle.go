// Package skill implements curated skill lifecycle semantics independently of delivery adapters.
package skill

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/catalog"
	"github.com/vantt/mcp-skill-hub/internal/mutation"
	"github.com/vantt/mcp-skill-hub/internal/skillruntime"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
	"gopkg.in/yaml.v3"
)

var (
	ErrNotFound                   = errors.New("skill not found")
	ErrAlreadyExists              = errors.New("skill already exists")
	ErrInvalidTransition          = errors.New("invalid skill lifecycle transition")
	ErrSnapshotExpired            = errors.New("snapshot_expired")
	ErrResourceDigestMismatch     = errors.New("resource_digest_mismatch")
	ErrResourceContentUnavailable = errors.New("resource_content_unavailable")
	ErrEditConflict               = errors.New("edit_conflict")
	ErrUntouchedScaffold          = errors.New("untouched_scaffold")
	ErrInvalidRuntime             = errors.New("invalid runtime block")
)

// EditConflictError indicates an optimistic concurrency check failed.
type EditConflictError struct {
	Path           string `json:"path"`
	ExpectedDigest string `json:"expected_digest"`
	ActualDigest   string `json:"actual_digest"`
}

func (e *EditConflictError) Error() string {
	return fmt.Sprintf("edit conflict on %s: expected digest %s, found %s", e.Path, e.ExpectedDigest, e.ActualDigest)
}

func (e *EditConflictError) Is(target error) bool {
	return target == ErrEditConflict || target == mutation.ErrConflict
}

// ProposalKind identifies the domain kind of a proposal.
type ProposalKind string

const (
	ProposalKindLifecycle      ProposalKind = "lifecycle"
	ProposalKindAdd            ProposalKind = "add"
	ProposalKindUpstreamUpdate ProposalKind = "upstream_update"
)

// ScaffoldMarker is the deterministic marker placed in untouched generated templates.
const ScaffoldMarker = "<!-- skillhub:scaffold -->"

// EditableContent contains canonical bytes and digest for an editable skill file.
type EditableContent struct {
	Path    string `json:"path"`
	Content []byte `json:"content"`
	Digest  string `json:"digest"`
}

var collectionPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// RoutingInput is the curated routing subset needed to activate a skill.
//
// When an update supplies RoutingInput, Operations, Triggers, NotFor, and
// MinScope are always written. Examples and CounterExamples are optional: a nil
// slice keeps the stored value and an explicit empty slice clears it. Routing
// keys this type does not name (requirements, relationships, boosts) are
// always preserved.
type RoutingInput struct {
	Operations      []string `yaml:"operations,omitempty" json:"operations,omitempty"`
	Triggers        []string `yaml:"triggers,omitempty" json:"triggers,omitempty"`
	NotFor          []string `yaml:"not_for,omitempty" json:"not_for,omitempty"`
	MinScope        string   `yaml:"min_scope,omitempty" json:"min_scope,omitempty"`
	Examples        []string `yaml:"examples,omitempty" json:"examples,omitempty"`
	CounterExamples []string `yaml:"counter_examples,omitempty" json:"counter_examples,omitempty"`
}

// CreateInput contains explicit draft fields. Content may be agent-authored,
// but it is still validated as untrusted input before planning a mutation.
type CreateInput struct {
	ID             string
	IdempotencyKey string
	Collection     string
	Name           string
	Description    string
	Content        []byte
	Routing        RoutingInput
	Rationale      string
}

// UpdateInput applies only explicitly supplied fields.
type UpdateInput struct {
	IdempotencyKey        string
	Name                  *string
	Description           *string
	Content               []byte
	SetContent            bool
	ExpectedContentDigest string
	Routing               *RoutingInput
	Rationale             *string
	// Runtime replaces the manifest's runtime block. Nil keeps the current
	// block; an empty non-nil map removes it. A non-empty block must satisfy
	// skillruntime.ParseSpec.
	Runtime map[string]any
	// ContentReviewedDigest records a human approval of a skill's whole
	// content. Only the CLI sets it; agent-facing inputs never expose it.
	ContentReviewedDigest *string
}

// DiffSummary gives progressive disclosure without requiring the full patch.
type DiffSummary struct {
	Added    []string `json:"added"`
	Modified []string `json:"modified"`
	Deleted  []string `json:"deleted"`
}

// RoutingImpact is deliberately small. A later evaluator may attach measured
// routing deltas without coupling lifecycle writes to a resolver implementation.
type RoutingImpact struct {
	Summary  string   `json:"summary"`
	Warnings []string `json:"warnings"`
}

// RoutingImpactHook is invoked during preview only. Implementations must be
// read-only and deterministic for the supplied base snapshot.
type RoutingImpactHook interface {
	Evaluate(ctx context.Context, root, skillID, baseSnapshot string, beforeMetadata, afterMetadata []byte) (RoutingImpact, error)
}

// Proposal is an immutable, pinned lifecycle preview.
type Proposal struct {
	ID             string
	Kind           ProposalKind
	Digest         string
	BaseSnapshot   string
	SkillID        string
	Command        string
	Summary        DiffSummary
	FullDiff       string
	RoutingImpact  *RoutingImpact
	RecoveryID     string
	CreatedAt      time.Time
	ExpiresAt      time.Time
	planned        mutation.Proposal
	alreadyApplied *mutation.Receipt
}

func (p Proposal) Planned() mutation.Proposal {
	return p.planned
}

func (p Proposal) AlreadyApplied() *mutation.Receipt {
	return p.alreadyApplied
}

func (p Proposal) WriteSet() mutation.WriteSet {
	return p.planned.WriteSet
}

// MutationResult reports both the canonical receipt and the generation that
// made the change active locally.
type MutationResult struct {
	OperationID     string
	ChangedPaths    []string
	CatalogSnapshot string
	Generation      string
	GitDirty        bool
}

// Manager owns domain planning and confirmation. Clock and hook are optional.
type Manager struct {
	Clock       func() time.Time
	RoutingHook RoutingImpactHook
}

func (manager Manager) now() time.Time {
	if manager.Clock != nil {
		return manager.Clock().UTC()
	}
	return time.Now().UTC()
}

func (manager Manager) PreviewCreate(ctx context.Context, root string, input CreateInput, fullDiff bool) (Proposal, error) {
	if err := ctx.Err(); err != nil {
		return Proposal{}, err
	}
	if err := validateIdentity(input.Collection, input.ID); err != nil {
		return Proposal{}, err
	}
	if strings.TrimSpace(input.Name) == "" || strings.TrimSpace(input.Description) == "" {
		return Proposal{}, errors.New("name and description are required")
	}
	if len(bytes.TrimSpace(input.Content)) == 0 {
		template := fmt.Sprintf("%s\n# %s\n\n%s\n\n## When to use\n\n- Describe when your agent should choose this skill.\n\n## Steps\n\n1. First step.\n2. Second step.\n\n## Examples\n\n- Example input or trigger scenario.\n", ScaffoldMarker, strings.TrimSpace(input.Name), strings.TrimSpace(input.Description))
		input.Content = []byte(template)
	}
	normalizedContent, err := ensureSkillFrontmatter(input.Content, input.ID, input.Description, nil)
	if err != nil {
		return Proposal{}, err
	}
	input.Content = normalizedContent
	requestDigest := createRequestDigest(input)
	idempotencyKey := lifecycleIdempotencyKey(input.IdempotencyKey, "skill_create", input.ID, requestDigest)
	if prior, found, err := mutation.LookupOperation(root, mutation.WriteSet{Command: "skill_create", IdempotencyKey: idempotencyKey, RequestDigest: requestDigest}); err != nil {
		return Proposal{}, err
	} else if found {
		return manager.appliedProposal(root, input.ID, "skill_create", prior, requestDigest, idempotencyKey), nil
	}
	directory := skillDirectory(input.Collection, input.ID)
	metadataPath := directory + "/skill.meta.yaml"
	entrypointPath := directory + "/SKILL.md"
	if exists(root, metadataPath) || exists(root, entrypointPath) {
		return Proposal{}, ErrAlreadyExists
	}
	now := manager.now().Format(time.RFC3339Nano)
	document := map[string]any{
		"schema_version": 1,
		"id":             input.ID,
		"name":           strings.TrimSpace(input.Name),
		"status":         "draft",
		"description":    strings.TrimSpace(input.Description),
		"routing":        mergeRouting(nil, input.Routing),
		"quality":        map[string]any{"reviewed": false},
		"provenance":     map[string]any{"created_by": "skillhub"},
		"history":        []any{map[string]any{"state": "draft", "occurred_at": now}},
		"created_at":     now,
		"updated_at":     now,
	}
	if strings.TrimSpace(input.Rationale) != "" {
		document["quality"].(map[string]any)["routing_review_rationale"] = strings.TrimSpace(input.Rationale)
	}
	metadata, err := marshalMetadata(document)
	if err != nil {
		return Proposal{}, err
	}
	changes := []mutation.Change{{Path: metadataPath, Contents: metadata}, {Path: entrypointPath, Contents: normalizeText(input.Content)}}
	return manager.plan(ctx, root, input.ID, "skill_create", changes, nil, metadata, fullDiff, idempotencyKey, requestDigest)
}

func (manager Manager) PreviewUpdate(ctx context.Context, root, id string, input UpdateInput, fullDiff bool) (Proposal, error) {
	requestDigest := updateRequestDigest(id, input)
	idempotencyKey := lifecycleIdempotencyKey(input.IdempotencyKey, "skill_edit", id, requestDigest)
	if prior, found, err := mutation.LookupOperation(root, mutation.WriteSet{Command: "skill_edit", IdempotencyKey: idempotencyKey, RequestDigest: requestDigest}); err != nil {
		return Proposal{}, err
	} else if found {
		return manager.appliedProposal(root, id, "skill_edit", prior, requestDigest, idempotencyKey), nil
	}
	metadataPath, beforeMetadata, document, err := loadSkill(root, id)
	if err != nil {
		return Proposal{}, err
	}
	if input.Name != nil {
		document["name"] = strings.TrimSpace(*input.Name)
	}
	if input.Description != nil {
		document["description"] = strings.TrimSpace(*input.Description)
	}
	if input.Routing != nil {
		document["routing"] = mergeRouting(mapValue(document, "routing"), *input.Routing)
	}
	if input.Rationale != nil {
		quality := mapValue(document, "quality")
		quality["routing_review_rationale"] = strings.TrimSpace(*input.Rationale)
		document["quality"] = quality
	}
	if input.Runtime != nil {
		if err := applyRuntimeBlock(document, input.Runtime); err != nil {
			return Proposal{}, err
		}
	}
	if input.ContentReviewedDigest != nil {
		quality := mapValue(document, "quality")
		quality["content_reviewed_digest"] = strings.TrimSpace(*input.ContentReviewedDigest)
		document["quality"] = quality
	}
	document["updated_at"] = manager.now().Format(time.RFC3339Nano)
	afterMetadata, err := marshalMetadata(document)
	if err != nil {
		return Proposal{}, err
	}
	changes := []mutation.Change{{Path: metadataPath, Contents: afterMetadata}}
	entrypointPath := filepath.ToSlash(filepath.Join(filepath.Dir(metadataPath), "SKILL.md"))
	description, _ := document["description"].(string)
	if input.SetContent {
		if len(bytes.TrimSpace(input.Content)) == 0 {
			return Proposal{}, errors.New("SKILL.md content must not be empty")
		}
		currentContent, err := readOptional(root, entrypointPath)
		if err != nil {
			return Proposal{}, err
		}
		if input.ExpectedContentDigest != "" {
			currentDigest := ""
			if currentContent != nil {
				sum := sha256.Sum256(currentContent)
				currentDigest = "sha256:" + hex.EncodeToString(sum[:])
			}
			if currentDigest != input.ExpectedContentDigest {
				return Proposal{}, &EditConflictError{
					Path:           entrypointPath,
					ExpectedDigest: input.ExpectedContentDigest,
					ActualDigest:   currentDigest,
				}
			}
		}
		normalized, err := ensureSkillFrontmatter(input.Content, id, description, currentContent)
		if err != nil {
			return Proposal{}, err
		}
		entrypointChange := mutation.Change{Path: entrypointPath, Contents: normalized}
		if input.ExpectedContentDigest != "" {
			entrypointChange.BeforeDigest = input.ExpectedContentDigest
		}
		changes = append(changes, entrypointChange)
	} else if input.Description != nil {
		// The distributed SKILL.md frontmatter must keep matching the metadata.
		currentContent, err := readOptional(root, entrypointPath)
		if err != nil {
			return Proposal{}, err
		}
		if header, body, ok := splitSkillFrontmatter(normalizeText(currentContent)); ok && currentContent != nil && frontmatterDescription(header) != strings.TrimSpace(description) {
			updated, err := withFrontmatterDescription(header, description)
			if err != nil {
				return Proposal{}, fmt.Errorf("existing SKILL.md frontmatter is invalid: %w", err)
			}
			contents := append(append(append([]byte("---\n"), updated...), []byte("---\n")...), body...)
			entrypointChange := mutation.Change{Path: entrypointPath, Contents: contents}
			if input.ExpectedContentDigest != "" {
				entrypointChange.BeforeDigest = input.ExpectedContentDigest
			}
			changes = append(changes, entrypointChange)
		} else if input.ExpectedContentDigest != "" {
			changes = append(changes, mutation.Change{
				Path:         entrypointPath,
				BeforeDigest: input.ExpectedContentDigest,
				Contents:     currentContent,
			})
		}
	} else if input.ExpectedContentDigest != "" {
		currentContent, err := readOptional(root, entrypointPath)
		if err != nil {
			return Proposal{}, err
		}
		changes = append(changes, mutation.Change{
			Path:         entrypointPath,
			BeforeDigest: input.ExpectedContentDigest,
			Contents:     currentContent,
		})
	}
	return manager.plan(ctx, root, id, "skill_edit", changes, beforeMetadata, afterMetadata, fullDiff, idempotencyKey, requestDigest)
}

func (manager Manager) PreviewTransition(ctx context.Context, root, id, target string, fullDiff bool, idempotencyKeys ...string) (Proposal, error) {
	requestDigest := transitionRequestDigest(id, target)
	explicitKey := ""
	if len(idempotencyKeys) != 0 {
		explicitKey = idempotencyKeys[0]
	}
	command := "skill_" + target
	idempotencyKey := lifecycleIdempotencyKey(explicitKey, command, id, requestDigest)
	if prior, found, err := mutation.LookupOperation(root, mutation.WriteSet{Command: command, IdempotencyKey: idempotencyKey, RequestDigest: requestDigest}); err != nil {
		return Proposal{}, err
	} else if found {
		return manager.appliedProposal(root, id, command, prior, requestDigest, idempotencyKey), nil
	}
	metadataPath, beforeMetadata, document, err := loadSkill(root, id)
	if err != nil {
		return Proposal{}, err
	}
	if target == "active" {
		entrypointPath := filepath.ToSlash(filepath.Join(filepath.Dir(metadataPath), "SKILL.md"))
		content, err := readOptional(root, entrypointPath)
		if err != nil {
			return Proposal{}, err
		}
		if IsUntouchedScaffold(content) {
			return Proposal{}, fmt.Errorf("%w: skill %s content is an untouched scaffold; replace placeholder instructions before activating", ErrUntouchedScaffold, id)
		}
	}
	current, _ := document["status"].(string)
	allowed := (current == "draft" && target == "active") || (current == "active" && target == "deprecated") || (current == "deprecated" && target == "archived")
	if !allowed {
		return Proposal{}, fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, current, target)
	}
	now := manager.now().Format(time.RFC3339Nano)
	document["status"] = target
	document["updated_at"] = now
	history, _ := document["history"].([]any)
	history = append(history, map[string]any{"from": current, "state": target, "occurred_at": now})
	document["history"] = history
	if target == "active" {
		quality := mapValue(document, "quality")
		quality["reviewed"] = true
		document["quality"] = quality
	}
	afterMetadata, err := marshalMetadata(document)
	if err != nil {
		return Proposal{}, err
	}
	return manager.plan(ctx, root, id, command, []mutation.Change{{Path: metadataPath, Contents: afterMetadata}}, beforeMetadata, afterMetadata, fullDiff, idempotencyKey, requestDigest)
}

func (manager Manager) plan(ctx context.Context, root, id, command string, changes []mutation.Change, beforeMetadata, afterMetadata []byte, fullDiff bool, idempotencyKey, requestDigest string) (Proposal, error) {
	if err := ctx.Err(); err != nil {
		return Proposal{}, err
	}
	before := make(map[string][]byte, len(changes))
	for _, change := range changes {
		contents, err := readOptional(root, change.Path)
		if err != nil {
			return Proposal{}, err
		}
		before[change.Path] = contents
	}
	planned, err := mutation.PlanMutation(root, mutation.WriteSet{
		Command: command, IdempotencyKey: idempotencyKey, RequestDigest: requestDigest, Changes: changes,
	})
	if err != nil {
		if errors.Is(err, mutation.ErrConflict) {
			for _, ch := range changes {
				if ch.BeforeDigest != "" {
					actual, _ := readOptional(root, ch.Path)
					actualDigest := ""
					if actual != nil {
						sum := sha256.Sum256(actual)
						actualDigest = "sha256:" + hex.EncodeToString(sum[:])
					}
					if actualDigest != ch.BeforeDigest {
						return Proposal{}, &EditConflictError{
							Path:           ch.Path,
							ExpectedDigest: ch.BeforeDigest,
							ActualDigest:   actualDigest,
						}
					}
				}
			}
		}
		return Proposal{}, err
	}
	createdAt := manager.now()
	kind := ProposalKindLifecycle
	if command == "skill_add" {
		kind = ProposalKindAdd
	}
	proposal := Proposal{
		ID: planned.ID, Kind: kind, Digest: planned.Digest, BaseSnapshot: planned.BaseCatalogSnapshot,
		SkillID: id, Command: command, Summary: summarize(changes, before), planned: planned,
		CreatedAt: createdAt, ExpiresAt: createdAt.Add(proposalLifetime),
	}
	if fullDiff {
		proposal.FullDiff = renderFullDiff(changes, before)
	}
	hook := manager.RoutingHook
	if hook == nil {
		hook = BasicRoutingImpactHook{}
	}
	if routingImpactRequired(command, beforeMetadata, afterMetadata) {
		impact, err := hook.Evaluate(ctx, root, id, planned.BaseCatalogSnapshot, beforeMetadata, afterMetadata)
		if err != nil {
			return Proposal{}, fmt.Errorf("evaluate routing impact: %w", err)
		}
		proposal.RoutingImpact = &impact
	}
	return proposal, nil
}

func (manager Manager) Confirm(ctx context.Context, root string, proposal Proposal) (MutationResult, error) {
	if err := ctx.Err(); err != nil {
		return MutationResult{}, err
	}
	if proposal.ExpiresAt.IsZero() || !manager.now().Before(proposal.ExpiresAt) {
		return MutationResult{}, ErrSnapshotExpired
	}
	if proposal.alreadyApplied != nil {
		return resultForAppliedOperation(ctx, root, *proposal.alreadyApplied)
	}
	receipt, err := mutation.ConfirmMutationWithOptions(root, proposal.planned, mutation.Confirmation{
		ProposalID: proposal.ID, ProposalDigest: proposal.Digest, BaseCatalogSnapshot: proposal.BaseSnapshot,
	}, mutation.Options{PostCanonical: func(expected string) (mutation.Publication, error) {
		built, err := catalog.BuildCatalogGenerationWhileLocked(ctx, root, expected, catalog.BuildOptions{})
		if err != nil {
			return mutation.Publication{}, err
		}
		return mutation.Publication{CatalogSnapshot: built.Pointer.CatalogSnapshot, Generation: built.Pointer.Generation}, nil
	}})
	if err != nil {
		return MutationResult{}, err
	}
	return MutationResult{
		OperationID: receipt.OperationID, ChangedPaths: receipt.ChangedPaths,
		CatalogSnapshot: receipt.CatalogSnapshot, Generation: receipt.Generation, GitDirty: receipt.GitDirty,
	}, nil
}

func (manager Manager) appliedProposal(root, id, command string, receipt mutation.Receipt, requestDigest, idempotencyKey string) Proposal {
	hash := sha256.Sum256([]byte(command + "\x00" + idempotencyKey + "\x00" + requestDigest + "\x00" + receipt.CatalogSnapshot))
	digest := "sha256:" + hex.EncodeToString(hash[:])
	now := manager.now()
	kind := ProposalKindLifecycle
	if command == "skill_add" {
		kind = ProposalKindAdd
	}
	return Proposal{
		ID: "PROP-" + hex.EncodeToString(hash[:10]), Kind: kind, Digest: digest, BaseSnapshot: receipt.CatalogSnapshot,
		SkillID: id, Command: command, Summary: DiffSummary{}, alreadyApplied: &receipt,
		CreatedAt: now, ExpiresAt: now.Add(proposalLifetime),
	}
}

func resultForAppliedOperation(ctx context.Context, root string, receipt mutation.Receipt) (MutationResult, error) {
	status, err := catalog.Inspect(ctx, root)
	if err != nil {
		return MutationResult{}, err
	}
	generation := ""
	if status.Pointer != nil && status.Pointer.CatalogSnapshot == receipt.CatalogSnapshot {
		generation = status.Pointer.Generation
	} else {
		generation, err = catalog.FindGenerationForOperation(ctx, root, receipt.OperationID, receipt.CatalogSnapshot)
		if err != nil {
			return MutationResult{}, fmt.Errorf("%w: published generation for applied operation %s is unavailable: %v", ErrSnapshotExpired, receipt.OperationID, err)
		}
	}
	return MutationResult{
		OperationID: receipt.OperationID, ChangedPaths: receipt.ChangedPaths,
		CatalogSnapshot: receipt.CatalogSnapshot, Generation: generation, GitDirty: receipt.GitDirty,
	}, nil
}

func lifecycleIdempotencyKey(explicit, command, id, requestDigest string) string {
	if strings.TrimSpace(explicit) != "" {
		return strings.TrimSpace(explicit)
	}
	return command + ":" + id + ":" + strings.TrimPrefix(requestDigest, "sha256:")
}

func normalizedDigest(value any) string {
	encoded, _ := json.Marshal(value)
	sum := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func createRequestDigest(input CreateInput) string {
	return normalizedDigest(struct {
		ID, Collection, Name, Description, Content, Rationale string
		Routing                                               routingDigestInput
	}{input.ID, input.Collection, strings.TrimSpace(input.Name), strings.TrimSpace(input.Description), string(normalizeText(input.Content)), strings.TrimSpace(input.Rationale), normalizeRouting(input.Routing)})
}

func updateRequestDigest(id string, input UpdateInput) string {
	normalizeOptional := func(value *string) *string {
		if value == nil {
			return nil
		}
		normalized := strings.TrimSpace(*value)
		return &normalized
	}
	var routing *routingDigestInput
	if input.Routing != nil {
		normalized := normalizeRouting(*input.Routing)
		routing = &normalized
	}
	content := ""
	if input.SetContent {
		content = string(normalizeText(input.Content))
	}
	// A pointer keeps an empty map (remove the block) distinct from nil (keep).
	var runtimeBlock *map[string]any
	if input.Runtime != nil {
		runtimeBlock = &input.Runtime
	}
	return normalizedDigest(struct {
		ID                    string
		Name                  *string
		Description           *string
		Content               string
		SetContent            bool
		ExpectedContentDigest string
		Routing               *routingDigestInput
		Rationale             *string
		ContentReviewedDigest *string         `json:",omitempty"`
		Runtime               *map[string]any `json:",omitempty"`
	}{id, normalizeOptional(input.Name), normalizeOptional(input.Description), content, input.SetContent, strings.TrimSpace(input.ExpectedContentDigest), routing, normalizeOptional(input.Rationale), normalizeOptional(input.ContentReviewedDigest), runtimeBlock})
}

// applyRuntimeBlock validates block with the same rules the catalog applies and
// sets it on the manifest document; an empty block removes the runtime key.
func applyRuntimeBlock(document map[string]any, block map[string]any) error {
	if len(block) == 0 {
		delete(document, "runtime")
		return nil
	}
	encoded, err := json.Marshal(map[string]any{"runtime": block})
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidRuntime, err)
	}
	if _, _, err := skillruntime.ParseSpec(encoded); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidRuntime, err)
	}
	document["runtime"] = block
	return nil
}

// routingDigestInput is the request-digest form of RoutingInput. The optional
// lists are pointers so "keep" (nil) and "clear" (empty) produce different
// digests, while requests without them keep their historical digest.
type routingDigestInput struct {
	Operations      []string  `json:"operations,omitempty"`
	Triggers        []string  `json:"triggers,omitempty"`
	NotFor          []string  `json:"not_for,omitempty"`
	MinScope        string    `json:"min_scope,omitempty"`
	Examples        *[]string `json:"examples,omitempty"`
	CounterExamples *[]string `json:"counter_examples,omitempty"`
}

func normalizeRouting(input RoutingInput) routingDigestInput {
	optional := func(values []string) *[]string {
		if values == nil {
			return nil
		}
		cleaned := cleanStrings(values)
		return &cleaned
	}
	return routingDigestInput{
		Operations:      cleanStrings(input.Operations),
		Triggers:        cleanStrings(input.Triggers),
		NotFor:          cleanStrings(input.NotFor),
		MinScope:        strings.TrimSpace(input.MinScope),
		Examples:        optional(input.Examples),
		CounterExamples: optional(input.CounterExamples),
	}
}

func transitionRequestDigest(id, target string) string {
	return normalizedDigest(struct{ ID, Target string }{id, target})
}

func routingImpactRequired(command string, before, after []byte) bool {
	if command == "skill_create" || command == "skill_active" {
		return true
	}
	if command != "skill_edit" {
		return false
	}
	return !bytes.Equal(routingBytes(before), routingBytes(after))
}

func routingBytes(metadata []byte) []byte {
	var document map[string]any
	if yaml.Unmarshal(metadata, &document) != nil {
		return metadata
	}
	encoded, _ := json.Marshal(document["routing"])
	return encoded
}

// BasicRoutingImpactHook is the deterministic production fallback used until a
// later phase supplies a semantic routing evaluator. It reports only direct
// routing-field and activation eligibility changes.
type BasicRoutingImpactHook struct{}

func (BasicRoutingImpactHook) Evaluate(_ context.Context, _, skillID, baseSnapshot string, beforeMetadata, afterMetadata []byte) (RoutingImpact, error) {
	var before, after map[string]any
	if len(beforeMetadata) != 0 {
		if err := yaml.Unmarshal(beforeMetadata, &before); err != nil {
			return RoutingImpact{}, fmt.Errorf("parse prior routing metadata: %w", err)
		}
	}
	if err := yaml.Unmarshal(afterMetadata, &after); err != nil {
		return RoutingImpact{}, fmt.Errorf("parse proposed routing metadata: %w", err)
	}
	beforeStatus, _ := before["status"].(string)
	afterStatus, _ := after["status"].(string)
	summary := fmt.Sprintf("Direct routing fields for %s were evaluated against %s.", skillID, baseSnapshot)
	if beforeStatus != "active" && afterStatus == "active" {
		summary = fmt.Sprintf("%s becomes directly eligible for routing; semantic resolver impact is not claimed.", skillID)
	} else if bytes.Equal(routingBytes(beforeMetadata), routingBytes(afterMetadata)) {
		summary = "Direct routing fields are unchanged; semantic resolver impact is not claimed."
	}
	return RoutingImpact{Summary: summary}, nil
}

func validateIdentity(collection, id string) error {
	if !collectionPattern.MatchString(collection) {
		return errors.New("collection must be a lowercase kebab-case identifier")
	}
	if !idPattern.MatchString(id) {
		return errors.New("skill id must be a lowercase kebab-case identifier")
	}
	return nil
}

func skillDirectory(collection, id string) string { return "skills/" + collection + "/" + id }

// mergeRouting overlays input onto a copy of the stored routing map. Keys the
// input does not name are preserved, so editing triggers never drops
// requirements, relationships, boosts, or examples. The four core fields are
// always written; the optional example lists follow the nil-keeps /
// empty-clears contract documented on RoutingInput.
func mergeRouting(existing map[string]any, input RoutingInput) map[string]any {
	merged := make(map[string]any, len(existing)+6)
	for key, value := range existing {
		merged[key] = value
	}
	merged["operations"] = cleanStrings(input.Operations)
	merged["triggers"] = cleanStrings(input.Triggers)
	merged["not_for"] = cleanStrings(input.NotFor)
	merged["min_scope"] = strings.TrimSpace(input.MinScope)
	for key, values := range map[string][]string{"examples": input.Examples, "counter_examples": input.CounterExamples} {
		switch cleaned := cleanStrings(values); {
		case values == nil:
		case len(cleaned) == 0:
			delete(merged, key)
		default:
			merged[key] = cleaned
		}
	}
	return merged
}

func cleanStrings(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func marshalMetadata(document map[string]any) ([]byte, error) {
	contents, err := yaml.Marshal(document)
	if err != nil {
		return nil, fmt.Errorf("encode skill metadata: %w", err)
	}
	return contents, nil
}

func loadSkill(root, id string) (string, []byte, map[string]any, error) {
	if !idPattern.MatchString(id) {
		return "", nil, nil, errors.New("skill id must be a lowercase kebab-case identifier")
	}
	handle, err := os.OpenRoot(root)
	if err != nil {
		return "", nil, nil, err
	}
	defer handle.Close()
	collections, err := fs.ReadDir(handle.FS(), "skills")
	if err != nil {
		return "", nil, nil, err
	}
	var matched string
	for _, collection := range collections {
		if !collectionPattern.MatchString(collection.Name()) || collection.Type()&os.ModeSymlink != 0 || !collection.IsDir() {
			return "", nil, nil, fmt.Errorf("invalid skill collection path %q", collection.Name())
		}
		directory := "skills/" + collection.Name() + "/" + id
		directoryInfo, directoryErr := handle.Lstat(directory)
		if errors.Is(directoryErr, os.ErrNotExist) {
			continue
		}
		if directoryErr != nil {
			return "", nil, nil, directoryErr
		}
		if directoryInfo.Mode()&os.ModeSymlink != 0 || !directoryInfo.IsDir() {
			return "", nil, nil, fmt.Errorf("unsafe skill directory %q", directory)
		}
		candidate := directory + "/skill.meta.yaml"
		info, statErr := handle.Lstat(candidate)
		if errors.Is(statErr, os.ErrNotExist) {
			continue
		}
		if statErr != nil {
			return "", nil, nil, statErr
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return "", nil, nil, fmt.Errorf("unsafe skill metadata path %q", candidate)
		}
		if matched != "" {
			return "", nil, nil, fmt.Errorf("duplicate skill id %q", id)
		}
		matched = candidate
	}
	if matched == "" {
		return "", nil, nil, ErrNotFound
	}
	contents, err := handle.ReadFile(matched)
	if err != nil {
		return "", nil, nil, err
	}
	var document map[string]any
	decoder := yaml.NewDecoder(bytes.NewReader(contents))
	if err := decoder.Decode(&document); err != nil {
		return "", nil, nil, fmt.Errorf("parse skill metadata: %w", err)
	}
	if document["id"] != id {
		return "", nil, nil, fmt.Errorf("skill metadata ID does not match canonical path")
	}
	return matched, contents, document, nil
}

func mapValue(document map[string]any, key string) map[string]any {
	value, _ := document[key].(map[string]any)
	if value == nil {
		value = make(map[string]any)
	}
	return value
}

func exists(root, relative string) bool {
	handle, err := os.OpenRoot(root)
	if err != nil {
		return false
	}
	defer handle.Close()
	info, err := handle.Lstat(relative)
	return err == nil && info.Mode()&os.ModeSymlink == 0
}

func readOptional(root, relative string) ([]byte, error) {
	handle, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer handle.Close()
	info, err := handle.Lstat(relative)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("unsafe canonical read path %q", relative)
	}
	return handle.ReadFile(relative)
}

func normalizeText(contents []byte) []byte {
	contents = bytes.ReplaceAll(contents, []byte("\r\n"), []byte("\n"))
	if !bytes.HasSuffix(contents, []byte("\n")) {
		contents = append(contents, '\n')
	}
	return contents
}

func ensureSkillFrontmatter(contents []byte, id, description string, current []byte) ([]byte, error) {
	contents = normalizeText(contents)
	if header, _, ok := splitSkillFrontmatter(contents); ok {
		if err := validateSkillFrontmatter(header, id); err != nil {
			return nil, err
		}
		return contents, nil
	}
	var header []byte
	if existing, _, ok := splitSkillFrontmatter(normalizeText(current)); ok {
		if err := validateSkillFrontmatter(existing, id); err != nil {
			return nil, fmt.Errorf("existing SKILL.md frontmatter is invalid: %w", err)
		}
		updated, err := withFrontmatterDescription(existing, description)
		if err != nil {
			return nil, fmt.Errorf("existing SKILL.md frontmatter is invalid: %w", err)
		}
		header = updated
	} else {
		generated, err := yaml.Marshal(struct {
			Name        string `yaml:"name"`
			Description string `yaml:"description"`
		}{Name: id, Description: strings.TrimSpace(description)})
		if err != nil {
			return nil, err
		}
		header = generated
	}
	if !bytes.HasSuffix(header, []byte("\n")) {
		header = append(header, '\n')
	}
	return normalizeText(append(append(append([]byte("---\n"), header...), []byte("---\n\n")...), contents...)), nil
}

// withFrontmatterDescription replaces only the description value, keeping every
// other frontmatter key and its order.
func withFrontmatterDescription(header []byte, description string) ([]byte, error) {
	var document yaml.Node
	if err := yaml.Unmarshal(header, &document); err != nil {
		return nil, fmt.Errorf("parse SKILL.md YAML frontmatter: %w", err)
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return nil, errors.New("SKILL.md frontmatter must be a mapping")
	}
	mapping := document.Content[0]
	description = strings.TrimSpace(description)
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value == "description" {
			mapping.Content[index+1] = &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: description}
			return marshalFrontmatter(&document)
		}
	}
	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "description"},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: description})
	return marshalFrontmatter(&document)
}

func frontmatterDescription(header []byte) string {
	var value map[string]any
	if yaml.Unmarshal(header, &value) != nil {
		return ""
	}
	description, _ := value["description"].(string)
	return strings.TrimSpace(description)
}

func marshalFrontmatter(document *yaml.Node) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := yaml.NewEncoder(&buffer)
	encoder.SetIndent(2)
	if err := encoder.Encode(document); err != nil {
		return nil, err
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func splitSkillFrontmatter(contents []byte) (header, body []byte, ok bool) {
	if !bytes.HasPrefix(contents, []byte("---\n")) {
		return nil, contents, false
	}
	end := bytes.Index(contents[4:], []byte("\n---\n"))
	if end < 0 {
		return nil, contents, false
	}
	end += 4
	return contents[4:end], contents[end+5:], true
}

func validateSkillFrontmatter(header []byte, id string) error {
	var value map[string]any
	if err := yaml.Unmarshal(header, &value); err != nil {
		return fmt.Errorf("parse SKILL.md YAML frontmatter: %w", err)
	}
	name, _ := value["name"].(string)
	description, _ := value["description"].(string)
	if name != "" && name != id {
		return fmt.Errorf("SKILL.md frontmatter name must be %q", id)
	}
	if strings.TrimSpace(description) == "" {
		return errors.New("SKILL.md frontmatter description must be non-empty")
	}
	return nil
}

func summarize(changes []mutation.Change, before map[string][]byte) DiffSummary {
	var result DiffSummary
	for _, change := range changes {
		switch {
		case change.Delete:
			result.Deleted = append(result.Deleted, change.Path)
		case before[change.Path] == nil:
			result.Added = append(result.Added, change.Path)
		case !bytes.Equal(before[change.Path], change.Contents):
			result.Modified = append(result.Modified, change.Path)
		}
	}
	sort.Strings(result.Added)
	sort.Strings(result.Modified)
	sort.Strings(result.Deleted)
	return result
}

func renderFullDiff(changes []mutation.Change, before map[string][]byte) string {
	var output strings.Builder
	copyChanges := append([]mutation.Change(nil), changes...)
	sort.Slice(copyChanges, func(i, j int) bool { return copyChanges[i].Path < copyChanges[j].Path })
	for _, change := range copyChanges {
		old := before[change.Path]
		if bytes.Equal(old, change.Contents) && !change.Delete {
			continue
		}
		fmt.Fprintf(&output, "--- a/%s\n+++ b/%s\n", change.Path, change.Path)
		for _, line := range strings.Split(strings.TrimSuffix(string(old), "\n"), "\n") {
			if old != nil {
				output.WriteString("-" + line + "\n")
			}
		}
		if !change.Delete {
			for _, line := range strings.Split(strings.TrimSuffix(string(change.Contents), "\n"), "\n") {
				output.WriteString("+" + line + "\n")
			}
		}
	}
	return output.String()
}

// ReadRouting returns the routing fields currently stored in a skill's
// metadata, in any lifecycle state, so callers can change one field without
// resetting the others.
func ReadRouting(root, id string) (RoutingInput, error) {
	_, _, document, err := loadSkill(root, id)
	if err != nil {
		return RoutingInput{}, err
	}
	routing := mapValue(document, "routing")
	list := func(key string) []string {
		items, _ := routing[key].([]any)
		values := make([]string, 0, len(items))
		for _, item := range items {
			if text, ok := item.(string); ok {
				values = append(values, text)
			}
		}
		return values
	}
	optionalList := func(key string) []string {
		if _, present := routing[key]; !present {
			return nil
		}
		return list(key)
	}
	minScope, _ := routing["min_scope"].(string)
	return RoutingInput{
		Operations: list("operations"), Triggers: list("triggers"), NotFor: list("not_for"), MinScope: minScope,
		Examples: optionalList("examples"), CounterExamples: optionalList("counter_examples"),
	}, nil
}

// ReadRationale returns the routing_review_rationale currently stored in a
// skill's quality metadata, in any lifecycle state.
func ReadRationale(root, id string) (string, error) {
	_, _, document, err := loadSkill(root, id)
	if err != nil {
		return "", err
	}
	quality := mapValue(document, "quality")
	rationale, _ := quality["routing_review_rationale"].(string)
	return rationale, nil
}

// ReadEditableSkill returns the current canonical entrypoint and its digest for a draft or
// active skill. It is intended to seed an external-editor temporary file or MCP read-modify-write.
func ReadEditableSkill(root, id string) (EditableContent, error) {
	metadataPath, _, _, err := loadSkill(root, id)
	if err != nil {
		return EditableContent{}, err
	}
	entrypointPath := filepath.ToSlash(filepath.Join(filepath.Dir(metadataPath), "SKILL.md"))
	contents, err := readOptionalRequired(root, entrypointPath)
	if err != nil {
		return EditableContent{}, err
	}
	sum := sha256.Sum256(contents)
	return EditableContent{
		Path:    entrypointPath,
		Content: contents,
		Digest:  "sha256:" + hex.EncodeToString(sum[:]),
	}, nil
}

// ReadEditableContent returns the current canonical entrypoint for a draft or
// active skill. It is intended only to seed an external-editor temporary file.
func ReadEditableContent(root, id string) ([]byte, error) {
	editable, err := ReadEditableSkill(root, id)
	if err != nil {
		return nil, err
	}
	return editable.Content, nil
}

// IsUntouchedScaffold reports whether content represents an untouched generated template.
func IsUntouchedScaffold(content []byte) bool {
	if len(bytes.TrimSpace(content)) == 0 {
		return true
	}
	if bytes.Contains(content, []byte(ScaffoldMarker)) {
		return true
	}
	s := string(content)
	hasWhen := strings.Contains(s, "Describe when your agent should choose this skill.")
	hasStep1 := strings.Contains(s, "1. First step.")
	hasStep2 := strings.Contains(s, "2. Second step.")
	hasExample := strings.Contains(s, "Example input or trigger scenario.")
	return hasWhen && hasStep1 && hasStep2 && hasExample
}

func readOptionalRequired(root, relative string) ([]byte, error) {
	contents, err := readOptional(root, relative)
	if err != nil {
		return nil, err
	}
	if contents == nil {
		return nil, os.ErrNotExist
	}
	return contents, nil
}

// ResolveWorkspace normalizes a path for callers that share this package without
// duplicating workspace discovery rules.
func ResolveWorkspace(path string) (string, error) { return workspace.Discover(path) }
