package skill

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/mutation"
)

const proposalLifetime = 24 * time.Hour

type proposalArtifact struct {
	Version        int               `json:"version"`
	Kind           ProposalKind      `json:"kind,omitempty"`
	CreatedAt      time.Time         `json:"created_at"`
	ExpiresAt      time.Time         `json:"expires_at"`
	ID             string            `json:"id"`
	Digest         string            `json:"digest"`
	BaseSnapshot   string            `json:"base_snapshot"`
	SkillID        string            `json:"skill_id"`
	Command        string            `json:"command"`
	Summary        DiffSummary       `json:"summary"`
	RoutingImpact  *RoutingImpact    `json:"routing_impact,omitempty"`
	WriteSet       mutation.WriteSet `json:"write_set"`
	AlreadyApplied *mutation.Receipt `json:"already_applied,omitempty"`
	RecoveryID     string            `json:"recovery_id,omitempty"`
}

// StoreProposal persists only the bounded, exact mutation proposal under the
// disposable runtime tree. Canonical paths remain workspace-relative.
func StoreProposal(root string, proposal Proposal, now time.Time) error {
	if proposal.ID == "" || proposal.Digest == "" || proposal.BaseSnapshot == "" {
		return errors.New("proposal pins are incomplete")
	}
	rootHandle, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer rootHandle.Close()
	if err := rootHandle.MkdirAll("runtime/proposals", 0o700); err != nil {
		return fmt.Errorf("create proposal runtime directory: %w", err)
	}
	if err := rejectSymlink(rootHandle, "runtime"); err != nil {
		return err
	}
	if err := rejectSymlink(rootHandle, "runtime/proposals"); err != nil {
		return err
	}
	if err := rootHandle.Chmod("runtime/proposals", 0o700); err != nil {
		return fmt.Errorf("restrict proposal runtime directory: %w", err)
	}
	if err := cleanupExpiredProposals(rootHandle, now.UTC()); err != nil {
		return err
	}
	createdAt := now.UTC()
	lifetime := proposalLifetime
	if !proposal.CreatedAt.IsZero() && proposal.ExpiresAt.After(proposal.CreatedAt) {
		lifetime = proposal.ExpiresAt.Sub(proposal.CreatedAt)
	}
	expiresAt := createdAt.Add(lifetime)
	kind := proposal.Kind
	if kind == "" {
		kind = ProposalKindLifecycle
	}
	artifact := proposalArtifact{
		Version: 1, Kind: kind, CreatedAt: createdAt, ExpiresAt: expiresAt,
		ID: proposal.ID, Digest: proposal.Digest, BaseSnapshot: proposal.BaseSnapshot,
		SkillID: proposal.SkillID, Command: proposal.Command, Summary: proposal.Summary,
		RoutingImpact: proposal.RoutingImpact,
		WriteSet:      proposal.planned.WriteSet, AlreadyApplied: proposal.alreadyApplied,
		RecoveryID: proposal.RecoveryID,
	}
	data, err := json.Marshal(artifact)
	if err != nil {
		return err
	}
	if len(data) > 64<<20 {
		return errors.New("proposal artifact exceeds size limit")
	}
	data = append(data, '\n')
	path := proposalArtifactPath(proposal.ID)
	temporary := path + ".tmp"
	_ = rootHandle.Remove(temporary)
	file, err := rootHandle.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create proposal artifact: %w", err)
	}
	if _, err = file.Write(data); err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = rootHandle.Remove(temporary)
		return err
	}
	if err := rootHandle.Rename(temporary, path); err != nil {
		_ = rootHandle.Remove(temporary)
		return fmt.Errorf("publish proposal artifact: %w", err)
	}
	return nil
}

