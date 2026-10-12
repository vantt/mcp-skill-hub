# Use one hub from several machines

This guide explains how agents on one computer use the Skill Hub on another computer. If you have not set up a hub yet, follow the [Quickstart](../README.md#quickstart) first.

In this guide **machine B** is the computer that holds your hub, curates the skills and serves them. **Machine A** is a computer whose agents use those skills. You can have several machines A.

## When you need this

There are two ways to have your skills on a second machine.

| | Fetch the hub with Git | Use the hub on machine B over the network |
|---|---|---|
| How | `skillhub init <git-url>` in an empty folder on machine A | Machine B runs `skillhub serve`; machine A connects to it |
| New or changed skills | Reach machine A after you push on B and pull on A | Reach machine A at once, on the next `skill_resolve` |
| Curation | On every machine that has a copy | Only on machine B |
| Usage records (telemetry) | Each machine keeps its own | All on machine B, one place |
| Works without a connection to B | Yes, fully | Skills already used on A keep working; new skills wait for the connection |
| Needs | Git access to a remote | Both machines on one LAN or on Tailscale, or an SSH login to B |

Fetch the hub with Git when machine A is often offline, or when you want to curate on A as well (see [Move to another machine](user-guide.md#move-to-another-machine)). Use one hub over the network when one machine curates and the others should always see its current skills.

## How it works

### Two ways of working

1. **One machine.** Your agent starts Skill Hub over stdio. No network port, no token and no extra process. Nothing in this guide changes this.
2. **Machine A uses machine B.** Machine B runs `skillhub serve`, a network server over the same hub. On machine A, `skillhub connect --hub` registers a small stdio proxy, `skillhub mcp proxy`, which your agent starts like any other MCP server. The proxy talks to machine B.

Machine B keeps serving its own agents over stdio at the same time; the server is a separate process over the same hub.

Each scope (one project, or your user account with `-g`) points at one hub: `connect` for a hub on this machine, `connect --hub` for a hub on another machine. Connecting a scope to the other kind replaces its entry.

### What runs where

| What | Where |
|---|---|
| `skill_resolve`, `skill_feedback` | Answered by machine B |
| `skill_get` | Machine A fetches the whole skill folder once, in one request, checks that it arrived intact and keeps it in a local cache; later uses of the same version need no request at all |
| A skill's `check`, `setup` and scripts | Run on machine A, from the cached copy (`local.path` and the `SKILLHUB_*` variables point at machine A) |
| Curation: creating, editing, approving and importing skills, the web UI | Only on machine B; the server offers the three runtime tools and nothing else |
| Usage records | Written on machine B; calls from machine A are labelled `remote:<agent>`, for example `remote:claude-code` |

Skills whose content waits for your approval stay blocked for machine A exactly as for local agents. Fetched skills live in `~/.local/state/skillhub/remote` on Linux (`$XDG_STATE_HOME/skillhub/remote` when it is set), `~/Library/Application Support/skillhub/remote` on macOS and `%LocalAppData%\skillhub\remote` on Windows.

## Before you start

- Install `skillhub` on both machines, ideally in the same version (`skillhub --version`). `doctor` on machine A tells you when one side is too old for the other.
- Machine B has a hub, for example `~/skill-hub`, set up with `skillhub init` as in the Quickstart.
- Pick how machine A reaches machine B:
  - **Same LAN or Tailscale**: machine A can reach one of machine B's addresses. This is the default setup below.
  - **SSH tunnel**: for any network you do not trust. You need an SSH login to machine B from machine A.

> [!WARNING]
> `skillhub serve` listens on every network interface (`0.0.0.0`) by default, over plain HTTP. Anyone who can watch that network can read the hub token and the task text your agents send. The web UI (`skillhub serve web`) works the same way, and it can change skills: anyone who gets its address can add or edit the skills every connected agent uses. Tailscale and SSH encrypt the traffic; on any other network you do not trust, use the [SSH tunnel](#on-an-untrusted-network-use-an-ssh-tunnel).

## Set up on your LAN or Tailscale

### 1. On machine B: start the server

```bash
cd ~/skill-hub && skillhub serve
```

The first start creates the hub token. For each address machine A can use, the server prints the two commands to run there:

```text
Skill Hub is serving skills to agents on other machines.
It serves the runtime tools only (skill_resolve, skill_get, skill_feedback); curation stays on this
machine.

Hub folder:  /home/you/skill-hub
Token file:  /home/you/skill-hub/runtime/config/.hub-token
WARNING: Listening on all network interfaces over plain HTTP; use --addr 127.0.0.1:7420 to keep it on this machine.

eth0 (192.168.1.20), on the other machine:
  $ scp -p you@192.168.1.20:skill-hub/runtime/config/.hub-token ~/.skillhub-token-192.168.1.20-7420
  $ skillhub connect --hub http://192.168.1.20:7420 --token-file ~/.skillhub-token-192.168.1.20-7420

Tailscale (100.101.102.103), on the other machine:
  $ scp -p you@100.101.102.103:skill-hub/runtime/config/.hub-token ~/.skillhub-token-100.101.102.103-7420
  $ skillhub connect --hub http://100.101.102.103:7420 --token-file ~/.skillhub-token-100.101.102.103-7420

The token itself is never printed. Stop the server with Ctrl-C.
```

Virtual interfaces such as Docker bridges are left out of the list. Leave the server running. To start it at boot, see [Keep the server running](#keep-the-server-running-on-machine-b).

### 2. On machine A: copy the token file

Every request needs the hub's token. It lives in one file on machine B and is never committed. Run the `scp` line the server printed:

```bash
scp -p you@192.168.1.20:skill-hub/runtime/config/.hub-token ~/.skillhub-token-192.168.1.20-7420
```

Each hub gets its own token file on machine A, named after the hub's address and port, so tokens of different hubs never mix. `-p` keeps the copy readable by you only; the proxy refuses a token file other users can read. `scp` needs an SSH login to machine B; without one, copy the file by any private means and run `chmod 600` on it.

### 3. On machine A: connect your agents

Run the `connect` line the server printed, inside your project:

```bash
cd <your-project> && skillhub connect --hub http://192.168.1.20:7420 --token-file ~/.skillhub-token-192.168.1.20-7420
```

```text
Agent connection for project
/home/you/your-project
to the hub at http://192.168.1.20:7420 written.
  - Claude Code: connected (.mcp.json, .claude/settings.local.json, CLAUDE.md)
  - Codex: connected (.codex/config.toml, AGENTS.md)
  - Gemini CLI: connected (.gemini/settings.json, GEMINI.md)

Hub:          http://192.168.1.20:7420
Token:        /home/you/.skillhub-token-192.168.1.20-7420
              (only this path is written, never the token)
Hub check:    reachable, token accepted, Skill Hub <version> (this machine: <version>)
Skill cache:  /home/you/.local/state/skillhub/remote/192.168.1.20-7420-<id>

  - Curation stays on the hub machine: no curation server and no system-curator skill are installed
    here.

Next: restart your agent so it starts the proxy; if it cannot reach the hub, run `skillhub doctor`
```

Output is trimmed. Add `-g` to connect every project in your user account instead, and `--host claude`, `--host codex` or `--host gemini` to limit it to one agent. `--dry-run` checks the hub and shows the changes without writing anything. `connect` refuses a token file that another connection already uses for a different hub.

Then restart your agent. It now resolves skills on machine B and runs them on machine A.

<details>
<summary>Windows (PowerShell)</summary>

Windows PowerShell 5.1 has no `&&`, so separate the commands with `;`. If `~` does not expand in the `scp` line, write the local path as `$HOME\.skillhub-token-<host>-<port>`:

```powershell
scp -p you@192.168.1.20:skill-hub/runtime/config/.hub-token $HOME\.skillhub-token-192.168.1.20-7420
```

```powershell
cd <your-project>; skillhub connect --hub http://192.168.1.20:7420 --token-file $HOME\.skillhub-token-192.168.1.20-7420
```

</details>

What gets written, per agent:

| Item | Hub on another machine (`--hub`) |
|---|---|
| MCP entry `skillhub` | `skillhub mcp proxy --hub <url> --token-file <path>`; only the token file's path, never the token |
| Allowed directories | The skill cache of that hub, `<state>/skillhub/remote/<hub-id>` |
| Claude Code permission rules | The same rules as for a hub on this machine |
| Instruction block in `CLAUDE.md`, `AGENTS.md`, `GEMINI.md` | The same as for a hub on this machine |
| `system-curator` skill and `skillhub-curation` entry | Not installed: curation stays on machine B |

### Choose the addresses the server listens on

`--addr` takes one to three addresses, separated by commas. Each gets its own listener, and all of them share one token. A missing port takes the default, `7420`. To listen only on machine B's Tailscale address:

```bash
cd ~/skill-hub && skillhub serve --addr 100.101.102.103:7420
```

To listen on the LAN and Tailscale addresses, but not on other interfaces:

```bash
cd ~/skill-hub && skillhub serve --addr 192.168.1.20,100.101.102.103
```

If one address cannot be opened, the server does not start and names the address and the reason.

### Connect with a name instead of an address

You can also use machine B's name. Its short name (`http://machine-b:7420`) works as it is when it equals machine B's host name, which is Tailscale's default. Any other name, for example the full MagicDNS name `machine-b.tailnet-name.ts.net`, must be accepted by the server first:

```bash
cd ~/skill-hub && skillhub serve --allow-host machine-b.tailnet-name.ts.net
```

### Prefer -g for a hub on another machine

`status` and `doctor` send the token only to a hub on this machine (an SSH tunnel) or to a hub your user account names, never to a hub that only a project's files name. A cloned repository could name any address. For such a project-only hub, they check that it answers, and `doctor --fix` prints the `connect --hub` command for you to run instead of sending the token. Connect with `-g` so the checks can test the token too.

## On an untrusted network: use an SSH tunnel

Keep the server on machine B's own loopback address. An SSH tunnel carries machine A's requests to it, encrypted. The examples use port `7420`; any free port works.

### 1. On machine B: start the server on this machine only

```bash
cd ~/skill-hub && skillhub serve --addr 127.0.0.1:7420
```

The server prints a tunnel block instead of addresses:

```text
This machine only (127.0.0.1:7420); on the other machine, open an SSH tunnel and keep it running:
  $ ssh -N -L 7420:127.0.0.1:7420 you@<this-machine>
Then, in another terminal there:
  $ scp -p you@<this-machine>:skill-hub/runtime/config/.hub-token ~/.skillhub-token-127.0.0.1-7420
  $ skillhub connect --hub http://127.0.0.1:7420 --token-file ~/.skillhub-token-127.0.0.1-7420
```

Output is trimmed. `<this-machine>` is the name or address you use to log in to machine B with SSH.

### 2. On machine A: open the tunnel

```bash
ssh -N -L 7420:127.0.0.1:7420 you@machine-b
```

This command keeps running and prints nothing. While it runs, `http://127.0.0.1:7420` on machine A reaches the server on machine B. To keep it up without a terminal, see [Keep the tunnel up](#keep-the-tunnel-up).

### 3. On machine A: copy the token file and connect

In another terminal, run the `scp` line, then the `connect` line inside your project:

```bash
scp -p you@machine-b:skill-hub/runtime/config/.hub-token ~/.skillhub-token-127.0.0.1-7420
```

```bash
cd <your-project> && skillhub connect --hub http://127.0.0.1:7420 --token-file ~/.skillhub-token-127.0.0.1-7420
```

Then restart your agent.

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

## Keep the server running on machine B

A systemd user unit on machine B starts the server at boot. Use the full path that `command -v skillhub` prints, and run the unit in the hub folder. Leave out `--addr 127.0.0.1:7420` when machine A reaches B on the LAN or over Tailscale. Save this as `~/.config/systemd/user/skillhub-server.service`:

```text
[Unit]
Description=Skill Hub server for other machines

[Service]
WorkingDirectory=/home/you/skill-hub
ExecStart=/home/you/.local/bin/skillhub serve --addr 127.0.0.1:7420
Restart=on-failure
RestartSec=10

[Install]
WantedBy=default.target
```

```bash
systemctl --user daemon-reload
systemctl --user enable --now skillhub-server
loginctl enable-linger "$USER"     # keep user services running when you are not logged in
```

The server stops cleanly on Ctrl-C or `systemctl --user stop`: running requests finish and the usage records are written.

## Two kinds of token

Skill Hub uses two different tokens. Keep both private.

| | Hub token | Web session token |
|---|---|---|
| What it is for | Lets agents on other machines use the hub | Lets your browser use the web UI |
| Where it lives | The file `runtime/config/.hub-token` in the hub, readable by you only; machine A keeps a copy in `~/.skillhub-token-<host>-<port>` | In the address `skillhub serve web` prints, after `#token=` |
| How long it lasts | Until you run `skillhub token rotate` | Until that `serve web` stops; every start makes a new one |
| Who uses it | The proxy that `connect --hub` registers on machine A | Your browser only |
| What someone who has it can do | Resolve tasks and read approved skills, like an agent on machine A | Add and change skills in the hub, which reaches every connected agent |

The hub token is never printed. Only `skillhub token show` prints it, and only on a terminal, so it never lands in a log or an agent transcript. If you prefer not to keep a token file on machine A, leave out `--token-file` and set `SKILLHUB_HUB_TOKEN` in the environment your agent starts in.

## Check the connection

Run `status` on machine A, in the connected project. It probes the hub once and prints the next command:

```bash
cd <your-project> && skillhub status
```

`doctor` runs every check:

```bash
cd <your-project> && skillhub doctor
```

```text
This project uses the hub at http://192.168.1.20:7420

Check             Status  Detail
Used by           ok      Claude Code, Codex, Gemini CLI (project /home/you/your-project)
Hub token         ok      read from /home/you/.skillhub-token-192.168.1.20-7420
Hub reachable     ok      answered in 1 ms
Token accepted    ok      the hub accepts the token
Hub version       ok      <version> (this machine: <version>)
Skill cache       ok      writable: /home/you/.local/state/skillhub/remote/192.168.1.20-7420-<id>
Last fetch        ok      never: no skill was fetched from this hub yet
Connection files  ok      current

Healthy.
```

On a machine without a hub, `doctor` checks only the remote hub. The token is never printed. When a check fails, `doctor` prints one cause and one fix, and exits with `1`. For example, with the server stopped:

```text
Hub reachable     FAILED   Nothing answers at http://127.0.0.1:7420 (connection refused): the hub
                           is stopped, or the SSH tunnel or the Tailscale connection is down.
Token accepted    skipped  not checked: the hub did not answer

FIX:  Hub reachable: Skill Hub cannot start a hub on another machine. Check that
      `skillhub serve --addr 127.0.0.1:7420` is running on machine B and that the SSH tunnel is
      open on this machine, for example `ssh -N -L 7420:127.0.0.1:7420 <machine-b>`; if you reach
      machine B by its address instead, connect with an address `skillhub serve` prints.
```

`doctor --fix` cannot repair a hub on another machine. It never starts a server there and never reconnects for you, because reconnecting sends the token; it prints the command to run instead.

| `doctor` says | What it means | What to do |
|---|---|---|
| The token file ... does not exist on this machine | The path in the entry has no file | Run the `scp` line from the server's start again |
| The token file ... can be read by other users, so the proxy refuses it | The copy is not private | `chmod 600 <token-file>` |
| The token file ... does not hold a hub token | The wrong file was copied | Run the `scp` line from the server's start again |
| The entry has no --token-file and SKILLHUB_HUB_TOKEN is not set in this shell | The entry relies on the environment variable | Connect again with `--token-file`, or set `SKILLHUB_HUB_TOKEN` where your agent starts. `doctor` sees only its own shell |
| Nothing answers at ... | The server is stopped, or the tunnel or Tailscale is down | Run `skillhub serve` on B; reopen the tunnel; `tailscale status` |
| The hub at ... refuses requests for the host name ... | You connected with a name the server does not know | Add `--allow-host <name>` to `skillhub serve` on B, or connect with its address |
| Something answers at ..., but it is not a Skill Hub listener | Another program uses that port | Check the port; connect with a URL the server printed |
| The hub refused the token | The token was rotated, or the file comes from another hub | Run the `scp` line from the server's start again, then restart the agent |
| The hub is refusing requests for a minute after too many attempts with a wrong token | Several wrong tokens were tried | Wait a minute, then copy the current token file |
| The hub on machine B runs an older Skill Hub | B was not updated | Update skillhub on B and restart the server |
| This machine runs an older Skill Hub | A was not updated | `skillhub update` on A, then restart the agent |
| The skill cache ... is not writable | Fetched skills cannot be stored | `chmod u+w <directory>`, or set `XDG_STATE_HOME` to a writable directory where your agent starts |

## When the hub or the tunnel is down

Your agent keeps working; it only loses new skills until the connection is back.

- **A skill used before on this machine** still loads. `skill_get` answers from the cache on machine A and says so: "The hub could not be reached, so this is the copy of the skill cached on this machine; it may be older than the hub's current version."
- **A skill never used on this machine** cannot be fetched. `skill_get` and `skill_resolve` answer with the error `hub_unreachable`, "The Skill Hub at http://127.0.0.1:7420 could not be reached, so no skill is available right now.", and the suggested action "Continue the task without a skill. To restore skills, check that the hub is running and that the SSH tunnel or VPN to it is up." The agent carries on without a skill.

- A download that stops in the middle is discarded, never kept half-written. If the connection broke, the agent is asked to retry; if the download did not finish within two minutes, the hub counts as unreachable and the cached copy is used as above.
- A skill that machine B has just marked as waiting for review is never served from the cache.
- When the hub answers but cannot serve a skill right now (busy, or a file changed on disk), the agent gets `hub_error` and no cached copy, because such an answer may concern the skill's trust or content.

As soon as the hub answers again, the next call works; you do not need to restart the agent. If the server on machine B restarted, the proxy opens a new session by itself.

## The skill cache on machine A

Each skill version is downloaded once and kept until it changes. A changed skill is fetched again; a change of only its routing metadata on machine B is not. Three versions per skill are kept, versions unused for 30 days are removed, and each hub's cache stays under 512 MB.

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

`cache clear` acts at once and prints what it removed; `--dry-run` shows it first.

## Security in plain words

**What travels between the machines.** The text of each task your agent resolves, the approved skill files, the skill's runtime description (its `check` and `setup` commands), resolve results and feedback. That is why the server prints a warning when it listens on a network address, and why an untrusted network needs the SSH tunnel.

**What never travels.** Curation tools, the case journal, any write to your skills or the Git repository, stored secrets (`skillhub skill env set` values), paths on machine B's disk, skills waiting for your approval, and the token itself, which appears in no answer, log line, printed command or agent config file.

**What a stolen hub token allows.** The same as an agent on machine A: resolving tasks, reading approved skills and writing usage records on machine B. It does not allow any curation, approving content, reading unapproved content or running anything on machine B. Requests without a valid token are refused, and repeated wrong tokens are slowed down.

**What the web UI allows.** `skillhub serve web` also listens on every network interface by default, and its address carries a session token. Anyone who has that address can add and change skills in the hub, and every connected agent on every machine then uses them. To keep it on machine B, run `cd ~/skill-hub && skillhub serve web --addr 127.0.0.1:7421`.

**Where scripts run.** Only on machine A, from the copy it fetched. Machine B never runs a skill's check, setup or scripts for machine A.

**What machine B records.** Calls from machine A appear in the usage records with the client `remote:<agent>`, for example `remote:claude-code`. Neither the token nor machine A's address is stored.

**Who asks before setup commands.** Claude Code asks you before it runs `init`, `connect`, `doctor --fix`, `token rotate`, `cache clear` or `serve`. Codex and Gemini CLI do not prompt before them.

### Rotate the token

Rotate the token when a copy may have leaked or when a machine should lose access. On machine B, in a terminal:

```bash
cd ~/skill-hub && skillhub token rotate --dry-run
```

```text
Dry run: the hub token was not replaced.
Token file:  /home/you/skill-hub/runtime/config/.hub-token
Would change:
  - The token is replaced with a new random one.
  - Every machine that connects to this hub stops working until it gets the new token file.
  - A running listener uses the new token from its next request; it does not need a restart.

To replace the token, run in a terminal:
  $ skillhub token rotate
```

Replace it:

```bash
cd ~/skill-hub && skillhub token rotate
```

`token rotate` acts at once, and only in a terminal: run by a script or an agent, it refuses. It prints the `scp` line to run on each machine A that should keep access.

## Go back to one machine

To stop using the hub on machine B from machine A, remove the connection. `--cache` also clears the skills fetched from that hub:

```bash
cd <your-project> && skillhub connect --remove --cache
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
    /home/you/.skillhub-token-127.0.0.1-7420)
  - Codex: the remote hub at http://127.0.0.1:7420 (token file
    /home/you/.skillhub-token-127.0.0.1-7420)
  - Gemini CLI: the remote hub at http://127.0.0.1:7420 (token file
    /home/you/.skillhub-token-127.0.0.1-7420)

Cleared cached skills:
Hub                    Skills  Versions  Size
http://127.0.0.1:7420  1       1         237 bytes

  - Unrelated host configuration stays as it is, and nothing changes on the hub.

Next: restart your agent so it stops starting the Skill Hub server
```

Files that held only the Skill Hub entry are deleted; files with other content keep it. Add `--dry-run` to see the changes first, and `-g` to remove a user-account connection. To use a hub on machine A itself instead, connect the scope to a hub folder there; this replaces the remote entry in one step:

```bash
cd <your-project> && skillhub connect ~/skill-hub
```

Then stop the tunnel and, on machine B, the server (Ctrl-C, or `systemctl --user disable --now skillhub-server`). You can delete the token copy on machine A, and `skillhub token rotate` on machine B makes every old copy useless.

## Time a run from machine A

To see how fast your own connection is, run `scripts/remote-timing.sh` from the Skill Hub repository on machine A. It starts the same proxy your agent starts, with a new, empty cache, resolves a task, gets one skill twice and prints the times. It needs `python3`, and it never reads or prints the token. There is no PowerShell version: on Windows run it in WSL or Git Bash with `python3`.

```bash
cd <your-project>            # a project connected with connect --hub
sh scripts/remote-timing.sh --skill <skill-id> --task "<a task that skill is for>"

# or name the hub directly:
sh scripts/remote-timing.sh --skill <skill-id> --hub http://127.0.0.1:7420 --token-file ~/.skillhub-token-127.0.0.1-7420
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
| Start the server on every interface | B | `skillhub serve` |
| Start the server on chosen addresses | B | `skillhub serve --addr <host:port>[,<host:port>...] [--allow-host <name>]` |
| Start the server for an SSH tunnel | B | `skillhub serve --addr 127.0.0.1:7420` |
| Show the token (terminal only) | B | `skillhub token show` |
| Replace the token (terminal only) | B | `skillhub token rotate [--dry-run]` |
| Copy the token file | A | `scp -p <user>@<machine-b>:skill-hub/runtime/config/.hub-token ~/.skillhub-token-<host>-<port>` |
| Open the tunnel | A | `ssh -N -L 7420:127.0.0.1:7420 <user>@<machine-b>` |
| Connect to the hub | A | `skillhub connect --hub <url> --token-file <file> [-g] [--host <name>] [--dry-run]` |
| Check the connection | A | `skillhub status`, `skillhub doctor` |
| Clear fetched skills | A | `skillhub cache clear [--hub <url>] [--dry-run]` |
| Remove the connection | A | `skillhub connect --remove [--cache] [-g] [--dry-run]` |
| Time a skill fetch | A | `sh scripts/remote-timing.sh --skill <id> [--task <text>]` |

Run the commands on machine B inside the hub: `cd ~/skill-hub && skillhub <command>`.
