# CLI Onboarding Patterns for Local + Remote Tools

Date: 2026-10-11. Checked this session against clig.dev, the gh manual, the Tailscale KB, the Supabase CLI reference, and the Docker MCP Toolkit docs.

**Bottom line:** Skip the big wizard. Give the tool one obvious first command that prompts on a terminal and takes flags in scripts. Add a few re-runnable noun-verb commands, print the next command after each success, use one `status` for diagnosis, and write errors that say how to fix the problem.

1. **Prompt on a terminal, take flags in scripts.** `gh auth login` prompts when run bare. In scripts, `--with-token` (stdin), `--hostname`, or `GH_TOKEN` replace the prompts. clig.dev: "never require a prompt," and honor `--no-input`.
   *Fit:* `init` and `connect` should prompt only on a terminal. Each answer needs a flag. In a script, a missing value should fail with an error that names the flag.

2. **Print the next command.** `fly launch` ends with deploy hints. clig.dev: "suggest commands the user should run."
   *Fit:* `init` prints `connect <host>`. `connect` tells the user to restart the agent, then run `status`.

3. **Make commands safe to re-run.** Re-running `tailscale up` with new flags updates the node to match. It also warns when a previously set flag is missing and prints a copy-paste command with all of them.
   *Fit:* Merge into existing Claude/Codex/Cursor configs instead of overwriting them. Report "already configured" when nothing changes. Overwrite only with an explicit `--force`, as `supabase init --force` does.

4. **One address setting for server and client.** Ollama's `OLLAMA_HOST` sets where `serve` listens and where `run` connects. Docker does the same with `DOCKER_HOST` and contexts.
   *Fit:* Store one URL and token for both the network server and the client proxy. Local mode is that setting left empty. Avoid separate local and remote command trees.

5. **The server prints its connection details.** `supabase status` lists URLs and keys.
   *Fit:* When the network server starts, print its URL and the exact client command. Show the token once or write it to a file. Add `--output env|json`.

6. **Keep secrets out of flag values.** gh reads tokens from stdin. Tailscale accepts `--auth-key=file:/path`. gh and Supabase store credentials in the OS keychain.
   *Fit:* Add `--token-file` and stdin input. Have `status` warn when a config file that holds the token is tracked by Git.

7. **One `status` command as the diagnostic.** `gh auth status` shows account, host, and storage, and exits non-zero on a problem.
   *Fit:* One `status` covers the workspace, each agent host, and server reachability and auth. Each failing line ends with the command that fixes it. Add `doctor` only if the checks become slow.

8. **Consistent noun-verb names.** Examples: `docker context create|use|ls` and `docker mcp client connect vscode`, which writes `.vscode/mcp.json` and recommends gitignoring it. clig.dev says to reuse the same verbs and flags, and to avoid near-synonyms such as update/upgrade.
   *Fit:* Keep `init`, `connect`, `status`, and `serve` at the top level. Use noun-verb elsewhere, such as `host connect|disconnect|ls`. Use the same `--remote` and `--token-file` flags everywhere.

9. **Help that starts with examples, errors written for people.** clig.dev: lead `--help` with examples; errors give cause and fix, key line last.
   *Fit:* Top-level help shows the three-command happy path. A 401 from the proxy reads "token rejected by <url>; run `connect --remote <url> --token-file …`".

10. **Pairing by URL or code (optional).** `tailscale up` prints a login URL, and `--qr` shows it as a QR code. syncthing pairs devices by ID. gh's `--clipboard` copies a one-time device code.
   *Fit:* Defer this. A one-time code that the server exchanges for a token would remove copy-pasting, but it needs new server-side state.

## Trade-offs, risk, limitations
A wizard helps on the first run but is hard to script or re-run; pattern 1 gives both. Adoption risk is low for patterns 1–9, which are established conventions; only pattern 10 adds server state. I did not re-fetch docs for 1Password, mise/direnv, or Caddy, and the ranking is my judgment, not user testing.

## Unresolved questions
- Do all three agent hosts accept a token reference such as an env var or file, or must the token be written inline?
- Is pairing worth the extra server state at the current scale?