// LoadProposal loads the exact previously displayed proposal. It never
// regenerates canonical contents or invokes an editor.
func LoadProposal(root, id string, now time.Time) (Proposal, error) {
	if !validProposalID(id) {
		return Proposal{}, errors.New("proposal ID is invalid")
	}
	rootHandle, err := os.OpenRoot(root)
	if err != nil {
		return Proposal{}, err
	}
	defer rootHandle.Close()
	path := proposalArtifactPath(id)
	info, err := rootHandle.Lstat(path)
	if err != nil {
		return Proposal{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || (runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0) {
		return Proposal{}, errors.New("proposal artifact is not a restrictive regular file")
	}
	data, err := rootHandle.ReadFile(path)
	if err != nil {
		return Proposal{}, err
	}
	if len(data) > 64<<20 {
		return Proposal{}, errors.New("proposal artifact exceeds size limit")
	}
	var artifact proposalArtifact
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&artifact); err != nil {
		return Proposal{}, fmt.Errorf("parse proposal artifact: %w", err)
	}
	if artifact.Version != 1 || artifact.ID != id || artifact.Digest == "" || artifact.BaseSnapshot == "" || !artifact.ExpiresAt.After(artifact.CreatedAt) {
		return Proposal{}, errors.New("proposal artifact is invalid")
	}
	if artifact.Kind == "" {
		artifact.Kind = ProposalKindLifecycle
	} else if artifact.Kind != ProposalKindLifecycle && artifact.Kind != ProposalKindAdd {
		return Proposal{}, fmt.Errorf("proposal artifact has unknown kind %q", artifact.Kind)
	}
	if !now.UTC().Before(artifact.ExpiresAt) {
		_ = rootHandle.Remove(path)
		return Proposal{}, errors.New("proposal artifact expired")
	}
	planned := mutation.Proposal{ID: artifact.ID, Digest: artifact.Digest, BaseCatalogSnapshot: artifact.BaseSnapshot, WriteSet: artifact.WriteSet}
	return Proposal{
		ID: artifact.ID, Kind: artifact.Kind, Digest: artifact.Digest, BaseSnapshot: artifact.BaseSnapshot,
		SkillID: artifact.SkillID, Command: artifact.Command, Summary: artifact.Summary,
		RoutingImpact: artifact.RoutingImpact, RecoveryID: artifact.RecoveryID, CreatedAt: artifact.CreatedAt, ExpiresAt: artifact.ExpiresAt,
		planned: planned, alreadyApplied: artifact.AlreadyApplied,
	}, nil
}

func cleanupExpiredProposals(root *os.Root, now time.Time) error {
	entries, err := fs.ReadDir(root.FS(), "runtime/proposals")
	_ = cleanupExpiredRecoveries(root, now)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		path := "runtime/proposals/" + entry.Name()
		if entry.Type()&os.ModeSymlink != 0 || entry.IsDir() {
			return fmt.Errorf("unsafe proposal artifact: %s", entry.Name())
		}
		if strings.HasSuffix(entry.Name(), ".tmp") {
			_ = root.Remove(path)
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".json") || !validProposalID(strings.TrimSuffix(entry.Name(), ".json")) {
			return fmt.Errorf("invalid proposal artifact name: %s", entry.Name())
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Size() > 64<<20 {
			return fmt.Errorf("proposal artifact exceeds size limit: %s", entry.Name())
		}
		data, readErr := root.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		var header struct {
			ExpiresAt time.Time `json:"expires_at"`
		}
		if json.Unmarshal(data, &header) == nil && !header.ExpiresAt.IsZero() && !now.Before(header.ExpiresAt) {
			if err := root.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
	}
	return nil
}

func proposalArtifactPath(id string) string { return "runtime/proposals/" + id + ".json" }

func validProposalID(id string) bool {
	if id == "" || len(id) > 128 || id == "." || id == ".." {
		return false
	}
	for _, r := range id {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

func rejectSymlink(root *os.Root, path string) error {
	info, err := root.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("unsafe runtime path: %s", path)
	}
	return nil
}
func recoveryArtifactPath(recoveryID string) string {
	return "runtime/edits/" + recoveryID + ".md"
}

func validRecoveryID(id string) bool {
	if len(id) < 5 || len(id) > 128 {
		return false
	}
	for _, ch := range id {
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_') {
			return false
		}
	}
	return true
}

// SaveEditorRecovery persists edited bytes into a private recovery file associated with proposalID.
// It returns an opaque recovery ID and writes no absolute paths to the artifact.
func SaveEditorRecovery(root, proposalID string, content []byte, now time.Time) (string, error) {
	if len(content) > 64<<20 {
		return "", errors.New("recovery content exceeds size limit")
	}
	rootHandle, err := os.OpenRoot(root)
	if err != nil {
		return "", err
	}
	defer rootHandle.Close()

	if err := rootHandle.MkdirAll("runtime/edits", 0o700); err != nil {
		return "", fmt.Errorf("create recovery runtime directory: %w", err)
	}
	if err := rejectSymlink(rootHandle, "runtime"); err != nil {
		return "", err
	}
	if err := rejectSymlink(rootHandle, "runtime/edits"); err != nil {
		return "", err
	}
	if err := rootHandle.Chmod("runtime/edits", 0o700); err != nil {
		return "", fmt.Errorf("restrict recovery runtime directory: %w", err)
	}
	_ = cleanupExpiredRecoveries(rootHandle, now.UTC())

	sum := sha256.Sum256(content)
	contentHash := hex.EncodeToString(sum[:])[:12]
	recoveryID := fmt.Sprintf("REC-%s-%s", proposalID, contentHash)
	if !validRecoveryID(recoveryID) {
		recoveryID = fmt.Sprintf("REC-%s", contentHash)
	}

	path := recoveryArtifactPath(recoveryID)
	temporary := path + ".tmp"
	_ = rootHandle.Remove(temporary)
	file, err := rootHandle.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", fmt.Errorf("create recovery artifact: %w", err)
	}
	if _, err = file.Write(content); err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = rootHandle.Remove(temporary)
		return "", err
	}
	if err := rootHandle.Rename(temporary, path); err != nil {
		_ = rootHandle.Remove(temporary)
		return "", fmt.Errorf("publish recovery artifact: %w", err)
	}
	return recoveryID, nil
}

