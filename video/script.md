# Skill Hub intro video: script and storyboard

Brief: [plans/261016-1000-intro-video/plan.md](../plans/261016-1000-intro-video/plan.md). Status: **approved at
CHECKPOINT 1 (2026-10-16)**; next are the voice-over and the edit. Changes to the scenes go back to the owner.

- Version filmed: `skillhub` 0.7.0, from the latest release. Film again on v0.7.1: it fixes the executable bit of
  `scripts/` files, which makes step 14 of `happy-path.sh` fail on 0.7.0.
- Voice-over: English, edge-tts (`en-US-AndrewNeural`). Subtitles: English and Vietnamese, from the narration
  below.
- Structure: **hub first, then agents**, the same order as the README Quickstart. Everyone does Install, Create and
  Connect. One more chapter covers an agent on another computer: it reuses the hub and only changes how the agent
  connects.
- Target length: about 4:45. Every scene's real length comes from its audio (phase 4).

## Sources for each shot

| Source | Made by | Output |
|---|---|---|
| `T01`–`T04`, `T05`, `T06` | VHS tapes in `video/demos/tapes/`, run by `render.sh` | `video/recordings/tapes/NN-*.mp4` |
| `W-edit`, `W-approve` | `video/demos/web/run.sh` (Playwright) | `video/recordings/web/web-*.webm` |
| `OBS-1`, `OBS-2`, `OBS-3` | the owner, in Claude Code; see "Shots the owner films" below | `video/recordings/obs/*.mp4` |
| `MG` | Remotion motion graphics | built while editing |

Each tape writes what this version really prints, in a sandbox. The tapes leave out only the steps the video shows
another way; each tape's header comment names them.

## Chapters (YouTube)

| Start | Chapter |
|---|---|
| 0:00 | Why Skill Hub |
| 0:45 | Install |
| 1:05 | Create your hub |
| 1:30 | Connect your agent |
| 2:05 | Your first skill |
| 3:05 | Edit in the browser |
| 3:30 | An agent on another computer |
| 4:30 | Tips and links |

The timestamps are estimates; phase 6 replaces them with the real ones.

---

## Scene 1. Cold open (0:00, about 8 s)

| | |
|---|---|
| Screen | `MG`: a terminal frame, then a cut to `OBS-2`: the agent answers "use /commit-style ..." and its commit message ends with `Checked-by: skillhub-demo`. Logo `skill-hub-logo-dark.svg` fades in. |
| On-screen text | **Your skills in one place. Pulled by any agent, on demand.** |
| Narration | "Your agent skills, in one place you own. Every agent pulls the right one, on demand." |

## Scene 2. The problem and the idea (0:08, about 37 s)

| | |
|---|---|
| Screen | `MG`, three beats. **(a)** Three project folders and two laptops, each with its own copy of `.claude/skills`; the copies drift apart (colour shifts, a red mismatch dot). **(b)** One box labelled "Hub: a Git repository you own". Lines run from it to Claude Code, Codex and Gemini CLI, and to two computers. **(c)** Three short cards appear one after another. |
| On-screen text | (c): **Your library, in Git** · **Review before agents get it** · **Nothing to install per project** |
| Narration | "Skills end up copied into every project and every computer, and the copies drift apart. Skill Hub keeps them in one Git repository you own. You collect skills from anywhere, edit them, and approve them before any agent gets them. Your agents connect to the hub over MCP, so there is nothing to copy into each project." |
| Note | A small footnote card at (b): "Verified end to end with Claude Code." Codex and Gemini CLI show as lighter icons. Do not say they are fully supported. |

## Scene 3. Install (0:45, about 20 s): chapter "Install"

| | |
|---|---|
| Screen | `T01`: the `curl ... install.sh \| sh` line, the install output, then `skillhub --version`. |
| Step title | **Step 1 · Install** |
| On-screen text | Lower third: "You need Git and an agent, such as Claude Code." Small card: "Windows: PowerShell command in the README." |
| Highlight | Box around `Installed skillhub 0.7.0 to ...`, then around `skillhub version: 0.7.0`. |
| Narration | "You need Git and an agent such as Claude Code. Install Skill Hub with one line. Open a new terminal, and check the version." |
| Note | `T01` cuts the opening of a new terminal off camera. The cosign line ("Signature not checked ...") is four lines long: speed it up or crop it. Do not hide it. |

## Scene 4. Create your hub (1:05, about 25 s): chapter "Create your hub"

