# Phase 4: Docs, repo AGENTS.md, release steps

## Context (current lines that describe the block or the protocol)

| File:line | Today | Change to |
|-----------|-------|-----------|
| `README.md:170`, `README.vi.md:172` | The sample output lists `CLAUDE.md` | Drop `CLAUDE.md` from the sample connect output |
| `docs/user-guide.md:40,66` | Sample output with `CLAUDE.md` | Drop it |
| `docs/user-guide.md:78-79` | The file table says connect writes "a short block between `skillhub:bootstrap` comments" into the three files | Remove those cells. Add one line saying connect writes no instruction files, and an old Skill Hub block is removed by `doctor --fix` |
| `docs/user-guide.md:86` | "Skill Hub manages only the marked block" | "Skill Hub only removes its old marked block; your text stays byte-for-byte" |
| `docs/user-guide.md:119` | `--remove` takes out the marked block | Keep. It still removes an old block. Reword to "an old marked block, if one is left" |
| `docs/user-guide.md:135` | "the connection block in `CLAUDE.md` carries that instruction" | "the Skill Hub MCP server sends that instruction when your agent connects (MCP server instructions)", plus one sentence from phase 3 naming the clients known to read it, and that a client which ignores server instructions gets the tools without the protocol |
| `docs/remote-hub-guide.md:163,205,543` | The output sample, the "Instruction block ... the same" row, and the removal sample | Drop the sample entries. Change the table row to "Instructions: sent by the proxy's MCP server; no file". The removal sample shows `CLAUDE.md` only when an old block was there. Add: "`doctor` reports an old block in a remote-connected project; run the `connect --hub` command it prints to remove it." |
| `docs/design/02-agent-hub-protocol.md:56-107` (Vietnamese) | Section 2.2 explains installing the versioned bootstrap block, and quotes the whole v1 block (lines 84-107) | Rewrite 2.2 in a few lines (Vietnamese): the protocol travels in MCP server `instructions`, generated from the registered tools in `internal/delivery/mcpserver/instructions.go`. `connect` writes no instruction file, and `doctor --fix` removes an old `skillhub:bootstrap` block. Delete the quoted block. Point to the generator rather than copying the text. Fix the `doctor` checklist line at `:70` to "no obsolete bootstrap block" |
| `docs/use-cases/01-agent-runtime-protocol.md:51-56` (Vietnamese) | "Bootstrap & Hook" stage: bootstrap text in AGENTS.md/CLAUDE.md | Say the protocol comes from MCP server instructions at connect time |
| `internal/delivery/cli/help.go:86` | "Writes the MCP registration, the Skill Hub instruction block, and the system-curator skill" | "Writes the MCP registration and the system-curator skill, and removes an old Skill Hub instruction block" |
| `internal/delivery/cli/help.go:117` | `--remove` "... its instruction block ..." | "... any old instruction block ..." |
| `AGENTS.md:62-86` (this repo) | v1 block | Delete the block and the blank line before it (repo content; see open question 3) |

Not changed: `docs/distillery/**` (source records, non-goal), and `docs/mcp-compatibility-matrix.json` (no
instruction fields; verified with `grep -i instruction`).

## Steps

1. Edit each row above. Keep the wording plain, and do not copy the instructions text into any doc.
2. Check help text tests: `go test ./internal/delivery/cli -run 'Help' -count=1`.
3. Check for leftovers:
   `rg -n "skillhub:bootstrap|instruction block|bootstrap instruction" README*.md docs/*.md docs/design docs/use-cases internal/delivery/cli/help.go AGENTS.md`.
   Every hit must describe removal only.
4. Run `make check`.

## Manual steps for the user after release (not done by the implementer)

1. Update the binary, then in each locally connected project and for the user scope run `skillhub doctor` and then
   `skillhub doctor --fix`. This removes the block from `~/.claude/CLAUDE.md`, `~/.codex/AGENTS.md`,
   `~/.gemini/GEMINI.md` and project files.
2. For remote-connected projects, run the `skillhub connect --hub ...` command that `doctor` prints.
3. In the hub folder, commit the instruction-file change that `hub_uncommitted` reports.
4. Restart Claude Code, run `/mcp`, and check that a new task makes the agent call `skill_resolve` with no block
   loaded.

## Risks

- **Docs drift from the generator.** Docs point to `instructions.go` and do not copy its text.
- **The Vietnamese design doc** must stay in Vietnamese. Keep that doc's terminology.

## Rollback

Revert this phase's commit. Docs only, plus `AGENTS.md`.
