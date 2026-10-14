# Phase 5: Remote submission

## Goal

An agent or a person on a machine that uses a remote hub runs `skillhub skill commit` as on the hub machine. The files
travel to a queue on the hub over a plain HTTP endpoint of `skillhub serve`. Nothing on the hub changes, and nothing is
served, until a person on the hub machine reviews and approves the submission on the web (D9) or in the terminal.

## Decisions implemented

- **D4**:
  - `skill commit` works from a remote-connected machine;
  - the transport is a plain HTTP endpoint on `skillhub serve` (port 7420), the same kind as `GET /mcp/bundle`, and no
    MCP tool is added;
  - any machine with the hub token can submit, with no switch and no extra token;
  - a submission only enters a queue on the hub, never changes a skill, and is released only after a person on the hub
    approves it (as third-party content);
  - `skillhub status` and the web on the hub report waiting submissions;
  - an agent on the other machine only runs CLI commands.
- **D9**: submissions appear on "Waiting for review", with the full-skill review screen and Approve. In the terminal,
  review is a y/N prompt. An agent never approves.

## Prerequisites

Phase 2 (the folder reader with the filter, the marker, the commit preview and chain) and phase 4 (review queue, full
review screen, approve).

## Context

- The `httpGate` allowlists paths (`internal/delivery/mcpserver/http.go:236-242`). It checks host, origin, path,
  shutdown, pair, token (`hubtoken.Source.Valid`) and the rate limiter (`:272`), then routes `/mcp/bundle`
  (`distribution.go:23, 123`).
- The bundle format: `skillruntime.WriteBundle(w, metadata, contents)` (`remote_cache.go:108`), an unexported
  `extractBundle` (`:245`) with `limitedReader`, and `ValidBundlePath` (`:167`). Limits: `BundleMaxFiles = 512`,
  `BundleMaxBytes = 16 MiB`, `BundleMaxMetadataBytes = 2 MiB` (`remote_cache.go:40-43`).
- The client side: remote entries come from `app.FindRemoteEntries` (`doctor_remote.go:360`), with `HubURL` and
  `TokenFile` (`app/role.go:33-45`). The token is resolved by `hubtoken.ClientPath` / `ClientStorePath`. The proxy
  HTTP client adds the bearer token only for the hub host (`mcpserver/proxy.go:209, 329-338`), and `requestBundle`
  (`:737-760`) is the GET pattern to copy for a POST.
- The guard that refuses curation in a remote project: `internal/delivery/cli/workspace_resolve.go:94-108`.
- The runtime `tools/list` budget test: `internal/delivery/mcpserver/tools_size_test.go` (budgets at `:20, :27`).

## Design

### Wire (open question 20)

