# Demo scripts for the intro video

These scripts build a throwaway demo machine, run the documented happy path on it, and record the web UI, so every
scene of the video can be made again after a release. They never touch your real HOME, `~/.claude`, `~/.config` or
your own hub. The brief is [plans/261016-1000-intro-video/plan.md](../../plans/261016-1000-intro-video/plan.md).

| File | What it does |
|---|---|
| `sandbox.sh` | Sourced by everything else. Turns the current bash shell into a demo machine in one folder: HOME, the XDG folders, the per-user Skill Hub config and Git's global config all point inside it, Git identity `Demo User <demo@example.com>`, prompt `$ `, the `skillhub` binary under test first on PATH, browser opening stubbed. |
| `happy-path.sh` | Runs the README Quickstart and the `docs/curating-skills.md` tasks in order, answers the `[y/N]` prompts in a pseudo-terminal like a person would, and writes one cleaned transcript per step. Stops at the first failed step and names it. |
| `lib.sh` | Helpers: the pseudo-terminal runner, the transcript cleaner, the step runner and the leak check. |
| `web/seed-review.sh` | Puts a skill under **Waiting for review** offline: starts `skillhub serve` on loopback, makes an invite, connects a second demo machine, sends the `release-checklist` fixture with `skill commit`, stops the server. |
| `web/record.mjs` | Starts `skillhub serve web` in the sandbox on a fixed port, records two scenes with Playwright's built-in video, stops the server. |
| `web/run.sh` | `seed-review.sh` and then `record.mjs`. |
| `tapes/` | The VHS tapes of the terminal scenes, one per scene of [`video/script.md`](../script.md), and `render.sh`, which records them in order into `video/recordings/tapes/` (needs `vhs`, `ttyd` and `ffmpeg` on PATH). Its header lists the scenes and the port it uses. |
| `fixtures/` | The invented skills the scripts use; see [fixtures/README.md](fixtures/README.md). |

## Prerequisites

- bash 3.2 or newer, Git, Perl and `script` (util-linux on Linux, BSD `script` on macOS; both ship with the OS).
- The binary under test, one of:
  - built from this repository: Go, and `make web-build` once, so the binary embeds the web UI;
  - `SKILLHUB_BIN=/path/to/skillhub`, for example a binary you downloaded;
  - `SKILLHUB_BIN=release`: the latest release, installed with the README's `install.sh` into the sandbox (network).
- For the web scenes: Node 20 or newer, `npm ci` in `web/` and `npx playwright install chromium` (once, from `web/`).

## Run

From the repository root:

```bash
make web-build                      # once per checkout, so serve web has pages
video/demos/happy-path.sh           # terminal steps -> video/recordings/transcripts/
video/demos/web/run.sh              # web scenes     -> video/recordings/web/
```

With network access, add a skill from GitHub as well (`docs/curating-skills.md` "Add a skill from GitHub": read with
`--diff`, add and approve, add routing, activate):

```bash
DEMO_NETWORK=1 video/demos/happy-path.sh
```

To try commands by hand, source the sandbox in a bash shell: `. video/demos/sandbox.sh`. In a VHS `.tape`, use
`Set Shell bash`, then source it while hidden, so the recording starts at a clean `$ ` prompt:

```text
Hide
Type ". video/demos/sandbox.sh /tmp/skillhub-demo && clear"
Enter
Show
```

Pass the folder explicitly when you source it from a script, since `.` passes the script's own arguments otherwise.

Settings, all optional:

| Variable | Default | Used by |
|---|---|---|
| `DEMO_SANDBOX` | `${TMPDIR:-/tmp}/skillhub-demo` (or the first argument) | all |
| `SKILLHUB_BIN`, `DEMO_REBUILD=1` | build from the repository | `sandbox.sh` |
| `DEMO_FRESH=1` | keep an existing sandbox; `happy-path.sh` always starts fresh | `sandbox.sh` |
| `DEMO_OPEN_BROWSER=1` | browser opening stubbed, URLs logged to `run/opened-urls.log` | `sandbox.sh` |
| `DEMO_OUT` | `video/recordings/transcripts` | `happy-path.sh`, `seed-review.sh` |
| `DEMO_NETWORK=1`, `DEMO_GITHUB_URL`, `DEMO_GITHUB_PATH`, `DEMO_GITHUB_SKILL` | off; `anthropics/skills`, `skills/webapp-testing` | `happy-path.sh` |
| `DEMO_HUB_PORT` | `17420` | `seed-review.sh` |
| `DEMO_PEER_LABEL` | `laptop` | `seed-review.sh` |
| `DEMO_WEB_PORT` | `17421` | `record.mjs` |
| `DEMO_RECORDINGS` | `video/recordings/web` | `record.mjs` |
| `DEMO_WEB_THEME` | `light` (or `dark`) | `record.mjs` |
| `DEMO_SLOWMO`, `DEMO_PAUSE` | `150`, `1200` ms | `record.mjs` |
| `DEMO_HEADED=1` | headless | `record.mjs` |

