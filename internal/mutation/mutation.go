// Package mutation is the sole durable write boundary for canonical workspaces.
package mutation

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/canonical"
)

var (
	ErrConflict            = errors.New("mutation precondition no longer matches canonical state")
	ErrIdempotencyConflict = errors.New("idempotency key was already used for a different request")
	ErrRecoveryRequired    = errors.New("workspace recovery is required before another mutation")
	ErrWorkspaceBusy       = errors.New("workspace is busy")
)

const (
	maxChanges          = 1024
	maxChangeBytes      = 16 << 20
	maxTransactionBytes = 64 << 20
)

// Change replaces or deletes one canonical, workspace-relative file. An empty
// BeforeDigest pins the path as absent.
type Change struct {
	Path         string `json:"path"`
	BeforeDigest string `json:"before"`
	Contents     []byte `json:"contents,omitempty"`
	Delete       bool   `json:"delete,omitempty"`
}

// WriteSet is the complete input to a managed canonical mutation.
type WriteSet struct {
	OperationID         string
	Command             string
	IdempotencyKey      string
	RequestDigest       string
	ProposalID          string
	ProposalDigest      string
	BaseCatalogSnapshot string
	Changes             []Change
}

// Proposal is an immutable preview. ConfirmMutation verifies every pin again
// while holding the exclusive workspace lock.
type Proposal struct {
	ID                  string
	Digest              string
	BaseCatalogSnapshot string
	WriteSet            WriteSet
}

// Confirmation contains the values a caller explicitly approved.
type Confirmation struct {
	ProposalID          string
	ProposalDigest      string
	BaseCatalogSnapshot string
}

// Receipt is returned after a successful durable mutation, including an
// idempotent retry after the original response was lost.
type Receipt struct {
	OperationID     string
	ChangedPaths    []string
	CatalogSnapshot string
	Generation      string
	GitDirty        bool
}

// Publication identifies the derived generation published for the exact
// canonical result snapshot while the mutation lock is still held.
type Publication struct {
	CatalogSnapshot string
	Generation      string
}

// FaultPoint identifies a durable transition used by deterministic crash tests.
type FaultPoint string

const (
	FaultTransactionCreated     FaultPoint = "transaction_created"
	FaultStagedFile             FaultPoint = "staged_file"
	FaultManifestPhaseUpdate    FaultPoint = "manifest_phase_update"
	FaultFirstCanonicalReplace  FaultPoint = "first_canonical_replace"
	FaultMiddleCanonicalReplace FaultPoint = "middle_canonical_replace"
	FaultLastCanonicalReplace   FaultPoint = "last_canonical_replace"
	FaultReceiptWrite           FaultPoint = "receipt_write"
	FaultCanonicalValidation    FaultPoint = "canonical_post_validation"
	FaultJournalCleanup         FaultPoint = "journal_cleanup"
	// FaultBeforeTargetDisplace runs after the optimistic digest check but
	// before the canonical target is moved into the transaction. It exists so
	// tests can deterministically model a late external edit or parent swap.
	FaultBeforeTargetDisplace FaultPoint = "before_target_displace"
	// FaultAfterTargetDisplace runs after the exact target has been moved into
	// the transaction and before its digest is revalidated.
	FaultAfterTargetDisplace FaultPoint = "after_target_displace"
)

// Options provides deterministic failure injection without mutable globals.
type Options struct {
	Fault         func(FaultPoint) error
	PostCanonical func(expectedCatalogSnapshot string) (Publication, error)
}

func (o Options) inject(point FaultPoint) error {
	if o.Fault == nil {
		return nil
	}
	return o.Fault(point)
}

// PlanMutation reads a stable snapshot under a shared lock, pins every affected
// path, and validates the proposed virtual tree without writing canonical data.
func PlanMutation(root string, set WriteSet) (Proposal, error) {
	if err := validWriteSet(set, false); err != nil {
		return Proposal{}, err
	}
	lock, err := AcquireSharedLock(context.Background(), root, DefaultLockTimeout)
	if err != nil {
		return Proposal{}, err
	}
	defer lock.Unlock()
	if ids, err := pendingUnlocked(root); err != nil {
		return Proposal{}, err
	} else if len(ids) != 0 {
		return Proposal{}, ErrRecoveryRequired
	}
	snapshot, err := canonical.Scan(root)
	if err != nil {
		return Proposal{}, err
	}
	if set.BaseCatalogSnapshot != "" && set.BaseCatalogSnapshot != snapshot.CatalogSnapshot {
		return Proposal{}, ErrConflict
	}
	set.BaseCatalogSnapshot = snapshot.CatalogSnapshot
	set.Changes = append([]Change(nil), set.Changes...)
	for index := range set.Changes {
		actual, err := digestAt(root, set.Changes[index].Path)
		if err != nil {
			return Proposal{}, err
		}
		if set.Changes[index].BeforeDigest != "" && set.Changes[index].BeforeDigest != actual {
			return Proposal{}, fmt.Errorf("%w: %s", ErrConflict, set.Changes[index].Path)
		}
		set.Changes[index].BeforeDigest = actual
	}
	sortChanges(set.Changes)
	if err := validateVirtual(root, set.Changes); err != nil {
		return Proposal{}, err
	}
	if set.OperationID == "" {
		set.OperationID, err = allocateOperationID()
		if err != nil {
			return Proposal{}, err
		}
	}
	digest := proposalDigest(set)
	id := set.ProposalID
	if id == "" {
		id = "PROP-" + strings.TrimPrefix(digest, "sha256:")[:20]
	}
	set.ProposalID, set.ProposalDigest = id, digest
	return Proposal{ID: id, Digest: digest, BaseCatalogSnapshot: set.BaseCatalogSnapshot, WriteSet: set}, nil
}