- `POST /mcp/submissions`, behind the same gate and bearer token as `/mcp/bundle`. The request body is the bundle
  archive written with `WriteBundle`: metadata carries `skill_id`, `base_version` (empty for a new skill),
  `client_label` (the submitting machine's hostname, a label only), and `excluded` (paths the client filter dropped).
  The file entries are the skill's content files.
- The server reads the archive with the same limited reader and path checks. `extractBundle` is exported as
  `ExtractBundle(archive, staging)`, with no digest pin for submissions. A size or count overflow answers 413 with the
  error envelope `resource_limits_exceeded`. Hub meta paths in the archive are rejected (400 `invalid_request`).
- Reply `202 {"submission_id": "SUB-…", "status": "waiting"}`. The submitter's view is the same whether the skill exists
  or not, so the endpoint reveals nothing new to a token holder (they can already list skills).

### Queue on the hub (open question 21)

- `runtime/submissions/SUB-<ulid>/submission.json` plus `files/…`. These are untracked, so the hub's Git stays clean.
  Written atomically (stage, then rename).
- Caps: at most 50 waiting submissions and 256 MiB in total. Over a cap, answer 429 with a clear message. A new
  submission for the same `skill_id` and `client_label` replaces the older waiting one.
- `internal/app/submissions.go`: `ListSubmissions`, `ReadSubmission`, `DiscardSubmission`, and
  `PreviewSubmission(id) (SkillProposal)`. The preview reuses phase 2's commit preview with the submission files as
  the folder, and its base is `base_version`, looked up as in phase 2.

### Remote `skill commit` (client)

- In `workspace_resolve.go`, the remote guard lets `skill commit`, `skill checkout` (open question 24) and
  `skill create` (local only) pass. Every other curation command still says "Skills are curated on the hub machine".
- `skill commit <id|folder>` in a remote project reads the folder with the same filter and marker rules. The preview
  is local (the file list, the excluded files, and a diff against the checked-out base when a marker exists). It then
  needs the prompt or `--yes` like every send:
  "Send review-checklist to the hub at http://hub-b:7420 for review? [y/N]".
- After sending: "Sent as SUB-… . A person on the hub machine reviews it; agents get it only after approval." The
  marker is not moved to a new base, because nothing is released yet.
- `skill checkout` in a remote project fetches `GET /mcp/bundle` (active, servable skills only) and writes the marker
  with the hub URL and the bundle digest.

### Review on the hub

- `status`: a `submission_waiting` action ("1 submitted skill waits for review: team-release from machine-a,
  2026-10-14") with the next command. `schemas/curation-home-v1.schema.json` gains the kind.
- CLI (open question 22): `skillhub skill commit SUB-…` previews like a folder commit. On a TTY it asks:
  apply → approve (always) → activate (new skill only). All answers land in one commit, and the commit message
  records "submitted from <client_label>" (open question 23). `--discard` drops it. Without a TTY it refuses to
  apply. `skill review <id>` names waiting submissions for that id.
- Web: `GET /api/v1/review-queue` gains `kind: "submission"`. `GET /api/v1/submissions/{sid}` returns the same shape
  as review-content. `POST /api/v1/submissions/{sid}/approve/preview` → proposal (apply plus approval in one change),
  confirmed through the existing proposal confirm. `POST /api/v1/submissions/{sid}/discard`. The full review screen
  from phase 4 renders submissions with "Submitted from machine-a" in the origin line.
- A submission is never applied without the approval in the same change. A new skill lands as an approved draft, and
  activation is the usual separate step (or the CLI chain's third prompt).

### Docs touched in this phase (contracts and threat model)

- `docs/design/remote-hub.md`, "What a stolen token allows": it can now fill the submission queue within the caps. It
  still cannot change or serve any skill.
- `docs/remote-hub-guide.md`: the "never travel" sentence (around `:491-493`) is updated, and the endpoint, caps and
  review path are added.
- `docs/contracts/web-api.md` and `docs/contracts/error-codes.md` (statuses 413, 429 on 7420).

## Files

- Create `internal/delivery/mcpserver/submission_http.go` and its test. Modify `http.go` (allowlist, body limit for
  the route).
- Modify `internal/skillruntime/remote_cache.go` (export `ExtractBundle`, with no behaviour change for bundles).
- Create `internal/app/submissions.go` and its test. Modify `internal/app/skill_commit.go` (the submission source),
  `curation_home.go`, `review_queue.go`.
- Modify `internal/delivery/cli/workspace_resolve.go`, `skill_commit.go` (remote send, `--discard`),
  `skill_checkout.go` (remote fetch), `status_doctor.go`.
- Create `internal/delivery/web/routes_submissions.go` and its test. Modify `server.go`.
- Modify `web/src/screens/review/WaitingForReviewScreen.tsx`, `FullReviewScreen.tsx` and `api/queries.ts`. Extend
  `web/e2e/review-queue.spec.ts`.
- Modify `schemas/curation-home-v1.schema.json`, `docs/design/remote-hub.md`, `docs/remote-hub-guide.md`, and
  `docs/contracts/web-api.md`.

## Steps (tests first)

1. Endpoint tests on `httptest`:
   - no or wrong token gives the existing 403;
   - a valid archive gives 202, and the queue dir appears;
   - 513 files or 17 MiB give 413;
   - a path traversal or hub meta path gives 400;
   - the 51st waiting submission gives 429;
   - a same skill and label replaces the older one;
   - the rate limiter applies.
2. Queue tests: atomic write, list order, discard, and the total size cap.
3. End to end (Go, like `fresh_clone_e2e_test.go`):
   - start `skillhub serve` on a random loopback port with a temp hub, and connect a temp project with
     `connect --hub … --token-file …`;
   - run `skill create`, edit the folder, then `skill commit --yes`;
   - the hub tree and HEAD are unchanged, `status --json` lists `submission_waiting`, and `skill_resolve` and
     `skill_get` know nothing of the skill;
   - then, on the hub, `skill commit SUB-…` with TTY answers "y y y" gives one commit, and the skill is served.
4. Remote checkout test: checkout through the bundle, edit, commit gives a submission with `base_version` = the bundle
   digest. The hub preview shows a diff against that version.
5. Web route tests and the Playwright waiting-page flow: submission → review → open the files → Approve.
6. `go test ./internal/delivery/mcpserver -run 'TestRuntimeToolListStaysWithinBudget|TestProxyRuntimeToolsMatchHubRuntimeProfile'`
   proves that `tools/list` did not grow.

## Mock outputs

Machine A:

```text
$ skillhub skill commit team-release
team-release is not on the hub at http://hub-b:7420: this submits a new skill for review.
Files (2, 227 B): SKILL.md, scripts/bump.sh
Left out: .env (looks like a secret)
Send team-release to the hub for review? [y/N] y
Sent as SUB-01JA2…  A person on the hub machine reviews it; agents get it only after approval.
```

Machine B:

```text
$ skillhub status
…
Waiting
  - 1 submitted skill waits for review: team-release from machine-a (2026-10-14)
    Review: skillhub skill commit SUB-01JA2…   or the web page "Waiting for review"

$ skillhub skill commit SUB-01JA2…
team-release (submitted from machine-a): a new skill.
Files (2, 227 B):
  SKILL.md            181 B
  scripts/bump.sh      46 B
Routing, suggested from the description (edit with --edit-routing): …
Submitted content is untrusted until you approve it.
Show the diff: skillhub skill commit SUB-01JA2… --diff
Add team-release as a draft? [y/N] y
Approve this content for agents? [y/N] y
Activate team-release now? [y/N] n
Committed 9a7c1e0 "Add team-release (submitted from machine-a), approve content".
Agents using this hub get it once you activate it.
```

## Validation

```bash
go test ./internal/skillruntime -run 'Bundle' -count=1
go test ./internal/app -run 'Submission|ReviewQueue' -count=1
go test ./internal/delivery/mcpserver -run 'Submission|Gate|Budget|ProxyRuntime' -count=1
go test ./internal/delivery/cli -run 'Commit|Checkout|Remote|Status' -count=1
go test ./internal/delivery/web -run 'Submission|ReviewQueue' -count=1
make web-e2e
make check
```

## Risks

- **A wider token surface** (D4 accepts this). Caps, the rate limit, review-only release, and threat-model text cover
  it. The per-invite submit permission stays deferred.
- **Disk filling.** Mitigated by the count and size caps. `status` shows the queue so it gets noticed.
- **A stale base on a remote checkout** (the hub changed, the bundle snapshot was GC'd). The phase 2 base lookup walks
  the history, else every differing file is a conflict on the hub. Nothing is applied silently.
- **The client label is spoofable.** It is shown as "submitted from" text only, never used for trust.

## Rollback

Revert the phase commits. The endpoint disappears (404 from the gate), and remote `skill commit` is refused again.
Delete `runtime/submissions/`.

## Effort

2.5 days.
