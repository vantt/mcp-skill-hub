# Host runtime directory access (R1) report

Status: done, uncommitted.

## Verified host keys
- Claude Code: `permissions.additionalDirectories` in `.claude/settings.json` / `~/.claude/settings.json` (grants file access only; project entries apply after workspace trust). https://code.claude.com/docs/en/permissions , https://code.claude.com/docs/en/settings
- Codex: `sandbox_workspace_write.writable_roots`, array of strings (no read-only variant exists). https://learn.chatgpt.com/docs/config-file/config-reference (redirected from developers.openai.com/codex/config-reference)
- Gemini CLI: `context.includeDirectories`, array of strings. https://geminicli.com/docs/reference/configuration/

## Behavior
Directories `<workspace>/runtime/{cache/skills,envs,config}` are added on `skillhub connect` (project and -g) and by `doctor --fix --yes`. Doctor needed no new code: it plans from the same inspection, so missing entries show as an action-required item. No disconnect path exists in the codebase.

## Files
- New: internal/hostintegration/runtime_access.go, runtime_access_test.go
- Edited (config wiring only, no bootstrap text): hostintegration/{types,integration,files,toml_update}.go, integration_test.go and scope_test.go (counts), app/workspace.go (kind list), app/workspace_test.go (counts), delivery/cli/connect.go (label).

Tests: `go test ./internal/hostintegration/... ./internal/delivery/cli/... ./internal/app/...` pass; go vet and gofmt clean.

Update: Claude project scope now writes `.claude/settings.local.json`; entries already in `.claude/settings.json` also satisfy doctor. User scope unchanged.
