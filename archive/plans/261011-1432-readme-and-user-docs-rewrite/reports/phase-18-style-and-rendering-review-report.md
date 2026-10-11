# Style, consistency and rendering review: report

Read-only review of `README.md`, `CONTRIBUTING.md` and `docs/{README,getting-started,installation,user-guide,
curating-skills,sources-and-learning,web-ui,troubleshooting,command-reference,release-runbook}.md` at `ce4f3d0`.
Checked against the 14 style rules, the glossary and the banned claims in `plan.md`, the frozen map and the
synthesis. Line numbers are source lines in each file.

## How it was checked

- **Reader path:** read README -> getting-started -> user-guide -> curating-skills -> sources-and-learning as a
  newcomer, then web-ui, troubleshooting, installation, command-reference and the two indexes.
- **Style:** read every prose line. A script (`/tmp/skillhub-docs-rewrite/render/style-scan.py`) flagged sentences
  over 20 words, paragraphs over 3 sentences and hype words; each hit was checked by hand.
- **Rendering:** every page went through the GitHub Markdown API (`gfm`, context `vantt/mcp-skill-hub`). It was
  wrapped in Primer `github-markdown-css` 5.8.1, with mermaid 11.4.1 for the diagram. Playwright Chromium from
  `web/` shot each page at 1280 px light, 1280 px dark and 390 px. In gfm mode, soft breaks render as `<br>`;
  the wrapper undoes that, except for the README header's backslash breaks, to match the repository file view.
  Scripts and shots are in `/tmp/skillhub-docs-rewrite/render/` (`shots/tiles/` holds the readable crops).
- **Sandbox:** `/tmp/skillhub-docs-rewrite/style-review` with the frozen binary. Re-running `init` on an existing
  workspace is harmless. `source watch ... --skill-id code-review` fails with `skill "code-review" not found`
  when that skill does not exist.

## Rendering results (all pages)

- The `<picture>` logo switches correctly between light and dark, and so does the README hero.
- The badges render: release `v0.3.0`, CI, MIT. The CI badge reads **failing** (see README findings).
- All 9 GitHub alerts render as alerts. The `<details>` blocks are collapsed by default.
- All 17 doc images load. Each one is 800 px wide, except `mobile-skills.png` at 260 px. Each file is at most
  116 KB.
- The mermaid diagram renders in light and dark. Phase 17 also rendered it with mermaid-cli.
- No page scrolls sideways at 390 px. Wide code blocks and tables scroll inside their own box (findings below).
- The logo mark is crisp at 16 and 32 px (`/tmp/skillhub-docs-rewrite/logo/zoom-small.png`).

## Findings by file

Severity: **High** = wrong or banned content a reader will act on; **Medium** = misleads, breaks the reader path
or breaks a plan rule visibly; **Low** = style-guide or consistency polish.

### README.md

