// Package systemskills provides bundled, non-operational system skill assets.
package systemskills

import (
	"crypto/sha256"
	_ "embed"
	"fmt"
)

const (
	// CuratorSkillID is the stable identity used by host and distribution code.
	CuratorSkillID = "system-curator"
	// CuratorSkillVersion versions the bundled instructions independently of the binary.
	CuratorSkillVersion = "1.5.3"
	// CuratorContractVersion versions the compatible tool and behavior contract.
	CuratorContractVersion = "2"
	// CuratorActivationPolicy prevents curation guidance from becoming an implicit
	// primary procedure for ordinary substantive work.
	CuratorActivationPolicy = "explicit-only"
	// CuratorCoordinationBoundary describes hosts without native activation hooks.
	CuratorCoordinationBoundary = "instruction-only-best-effort"
)

// CuratorSkill is the bundled System Curator asset. Domain mutation remains in
// application services; this asset is guidance only.
//
//go:embed curator/SKILL.md
var CuratorSkill string

var curatorCompatibleTools = []string{
	"hub_status",
	"source_list",
	"source_check",
	"skill_upstream_status",
	"source_watch_preview",
	"source_watch_confirm",
	"source_import_preview",
	"source_import_confirm",
	"skill_add_preview",
	"skill_add_confirm",
	"skill_create_preview",
	"skill_create_confirm",
	"skill_transition_preview",
	"skill_transition_confirm",
	"skill_list",
	"skill_review",
	"skill_update_preview",
	"skill_update_confirm",
	"routing_evaluate",
	"curation_session_record",
	"workspace_validate",
	"workspace_rebuild",
	"workspace_diff",
}

// CompatibilityMetadata is the versioned host contract for the bundled
// curator. The boolean boundaries are declarative: the package exposes no
// mutation or tool-execution capability.
type CompatibilityMetadata struct {
	SkillID                    string
	SkillVersion               string
	ContractVersion            string
	ActivationPolicy           string
	CoordinationBoundary       string
	CompatibleTools            []string
	InstructionOnly            bool
	BestEffortCoordination     bool
	RequiresApplicationService bool
}

// BundledSkill is a distribution-ready snapshot of an embedded system skill.
type BundledSkill struct {
	Metadata     CompatibilityMetadata
	Instructions string
	Digest       string
}

// CuratorMetadata returns a defensive copy of the curator compatibility
// contract so callers cannot mutate package-level metadata.
func CuratorMetadata() CompatibilityMetadata {
	tools := append([]string(nil), curatorCompatibleTools...)
	return CompatibilityMetadata{
		SkillID:                    CuratorSkillID,
		SkillVersion:               CuratorSkillVersion,
		ContractVersion:            CuratorContractVersion,
		ActivationPolicy:           CuratorActivationPolicy,
		CoordinationBoundary:       CuratorCoordinationBoundary,
		CompatibleTools:            tools,
		InstructionOnly:            true,
		BestEffortCoordination:     true,
		RequiresApplicationService: true,
	}
}

// CuratorBundle returns the versioned instructions and compatibility metadata
// for later distribution and host-bootstrap code. The digest pins the exact
// embedded bytes, including frontmatter.
func CuratorBundle() BundledSkill {
	sum := sha256.Sum256([]byte(CuratorSkill))
	return BundledSkill{
		Metadata:     CuratorMetadata(),
		Instructions: CuratorSkill,
		Digest:       fmt.Sprintf("sha256:%x", sum),
	}
}
