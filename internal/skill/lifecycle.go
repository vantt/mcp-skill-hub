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
	"github.com/vantt/mcp-skill-hub/internal/workspace"
	"gopkg.in/yaml.v3"
)

var (
	ErrNotFound               = errors.New("skill not found")
	ErrAlreadyExists          = errors.New("skill already exists")
	ErrInvalidTransition      = errors.New("invalid skill lifecycle transition")
	ErrSnapshotExpired        = errors.New("snapshot_expired")
	ErrResourceDigestMismatch = errors.New("resource_digest_mismatch")
)

var collectionPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// RoutingInput is the curated routing subset needed to activate a skill.
type RoutingInput struct {
	Operations []string `yaml:"operations,omitempty" json:"operations"`
	Triggers   []string `yaml:"triggers,omitempty" json:"triggers"`
	NotFor     []string `yaml:"not_for" json:"not_for"`
	MinScope   string   `yaml:"min_scope,omitempty" json:"min_scope"`
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
	IdempotencyKey string
	Name           *string
	Description    *string
	Content        []byte
	SetContent     bool
	Routing        *RoutingInput
	Rationale      *string
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
	Digest         string
	BaseSnapshot   string
	SkillID        string
	Command        string
	Summary        DiffSummary
	FullDiff       string
	RoutingImpact  *RoutingImpact
	CreatedAt      time.Time
	ExpiresAt      time.Time
	planned        mutation.Proposal
	alreadyApplied *mutation.Receipt
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
		input.Content = []byte("# " + strings.TrimSpace(input.Name) + "\n\n" + strings.TrimSpace(input.Description) + "\n")
	}
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
		"routing":        routingDocument(input.Routing),
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
		document["routing"] = routingDocument(*input.Routing)
	}
	if input.Rationale != nil {
		quality := mapValue(document, "quality")
		quality["routing_review_rationale"] = strings.TrimSpace(*input.Rationale)
		document["quality"] = quality
	}
	document["updated_at"] = manager.now().Format(time.RFC3339Nano)
	afterMetadata, err := marshalMetadata(document)
	if err != nil {
		return Proposal{}, err
	}
	changes := []mutation.Change{{Path: metadataPath, Contents: afterMetadata}}
	if input.SetContent {
		if len(bytes.TrimSpace(input.Content)) == 0 {
			return Proposal{}, errors.New("SKILL.md content must not be empty")
		}
		changes = append(changes, mutation.Change{Path: filepath.ToSlash(filepath.Join(filepath.Dir(metadataPath), "SKILL.md")), Contents: normalizeText(input.Content)})
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
		return Proposal{}, err
	}
	createdAt := manager.now()
	proposal := Proposal{
		ID: planned.ID, Digest: planned.Digest, BaseSnapshot: planned.BaseCatalogSnapshot,
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
	return Proposal{
		ID: "PROP-" + hex.EncodeToString(hash[:10]), Digest: digest, BaseSnapshot: receipt.CatalogSnapshot,
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
		Routing                                               RoutingInput
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
	var routing *RoutingInput
	if input.Routing != nil {
		normalized := normalizeRouting(*input.Routing)
		routing = &normalized
	}
	content := ""
	if input.SetContent {
		content = string(normalizeText(input.Content))
	}
	return normalizedDigest(struct {
		ID          string
		Name        *string
		Description *string
		Content     string
		SetContent  bool
		Routing     *RoutingInput
		Rationale   *string
	}{id, normalizeOptional(input.Name), normalizeOptional(input.Description), content, input.SetContent, routing, normalizeOptional(input.Rationale)})
}

func normalizeRouting(input RoutingInput) RoutingInput {
	return RoutingInput{Operations: cleanStrings(input.Operations), Triggers: cleanStrings(input.Triggers), NotFor: cleanStrings(input.NotFor), MinScope: strings.TrimSpace(input.MinScope)}
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

func routingDocument(input RoutingInput) map[string]any {
	return map[string]any{
		"operations": cleanStrings(input.Operations),
		"triggers":   cleanStrings(input.Triggers),
		"not_for":    cleanStrings(input.NotFor),
		"min_scope":  strings.TrimSpace(input.MinScope),
	}
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

// ReadEditableContent returns the current canonical entrypoint for a draft or
// active skill. It is intended only to seed an external-editor temporary file.
func ReadEditableContent(root, id string) ([]byte, error) {
	metadataPath, _, _, err := loadSkill(root, id)
	if err != nil {
		return nil, err
	}
	return readOptionalRequired(root, filepath.ToSlash(filepath.Join(filepath.Dir(metadataPath), "SKILL.md")))
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
