package mcpserver

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/systemskills"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
	"gopkg.in/yaml.v3"
)

func TestCuratorServerBoundaryAndCompatibleTools(t *testing.T) {
	t.Parallel()

	// 1. Verify system-skills/curator/SKILL.md and internal/systemskills/curator/SKILL.md are identical
	root := filepath.Join("..", "..", "..")
	systemSkillsPath := filepath.Join(root, "system-skills", "curator", "SKILL.md")
	internalSkillsPath := filepath.Join(root, "internal", "systemskills", "curator", "SKILL.md")

	systemSkillsData, err := os.ReadFile(systemSkillsPath)
	if err != nil {
		t.Fatalf("read system-skills curator SKILL.md: %v", err)
	}
	internalSkillsData, err := os.ReadFile(internalSkillsPath)
	if err != nil {
		t.Fatalf("read internal systemskills curator SKILL.md: %v", err)
	}

	if string(systemSkillsData) != string(internalSkillsData) {
		t.Fatal("system-skills/curator/SKILL.md and internal/systemskills/curator/SKILL.md differ; both copies must be kept identical")
	}

	if systemskills.CuratorSkill != string(systemSkillsData) {
		t.Fatal("embedded systemskills.CuratorSkill differs from canonical system-skills asset")
	}

	// 2. Parse frontmatter compatible-tools
	end := strings.Index(string(systemSkillsData)[4:], "\n---\n")
	if end < 0 {
		t.Fatal("curator SKILL.md has no terminated frontmatter")
	}
	var fm struct {
		CompatibleTools []string `yaml:"compatible-tools"`
	}
	if err := yaml.Unmarshal(systemSkillsData[4:4+end], &fm); err != nil {
		t.Fatalf("unmarshal curator frontmatter: %v", err)
	}
	if len(fm.CompatibleTools) == 0 {
		t.Fatal("curator declares no compatible-tools")
	}

	compatibleSet := make(map[string]bool, len(fm.CompatibleTools))
	for _, tool := range fm.CompatibleTools {
		compatibleSet[tool] = true
	}

	// 3. Extract backtick tool references from markdown body
	body := string(systemSkillsData[4+end+4:])
	toolRegex := regexp.MustCompile("`([a-z]+(?:_[a-z0-9]+)+)`")
	matches := toolRegex.FindAllStringSubmatch(body, -1)

	usedTools := make(map[string]bool)
	for _, match := range matches {
		name := match[1]
		// Exclude known non-tool keywords, flags, arguments or cross-references
		switch name {
		case "single_step", "multi_step", "skill_id", "source_id", "run_id", "insight_id", "proposal_id",
			"proposal_digest", "base_version", "action_class", "source_action", "crlf_to_lf",
			"skillhub_state_dir", "skillhub_skill_dir", "skillhub_config_dir", "vendor_token",
			"review_required", "when_to_use", "user_invocable", "argument_hint", "requires_application_service",
			"instruction_only", "best_effort_coordination", "activation_policy", "coordination_boundary",
			"install_prose_detected", "missing_runtime_block", "missing_lockfiles", "setup_check",
			"package_json", "cargo_lock", "go_mod", "poetry_lock", "pip_install", "npm_ci", "uv_sync",
			"git_diff", "autoreview_run", "auto_approval", "openclaw_testing", "crabbox_rules",
			"openclaw_pr_maintainer", "check_changed", "check_run_closed", "system_curator":
			continue
		default:
			usedTools[name] = true
		}
	}

	// 4. Start an MCP server and inspect all registered tools
	serverRoot := t.TempDir()
	if _, err := workspace.Apply(serverRoot); err != nil {
		t.Fatal(err)
	}
	adapter, server, err := New(serverRoot, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if adapter == nil || server == nil {
		t.Fatal("nil adapter or server")
	}

	session := newClientSession(t, server)
	listedTools, err := session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("session.ListTools: %v", err)
	}

	registeredTools := make(map[string]bool)
	for _, tool := range listedTools.Tools {
		registeredTools[tool.Name] = true
	}

	// 5. Verify every compatible-tool is registered on the server
	for _, tool := range fm.CompatibleTools {
		if !registeredTools[tool] {
			t.Errorf("curator compatible-tool %q is not registered on the MCP server", tool)
		}
	}

	// 6. Verify every tool used in body is listed in compatible-tools AND registered on server
	for tool := range usedTools {
		if !registeredTools[tool] {
			t.Errorf("curator body uses tool %q, which is not registered on the MCP server", tool)
		}
		if tool != "skill_resolve" && tool != "skill_feedback" && !compatibleSet[tool] {
			t.Errorf("curator body uses tool %q, which is not listed in compatible-tools", tool)
		}
	}

	// 7. Server-boundary profile split test (§5.1, decisions 1-5):
	// runtime = skill_resolve, skill_get, skill_feedback, plus skills extension methods
	// curation = every other tool
	// compatible-tools ⊆ curationTools, runtime ∪ curation = all, no overlap except skills extension methods
	_, runtimeServer, err := New(serverRoot, nil, ProfileRuntime)
	if err != nil {
		t.Fatalf("New ProfileRuntime: %v", err)
	}
	_, curationServer, err := New(serverRoot, nil, ProfileCuration)
	if err != nil {
		t.Fatalf("New ProfileCuration: %v", err)
	}
	_, allServer, err := New(serverRoot, nil, ProfileAll)
	if err != nil {
		t.Fatalf("New ProfileAll: %v", err)
	}

	runtimeSession := newClientSession(t, runtimeServer)
	runtimeListed, err := runtimeSession.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("runtime ListTools: %v", err)
	}
	curationSession := newClientSession(t, curationServer)
	curationListed, err := curationSession.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("curation ListTools: %v", err)
	}
	allSession := newClientSession(t, allServer)
	allListed, err := allSession.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("all ListTools: %v", err)
	}

	runtimeTools := make(map[string]bool)
	for _, tool := range runtimeListed.Tools {
		runtimeTools[tool.Name] = true
	}
	curationTools := make(map[string]bool)
	for _, tool := range curationListed.Tools {
		curationTools[tool.Name] = true
	}
	allTools := make(map[string]bool)
	for _, tool := range allListed.Tools {
		allTools[tool.Name] = true
	}

	wantRuntime := map[string]bool{
		"skill_resolve":  true,
		"skill_get":      true,
		"skill_feedback": true,
	}
	if !reflect.DeepEqual(runtimeTools, wantRuntime) {
		t.Fatalf("runtime profile tools mismatch:\ngot:  %+v\nwant: %+v", runtimeTools, wantRuntime)
	}

	for tool := range runtimeTools {
		if curationTools[tool] {
			t.Errorf("profile tool overlap: tool %q is in both runtime and curation profiles", tool)
		}
	}

	combined := make(map[string]bool)
	for tool := range runtimeTools {
		combined[tool] = true
	}
	for tool := range curationTools {
		combined[tool] = true
	}
	if !reflect.DeepEqual(combined, allTools) {
		t.Fatalf("runtime ∪ curation does not equal all tools:\ncombined: %+v\nall:      %+v", combined, allTools)
	}

	for _, tool := range fm.CompatibleTools {
		if !curationTools[tool] {
			t.Errorf("curator compatible-tool %q is not in curation profile tools", tool)
		}
	}

	if _, err := mcp.CallCustomMethod[*listSkillsParams, *listSkillsResult](t.Context(), runtimeSession, "skills/list", &listSkillsParams{}); err != nil {
		t.Fatalf("runtime skills/list extension method failed: %v", err)
	}
	if _, err := mcp.CallCustomMethod[*listSkillsParams, *listSkillsResult](t.Context(), curationSession, "skills/list", &listSkillsParams{}); err != nil {
		t.Fatalf("curation skills/list extension method failed: %v", err)
	}
}
