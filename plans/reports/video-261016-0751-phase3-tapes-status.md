# Intro video: phase 3 tapes, status

Last updated 2026-10-16 09:35 UTC, on `main` after PR #11 merged. Storyboard approved at CHECKPOINT 1.

## Done

- **Storyboard** [`video/script.md`](../../video/script.md), approved at CHECKPOINT 1. It has 10 scenes and about
  4:45 of runtime, and follows the README order: create the hub first, then connect agents.
- **Brief** [`plan.md`](../261016-1000-intro-video/plan.md) was updated to that structure. Sections 2, 3, 4 and 8 no
  longer use the "Case A / Case B" wording.
- **Tapes** in `video/demos/tapes/`. All six render on v0.7.0 with no errors and no wrapped lines (font 26):

  | Tape | Length |
  |---|---|
  | `01-install` | 20 s |
  | `02-create-hub` | 20 s |
  | `03-connect` | 20 s |
  | `04-first-skill` | 49 s |
  | `05-hub-invite` | 12 s |
  | `06-agent-connect` | 39 s |

  `render.sh` runs them in order. Its remote scene serves on port 7420 off camera, stops the server on any exit, and
  generates `06` from the invite line that `05` printed.
- `video/demos/README.md` now lists `tapes/`. `.gitignore` ignores the generated `06-agent-connect.tape`.
- From before: `happy-path.sh` passes steps 01–13 on v0.7.0. Step 14 fails on the executable-bit bug, which is fixed
  on `main`. The web scenes are recorded.

## Lessons from the tapes

- Do not use `Wait+Screen` in VHS after output that scrolls; it misses the text. Use `Sleep` instead.
- `sandbox.sh` ends with `cd "$HOME"`. Source helpers through `$DEMO_DIR/...`, not through a relative path.
- In `cd X && cmd &`, `$!` is the subshell's PID, not the command's. Run `cd X` on its own line first.

## Next

1. Phase 4: voice-over per scene with edge-tts, measured lengths, subtitles.
2. After v0.7.1 ships: run `happy-path.sh` and `render.sh` again.
3. The owner films OBS-1, OBS-2 and OBS-3, following "Shots the owner films" in `script.md`.

## Problems to report (not fixed)

- `SKILLHUB_BIN=release` does not work: `sandbox.sh` line 140 unsets `SKILLHUB_*` before line 165 reads it.
- `install.sh` run outside a sandbox edits the real `~/.bashrc` and `~/.zshrc`. That happened once; both files were
  restored.
- `T05` and `T06` show the filming computer's private LAN address; the owner keeps it as is.
- `skill create` writes "Use when write a commit message." `init` suggests `connect` for one project, while the
  README says `connect -g`.
- Web UI at 150 %: the label "Waiting for rev..." is cut off. The approve scene's header says "Home". Some icons
  render as an empty box in headless Chromium.
- `write_instructions` exists twice, in `happy-path.sh` and in `tapes/helpers.sh`. Proposal: move it into `lib.sh`.

## Tools

vhs v0.12.1, ttyd 1.7.7 and ffmpeg 8.1 (static) are in the session scratchpad, under `/tmp`.