// ReadEditorRecovery reads recovery content by opaque recovery ID.
func ReadEditorRecovery(root, recoveryID string) ([]byte, error) {
	if !validRecoveryID(recoveryID) {
		return nil, errors.New("recovery ID is invalid")
	}
	rootHandle, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer rootHandle.Close()
	path := recoveryArtifactPath(recoveryID)
	info, err := rootHandle.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || (runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0) {
		return nil, errors.New("recovery artifact is not a restrictive regular file")
	}
	data, err := rootHandle.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) > 64<<20 {
		return nil, errors.New("recovery artifact exceeds size limit")
	}
	return data, nil
}

// DeleteEditorRecovery removes a recovery artifact by opaque recovery ID.
func DeleteEditorRecovery(root, recoveryID string) error {
	if !validRecoveryID(recoveryID) {
		return errors.New("recovery ID is invalid")
	}
	rootHandle, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer rootHandle.Close()
	return rootHandle.Remove(recoveryArtifactPath(recoveryID))
}

// CleanupExpiredRecoveries removes recovery files older than the proposal TTL.
func CleanupExpiredRecoveries(root string, now time.Time) error {
	rootHandle, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer rootHandle.Close()
	return cleanupExpiredRecoveries(rootHandle, now.UTC())
}

func cleanupExpiredRecoveries(root *os.Root, now time.Time) error {
	entries, err := fs.ReadDir(root.FS(), "runtime/edits")
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		path := "runtime/edits/" + entry.Name()
		if entry.Type()&os.ModeSymlink != 0 {
			_ = root.Remove(path)
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if now.Sub(info.ModTime().UTC()) > proposalLifetime {
			_ = root.Remove(path)
		}
	}
	return nil
}

// ProposalHandler is a confirmation handler for a specific proposal kind.
type ProposalHandler func(ctx context.Context, root string, proposal Proposal, pins mutation.Confirmation) (MutationResult, error)

// ProposalDispatcher dispatches proposal confirmation by proposal kind.
type ProposalDispatcher struct {
	handlers map[ProposalKind]ProposalHandler
}

// NewProposalDispatcher creates an empty proposal dispatcher.
func NewProposalDispatcher() *ProposalDispatcher {
	return &ProposalDispatcher{
		handlers: make(map[ProposalKind]ProposalHandler),
	}
}

// Register registers a handler for a proposal kind.
func (d *ProposalDispatcher) Register(kind ProposalKind, handler ProposalHandler) {
	d.handlers[kind] = handler
}

// Dispatch executes the confirmation handler registered for the proposal kind.
// If no handler is registered and the proposal is a lifecycle proposal, it executes Manager.Confirm.
func (d *ProposalDispatcher) Dispatch(ctx context.Context, root string, proposal Proposal, pins mutation.Confirmation) (MutationResult, error) {
	handler, ok := d.handlers[proposal.Kind]
	if !ok {
		if proposal.Kind == ProposalKindLifecycle || proposal.Kind == "" {
			return (Manager{}).Confirm(ctx, root, proposal)
		}
		return MutationResult{}, fmt.Errorf("unsupported proposal kind %q", proposal.Kind)
	}
	return handler(ctx, root, proposal, pins)
}