| | |
|---|---|
| Screen | `T02`: `git config user.email` prints nothing; set the name and email; `mkdir -p ~/skill-hub && cd ~/skill-hub && skillhub init`. |
| Step title | **Step 2 · Create your hub** |
| Machine label | "Hub computer" (shown from here on whenever the chapter on another computer could make it unclear; here it can stay small). |
| Highlight | `Hub created at ...` and `This is now the hub of this machine`. |
| Narration | "The hub is a folder in its own Git repository. Git records every change with your name, so set them once if you have not. Then create the hub." |
| Note | `init` ends with "Next: connect a project ... `skillhub connect`". Blur or cut those two lines: the README says to use step 3 instead, which connects every project. The narration does not mention them. |

## Scene 5. Connect your agent and check (1:30, about 35 s): chapter "Connect your agent"

| | |
|---|---|
| Screen | `T03`, part 1: `skillhub connect -g`. Part 2: `cd ~/my-project && skillhub doctor`. |
| Step title | **Step 3 · Connect your agent**, then **Step 4 · Check** |
| Highlight | Part 1: the `Claude Code: connected` line, then `Next: restart your agent ...`. Part 2: the last line, `Healthy.` |
| On-screen text | Over the `WARNING:` lines: "Expected: each agent still asks you for permission." Small card: "Claude Code asks you to trust `Bash(skillhub:*)` the first time. Allow it." |
| Narration | "Connect your agents once, for every project. The warning in the middle is expected. Then run doctor from any project. Healthy means you are ready." |
| Note | `doctor` runs in a project folder, not in `~/skill-hub`: inside the hub folder it adds "not used here" lines. Before filming, make sure no hub server of yours listens on port 7420, or doctor adds a "Network server" notice. |

## Scene 6. Ask your agent (2:05, about 15 s)

| | |
|---|---|
| Screen | `OBS-1`: Claude Code opened in `~/my-project`; the person types **curate my Skill Hub**; the agent runs `skillhub status` and suggests creating a first skill. |
| On-screen text | **Step 5 · Ask your agent** · the prompt in a large speech bubble. |
| Narration | "Restart your agent, open any project, and say: curate my Skill Hub. A new hub has no skills yet, so the agent suggests your first one." |

## Scene 7. Your first skill (2:20, about 45 s): chapter "Your first skill"

| | |
|---|---|
| Screen | **(a)** `T04` part 1: `skillhub skill create commit-style` and its four answers. **(b)** `MG` editor shot: `SKILL.md` with the **Instructions** section filled in (the three numbered rules) and two more examples. Built from the `cat` frame of `T04`. **(c)** `OBS-2`: "use /commit-style to write a commit message for my staged changes"; the message ends with `Checked-by: skillhub-demo`. **(d)** `T04` part 2: `skillhub skill commit commit-style`, answer `y` twice, then `skillhub skill list --state active`. |
| Step titles | **Start the skill** · **Write the instructions** · **Try it** · **Publish it** |
| Highlight | (a) the four answers; (b) the `Checked-by: skillhub-demo` line; (c) the same line in the agent's answer; (d) `Activate commit-style now? [y/N] y` and the `active` row. |
| Narration | "A skill is a folder with instructions your agent follows. Start one in your project: say what it does, when to use it, when not to, and give an example. Write the instructions. Try it with your agent: here, every commit message ends with a check line. When it works, publish it. Say yes to add it, and yes to activate it. Every agent that uses your hub gets it now, in every project." |
| Note | (b) is done off camera in `T04` by `helpers.sh`. The video shows the result as a file, not as typing. |

## Scene 8. Edit in the browser (3:05, about 25 s): chapter "Edit in the browser"

| | |
|---|---|
| Screen | Command card `cd ~/skill-hub && skillhub serve web`, then `W-edit`: **Skills**, `commit-style`, **Files**, `SKILL.md`, **Edit**, a typed line, **Review**, **Commit to the hub**, ending on "Committed .... This machine's agents get it now." |
| On-screen text | "Anyone with the link can change your skills. Keep it on this machine: see the Web UI guide." |
| Narration | "Prefer clicking? Start the web UI from your hub folder. Open a skill, edit a file, review the change, and commit it to the hub." |
| Note | The command is shown as a card, not as a terminal recording. `serve web` prints a link that holds a session token, so it must never be filmed. `W-edit` has no address bar. |

## Scene 9. An agent on another computer (3:30, about 60 s): chapter "An agent on another computer"

