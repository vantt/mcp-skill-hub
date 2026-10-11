package systemskills

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestCuratorSkillIsEmbeddedAndVersioned(t *testing.T) {
	t.Parallel()

	if strings.TrimSpace(CuratorSkill) == "" {
		t.Fatal("CuratorSkill is empty")
	}

	canonicalSkill, err := os.ReadFile("../../system-skills/curator/SKILL.md")
	if err != nil {
		t.Fatalf("read canonical Curator Skill: %v", err)
	}
	if CuratorSkill != string(canonicalSkill) {
		t.Fatal("embedded Curator Skill differs from the canonical system-skills asset")
	}

	for _, want := range []string{
		"name: " + CuratorSkillID,
		"version: " + CuratorSkillVersion,
		"contract-version: \"" + CuratorContractVersion + "\"",
	} {
		if !strings.Contains(CuratorSkill, want) {
			t.Errorf("CuratorSkill does not declare %q", want)
		}
	}

	bundle := CuratorBundle()
	if bundle.Instructions != CuratorSkill {
		t.Fatal("CuratorBundle instructions differ from embedded content")
	}
	sum := sha256.Sum256([]byte(CuratorSkill))
	wantDigest := fmt.Sprintf("sha256:%x", sum)
	if bundle.Digest != wantDigest {
		t.Fatalf("CuratorBundle digest = %q, want %q", bundle.Digest, wantDigest)
	}
}

