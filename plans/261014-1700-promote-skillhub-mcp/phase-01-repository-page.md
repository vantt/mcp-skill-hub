# Phase 1: Repository page

## Context
[plan.md](plan.md). The page a visitor sees after clicking from any post or listing decides whether they try the tool.

## Requirements
- **Description (350 characters at most) and homepage.** One sentence that says what it is and why, for example: "Your
  agent skills in one Git repository you own, served to Claude Code, Codex and Gemini CLI over MCP: nothing to install
  per project." Homepage: the latest release page or the README anchor, until there is a site.
- **Topics (up to 20).** `mcp`, `model-context-protocol`, `agent-skills`, `claude-code`, `codex`, `gemini-cli`,
  `ai-agents`, `skills`, `llm-tools`, `git`, `golang`, `cli`, `developer-tools`. Check which ones other relevant projects use;
  keep topics people actually browse.
- **Social preview image** 1280x640 PNG: logo, tagline, one line "Skills over MCP, in a Git repo you own". Built from
  the existing logo files in `docs/images/brand/` and a web UI screenshot from `docs/images/web/`. No personal data.
- **Demo asset.** A 30 to 60 second terminal recording (asciinema converted to a GIF or SVG, or a short MP4) that shows
  only the happy path: install, `skillhub init`, `skillhub connect -g`, ask the agent; then, second part, a skill edited
  at the hub and used from another computer. Recorded on a throwaway hub with an invented skill, in an isolated HOME.
  Stored under `docs/images/demo/` and linked at the top of the README.
- **Discussions on**, with categories Q&A, Ideas and Show and tell. **Issue templates:** bug (asks for `skillhub version`,
  OS, the exact command, `skillhub doctor` output with no tokens) and feature request.
- **FAQ in `docs/`** (linked from the README): how it differs from symlinking `~/.claude/skills`, from skill registries
  and from the agents' own plugins; what "over MCP" means in practice; what leaves the machine; what is verified.
- **Honest comparison paragraph** (README or FAQ): the audience's own question "why not just symlink a folder?" answered
  in four sentences, with the cases where symlinks are enough.
- **Pin the repo and add a "Used by" or "Works with" line** only with true entries.

## Files
`docs/images/demo/*` (new), `docs/faq.md` (new), `README.md` and `README.vi.md` (demo link, FAQ link),
`.github/ISSUE_TEMPLATE/*` (new), `.github/FUNDING.yml` is out of scope. GitHub settings (description, topics,
homepage, social preview, Discussions) are applied by the owner or by `gh repo edit` after the owner says go.

## Steps
1. Draft the description, topics and issue templates; show them to the owner.
2. Write the FAQ and the comparison paragraph; check every claim against the docs or the code.
3. Record the demo in an isolated HOME on a temp hub; review the recording frame by frame for personal paths,
   addresses and tokens; keep the script next to the asset so it can be re-recorded after a release.
4. Build the social preview image.
5. The owner applies the settings (or says "apply" and `gh repo edit` runs with the agreed values).

## Validation
Open the repository page logged out and on a phone: description, topics, preview image and the top of the README read
well and no private path or token appears anywhere on the page.

## Risk and rollback
Everything here is editable at any time. The only irreversible thing is a leaked secret in a recording: review before upload.
