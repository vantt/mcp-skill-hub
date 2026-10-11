# Use one hub from several machines

This guide explains how agents on one computer use the Skill Hub that runs on another computer. If you have not set up a hub yet, follow the Quickstart in the [README](../README.md) and the [user guide](user-guide.md) first.

In this guide **machine B** is the computer that holds your workspace, curates the skills and serves them. **Machine A** is a computer whose agents use those skills. You can have several machines A.

## When you need this

There are two ways to have your skills on a second machine.

| | Clone the workspace with Git | Use the hub on machine B over the network |
|---|---|---|
| How | `git clone` the workspace on machine A and run `skillhub maintain rebuild` | Machine B runs a listener; machine A connects to it |
| New or changed skills | Reach machine A after you push on B and pull on A | Reach machine A at once, on the next `skill_resolve` |
| Curation | On every machine that has a clone | Only on machine B |
| Usage records (telemetry) | Each machine keeps its own | All on machine B, one place |
| Works without a connection to B | Yes, fully | Skills already used on A keep working; new skills wait for the connection |
| Needs | Git access to a remote | An SSH login to B, or both machines on Tailscale |

Clone with Git when machine A is often offline, or when you want to curate on A as well (see [Move to another machine](user-guide.md#move-to-another-machine)). Use one hub over the network when one machine curates and the others should always see its current skills.

## How it works

### Two ways of working

1. **One machine (as before).** Your agent host starts `skillhub mcp serve` over stdio. No network port, no token and no extra process. Nothing in this guide changes this.
2. **Machine A uses machine B.** Machine B runs `skillhub serve mcp --addr ...`, a network listener over the same workspace. On machine A, `skillhub connect --hub` registers a small stdio proxy, `skillhub mcp proxy`, which your agent host starts like any other MCP server. The proxy talks to machine B.

Machine B keeps serving its own agents over stdio at the same time; the listener is a separate process over the same workspace.

Each scope (one project, or your user account with `-g`) points at one hub: `connect <path>` for a hub on this machine, `connect --hub` for a hub on another machine. Connecting a scope to the other kind replaces its entry.

### What runs where

| What | Where |
|---|---|
| `skill_resolve`, `skill_feedback` | Answered by machine B |
| `skill_get` | Machine A fetches the whole skill folder once, in one request, checks it against its digest and keeps it in a local cache; later uses of the same version need no request at all |
| A skill's `check`, `setup` and scripts | Run on machine A, from the cached copy (`local.path` and the `SKILLHUB_*` variables point at machine A) |
| Curation: creating, editing, approving and importing skills, the web UI | Only on machine B; the listener offers the three runtime tools and nothing else |
| Usage records | Written on machine B; calls from machine A are labelled `remote:<host>`, for example `remote:claude-code` |

Skills whose content waits for your approval stay blocked for machine A exactly as for local agents. Fetched skills live in `~/.local/state/skillhub/remote` on Linux (`$XDG_STATE_HOME/skillhub/remote` when it is set), `~/Library/Application Support/skillhub/remote` on macOS and `%LocalAppData%\skillhub\remote` on Windows.

## Before you start

- Install `skillhub` on both machines, ideally in the same version (`skillhub version`). `doctor` on machine A tells you when one side is too old for the other.
- Machine B has a workspace, for example `~/skillhub`, connected and working as described in the user guide.
- Pick how machine A reaches machine B:
  - **SSH tunnel**: you can log in to machine B with SSH from machine A. Nothing else is needed, and the listener stays on machine B's loopback address.
  - **Tailscale** (or another private VPN): both machines are in your tailnet.

The listener speaks plain HTTP and has no TLS. It refuses any address other machines could reach unless you say the network is private, so task text and skills never cross an open network unencrypted. SSH and Tailscale both encrypt the traffic.

## Set up with an SSH tunnel

The listener stays on `127.0.0.1` on machine B. An SSH tunnel carries machine A's requests to it. The examples use port `7420`; any free port works.

### 1. On machine B: start the listener

```bash
skillhub serve mcp --addr 127.0.0.1:7420 --workspace ~/skillhub
```

The first start creates the hub token and prints what to do on machine A:

```text
Skill Hub MCP listener is running with the runtime tools only (skill_resolve, skill_get,
skill_feedback).

Listening on:  127.0.0.1:7420
Workspace:     /home/you/skillhub
Token file:    /home/you/skillhub/runtime/config/.hub-token
Transport:     loopback only; other machines reach it through an SSH tunnel

On machine A:
Open the tunnel (keep it running):
  $ ssh -N -L 7420:127.0.0.1:7420 <user>@<this-machine>
Copy the token file to machine A (for example with scp) and pass its path there, then connect:
  $ skillhub connect --hub http://127.0.0.1:7420 --token-file /home/you/skillhub/runtime/config/.hub-token

The token itself is never printed; `skillhub token show` prints it on a terminal. Stop the listener
with Ctrl-C.
```

Leave it running. To start it at boot, see [Keep the listener running](#keep-the-listener-running-on-machine-b).

### 2. Bring the token to machine A

Every request needs the hub's token. It lives in one file on machine B, `<workspace>/runtime/config/.hub-token`, readable by you only and never committed. Copy that file to a fixed place on machine A and make it private:

```bash
mkdir -p ~/.config/skillhub
scp machine-b:skillhub/runtime/config/.hub-token ~/.config/skillhub/hub-token
chmod 600 ~/.config/skillhub/hub-token
```

The proxy refuses a token file other users can read. If you prefer not to keep a file, leave out `--token-file` in the next steps and set `SKILLHUB_HUB_TOKEN` to the token in the environment your agent host starts in. `skillhub token show` on machine B prints the token, and only on a terminal, so it never lands in a log or an agent transcript.

### 3. On machine A: open the tunnel

```bash
ssh -N -L 7420:127.0.0.1:7420 you@machine-b
```

This command keeps running and prints nothing. While it runs, `http://127.0.0.1:7420` on machine A reaches the listener on machine B. To keep it up without a terminal, see [Keep the tunnel up](#keep-the-tunnel-up).

### 4. On machine A: connect your agents

```bash
cd your-project
skillhub connect --hub http://127.0.0.1:7420 --token-file ~/.config/skillhub/hub-token --dry-run  # show the changes
skillhub connect --hub http://127.0.0.1:7420 --token-file ~/.config/skillhub/hub-token            # write files
```

Add `-g` to connect every project in your user account instead, and `--host claude`, `--host codex` or `--host gemini` to limit it to one agent. `--dry-run` checks the hub and shows the changes without writing anything:

```text
Dry run: agent connection for project /home/you/your-project to the hub at
http://127.0.0.1:7420 was not written.
Would change:
  - Claude Code: will connect (.mcp.json, .claude/settings.local.json, CLAUDE.md)
  - Codex: will connect (.codex/config.toml, AGENTS.md)
  - Gemini CLI: will connect (.gemini/settings.json, GEMINI.md)

Hub:          http://127.0.0.1:7420
Token:        /home/you/.config/skillhub/hub-token
              (only this path is written, never the token)
Hub check:    reachable, token accepted, Skill Hub <version> (this machine: <version>)
Skill cache:  /home/you/.local/state/skillhub/remote/127.0.0.1-7420-bd74e9ccea

  - Curation stays on the hub machine: no curation server and no system-curator skill are installed
    here.
  - To get the token to this machine, copy the hub's token file from machine B (the hub prints its
    path when `skillhub serve mcp --addr ...` starts, and `skillhub token show` there shows it), for
    example `scp <machine-b>:<workspace>/runtime/config/.hub-token ~/.config/skillhub/hub-token`,
    then `chmod 600` it; or set SKILLHUB_HUB_TOKEN in the environment the agent host starts the
    proxy with.

To write these files, run:
  $ skillhub connect --hub http://127.0.0.1:7420 --token-file /home/you/.config/skillhub/hub-token
```

Then restart your agent. It now resolves skills on machine B and runs them on machine A.

What gets written, per agent:

| Item | Hub on another machine (`--hub`) |
|---|---|
| MCP entry `skillhub` | `skillhub mcp proxy --hub <url> --token-file <path>`; only the token file's path, never the token |
| Allowed directories | The skill cache of that hub, `<state>/skillhub/remote/<hub-id>` |
| Claude Code permission rules | The same four rules as for a local hub |
| Instruction block in `CLAUDE.md`, `AGENTS.md`, `GEMINI.md` | The same as for a local hub |
| `system-curator` skill and `skillhub-curation` entry | Not installed: curation stays on machine B |

### Keep the tunnel up

Pick one of these on machine A. All of them need an SSH key that logs in without a password prompt, for example through `ssh-agent`.

**An entry in `~/.ssh/config`**, so the tunnel has a short name and closes cleanly when the connection dies:

```text
Host skillhub-tunnel
    HostName machine-b
    User you
    LocalForward 7420 127.0.0.1:7420
    ServerAliveInterval 30
    ServerAliveCountMax 3
    ExitOnForwardFailure yes
```

```bash
ssh -N skillhub-tunnel
```

**autossh** reopens the tunnel when it drops (install it with your package manager, for example `brew install autossh` or `sudo apt install autossh`):

```bash
autossh -M 0 -N skillhub-tunnel
```

**A systemd user unit** (Linux) starts the tunnel at login and restarts it when it fails. Save this as `~/.config/systemd/user/skillhub-tunnel.service`:

```text
[Unit]
Description=SSH tunnel to the Skill Hub on machine B
After=network-online.target

[Service]
ExecStart=/usr/bin/ssh -N -o BatchMode=yes skillhub-tunnel
Restart=always
RestartSec=10

[Install]
WantedBy=default.target
```

```bash
systemctl --user daemon-reload
systemctl --user enable --now skillhub-tunnel
systemctl --user status skillhub-tunnel
```

### Keep the listener running on machine B

A systemd user unit on machine B starts the listener at boot. Use the full path that `command -v skillhub` prints. Save this as `~/.config/systemd/user/skillhub-listener.service`:

```text
[Unit]
Description=Skill Hub MCP listener

[Service]
ExecStart=/home/you/.local/bin/skillhub serve mcp --addr 127.0.0.1:7420 --workspace /home/you/skillhub
Restart=on-failure
RestartSec=10

[Install]
WantedBy=default.target
```

```bash
systemctl --user daemon-reload
systemctl --user enable --now skillhub-listener
loginctl enable-linger "$USER"     # keep user services running when you are not logged in
```

The listener stops cleanly on Ctrl-C or `systemctl --user stop`: running requests finish and the usage records are written.

## Set up with Tailscale

With Tailscale the listener binds to machine B's Tailscale address, and no tunnel is needed. Steps 2 and 4 of the SSH setup stay the same; only the address changes.

### On machine B: start the listener on the Tailscale address

```bash
tailscale ip -4                       # for example 100.101.102.103
skillhub serve --addr 100.101.102.103:7420 --workspace ~/skillhub
```

Without `--addr`, `skillhub serve` listens on every interface (`0.0.0.0:7420`), Tailscale included. Either way the transport is plain HTTP, and the banner says so in one warning line:

```text
WARNING: Listening on a network address over plain HTTP; use --addr 127.0.0.1:7420 to keep it on this machine.
```

### On machine A: connect

Copy the token as in [step 2](#2-bring-the-token-to-machine-a), then connect your user account with the Tailscale address:

```bash
skillhub connect -g --hub http://100.101.102.103:7420 --token-file ~/.config/skillhub/hub-token
```

You can also use machine B's name. Its short name (`http://machine-b:7420`) works as it is when it equals machine B's host name, which is Tailscale's default. Any other name, for example the full MagicDNS name `machine-b.tailnet-name.ts.net`, must be accepted by the listener first: add `--allow-host machine-b.tailnet-name.ts.net` to `skillhub serve mcp` on machine B.

Prefer `-g` for a Tailscale hub. `doctor` sends the token only to a hub on this machine (an SSH tunnel) or to a hub your user account names, never to a hub that only a project's files name, because a cloned repository could name any address. For such a project-only hub, `doctor` checks that it answers and tells you to check the token with `connect --hub ... --dry-run`, which writes nothing.

## Check the connection

Run `doctor` on machine A, in the connected project or anywhere for a `-g` connection:

```bash
skillhub doctor
```

```text
The connection to the remote hub works.

Remote hub:  http://127.0.0.1:7420
Used by:     Claude Code, Codex, Gemini CLI (project /home/you/your-project)
Token file:  /home/you/.config/skillhub/hub-token

Check           Status  Detail
Hub token       ok      read from /home/you/.config/skillhub/hub-token
Hub reachable   ok      answered in 1 ms
Token accepted  ok      the hub accepts the token
Hub version     ok      <version> (this machine: <version>)
Skill cache     ok      writable: /home/you/.local/state/skillhub/remote/127.0.0.1-7420-bd74e9ccea
Last fetch              2026-10-11 16:56 (1 skill cached)
```

On a machine without a workspace, `doctor` checks only the remote hub. The token is never printed. When a check fails, `doctor` prints one cause and one fix. For example, with the tunnel closed:

```text
Hub reachable   FAILED   Nothing answers at http://127.0.0.1:7420 (connection refused): the hub is
                         stopped, or the SSH tunnel or the Tailscale connection is down.
Token accepted  skipped  not checked: the hub did not answer

FIX:  Check that `skillhub serve mcp --addr 127.0.0.1:7420` is running on machine B and that the
      SSH tunnel is open on this machine, for example `ssh -N -L 7420:127.0.0.1:7420 <machine-b>`;
      if you use Tailscale instead, check `tailscale status` on both machines.
```

| `doctor` says | What it means | What to do |
|---|---|---|
| The token file ... does not exist on this machine | The path in the entry has no file | Copy the token file from machine B again and `chmod 600` it |
| The token file ... can be read by other users, so the proxy refuses it | The copy is not private | `chmod 600 <token-file>` |
| The token file ... does not hold a hub token | The wrong file was copied | Copy `<workspace>/runtime/config/.hub-token` from machine B |
| The entry has no --token-file and SKILLHUB_HUB_TOKEN is not set in this shell | The entry relies on the environment variable | Connect again with `--token-file`, or set `SKILLHUB_HUB_TOKEN` where your agent host starts. `doctor` sees only its own shell |
| Nothing answers at ... | The listener is stopped, or the tunnel or Tailscale is down | Start the listener on B; reopen the tunnel; `tailscale status` |
| The hub at ... refuses requests for the host name ... | You connected with a name the listener does not know | Add `--allow-host <name>` to `skillhub serve mcp` on B, or connect with its address |
| Something answers at ..., but it is not a Skill Hub listener | Another program uses that port | Check the port; connect with the URL the listener printed |
| The hub refused the token | The token was rotated, or the file comes from another hub | Copy the current token file from machine B again, then restart the agent |
| The hub is refusing requests for a minute after too many attempts with a wrong token | Several wrong tokens were tried | Wait a minute, then copy the current token file |
| The hub on machine B runs an older Skill Hub | B was not updated | Update skillhub on B and restart the listener |
| This machine runs an older Skill Hub | A was not updated | `skillhub update` on A, then restart the agent |
| The skill cache ... is not writable | Fetched skills cannot be stored | `chmod u+w <directory>`, or set `XDG_STATE_HOME` to a writable directory where the agent host starts |

## When the hub or the tunnel is down

Your agent keeps working; it only loses new skills until the connection is back.

- **A skill used before on this machine** still loads. `skill_get` answers from the cache on machine A and says so: "The hub could not be reached, so this is the copy of the skill cached on this machine; it may be older than the hub's current version."
- **A skill never used on this machine** cannot be fetched. `skill_get` and `skill_resolve` answer with the error `hub_unreachable`, "The Skill Hub at http://127.0.0.1:7420 could not be reached, so no skill is available right now.", and the suggested action "Continue the task without a skill. To restore skills, check that the hub is running and that the SSH tunnel or VPN to it is up." The agent carries on without a skill.

- A download that stops in the middle is discarded, never kept half-written. If the connection broke, the agent is asked to retry; if the download did not finish within two minutes, the hub counts as unreachable and the cached copy is used as above.
- A skill that machine B has just marked as waiting for review is never served from the cache.
- When the hub answers but cannot serve a skill right now (busy, or a file changed on disk), the agent gets `hub_error` and no cached copy, because such an answer may concern the skill's trust or content.

As soon as the hub answers again, the next call works; you do not need to restart the agent. If the listener on machine B restarted, the proxy opens a new session by itself.

## The skill cache on machine A

Each skill version is downloaded once and kept by its digest. A changed skill is fetched again; a change of only its routing metadata on machine B is not. Three versions per skill are kept, versions unused for 30 days are removed, and each hub's cache stays under 512 MB.

To remove the cached copies yourself:

```bash
skillhub cache clear --dry-run                         # show every hub's cache
skillhub cache clear --hub http://127.0.0.1:7420       # clear one hub's cache
```

```text
Dry run: the copies of skills fetched from remote hubs were not removed.
Cache directory:  /home/you/.local/state/skillhub/remote

Would change:
Hub                    Skills  Versions  Size
http://127.0.0.1:7420  1       1         237 bytes

  - An agent fetches each skill again from its hub the next time it uses it.
  - The skills' working directories (installed dependencies) and stored secrets are kept.
  - Nothing on the hub changes.

Next: Clear the cache:
  $ skillhub cache clear
```

## Security in plain words

**What travels between the machines.** The text of each task your agent resolves, the approved skill files with their digests, the skill's runtime description (its `check` and `setup` commands), resolve results and feedback. That is why the listener accepts only loopback (an SSH tunnel) or a network you confirmed as private.

**What never travels.** Curation tools, the case journal, any write to your skills or the Git repository, stored secrets (`skillhub skill env set` values), paths on machine B's disk, skills waiting for your approval, and the token itself, which appears in no answer, log line, printed command or host config file.

**What a stolen token allows.** The same as an agent on machine A: resolving tasks, reading approved skills and writing usage records on machine B. It does not allow any curation, approving content, reading unapproved content or running anything on machine B. Requests without a valid token are refused, and repeated wrong tokens are slowed down.

**Where scripts run.** Only on machine A, from the copy it fetched. Machine B never runs a skill's check, setup or scripts for machine A.

**What machine B records.** Calls from machine A appear in the usage records with the client `remote:<host>`, for example `remote:claude-code`. Neither the token nor machine A's address is stored.

### Rotate the token

Rotate the token when a copy may have leaked or when a machine should lose access. On machine B:

```bash
skillhub token rotate --workspace ~/skillhub --dry-run   # show what changes
skillhub token rotate --workspace ~/skillhub             # replace the token (in a terminal)
```

```text
Dry run: the hub token was not replaced.
Token file:  /home/you/skillhub/runtime/config/.hub-token
Would change:
  - The token is replaced with a new random one.
  - Every machine that connects to this hub stops working until it gets the new token file.
  - A running listener uses the new token from its next request; it does not need a restart.

To replace the token, run in a terminal:
  $ skillhub token rotate --workspace /home/you/skillhub
```

Then copy the new token file to every machine A that should keep access, as in [step 2](#2-bring-the-token-to-machine-a).

## Go back to one machine

To stop using the hub on machine B from machine A, remove the connection. `--cache` also clears the skills fetched from that hub:

```bash
cd your-project
skillhub connect --remove --cache --dry-run  # show what is removed
skillhub connect --remove --cache            # remove the entry and clear the cache
```

```text
Skill Hub connection for project
/home/you/your-project
removed.
  - Claude Code: removed (.mcp.json deleted, .claude/settings.local.json deleted, CLAUDE.md deleted)
  - Codex: removed (.codex/config.toml deleted, AGENTS.md deleted)
  - Gemini CLI: removed (.gemini/settings.json deleted, GEMINI.md deleted)
The removed entries pointed at:
  - Claude Code: the remote hub at http://127.0.0.1:7420 (token file
    /home/you/.config/skillhub/hub-token)
  - Codex: the remote hub at http://127.0.0.1:7420 (token file
    /home/you/.config/skillhub/hub-token)
  - Gemini CLI: the remote hub at http://127.0.0.1:7420 (token file
    /home/you/.config/skillhub/hub-token)

Cleared cached skills:
Hub                    Skills  Versions  Size
http://127.0.0.1:7420  1       1         237 bytes

  - Unrelated host configuration stays as it is, and nothing changes on the hub.

Next: restart your agent so it stops starting the Skill Hub server
```

Files that held only the Skill Hub entry are deleted; files with other content keep it. Use `-g` to remove a user-account connection. To use a hub on machine A itself instead, connect the scope to a workspace there; this replaces the remote entry in one step:

```bash
skillhub connect ~/skillhub
```

Then stop the tunnel and, on machine B, the listener (Ctrl-C, or `systemctl --user disable --now skillhub-listener`). You can delete the token copy on machine A, and `skillhub token rotate` on machine B makes every old copy useless.

## Time a run from machine A

To see how fast your own connection is, run `scripts/remote-timing.sh` from the Skill Hub repository on machine A. It starts the same proxy your agent host starts, with a new, empty cache, resolves a task, gets one skill twice and prints the times. It needs `python3`, and it never reads or prints the token.

```bash
cd your-project            # a project connected with connect --hub
sh scripts/remote-timing.sh --skill <skill-id> --task "<a task that skill is for>"

# or name the hub directly:
sh scripts/remote-timing.sh --skill <skill-id> --hub http://127.0.0.1:7420 --token-file ~/.config/skillhub/hub-token
```

```text
Skill Hub remote timing
  Hub:     http://127.0.0.1:7420
  Entry:   ./.mcp.json
  Skill:   release-notes
  Task:    write the release notes for a tagged version
  Cache:   a new, empty cache for this run (removed afterwards)

  Step                           Time  Result
  start the proxy             25.5 ms  ready
  skill_resolve               20.6 ms  recommends release-notes
  skill_get, first fetch      47.7 ms  downloaded from the hub
  skill_get, cached repeat     0.9 ms  served from the cache on this machine

Targets: first fetch within 500 ms on a LAN and within 2 s on a 100 ms link;
cached repeat within 20 ms. The first call of a session also opens the
connection to the hub, so skill_resolve pays a few extra round trips here.
```

On a link with a long round trip, the first call of each agent session takes a few round trips longer than the others, because it opens the connection to the hub.

## Command cheat sheet

| Task | Where | Command |
|---|---|---|
| Start the listener (SSH tunnel) | B | `skillhub serve mcp --addr 127.0.0.1:7420 [--workspace <path>]` |
| Start the listener (Tailscale) | B | `skillhub serve --addr <tailscale-ip>:7420 [--allow-host <name>]` |
| Show the token (terminal only) | B | `skillhub token show` |
| Replace the token | B | `skillhub token rotate [--dry-run]` |
| Open the tunnel | A | `ssh -N -L 7420:127.0.0.1:7420 <user>@<machine-b>` |
| Connect to the hub | A | `skillhub connect --hub <url> --token-file <path> [-g] [--host <name>] [--dry-run]` |
| Check the connection | A | `skillhub doctor` |
| Clear fetched skills | A | `skillhub cache clear [--hub <url>] [--dry-run]` |
| Remove the connection | A | `skillhub connect --remove [--cache] [-g] [--dry-run]` |
| Time a skill fetch | A | `sh scripts/remote-timing.sh --skill <id> [--task <text>]` |