func TestCuratorCompatibilityMetadata(t *testing.T) {
	t.Parallel()

	metadata := CuratorMetadata()
	if metadata.SkillID != CuratorSkillID || metadata.SkillVersion != CuratorSkillVersion {
		t.Fatalf("identity metadata = %q@%q", metadata.SkillID, metadata.SkillVersion)
	}
	if metadata.ContractVersion != CuratorContractVersion {
		t.Fatalf("contract version = %q, want %q", metadata.ContractVersion, CuratorContractVersion)
	}
	if metadata.ActivationPolicy != "explicit-only" {
		t.Fatalf("activation policy = %q, want explicit-only", metadata.ActivationPolicy)
	}
	if metadata.CoordinationBoundary != "instruction-only-best-effort" {
		t.Fatalf("coordination boundary = %q", metadata.CoordinationBoundary)
	}
	if !metadata.InstructionOnly || !metadata.BestEffortCoordination || !metadata.RequiresApplicationService {
		t.Fatalf("unsafe curator boundaries: %+v", metadata)
	}

	wantTools := []string{
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
	if strings.Join(metadata.CompatibleTools, "\n") != strings.Join(wantTools, "\n") {
		t.Fatalf("compatible tools do not match the contract:\n got %v\nwant %v", metadata.CompatibleTools, wantTools)
	}

	seen := make(map[string]struct{}, len(metadata.CompatibleTools))
	validName := regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	for _, tool := range metadata.CompatibleTools {
		if !validName.MatchString(tool) {
			t.Errorf("invalid tool name %q", tool)
		}
		if _, exists := seen[tool]; exists {
			t.Errorf("duplicate tool name %q", tool)
		}
		seen[tool] = struct{}{}
		if !strings.Contains(CuratorSkill, "\n"+tool) {
			t.Errorf("compatible tool %q is absent from embedded guidance", tool)
		}
	}

	metadata.CompatibleTools[0] = "mutated_by_caller"
	if CuratorMetadata().CompatibleTools[0] != "hub_status" {
		t.Fatal("CuratorMetadata returned mutable package-level tool metadata")
	}
}

// The curator's SKILL.md is served verbatim, so its frontmatter is where the
// compatibility contract lives; this keeps it equal to CuratorMetadata.
func TestCuratorFrontmatterDeclaresCompatibilityMetadata(t *testing.T) {
	t.Parallel()

	end := strings.Index(CuratorSkill[4:], "\n---\n")
	if !strings.HasPrefix(CuratorSkill, "---\n") || end < 0 {
		t.Fatal("CuratorSkill has no frontmatter")
	}
	var frontmatter struct {
		Name                       string   `yaml:"name"`
		Version                    string   `yaml:"version"`
		ContractVersion            string   `yaml:"contract-version"`
		ActivationPolicy           string   `yaml:"activation-policy"`
		CoordinationBoundary       string   `yaml:"coordination-boundary"`
		InstructionOnly            bool     `yaml:"instruction-only"`
		BestEffortCoordination     bool     `yaml:"best-effort-coordination"`
		RequiresApplicationService bool     `yaml:"requires-application-service"`
		CompatibleTools            []string `yaml:"compatible-tools"`
	}
	if err := yaml.Unmarshal([]byte(CuratorSkill[4:4+end]), &frontmatter); err != nil {
		t.Fatalf("parse frontmatter: %v", err)
	}
	metadata := CuratorMetadata()
	got := CompatibilityMetadata{
		SkillID:                    frontmatter.Name,
		SkillVersion:               frontmatter.Version,
		ContractVersion:            frontmatter.ContractVersion,
		ActivationPolicy:           frontmatter.ActivationPolicy,
		CoordinationBoundary:       frontmatter.CoordinationBoundary,
		CompatibleTools:            frontmatter.CompatibleTools,
		InstructionOnly:            frontmatter.InstructionOnly,
		BestEffortCoordination:     frontmatter.BestEffortCoordination,
		RequiresApplicationService: frontmatter.RequiresApplicationService,
	}
	if !reflect.DeepEqual(got, metadata) {
		t.Fatalf("SKILL.md frontmatter = %#v\nCuratorMetadata  = %#v", got, metadata)
	}
}

// readSkillTree reads every file under root the way a directory go:embed does:
// names that start with "." or "_" are skipped at any depth.
func readSkillTree(t *testing.T, root string) map[string][]byte {
	t.Helper()
	files := map[string][]byte{}
	err := filepath.WalkDir(root, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if name != root && (strings.HasPrefix(entry.Name(), ".") || strings.HasPrefix(entry.Name(), "_")) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, name)
		if err != nil {
			return err
		}
		content, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = content
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return files
}

func TestDistillLabMirrorEqualsSource(t *testing.T) {
	t.Parallel()

	const fix = "copy .agents/skills/distill-lab/ over internal/systemskills/distill-lab/ (the .agents copy is the editable source)"
	source := readSkillTree(t, filepath.Join("..", "..", ".agents", "skills", "distill-lab"))
	mirror := readSkillTree(t, "distill-lab")
	if len(source) == 0 {
		t.Fatal(".agents/skills/distill-lab/ holds no files")
	}
	for path, content := range source {
		got, ok := mirror[path]
		if !ok {
			t.Errorf("%s is missing from the embed mirror; %s", path, fix)
			continue
		}
		if !bytes.Equal(got, content) {
			t.Errorf("%s differs between the source and the embed mirror; %s", path, fix)
		}
	}
	for path := range mirror {
		if _, ok := source[path]; !ok {
			t.Errorf("%s exists only in the embed mirror; %s", path, fix)
		}
	}

	var embedded map[string][]byte
	for _, skill := range SystemSkills() {
		if skill.ID == DistillSkillID {
			embedded = map[string][]byte{}
			for _, file := range skill.Files {
				embedded[file.Path] = file.Content
			}
		}
	}
	if !reflect.DeepEqual(embedded, mirror) {
		t.Fatalf("embedded distill-lab files differ from internal/systemskills/distill-lab/ on disk")
	}
}

func TestSystemSkillsListsBundlesInOrder(t *testing.T) {
	t.Parallel()

	skills := SystemSkills()
	if len(skills) != 2 || skills[0].ID != CuratorSkillID || skills[1].ID != DistillSkillID {
		t.Fatalf("system skill ids = %v, want [%s %s]", skillIDs(skills), CuratorSkillID, DistillSkillID)
	}

	curator, distill := skills[0], skills[1]
	if curator.Version != CuratorSkillVersion || curator.HubOnly || !curator.ServedOverMCP {
		t.Fatalf("curator = %s@%s hubOnly=%v servedOverMCP=%v", curator.ID, curator.Version, curator.HubOnly, curator.ServedOverMCP)
	}
	if len(curator.Files) != 1 || string(curator.Files[0].Content) != CuratorSkill {
		t.Fatal("curator bundle is not exactly the embedded CuratorSkill")
	}
	if distill.Version != DistillSkillVersion || DistillSkillVersion != "0.1.0" || !distill.HubOnly || distill.ServedOverMCP {
		t.Fatalf("distill-lab = %s@%s hubOnly=%v servedOverMCP=%v", distill.ID, distill.Version, distill.HubOnly, distill.ServedOverMCP)
	}

	for _, skill := range skills {
		if len(skill.Files) == 0 || skill.Files[0].Path != "SKILL.md" {
			t.Fatalf("%s: first file is not SKILL.md", skill.ID)
		}
		rest := make([]string, 0, len(skill.Files)-1)
		for _, file := range skill.Files {
			if len(file.Content) == 0 {
				t.Errorf("%s: %s is empty", skill.ID, file.Path)
			}
			if file.Path != "SKILL.md" {
				rest = append(rest, file.Path)
			}
		}
		if !sort.StringsAreSorted(rest) {
			t.Errorf("%s: files after SKILL.md are not sorted: %v", skill.ID, rest)
		}
		if !strings.Contains(string(skill.Files[0].Content), "\nname: "+skill.ID+"\n") {
			t.Errorf("%s: SKILL.md does not declare its name", skill.ID)
		}
	}
}

func TestSystemSkillsReturnsDeepCopies(t *testing.T) {
	t.Parallel()

	first := SystemSkills()
	first[0].ID = "mutated"
	first[1].Files[0].Content[0] = 'X'
	first[1].Files[0].Path = "mutated"
	first[1].Files = first[1].Files[:0]

	second := SystemSkills()
	if second[0].ID != CuratorSkillID {
		t.Fatal("mutating a returned skill changed the next result's id")
	}
	if len(second[1].Files) == 0 || second[1].Files[0].Path != "SKILL.md" || second[1].Files[0].Content[0] != '-' {
		t.Fatal("mutating a returned skill changed the next result's files")
	}
}

func TestKnownFileDigest(t *testing.T) {
	t.Parallel()

	wellFormed := regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	for id, paths := range knownDigests {
		for path, digests := range paths {
			for _, digest := range digests {
				if !wellFormed.MatchString(digest) {
					t.Errorf("%s %s: malformed digest %q", id, path, digest)
				}
				if !KnownFileDigest(id, path, digest) {
					t.Errorf("%s %s: listed digest %q is not recognized", id, path, digest)
				}
			}
		}
	}

	for _, skill := range SystemSkills() {
		for _, file := range skill.Files {
			digest := FileDigest(file.Content)
			if !KnownFileDigest(skill.ID, file.Path, digest) {
				t.Errorf("%s %s: current bundle bytes are not recognized", skill.ID, file.Path)
			}
			if KnownFileDigest(skill.ID, file.Path+".other", digest) {
				t.Errorf("%s %s: digest is recognized under another path", skill.ID, file.Path)
			}
		}
	}

	if CuratorBundle().Digest != FileDigest([]byte(CuratorSkill)) {
		t.Fatal("FileDigest and CuratorBundle disagree on the digest form")
	}
	arbitrary := FileDigest([]byte("content Skill Hub never wrote\n"))
	for _, id := range []string{CuratorSkillID, DistillSkillID, "unknown"} {
		if KnownFileDigest(id, "SKILL.md", arbitrary) {
			t.Errorf("%s: arbitrary digest is recognized", id)
		}
	}
	curatorDigest := FileDigest([]byte(CuratorSkill))
	if KnownFileDigest(DistillSkillID, "SKILL.md", curatorDigest) {
		t.Error("curator bytes are recognized as a distill-lab file")
	}
}

func skillIDs(skills []SystemSkill) []string {
	ids := make([]string, len(skills))
	for i, skill := range skills {
		ids[i] = skill.ID
	}
	return ids
}