// ConfirmMutation rejects a modified or stale proposal before delegating to the
// durable commit protocol.
func ConfirmMutation(root string, proposal Proposal, confirmation Confirmation) (Receipt, error) {
	return ConfirmMutationWithOptions(root, proposal, confirmation, Options{})
}

// ConfirmMutationWithOptions confirms a proposal and keeps its WAL and
// exclusive workspace lock through an optional post-canonical publication.
func ConfirmMutationWithOptions(root string, proposal Proposal, confirmation Confirmation, options Options) (Receipt, error) {
	if confirmation.ProposalID == "" || confirmation.ProposalID != proposal.ID ||
		confirmation.ProposalDigest != proposal.Digest || confirmation.BaseCatalogSnapshot != proposal.BaseCatalogSnapshot {
		return Receipt{}, ErrConflict
	}
	set := proposal.WriteSet
	if set.ProposalID != proposal.ID || set.ProposalDigest != proposal.Digest ||
		set.BaseCatalogSnapshot != proposal.BaseCatalogSnapshot || proposalDigest(set) != proposal.Digest {
		return Receipt{}, ErrConflict
	}
	return CommitWithOptions(root, set, options)
}

// Commit applies a write set atomically per path, with the operation receipt
// last. Commit is retained as the lower-level ExecuteCanonicalMutation boundary.
func Commit(root string, set WriteSet) (Receipt, error) {
	return CommitWithOptions(root, set, Options{})
}

// CommitWithOptions is Commit with deterministic fault injection for tests.
func CommitWithOptions(root string, set WriteSet, options Options) (Receipt, error) {
	if err := validWriteSet(set, true); err != nil {
		return Receipt{}, err
	}
	lock, err := AcquireExclusiveLock(context.Background(), root, DefaultLockTimeout)
	if err != nil {
		return Receipt{}, err
	}
	defer lock.Unlock()
	return commitWhileLocked(root, set, options)
}

func commitWhileLocked(root string, set WriteSet, options Options) (Receipt, error) {
	if ids, err := pendingUnlocked(root); err != nil {
		return Receipt{}, err
	} else if len(ids) != 0 {
		return Receipt{}, ErrRecoveryRequired
	}
	if recorded, found, err := existingOperation(root, set); err != nil {
		return Receipt{}, err
	} else if found {
		return recorded, nil
	}
	if set.BaseCatalogSnapshot != "" {
		snapshot, err := canonical.Scan(root)
		if err != nil {
			return Receipt{}, err
		}
		if snapshot.CatalogSnapshot != set.BaseCatalogSnapshot {
			return Receipt{}, ErrConflict
		}
	}
	changes := append([]Change(nil), set.Changes...)
	sortChanges(changes)
	for _, item := range changes {
		actual, err := digestAt(root, item.Path)
		if err != nil {
			return Receipt{}, err
		}
		if actual != item.BeforeDigest {
			return Receipt{}, fmt.Errorf("%w: %s", ErrConflict, item.Path)
		}
	}
	if set.ProposalID != "" {
		if set.ProposalDigest == "" || proposalDigest(set) != set.ProposalDigest {
			return Receipt{}, ErrConflict
		}
	}
	resultSnapshot, err := validateVirtualSnapshot(root, changes)
	if err != nil {
		return Receipt{}, err
	}
	return commitPrepared(root, set, changes, resultSnapshot.CatalogSnapshot, options)
}

// LookupOperation returns an already-applied result before callers inspect the
// current entity state. This is required for safe retries of creates and state
// transitions whose successful result makes ordinary preconditions false.
func LookupOperation(root string, set WriteSet) (Receipt, bool, error) {
	if set.Command == "" || set.IdempotencyKey == "" || len(set.IdempotencyKey) > 512 || strings.ContainsAny(set.IdempotencyKey, "\r\n\x00") || !validOptionalDigest(set.RequestDigest) || set.RequestDigest == "" {
		return Receipt{}, false, errors.New("valid command, idempotency key, and normalized request digest are required")
	}
	lock, err := AcquireSharedLock(context.Background(), root, DefaultLockTimeout)
	if err != nil {
		return Receipt{}, false, err
	}
	defer lock.Unlock()
	return existingOperation(root, set)
}