| Line | What is wrong | Fix | Severity |
|---|---|---|---|
| 1-2 | The logo already shows the "Skill Hub" wordmark. The H1 `# Skill Hub` sits right under it, so the name appears twice at the top (seen at 1280 px in light and dark, and at 390 px). | Choose one. Either wrap the logo in the heading (`<h1><picture>...</picture></h1>` with `alt="Skill Hub"`) and drop the plain `# Skill Hub`, or keep the H1 and show the square mark (`skill-hub-mark-*.svg`, about 40 px) next to it instead of the horizontal logo. | Medium |
| 5 | The CI badge links to `main` and currently renders **CI failing** in red, right under the tagline. | Get `main` green before publishing, or drop the CI badge. This is an owner call; see "Decided items touched". | Medium |
| 94 | "Your agent shows the state of your hub and suggests one next step." states agent behavior as fact. Nobody saw it in a live session (frozen map: "describe it modestly"). | "Your agent can then check your hub and suggest a next step." | Medium |
| 99 | `Add your first skill -> [Get started](docs/getting-started.md)` lands on the top of Get started. That page repeats install, init and connect, which the reader has just done. The `->` also renders as literal ASCII. | Link to `docs/getting-started.md#create-your-first-skill`, for example "Next: [create your first skill](docs/getting-started.md#create-your-first-skill)." | Medium |
| 9 | "When your agent starts a task, it asks the hub which skill fits" is close to the banned "the agent always asks the hub". Line 30 already uses the approved "is told to ask". | "When your agent starts a task, it is told to ask the hub which skill fits, and the hub suggests one." This line comes from the voice calibration; see "Decided items touched". | Low |
| 9, 21 | "Nothing changes in your library without your say-so" and "You approve every change" are a little too strong. `source capture` and `skill env set` write without a preview. Claude Code allows them under `Bash(skillhub:*)` without asking, and permission-bypass modes skip the ask rules. | Narrow to skills: "Nothing changes in your skills without your say-so." For the bullet: "**You approve every skill change.**" | Low |
| 84 | The expected output "Workspace is healthy." sits in the sentence, not in a separate `text` block (rule 7). Phase 9 did this to stay in the line budget. | Leave it if the budget is tight; otherwise add a `text` block. | Low |
| 130 | Comma splice: "Skill Hub is released under the MIT License, see [LICENSE](LICENSE)." | "...under the MIT License; see [LICENSE](LICENSE)." | Low |

### docs/getting-started.md

| Line | What is wrong | Fix | Severity |
|---|---|---|---|
| 137-141 | The first-win prompt is "write a commit message for my staged changes". In a fresh `~/myproject`, or any project with nothing staged, the agent has nothing to write about, so the first win fails for a reason unrelated to Skill Hub. | Add one step before the prompt: "Stage a change in your project first, for example `git add <file>`." | Medium |
| 59-60, 78-79 | The CAUTION and the TIP render within one 1280x900 screen (about 550 px apart). This breaks rule 10, at most one callout per screen. | Make the `-g` hint a plain sentence, or move the TIP below the `doctor` output (after line 91). | Low |
| 150 | The one-line request JSON is cut off at 1280 px, right before `"scope"`, the key the sentence above tells readers to keep. | Pretty-print the JSON over several lines so `"scope":"single_step"` is visible. The same applies to `user-guide.md:126` and `troubleshooting.md:89`. | Low |
| 5 | The paragraph has 4 sentences (rule 2). It uses `connect` and "connection files" before they are explained at line 64 (rule 4). | Split it after "Claude Code." In the agent-support sentence, write "Skill Hub also writes setup files for Codex and Gemini CLI; ...", or move that sentence below line 64. | Low |
| 47 | One sentence holds two ideas (rule 2). It also does not say why the reader should ignore `init`'s next steps: hint 1 is `git add -A`. | "`<home>` stands for your home folder. Skip the next steps that `init` prints; its commit hint stages files that must stay out of Git." | Low |
| 160 | The output line "Host approval is required before activation." uses the avoid-term "host" on the first-win page, and nothing explains it. | Add one line under the block: "The second sentence means your agent still decides whether to load the skill." The same applies to `user-guide.md:136`. | Low |
| 199 | "The [Web UI](web-ui.md) guide": capital "Web UI" in prose breaks the glossary ("web UI"). | "The [web UI guide](web-ui.md) tours every screen." | Low |
| 203 | The caption is plain text. Every other page sets captions in italics (`*...*` or `<em>`). | `*Skills: every skill with its state, and filters to find one.*` | Low |
| 208 | The sentence is 22 words (rule 2). | "Your agent does not use the hub: open a new Claude Code session in the connected folder. Then run `skillhub doctor` there." | Low |
| 217 | "lessons" is used without its one-line definition (rule 4). | "...propose **lessons**, improvements you decide on, from other repositories". | Low |

### docs/installation.md

| Line | What is wrong | Fix | Severity |
|---|---|---|---|
| 47-50 | Four source lines join into one rendered paragraph of 4 sentences (rule 2). | Put a blank line before "Set `SKILLHUB_REQUIRE_SIGNATURE=1` ...". | Low |
| 42 | The sentence is 21 words. | "On Linux and macOS, each variable also has a flag. Run `sh install.sh --help` on a downloaded copy to list them." | Low |
| 80 | "Node.js" has no version. `CONTRIBUTING.md:4` says Node.js 24 (`web/.nvmrc`). | "Node.js 24". | Low |

