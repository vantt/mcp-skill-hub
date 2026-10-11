# Sources and learning page: report

Wrote `docs/sources-and-learning.md` (277 lines, budget 280) with the four frozen H2s: Keep copied skills up to
date, Learn from other repositories, Import many skills from a repository, Triage candidate repositories.

## What changed from the phase brief

- The inbox step is replaced by the verified flow (frozen map section 8): **Sources** > **Distill with Curator
  Agent** > **Copy handoff** > an agent with the distill-lab skill records lessons in `.meta/distill.yaml` >
  read them on the skill's **Distill** tab; decisions go through the curator agent (UI text in `DistillTab.tsx`).
- `skillhub check` is the documented top-level command; `source check` is named as the same command.
- "three-way merge" appears once, explained; "vendored" does not appear. No `inbox`, `insight`, `git add -A`,
  Codex or Gemini claims. Commit line uses the frozen safe add list.
- Links: `curating-skills.md#add-a-skill-skillhub-skill-add`, `#review-diagnostic-facts-skillhub-skill-review`,
  `#review-and-commit-workspace-changes`; `web-ui.md#start-the-web-ui`, `#hand-a-source-to-your-curator-agent`.
  Image: `images/web/sources.png` with manifest alt and caption.

## Verified (sandbox `/tmp/skillhub-docs-rewrite/sources`)

Offline: `skill outdated`, `skill upstream`, `skill update` (no tracked skill), `source backfill`,
`source capture`, `source list`, `source show`, `source triage --decision defer`, `source watch` preview and
refusal without `--skill-id`, `source watch --yes`, `source detach` and `source unwatch` previews, `skill review`
content-trust block. Network: `source attach` preview (it contacts the repository), one `check --all`
(`anthropic-skills: needs_analysis`), two `source import` previews, then the stored-proposal import (offline).
Update semantics (`--accept`, `--manual`, refusal with undecided files, review required after a third-party
update) read from `internal/app/upstream_update.go` and `internal/delivery/cli/skill_upstream.go`; status words
from `formatStatusWords`.

## Surprises

1. `skillhub source import https://github.com/anthropics/skills --path skills/pdf` fails on the 8 MiB limit
   (`skill add` with the same `--path` works). Only the `/tree/main/skills/pdf` URL works; the page uses it and
   says so. The FIX line suggests `source triage ... --path`, which does not help a direct import. Owner follow-up.
2. A skill imported with `source import` shows `not tracked` in `skill outdated`, and `source backfill` says
   `Nothing to backfill.` for it. The page says imported skills show as not tracked and does not promise backfill
   fixes them.
3. `skill review pdf` showed `Git: clean` for an untracked imported skill folder (owner follow-up 5 does
   reproduce for imports, unlike the frozen map's `skill create` case).
4. `skill update <id>` on a skill without upstream prints `Update code-review from - to - (source ).` and exits 0
   instead of an error.

## Unverified

- `skill outdated --check`, `skill upstream --check` and a real `skill update` with changes or conflicts (need a
  tracked skill and network); described from help and code only.
- `skill edit <id> --approve-content <digest>` (blocked by this session's permission rules); the command form is
  copied from the `skill review` output.
- `source triage --decision accept|import|reject` and `source confirm` (help text only).
- The handoff and Distill tab flow in a live browser and a curator agent recording lessons.

Status: DONE_WITH_CONCERNS
Summary: The sources and learning page is written within budget with the verified distill flow and no
inbox or insight commands. Update and approval paths that need network or permissions are documented from help
and code.
