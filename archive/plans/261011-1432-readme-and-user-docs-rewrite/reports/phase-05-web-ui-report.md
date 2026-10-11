# Report: docs/web-ui.md

## What changed

- New `docs/web-ui.md` (193 lines). H2s and H3s match the frozen map exactly (including "Link and check sources").
- Intro carries the never-does line as the one IMPORTANT alert; the network section has one WARNING alert and a
  `<details>` block for binding, `--loopback-only`, `--addr`, `--allow-host`.
- 13 images from `docs/images/web/`, alt and caption copied from `manifest.json`, `width="800"` (phone 260);
  `create-skill.png` inside `<details>`. All files exist and are under 120 KB. Image and caption share one line
  (`<img ...><br><em>caption</em>`) to stay inside the 200-line budget.
- Task texts kept close to the signed-off walk-through; UI labels set in bold and checked against the screenshots
  and the label strings in `web/src`.

## Verified (sandbox `/tmp/skillhub-docs-rewrite/web-ui`, frozen binary)

- `skillhub serve web --no-open`: on this multi-interface machine prints the plain-HTTP WARNING line, then
  `Skill Hub web UI: http://127.0.0.1:7421/#token=<token>` and one `<interface>: http://<ip>:7421/#token=<token>`
  line per interface. With `--loopback-only --addr 127.0.0.1:47392` and with `--addr 127.0.0.1:8080` it prints only
  the one line. SIGINT (Ctrl+C) stops it with exit 0.
- An unknown `Host` header gets HTTP 421; the normal request gets 200.
- `skillhub help serve`: `--workspace`, `--addr`, `--loopback-only`, `--allow-host` (repeatable), `--no-open`; the
  listen rule "two or more non-loopback IPv4 addresses".
- `skillhub skill archive <id>` without `--yes` only previews ("No files changed"); with `--yes` it archives a
  deprecated skill. The doc shows the `--yes` form and says what happens without it.
- Help only: `skill outdated --check`, `source backfill --skill <id>`, `skill edit <id> --approve-content <digest>`,
  `skillhub check` (Check due sources / Check all mirror `--all-due` / `--all`).

## Notes for the lead

- The web UI code has an **Archive** button for deprecated skills (`web/src/screens/skill-detail/SkillDetailScreen.tsx:42`),
  but no screenshot shows it and the signed-off text sends readers to the terminal. The doc keeps the terminal
  route; adding the button is a one-line follow-up if wanted.
- Home copy: the doc does not quote the Next action text, so it stays true after `home.png` is re-shot.

## Unverified

- Every click-flow described (Resources editor, conflict drawer, Track upstream, Review update, Add from GitHub
  review step, Distill tab) was not clicked through in this phase; the text follows the signed-off walk-through
  and the label strings in `web/src`.
- Rendered preview on GitHub (light, dark, phone width) not checked.

Status: DONE
Summary: Wrote docs/web-ui.md with the frozen headings, 13 manifest images and verified serve/network commands, under 200 lines.