### docs/user-guide.md

| Line | What is wrong | Fix | Severity |
|---|---|---|---|
| 139 | "Add `--json` for the full answer; the format is in [the request schema]" links to `skill-resolve-request-v1.schema.json`, which describes the request, not the answer. | Link the answer format to `../schemas/skill-resolve-response-v1.schema.json`, or write "the request format is in the request schema". | Medium |
| 163, 165 | The table states as fact what the agent is only told to do. "The agent runs a check first" and "They are installed into the skill's own `$SKILLHUB_STATE_DIR`, never globally" come from the instruction block, which is best effort (connect prints "instruction-only and best effort"). | "Your agent is told to run a check first ..." and "Your agent is told to install them into ... , never globally." | Medium |
| 13 | "It is the only thing you need to back up." contradicts line 241: secrets in `runtime/config/` stay outside Git and outside the workspace history. | "It holds everything except the secrets you store per skill." | Low |
| 37, 49, 238 | `connect --workspace ~/skillhub` without `--host claude` also writes Codex and Gemini CLI files (`AGENTS.md`, `GEMINI.md`, `.codex/`, `.gemini/`) into the project. Get started and the README use `--host claude`. | Use `--host claude` in the three commands and keep line 52 for the other agents. If all agents are wanted on purpose, say so in the sentence above. | Low |
| 67-68, 100-101 | The TIP and the CAUTION render within one 1280x900 screen once the permission `<details>` is closed (rule 10). | Make the TIP a plain sentence. | Low |
| 19, 96, 205 | Paragraphs of 4 sentences (rule 2). | Split each paragraph into two. | Low |
| 117 | The sentence is 23 words. | "Your agent is told to ask the hub which skill fits at the start of each new task. Trivial edits skip this." | Low |

### docs/curating-skills.md

