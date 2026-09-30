package insight

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"time"
)

const proposalLifetime = 24 * time.Hour

// RuntimeProposal is the exact immutable preview. It is disposable but must be
// persisted between preview and confirmation; durable audit records are written on apply.
type RuntimeProposal struct {
	Version          int                    `json:"version"`
	CreatedAt        time.Time              `json:"created_at"`
	ExpiresAt        time.Time              `json:"expires_at"`
	ID               string                 `json:"id"`
	Digest           string                 `json:"digest"`
	InsightID        string                 `json:"insight_id"`
	SkillID          string                 `json:"skill_id"`
	BaseSnapshot     string                 `json:"base_snapshot"`
	PathPins         []PathPin              `json:"path_pins"`
	Mappings         []SourceToLocalMapping `json:"mappings"`
	FullDiff         string                 `json:"full_diff"`
	MutationProposal json.RawMessage        `json:"mutation_proposal"`
}

func StoreRuntimeProposal(root string, proposal RuntimeProposal) error {
	if proposal.Version != 1 || !validID(proposal.ID) || !digestPattern.MatchString(proposal.Digest) || !digestPattern.MatchString(proposal.BaseSnapshot) || !proposal.ExpiresAt.After(proposal.CreatedAt) || proposal.ExpiresAt.Sub(proposal.CreatedAt) > proposalLifetime || len(proposal.PathPins) == 0 || len(proposal.Mappings) == 0 || !json.Valid(proposal.MutationProposal) {
		return errors.New("runtime application proposal pins are incomplete or invalid")
	}
	for _, pin := range proposal.PathPins {
		if !safePath(pin.Path) || !validOptionalDigest(pin.Before) || !digestPattern.MatchString(pin.After) || pin.Before == pin.After {
			return errors.New("runtime application proposal contains an invalid path pin")
		}
	}
	for _, mapping := range proposal.Mappings {
		if !validID(mapping.ObservationID) || !safePath(mapping.ArtifactPath) || strings.TrimSpace(mapping.Concept) == "" {
			return errors.New("runtime application proposal contains an invalid mapping")
		}
	}
	handle, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer handle.Close()
	if err := handle.MkdirAll("runtime/insight-proposals", 0o700); err != nil {
		return fmt.Errorf("create proposal directory: %w", err)
	}
	for _, path := range []string{"runtime", "runtime/insight-proposals"} {
		info, statErr := handle.Lstat(path)
		if statErr != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("unsafe runtime proposal path: %s", path)
		}
	}
	if err := handle.Chmod("runtime/insight-proposals", 0o700); err != nil {
		return err
	}
	data, err := json.Marshal(proposal)
	if err != nil {
		return err
	}
	if len(data) > 64<<20 {
		return errors.New("runtime application proposal exceeds size limit")
	}
	path := runtimeProposalPath(proposal.ID)
	if _, err := handle.Lstat(path); err == nil {
		return errors.New("immutable application proposal already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	temporary := path + ".tmp"
	_ = handle.Remove(temporary)
	file, err := handle.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err = file.Write(append(data, '\n')); err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = handle.Remove(temporary)
		return err
	}
	if err := handle.Rename(temporary, path); err != nil {
		_ = handle.Remove(temporary)
		return err
	}
	return nil
}

func LoadRuntimeProposal(root, id string, now time.Time) (RuntimeProposal, error) {
	if !validID(id) {
		return RuntimeProposal{}, errors.New("application proposal ID is invalid")
	}
	handle, err := os.OpenRoot(root)
	if err != nil {
		return RuntimeProposal{}, err
	}
	defer handle.Close()
	path := runtimeProposalPath(id)
	info, err := handle.Lstat(path)
	if err != nil {
		return RuntimeProposal{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || (runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0) || info.Size() > 64<<20 {
		return RuntimeProposal{}, errors.New("application proposal artifact is unsafe")
	}
	data, err := handle.ReadFile(path)
	if err != nil {
		return RuntimeProposal{}, err
	}
	var proposal RuntimeProposal
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&proposal); err != nil {
		return RuntimeProposal{}, fmt.Errorf("parse application proposal: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return RuntimeProposal{}, errors.New("application proposal must contain exactly one JSON object")
	}
	if proposal.Version != 1 || proposal.ID != id || !digestPattern.MatchString(proposal.Digest) || !digestPattern.MatchString(proposal.BaseSnapshot) || !proposal.ExpiresAt.After(proposal.CreatedAt) || proposal.ExpiresAt.Sub(proposal.CreatedAt) > proposalLifetime || len(proposal.PathPins) == 0 || len(proposal.Mappings) == 0 || !json.Valid(proposal.MutationProposal) {
		return RuntimeProposal{}, errors.New("application proposal artifact is invalid")
	}
	for _, pin := range proposal.PathPins {
		if !safePath(pin.Path) || !validOptionalDigest(pin.Before) || !digestPattern.MatchString(pin.After) || pin.Before == pin.After {
			return RuntimeProposal{}, errors.New("application proposal artifact contains an invalid path pin")
		}
	}
	for _, mapping := range proposal.Mappings {
		if !validID(mapping.ObservationID) || !safePath(mapping.ArtifactPath) || strings.TrimSpace(mapping.Concept) == "" {
			return RuntimeProposal{}, errors.New("application proposal artifact contains an invalid mapping")
		}
	}
	if !now.UTC().Before(proposal.ExpiresAt) {
		return RuntimeProposal{}, errors.New("application proposal expired")
	}
	return proposal, nil
}

func NewRuntimeProposal(now time.Time) (time.Time, time.Time) {
	created := now.UTC()
	return created, created.Add(proposalLifetime)
}
func runtimeProposalPath(id string) string { return "runtime/insight-proposals/" + id + ".json" }
