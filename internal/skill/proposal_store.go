package skill

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/mutation"
)

const proposalLifetime = 24 * time.Hour

type proposalArtifact struct {
	Version        int               `json:"version"`
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
	artifact := proposalArtifact{
		Version: 1, CreatedAt: createdAt, ExpiresAt: expiresAt,
		ID: proposal.ID, Digest: proposal.Digest, BaseSnapshot: proposal.BaseSnapshot,
		SkillID: proposal.SkillID, Command: proposal.Command, Summary: proposal.Summary,
		RoutingImpact: proposal.RoutingImpact,
		WriteSet:      proposal.planned.WriteSet, AlreadyApplied: proposal.alreadyApplied,
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
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
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
	if !now.UTC().Before(artifact.ExpiresAt) {
		_ = rootHandle.Remove(path)
		return Proposal{}, errors.New("proposal artifact expired")
	}
	planned := mutation.Proposal{ID: artifact.ID, Digest: artifact.Digest, BaseCatalogSnapshot: artifact.BaseSnapshot, WriteSet: artifact.WriteSet}
	return Proposal{
		ID: artifact.ID, Digest: artifact.Digest, BaseSnapshot: artifact.BaseSnapshot,
		SkillID: artifact.SkillID, Command: artifact.Command, Summary: artifact.Summary,
		RoutingImpact: artifact.RoutingImpact, CreatedAt: artifact.CreatedAt, ExpiresAt: artifact.ExpiresAt,
		planned: planned, alreadyApplied: artifact.AlreadyApplied,
	}, nil
}

func cleanupExpiredProposals(root *os.Root, now time.Time) error {
	entries, err := fs.ReadDir(root.FS(), "runtime/proposals")
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