| Line | What is wrong | Fix | Severity |
|---|---|---|---|
| 25 | "it asks you before any command that changes files" is stronger than the product. The ask rules cover only `--yes` and `confirm`. `source capture` and `skill env set` change files without either, and bypass modes skip the rules. | "Claude Code asks you before any `skillhub` command with `--yes` or `confirm`." | Medium |
| 13 | "Your agent checks the hub and suggests one next step." is an unverified live-session claim, the same as README:94. | "Your agent can check the hub and suggest one next step." | Medium |
| 269 | "`init` and `connect` write connection files into the workspace": `connect` writes into the project it connects, and into the workspace only when run there. | "`init` writes connection files into the workspace, and some hold absolute paths for this machine." | Low |
| 237 | The link text "Activation refuses a draft" names an H3, but the link goes to the H2 `troubleshooting.md#skills`. | Use `troubleshooting.md#activation-refuses-a-draft`, which exists. | Low |
| 167, 177 | These one-line commands overflow their code box at 1280 px; the flags at the end are hidden. | Break them with `\` continuation lines, as `skill create` does at 105-108. | Low |
| 106 | "Write git commit messages": the product name is Git. The same string is at `getting-started.md:116`. | "Write Git commit messages in our team style" in both files. The frozen transcript ran the lowercase form; the change does not affect output. | Low |
| 144, 158 | Paragraphs of 4 sentences (rule 2). | Split each paragraph into two. | Low |
| 35 | "Every command that changes files shows a preview first" has exceptions: `source capture`, `skill env set`. | "Commands that change skills show a preview first." | Low |

### docs/sources-and-learning.md

| Line | What is wrong | Fix | Severity |
|---|---|---|---|
| 129 | The example watches for `--skill-id code-review`. Readers coming from Get started have only `commit-style`, so the command fails with `skill "code-review" not found` (checked in the sandbox). | Use `--skill-id <skill-id>` and add "The skill must already exist." | Medium |
| 181 | "Start the web UI with `skillhub serve web`": on a machine with several network interfaces, the bare command listens on every one over plain HTTP. README and Get started use `--loopback-only`. | `skillhub serve web --loopback-only`. | Low |
| 194-198 | "Commit the new lesson file along with your other changes:" is followed only by `git add`, with no commit. | Add `git -C ~/skillhub commit -m "Record lessons"` in its own block. | Low |
| 105 | The placeholder is `sha256:<digest>` here, but `<digest>` in `web-ui.md:61`, `troubleshooting.md:100` and `command-reference.md:36`. | Use one form everywhere, for example `<digest>` with the note "the full value from the review, including `sha256:`". | Low |
| 8, 122 | "curator agent" is used at line 8 and defined only at line 179 (rule 4). | Define it at first use (line 8): "...a source your **curator agent** (your own agent with the distill-lab skill) reads for ideas". Keep line 179 short. | Low |
| 224 | The paragraph has 4 sentences (rule 2). | Split it after "never overwritten." | Low |

### docs/web-ui.md

| Line | What is wrong | Fix | Severity |
|---|---|---|---|
| 12-20 | The page's first command is bare `skillhub serve web`, and its output block shows a single line. On a machine with several interfaces, the bare command exposes the web UI to the network over plain HTTP, with a WARNING and extra lines (frozen map section 7). README and Get started use `--loopback-only`. | Show `skillhub serve web --loopback-only` here. Leave the bare form and its output to "Keep it safe on your network". | Medium |
| 26 | "The pictures use invented skills and a demo hub, so your screens list your own skills." The "so" does not follow. | "The pictures come from a demo hub with invented skills; your screens list your own skills." | Low |
| 32, 140 | "distill", "lessons", "source", "learning reference" and "curator agent" appear before they are defined at lines 147 and 155 (rule 4). | Line 32: "...the sources ready for your curator agent and the lessons waiting for your decision". Move the one-line source and curator-agent definitions up to "Manage one skill's sources" (line 140). | Low |
| 142, 143 | Each bullet is a 5-sentence paragraph (rule 2). | Turn each bullet into short numbered steps, or split it into two sentences of setup and two of result. | Low |
| 70 | The sentence is 22 words. | Split at the semicolon. | Low |

### docs/troubleshooting.md

| Line | What is wrong | Fix | Severity |
|---|---|---|---|
| 41 | The reconnect command has no `--host claude`, so it also writes Codex and Gemini CLI files, unlike Get started. | `skillhub connect --workspace ~/skillhub --host claude --yes`. | Low |
| 48 | "the `skillhub` MCP server" puts "server" in user prose (glossary). The claim itself is unverified (Phase 6 report). | "Claude Code may also ask you to approve the `skillhub` entry in `.mcp.json`." Or drop the sentence. | Low |
| 128 | Grammar: "agents stop receiving them rather than get old copies". | "...agents stop receiving them instead of getting old copies." | Low |
| 143 | It tells readers to choose **Run anyway** before checking signatures. | "First [check the signatures](installation.md#check-signatures). Then choose **More info** and **Run anyway**." | Low |
| 79 | The one-line `skill edit` command overflows its box at 1280 px. | Break it with `\` continuation lines. | Low |
| 13, 48 | Sentences of 22 words. | Split each at its comma or semicolon. | Low |

### docs/command-reference.md

| Line | What is wrong | Fix | Severity |
|---|---|---|---|
| 74 | It prints `git add -A`, which the banned list says is never shown, even as a "never". Phase 17 step 3 flags the same line. | "Add the workspace's own folders by name; never stage the whole folder." | Medium |
| 7 | "`source capture` is the one exception: it writes at once." `skill env set` also writes at once (help: "env set ... Store secrets", no `--yes`). | "`source capture` and `skill env set` are the exceptions: they write at once." | Low |
| 86 | "Run the MCP server by hand" puts "server" in user prose (glossary). | "Start the hub for agents by hand (your agent starts it)". | Low |
| 62 | The sentence is 25 words. | Split it after "Distill with Curator Agent**." | Low |
| tables (390 px) | At phone width, the Command column is cut off and scrolls inside the table, so flags at the end are hidden until the reader scrolls. Every table on the page is affected. | Acceptable on GitHub; no change required. If wanted, shorten the longest cells (`skill create`, `skill edit`) by moving optional flags into the note under the table. | Low |

### docs/README.md

| Line | What is wrong | Fix | Severity |
|---|---|---|---|
| 25 | "which MCP clients ... were checked" uses "client", a do-not-write term for agent (glossary). | "which agents and protocol revisions were checked". | Low |
| 15 | The link text is "Web UI"; the page H1 is "The web UI" (glossary: "web UI"). The README (line 124) does the same. | "[The web UI](web-ui.md)" or "[Web UI guide](web-ui.md)", the same in both indexes. | Low |

### CONTRIBUTING.md

| Line | What is wrong | Fix | Severity |
|---|---|---|---|
| 8, 43 | Sentences of 21 words (rule 2). This is maintainer-facing, so it is borderline. | Split line 8 after the colon into a short list. Split line 43 after "configuration". | Low |

### docs/release-runbook.md

No defects in the trimmed parts. The install section is one link plus one sentence, and `#manual-artifact-verification`
still resolves. The heading keeps its anchor, and the "Prerelease tags" paragraph now renders outside the list.