func validWriteSet(set WriteSet, requireOperation bool) error {
	if requireOperation && !validOpaqueID(set.OperationID) {
		return errors.New("operation ID must be a non-empty opaque identifier")
	}
	if set.Command == "" || strings.ContainsRune(set.Command, '\x00') || len(set.Changes) == 0 {
		return errors.New("command and at least one change are required")
	}
	if set.ProposalID != "" && !validOpaqueID(set.ProposalID) {
		return errors.New("proposal ID is invalid")
	}
	if set.ProposalDigest != "" && !validOptionalDigest(set.ProposalDigest) {
		return errors.New("proposal digest is invalid")
	}
	if len(set.Changes) > maxChanges {
		return fmt.Errorf("mutation exceeds %d changed paths", maxChanges)
	}
	if set.IdempotencyKey != "" && (len(set.IdempotencyKey) > 512 || strings.ContainsAny(set.IdempotencyKey, "\r\n\x00")) {
		return errors.New("idempotency key is invalid")
	}
	if !validOptionalDigest(set.RequestDigest) {
		return errors.New("request digest is invalid")
	}
	seen := make(map[string]bool, len(set.Changes))
	total := 0
	for _, item := range set.Changes {
		if !canonicalPath(item.Path) || seen[item.Path] {
			return fmt.Errorf("invalid or duplicate canonical path: %s", item.Path)
		}
		if item.Delete && len(item.Contents) != 0 {
			return fmt.Errorf("delete change contains an after-image: %s", item.Path)
		}
		if len(item.Contents) > maxChangeBytes {
			return fmt.Errorf("change exceeds size limit: %s", item.Path)
		}
		total += len(item.Contents)
		seen[item.Path] = true
	}
	if total > maxTransactionBytes {
		return errors.New("mutation exceeds total size limit")
	}
	return nil
}

func canonicalPath(path string) bool {
	if path == ".gitignore" || path == ".skillhub/schema-version" {
		return true
	}
	if path == "" || strings.Contains(path, `\`) || strings.HasPrefix(path, ".") || strings.HasPrefix(path, "/") || strings.ContainsRune(path, '\x00') {
		return false
	}
	parts := strings.Split(path, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return !strings.HasPrefix(path, "history/operations/")
}

func validOpaqueID(value string) bool {
	if value == "" || len(value) > 128 || value == "." || value == ".." {
		return false
	}
	for _, r := range value {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

func validOptionalDigest(value string) bool {
	if value == "" {
		return true
	}
	if len(value) != len("sha256:")+sha256.Size*2 || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil && strings.ToLower(value) == value
}

func sortChanges(changes []Change) {
	sort.Slice(changes, func(i, j int) bool { return changes[i].Path < changes[j].Path })
}

func proposalDigest(set WriteSet) string {
	changes := append([]Change(nil), set.Changes...)
	sortChanges(changes)
	hash := sha256.New()
	fmt.Fprintf(hash, "%s\x00%s\x00%s\x00%s\n", set.Command, set.IdempotencyKey, set.RequestDigest, set.BaseCatalogSnapshot)
	for _, item := range changes {
		fmt.Fprintf(hash, "%s\x00%s\x00%t\x00%s\n", item.Path, item.BeforeDigest, item.Delete, digest(item.Contents))
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil))
}

// requestDigest identifies the normalized command payload. Planning artifacts,
// operation IDs, idempotency keys, snapshots, and optimistic before-digests are
// deliberately excluded so a retry after a lost response may replan against the
// resulting state and still recover the original receipt.
func requestDigest(set WriteSet) string {
	if set.RequestDigest != "" {
		return set.RequestDigest
	}
	hash := sha256.New()
	fmt.Fprintf(hash, "%s\n", set.Command)
	changes := append([]Change(nil), set.Changes...)
	sortChanges(changes)
	for _, item := range changes {
		fmt.Fprintf(hash, "%s\x00%t\x00%s\n", item.Path, item.Delete, digest(item.Contents))
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil))
}

func effectiveIdempotencyKey(set WriteSet) string {
	if set.IdempotencyKey != "" {
		return set.IdempotencyKey
	}
	return set.OperationID
}

func digest(contents []byte) string {
	sum := sha256.Sum256(contents)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func allocateOperationID() (string, error) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", fmt.Errorf("allocate operation ID: %w", err)
	}
	return "OP-" + hex.EncodeToString(random[:]), nil
}

// operationTime is captured once so receipt path and occurred_at agree.
func operationTime() time.Time { return time.Now().UTC() }