| | |
|---|---|
| Screen | **(a)** `MG`: two computers on one network, "Hub computer" (dark terminal) and "Agent computer" (light terminal). **(b)** Hub side: card with `cd ~/skill-hub && skillhub serve --install-service`, then `T05`: `skillhub token invite` and the connect line it prints. **(c)** Agent side: `T06`: the pasted line with `-g`, `skillhub doctor`, `Healthy.` **(d)** `T06` end: `skillhub skill commit release-checklist` and `Sent as SUB-...`. **(e)** Hub side: `W-approve`: **Waiting for review**, open each file, **Approve**. |
| Step titles | **On the hub computer** · **On the agent computer** · **Send a skill for review** · **Approve it on the hub** |
| Machine label | Always on, top left: "Hub computer" or "Agent computer", in the same colour as that terminal. For (c) and (d), split screen with the invite on the left. |
| Highlight | (b) the `skillhub connect --hub ... --invite ...` line; (c) the pasted line and `Hub check: reachable, token accepted`; (d) `Sent as SUB-...`; (e) the **Approve** button. |
| Narration | "Your hub can serve agents on other computers, on the same network or over Tailscale. On the hub computer, start the server as a background service and make a one-time code. On the other computer, install Skill Hub and paste the line it printed. Doctor checks the connection. That computer keeps no skills: its agents ask the hub. Curation stays on the hub. A skill made on the other computer is sent for review, and you approve it on the hub." |
| Note | `serve --install-service` is a card: it installs a real background service, so `render.sh` runs `skillhub serve` off camera. The code works once and expires in 10 minutes. On Windows, `skillhub serve` runs in its own terminal. `T05`/`T06` show the LAN address of the filming computer (`192.168.x.x`), a private address that is not reachable from the internet: kept as is (owner, 2026-10-16). **The final cut needs `OBS-3` from two real computers**, the gate of the promotion plan; until it exists, the scene says "simulated on one computer". |

## Scene 10. Tips and links (4:30, about 15 s): chapter "Tips and links"

| | |
|---|---|
| Screen | `MG`: two command cards, then the end card. |
| On-screen text | `skillhub status`: "what to do next" · `skillhub doctor`: "what is wrong, and one fix" · end card: `github.com/vantt/skillhub-mcp` · "Docs: Using Skill Hub, Curating skills, One hub from several machines" · "Questions and ideas: GitHub Discussions" · "Verified with Claude Code". |
| Narration | "Lost? skillhub status tells you the next step, and doctor tells you what is wrong. The code and the guides are on GitHub. Tell us what works, and what does not." |

---

## Shots the owner films (OBS)

Film them in a demo account or a fresh user, not in your daily setup: no real projects, chats or notifications on
screen. Terminal and Claude Code at 1920x1080, font large enough for a phone. Prompt `$ `.

1. **OBS-1, "curate my Skill Hub"** (scene 6). Before you start, run `video/demos/tapes/render.sh create-hub
   connect` and open Claude Code in `~/my-project` of that sandbox (`. video/demos/sandbox.sh /tmp/skillhub-tape-a`,
   then `cd ~/my-project && claude`). Approve the `Bash(skillhub:*)` trust dialog on camera. Type `curate my Skill
   Hub`; keep filming until the agent suggests creating a first skill.
2. **OBS-2, "use /commit-style"** (scenes 1 and 7c). After `render.sh first-skill`, in the same sandbox, change a
   file and stage only that file (`git add <file>`). Ask: `use /commit-style to write a commit message for my staged
   changes`. Keep filming until the message ending in `Checked-by: skillhub-demo` is on screen.
3. **OBS-3, two real computers** (scene 9). Do the README chapter "Hub and agent on different computers" on two
   computers: install on both; on the hub computer `serve --install-service` and `token invite`; on the agent
   computer paste the line with `-g`, run `doctor`, restart Claude Code, then ask it something the `commit-style`
   skill covers. Check every frame for addresses, hostnames and user names before you hand it over.

Claude Code needs to be signed in, and the sandbox has no credentials. Use a throwaway login, or film OBS-1 and OBS-2
on a separate machine with the same commands.

## Checks before the final cut

- Every command on screen matches `skillhub help <command>` of the filmed version.
- No real path, hostname, user name, e-mail, token or session link on screen. The private LAN address in `T05` and
  `T06` is allowed; check every `OBS` shot.
- Each chapter works alone: a viewer who jumps to "An agent on another computer" sees which earlier steps it reuses
  (Install, Create your hub, Check).