## Decided items touched (for the owner, not for the fix pass)

1. **CI badge (README:5).** The synthesis decided on release, CI and license badges. CI on `main` currently
   renders "failing". Options: fix CI before publishing, or drop the CI badge.
2. **Voice calibration vs banned claim (README:9).** The decided calibration text says "it asks the hub which
   skill fits"; the plan's banned list wants "is told to ask". The suggested fix is above.
3. **Mermaid label "Web UI or CLI" (README:43) and "Git repo" (README:36).** The diagram is decided to be
   byte-identical to the synthesis. The labels break the glossary ("web UI", "repository"). Keep as decided, or
   allow a two-word label change.
4. **Header link row "User guide" (README:6).** It is decided ("Quickstart · User guide · Curating skills"), but
   the target page is now titled "Using Skill Hub", and the Documentation list uses that name. Consider renaming
   the link to "Using Skill Hub".
5. **Logo plus H1 (README:1-2).** The owner chose a horizontal logo in the header. Removing the duplicate H1 or
   switching to the mark changes how that decision looks. Neither option reverses it.

## Pre-publish checklist (synthesis)

| Item | State |
|---|---|
| Every link and anchor resolves | Pass (Phase 17 step 1). |
| Every command run on a clean HOME, output matches | Phase 17 step 2 (not repeated here); the sandbox checks above agree. |
| Every image exists, has alt text, at most 300 KB | Pass (17 refs; largest 116 KB). |
| Rendered preview light, dark, mobile | Done here; findings above. |
| Product names (Claude Code, Codex, Gemini CLI, GitHub, Git, Skill Hub) | One miss: "git commit messages" (curating:106, getting-started:116). |
| Tone pass against the style guide | No hype words, emoji or exclamation marks. Long sentences, long paragraphs and callout spacing are listed above. |
| Badges point to real targets | Pass (Phase 16 checked the URLs); CI shows "failing". |

## Unverified

- GitHub's own mermaid renderer: this review used mermaid 11.4.1 locally. Phase 17 also rendered the diagram.
  GitHub's live view was not opened, because the branch is not pushed.
- The `themed-picture` behavior on github.com: `<picture>` with `prefers-color-scheme` was emulated in Chromium.
  The API output wraps it the same way GitHub does.

Status: DONE_WITH_CONCERNS
Summary: Rendering is sound in light, dark and 390 px: alerts, `<details>`, images, the logo switch and mermaid
all work. The review found 61 concrete defects (12 Medium, 49 Low, none High), led by the duplicate logo and H1
in the README, the shown `git add -A`, a first win that needs staged changes, bare `serve web` in the web UI
guide, and a few overstated agent claims.
Concerns/Blockers: CI badge renders "failing"; five findings touch decided items and need the owner.
