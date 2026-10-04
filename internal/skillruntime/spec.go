// Package skillruntime describes what a skill needs from the host machine,
// decides whether its executable content is trusted, and checks a machine
// against those needs. It is transport-neutral: it has no dependency on the
// app, catalog, or delivery layers, and only RunDoctor ever executes anything.
package skillruntime

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// Spec is the parsed runtime block of a skill manifest.
type Spec struct {
	Requires Requires `json:"requires"`
	Setup    Setup    `json:"setup"`
}

// Requires lists host prerequisites. Env holds variable names only; values are
// never stored.
type Requires struct {
	Bins      []Bin    `json:"bins"`
	Env       []string `json:"env"`
	Platforms []string `json:"platforms"`
}

// Bin is one required executable with an optional version constraint such as
// ">=18" or "3.11".
type Bin struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Setup holds the optional install and verification commands. They are shell
// text from metadata and are only ever run by the explicit doctor command.
type Setup struct {
	Command string `json:"command"`
	Check   string `json:"check"`
}

const (
	maxCommandBytes   = 1024
	errSpecValidation = "invalid runtime block"
)

var (
	binNamePattern = regexp.MustCompile(`^[A-Za-z0-9._+-]{1,64}$`)
	envNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,127}$`)
	knownPlatforms = []string{"linux", "darwin", "windows", "freebsd"}
)

// ParseSpec reads the runtime block from a skill manifest encoded as JSON (the
// catalog's content_json). It reports false when the manifest has no runtime
// block. Bins entries may be bare names or {name, version} objects.
func ParseSpec(contentJSON []byte) (Spec, bool, error) {
	var document struct {
		Runtime json.RawMessage `json:"runtime"`
	}
	if err := json.Unmarshal(contentJSON, &document); err != nil {
		return Spec{}, false, fmt.Errorf("decode skill manifest: %w", err)
	}
	raw := bytes.TrimSpace(document.Runtime)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return Spec{}, false, nil
	}
	var block struct {
		Requires *struct {
			Bins      []json.RawMessage `json:"bins"`
			Env       []string          `json:"env"`
			Platforms []string          `json:"platforms"`
		} `json:"requires"`
		Setup *Setup `json:"setup"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&block); err != nil {
		return Spec{}, false, fmt.Errorf("%s: %w", errSpecValidation, err)
	}
	var spec Spec
	if block.Requires != nil {
		for _, entry := range block.Requires.Bins {
			bin, err := parseBin(entry)
			if err != nil {
				return Spec{}, false, err
			}
			spec.Requires.Bins = append(spec.Requires.Bins, bin)
		}
		spec.Requires.Env = block.Requires.Env
		spec.Requires.Platforms = block.Requires.Platforms
	}
	if block.Setup != nil {
		spec.Setup = *block.Setup
	}
	if err := spec.validate(); err != nil {
		return Spec{}, false, err
	}
	return spec, true, nil
}

func parseBin(raw json.RawMessage) (Bin, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) > 0 && trimmed[0] == '"' {
		var name string
		if err := json.Unmarshal(trimmed, &name); err != nil {
			return Bin{}, fmt.Errorf("%s: bins entry: %w", errSpecValidation, err)
		}
		return Bin{Name: name}, nil
	}
	var bin Bin
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&bin); err != nil {
		return Bin{}, fmt.Errorf("%s: bins entries must be a name string or a {name, version} object: %w", errSpecValidation, err)
	}
	return bin, nil
}

func (s Spec) validate() error {
	seen := make(map[string]bool, len(s.Requires.Bins))
	for _, bin := range s.Requires.Bins {
		if !binNamePattern.MatchString(bin.Name) {
			return fmt.Errorf("%s: bin name %q is not a plain executable name", errSpecValidation, bin.Name)
		}
		if seen[bin.Name] {
			return fmt.Errorf("%s: duplicate bin %q", errSpecValidation, bin.Name)
		}
		seen[bin.Name] = true
		if bin.Version != "" {
			if _, err := parseConstraint(bin.Version); err != nil {
				return fmt.Errorf("%s: bin %q: %w", errSpecValidation, bin.Name, err)
			}
		}
	}
	for _, name := range s.Requires.Env {
		if !ValidEnvName(name) {
			return fmt.Errorf("%s: env entries must be variable names", errSpecValidation)
		}
	}
	for _, platform := range s.Requires.Platforms {
		if !slices.Contains(knownPlatforms, platform) {
			return fmt.Errorf("%s: unknown platform %q", errSpecValidation, platform)
		}
	}
	for field, command := range map[string]string{"command": s.Setup.Command, "check": s.Setup.Check} {
		if command == "" {
			continue
		}
		if strings.TrimSpace(command) == "" || strings.ContainsAny(command, "\r\n") || len(command) > maxCommandBytes {
			return fmt.Errorf("%s: setup.%s must be a non-empty single-line string of at most %d bytes", errSpecValidation, field, maxCommandBytes)
		}
	}
	return nil
}

// HasCommands reports whether the spec declares any setup shell command.
func (s Spec) HasCommands() bool {
	return s.Setup.Command != "" || s.Setup.Check != ""
}

// canonical returns a normalized copy: list order carries no meaning, so bins,
// env, and platforms are sorted and nil lists become empty.
func (s Spec) canonical() Spec {
	out := Spec{Setup: s.Setup}
	out.Requires.Bins = append([]Bin{}, s.Requires.Bins...)
	slices.SortFunc(out.Requires.Bins, func(a, b Bin) int { return strings.Compare(a.Name, b.Name) })
	out.Requires.Env = append([]string{}, s.Requires.Env...)
	slices.Sort(out.Requires.Env)
	out.Requires.Platforms = append([]string{}, s.Requires.Platforms...)
	slices.Sort(out.Requires.Platforms)
	return out
}

// CanonicalJSON is the deterministic encoding used for fingerprints and the
// execution digest.
func (s Spec) CanonicalJSON() []byte {
	// Marshalling a struct of strings and slices cannot fail.
	encoded, _ := json.Marshal(s.canonical())
	return encoded
}

// Fingerprint is the lowercase hex SHA-256 of the canonical JSON.
func (s Spec) Fingerprint() string {
	sum := sha256.Sum256(s.CanonicalJSON())
	return hex.EncodeToString(sum[:])
}

// ValidEnvName reports whether name is an acceptable environment variable name.
func ValidEnvName(name string) bool { return envNamePattern.MatchString(name) }
