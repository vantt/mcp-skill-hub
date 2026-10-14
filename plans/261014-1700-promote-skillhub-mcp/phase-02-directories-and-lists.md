# Phase 2: Directories and lists

## Context
[plan.md](plan.md). People looking for MCP servers and agent skills browse directories and "awesome" lists. This
phase is low effort and slow-burning.

## Requirements
- Build a target table in `plans/reports/` with: name, URL, what it lists, how to submit (pull request, form, automatic
  from GitHub), the rules and format it asks for, whether it accepts a tool like this one (a hub and curator rather than a
  single server), status, and the date checked. **Verify each row on the target's own page before submitting; do not
  rely on memory of what a list accepts.**
- Candidate targets to check: the official MCP registry, MCP server directories (for example Smithery, Glama, mcp.so,
  PulseMCP), awesome lists for MCP servers, for Claude Code, and for agent skills, GitHub topic pages, and lists of
  Go command-line tools.
- For each pull request: follow the list's format exactly, one line, alphabetical or categorical placement as required,
  honest description with the same positioning sentence, no marketing adjectives.
- Track submissions and answers in the same table; follow up once after two weeks, then leave it.

## Files
`plans/reports/promote-targets-<date>.md` (new). Pull request texts drafted in the same report. No code changes, unless a
registry needs a manifest file in the repository (then a small, reviewed commit: for example a server manifest the
official registry asks for).

## Steps
1. Research and fill the target table (can use a researcher agent; read-only).
2. Rank by fit and effort; the owner picks.
3. Draft each submission; the owner submits it (pull requests are made from the owner's account), or says "submit".
4. Update the table as answers arrive.

## Validation
At least three listings are live or three pull requests are open by day 30, each accurate and in the list's own format.

## Risk and rollback
A rejected submission costs nothing; do not argue with maintainers. Never submit to a list that forbids self-submission.
