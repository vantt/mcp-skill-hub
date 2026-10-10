# Web UI walk-through: the five flows that matter most

Status: **not yet walked through.** The notes section at the end is empty on purpose: the agent that
scored the screens cannot tell what feels clear to the person who curates the hub, so this page waits for
your notes. Scores and fixes so far are in [the scorecard](web-ux-scorecard.md).

Time: about 20 minutes. Nothing here changes your hub if you follow the "safe setup" below; every step
that could write stops at a preview or a Cancel button.

## Safe setup (a clone, not your live hub)

Run this in a terminal. It serves a throwaway copy of your hub with its own home and config folders, so
the web UI cannot touch the real one.

```sh
WALK=$(mktemp -d)
git clone --no-hardlinks ~/skill-hub "$WALK/hub"
export HOME="$WALK/home" XDG_CONFIG_HOME="$WALK/xdg/config" XDG_DATA_HOME="$WALK/xdg/data" \
       XDG_STATE_HOME="$WALK/xdg/state" XDG_CACHE_HOME="$WALK/xdg/cache"
mkdir -p "$HOME"
SKILLHUB_WORKSPACE="$WALK/hub" skillhub rebuild
skillhub serve web --workspace "$WALK/hub" --no-open
```

Open the address it prints (it contains a token). Test the phone layout by narrowing the browser window
to about 390 px, or with the browser's device toolbar. When you are done, stop the server with Ctrl+C
and delete `$WALK`.

## How to judge

For each step, answer one question: **could you do this without reading any documentation?** If you
hesitated, re-read a sentence twice, or did not know what a button would do, write it down; that is
exactly what is wanted. Wrong wording, a missing explanation and a layout problem all count.

## Flow 1: Home, what needs me

1. Open the address. Look at the whole page without clicking.
2. Say out loud what needs attention right now.
3. Click the suggested next action. If it shows a command, say whether you would know where to run it.

Ask yourself: Is the next action obvious? Do the counts mean something to you? Is anything named in a way you do not recognise?

## Flow 2: Find a skill

1. Open **Skills**.
2. Find one skill by typing part of its name, then find skills in one lifecycle state (for example
   Draft) with the filters.
3. Open one skill from the list.

Ask yourself: Do the three filters say what they filter? Is it clear what the list columns mean? On a narrow window, can you still read each row?

## Flow 3: Review one skill, can agents use it

1. On the skill page, stay on the **Review** tab.
2. Say whether agents can use this skill, and why or why not.
3. Find the command that makes you trust the skill's content, and use its copy button (paste it somewhere
   to see that all of it was copied; do not run it unless you are on the clone).
4. Open the **Runtime** tab and say what this machine still needs.

Ask yourself: Did the badges and cards say what they mean without jargon? Did you know what to do next?

## Flow 4: Edit a skill's routing fields (preview only)

1. Open the **Editor** tab of a skill.
2. Change the description by a few words and choose **Preview changes**.
3. Read the dialog: say exactly what will change before you would confirm.
4. Choose **Cancel**. Do not save.

Ask yourself: Do the field hints explain operations, triggers, "not for" and minimum scope? Did the preview tell you what Save would do?

## Flow 5: Add skills from GitHub (discover only)

1. Open **Skills**, then **Add skills from GitHub**.
2. Paste the address of a public repository that has skills, open **Advanced** and read the options.
3. Choose **Discover**, and look at what it lists. Stop there: do not confirm an import.

Ask yourself: Did you know what Discover does and what comes next? Did Advanced make sense without leaving the page?

## Notes (fill in during or after the walk-through)

| Flow | Step | What was unclear or wrong | Window width (1280 or 390) | Fix before release? (yes/no) |
|---|---|---|---|---|
| 1 Home | | | | |
| 2 Find a skill | | | | |
| 3 Review one skill | | | | |
| 4 Edit routing | | | | |
| 5 Add from GitHub | | | | |

Anything that is not fixed goes to [the observation backlog](../plans/2026-10-10-observation-backlog.md).

Walked through by: _(name, date)_