The ports differ from the real defaults (7420, 7421), so a hub server of yours keeps running. A busy port is an error
that names the port: stop what listens there (`lsof -i :17421`); the scripts never move to another port.

## Outputs

Everything goes under `video/recordings/`, which Git ignores:

- `transcripts/NN-<step>.txt`: one file per step, starting with the command line(s) after `$ `, then the cleaned
  output. Use them for the `.tape` files and the storyboard: they are what this version really prints.
- `transcripts/remote/`: the steps of `seed-review.sh` (invite, connect from a second machine, send for review,
  `status`), raw material for the "hub on another computer" scene. The second machine is on loopback, not a real one.
- `web/web-edit.webm`: Skills, `commit-style`, **Files**, `SKILL.md`, **Edit**, typing a line, **Review**,
  **Commit to the hub**.
- `web/web-approve.webm`: **Waiting for review**, `release-checklist`, opening every changed file, **Approve**.
- `web/<scene>-end.png`: the last screen of each scene.

Videos are 1920x1080 (a 1280x720 page at 150 % zoom, so text stays readable on a phone). The raw, uncleaned output of
each step stays in the sandbox (`logs/`); never use it in the video.

The happy path leaves the demo hub in the sandbox for the web scenes. Remove it when done:
`rm -rf "${TMPDIR:-/tmp}/skillhub-demo"`.

## After each release

1. Install or build the new version, then run `video/demos/happy-path.sh` (`SKILLHUB_BIN=release` takes the published
   release) and `video/demos/web/run.sh`.
2. Compare the new transcripts with the ones the storyboard quotes (`diff -r` against a copy); update the `.tape`
   files and the narration where the output changed, and record those scenes again.
3. If a step fails, the docs and the binary disagree: report it, do not edit the script to hide it.

The scripts depend on commands, flags, and on the web UI's roles and labels, not on the wording of CLI messages,
so a release that rewords output still runs. `happy-path.sh` was checked against `main` and the `fix/happy-path-cli`
branch, the web scenes against `main`.

## Safety rules the scripts enforce

- **Isolated machine.** HOME, XDG_CONFIG/DATA/STATE/CACHE_HOME, XDG_RUNTIME_DIR, `SKILLHUB_USER_CONFIG_DIR`,
  `GIT_CONFIG_GLOBAL` (with `GIT_CONFIG_NOSYSTEM=1`) point into the sandbox, and `GIT_CEILING_DIRECTORIES` stops Git
  from finding a repository above it. PATH holds only the sandbox `bin` and the system folders. `sandbox.sh` refuses
  `/`, your HOME, a folder that contains your HOME, this repository, and any non-empty folder it did not create.
- **No inherited secrets.** `SKILLHUB_*`, `CLAUDE*`, `ANTHROPIC_*`, `CODEX_*`, `GEMINI_*`, `OPENAI_*` and any variable
  whose name holds `TOKEN`, `SECRET`, `PASSWORD` or `API_KEY` are unset, as are Git author overrides and the SSH agent.
- **Neutral identity.** Git commits as `Demo User <demo@example.com>`; the prompt is `$ `; `HOSTNAME` is `demo-host`.
- **Clean transcripts.** The cleaner turns sandbox paths into `~/...`, the repository into `<repo>` and other sandbox
  paths into `<demo>`; replaces the real HOME, user name, hostname and Git identity, session tokens (`#token=`),
  invite codes, content digests, submission IDs and LAN addresses (`192.0.2.x`), and numbers commit hashes
  (`<commit-1>`, ...). The real values are recorded once, before HOME changes, in the sandbox's `.private`
  (mode 600). After the run, `demo_check_clean` fails the script if any of them, a token or an invite code is still
  in a transcript.
- **No real hostname on screen.** A submission is labelled with the sending machine's hostname and there is no flag to
  change it, so `seed-review.sh` renames the queued submission to `laptop`. This is the only file the scripts edit
  behind the CLI's back.
- **Nothing left running.** Servers started by the scripts run on loopback only and are stopped when the script ends,
  also on failure; their PIDs are kept in `run/pids`, and a fresh sandbox stops any that are left.
- **No token in the web videos.** The session token is in the URL fragment, and a Playwright video has no address bar.

## Not covered by the scripts

- The install line (`curl ... install.sh | sh`) and `skillhub --version` right after it: film them with VHS in a
  sandbox with `SKILLHUB_BIN=release`, or run the curl line yourself in a sourced sandbox.
- The agent scenes ("curate my Skill Hub", "use /commit-style") and the two-machine scene: Claude Code and a second
  computer are filmed by hand with OBS.
- `skillhub serve --install-service`: it installs a real background service.

## Known on this version

- `skillhub doctor` in the sandbox reports a **Network server** notice when a hub server of yours answers on
  127.0.0.1:7420; stop it before filming `doctor`, or cut that line.
- macOS: the scripts use only bash 3.2, BSD tools and BSD `script`, but have only been run on Linux so far.
