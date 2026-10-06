# Web API Contract

**Status:** Shipped in V1  
**Base Path:** `/api/v1`  
**Delivery Adapter:** `internal/delivery/web` (`skillhub serve web`)  
**Transport:** HTTP/1.1 with JSON payloads  

---

## 1. Overview and Security Model

The Skill Hub Web API provides a local HTTP interface for the embedded single-page application. It wraps the same application services in `internal/app` that power the CLI and MCP surfaces.

### 1.1 Authentication
- On startup, `skillhub serve web` generates a 32-byte cryptographically secure random token (64 lowercase hexadecimal characters). The token is held in memory only for the lifetime of the process.
- The startup output prints the token in the URL fragment (`http://<host>:<port>/#token=<token>`). Browsers do not send URL fragments in HTTP requests or server access logs.
- The Web UI client extracts the token from the fragment, stores it in application memory, and includes it on every API call as an HTTP header:
  ```http
  Authorization: Bearer <token>
  ```
- Any request to `/api/*` lacking a valid Bearer token returns `HTTP 401 Unauthorized`.

### 1.2 Host Header Validation
- The server inspects the HTTP `Host` header on every request.
- Permitted hosts include `127.0.0.1`, `localhost`, the bound IP address, and any explicit hostnames or ports passed via `--allow-host <host[:port]>`.
- Requests with an unrecognized or untrusted `Host` header (for example, DNS rebinding attacks using external domains) are rejected with `HTTP 421 Misdirected Request`.

### 1.3 Origin and Referrer Restrictions
- Cross-origin requests are forbidden. The server checks the `Origin` header on state-changing requests and requires it to match the server's own address.
- Security response headers applied to all responses:
  - `Content-Security-Policy: default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'`
  - `X-Content-Type-Options: nosniff`
  - `Referrer-Policy: no-referrer`
  - `Cache-Control: no-store`

### 1.4 Network Binding and Listen Rules
- **Automatic Multi-IP Detection:** When the host machine has two or more non-loopback IPv4 addresses (such as Wi-Fi, Ethernet, Docker bridges, or Tailscale interfaces), the server binds to `0.0.0.0` (all interfaces) to allow browser access across local network interfaces.
- **Single-Interface / Default:** On machines with only one non-loopback address or when loopback mode is forced, the server binds strictly to `127.0.0.1`.
- **Flags:**
  - `--loopback-only`: Forces listening strictly on `127.0.0.1` regardless of network interfaces.
  - `--addr <host:port>`: Overrides the listen address and port.
  - `--allow-host <host[:port]>`: Adds an allowed host header.
  - `--no-open`: Prevents automatic browser launch.

---

## 2. Safety Boundaries and Invariants

1. **No distill run mutations:** No web route starts, retries, or submits a distill run. The web UI only displays handoff instructions (`/sources/distill`) and reads run status (`/sources/runs/:id`). Distill execution is strictly reserved for the curator agent via MCP (`curation_run_start`, `curation_run_retry`, `curation_run_submit`).
2. **No automatic content approval:** No web route approves third-party skill content. When third-party skills require review, the Web UI displays a read-only codebox showing the CLI command `skillhub skill review <id> --approve`. Content approval requires human review via the CLI.
3. **No orphan sources:** Source creation and watch operations require a skill target.
4. **Two-step mutations:** Mutating endpoints use the Preview → Confirm protocol with immutable proposal pins (`proposal_id`, `proposal_digest`, `base_version`).

---

## 3. Endpoints

Every endpoint maps directly to an application service method in `internal/app`.

| Route Pattern | Handler | Application Service Call | Description |
|---|---|---|---|
| `/` | `s.handleAssets` | Embedded assets | Serves SPA `index.html` or static assets (`internal/delivery/web/dist`) |
| `/api/` | `s.handleUnknownAPI` | N/A | Returns 404 with standard error envelope for unrecognized API routes |
| `GET /api/v1/session` | `s.handleSession` | Direct workspace info | Returns current session workspace ID, name, and version |
| `GET /api/v1/home` | `s.handleHome` | `s.app.GetCurationHome` | Returns workspace status, counts, and ranked next actions |
| `GET /api/v1/skills` | `s.handleSkills` | `s.app.ListSkills` | Lists all skills with lifecycle and enriched upstream status |
| `GET /api/v1/skills/{id}` | `s.handleSkillDetail` | `s.app.GetSkillDetail` | Returns full skill details, instructions, metadata, and provenance |
| `GET /api/v1/skills/{id}/review` | `s.handleSkillReview` | `s.app.ReviewSkill` | Returns comprehensive diagnostic review and validation facts |
| `GET /api/v1/skills/{id}/runtime` | `s.handleSkillRuntime` | `s.app.GetSkillRuntime` | Returns requirements, environment keys, and content trust info |
| `GET /api/v1/skills/{id}/usage` | `s.handleSkillUsage` | `s.app.GetSkillUsage` | Returns activation, recommendation, and doctor telemetry metrics |
| `POST /api/v1/skills/add/preview` | `s.handleSkillAddPreview` | `s.app.PreviewAddSkill` | Previews adding a draft skill from a remote GitHub repository |
| `POST /api/v1/skills/add/confirm` | `s.handleSkillAddConfirm` | `s.app.ConfirmAddSkill` | Confirms draft skill addition with pins |
| `POST /api/v1/skills/create/preview` | `s.handleSkillCreatePreview` | `s.app.CreateSkillPreview` | Previews creating a new draft skill |
| `POST /api/v1/skills/{id}/update/preview` | `s.handleSkillUpdatePreview` | `s.app.PreviewSkillUpdate` | Previews metadata or instruction edits |
| `POST /api/v1/skills/{id}/transitions/preview` | `s.handleSkillTransitionPreview` | `s.app.PreviewSkillTransition` | Previews lifecycle transition (`active`, `deprecated`, `archived`) |
| `POST /api/v1/skills/proposals/{proposal_id}/confirm` | `s.handleSkillConfirm` | `s.app.ConfirmSkillProposal` | Confirms skill mutation proposal with pins |
| `GET /api/v1/skills/{id}/sources` | `s.handleSkillSources` | `s.app.GetSkillSources` | Returns upstream source details, drift, and learning links |
| `POST /api/v1/skills/{id}/upstream/check` | `s.handleSkillUpstreamCheck` | `s.app.CheckSkillUpstream` | Probes remote upstream repository for skill drift |
| `POST /api/v1/skills/{id}/upstream/review` | `s.handleSkillUpstreamReview` | `s.app.ReviewSkillUpstream` | Previews 3-way merge update from upstream |
| `POST /api/v1/upstream/proposals/{proposal_id}/confirm` | `s.handleUpstreamConfirm` | `s.app.ConfirmUpstreamUpdate` | Confirms 3-way merge upstream update |
| `POST /api/v1/skills/{id}/sources/attach/preview` | `s.handleSkillSourceAttachPreview` | `s.app.PreviewAttachSource` | Previews attaching a learning reference to a skill |
| `POST /api/v1/skills/{id}/sources/{source_id}/detach/preview` | `s.handleSkillSourceDetachPreview` | `s.app.PreviewDetachSource` | Previews detaching a learning reference from a skill |
| `GET /api/v1/sources` | `s.handleSourcesList` | `s.app.ListSources` | Lists watched sources grouped by repository and ref |
| `POST /api/v1/sources/check` | `s.handleSourcesCheck` | `s.app.CheckSources` | Probes watched sources for revision updates |
| `POST /api/v1/sources/{id}/unwatch/preview` | `s.handleSourceUnwatchPreview` | `s.app.PreviewUnwatchSource` | Previews unwatching and deleting an unreferenced source |
| `POST /api/v1/sources/proposals/{proposal_id}/confirm` | `s.handleSourceConfirm` | `s.app.ConfirmSourceProposal` | Confirms source watch or unwatch proposal |
| `POST /api/v1/sources/{id}/import/preview` | `s.handleSourceImportPreview` | `s.app.PreviewSourceImport` | Discovers skills in source and previews draft import |
| `POST /api/v1/sources/import/confirm` | `s.handleSourceImportConfirm` | `s.app.ConfirmSourceImport` | Confirms skill import proposal |
| `GET /api/v1/runs/{id}` | `s.handleRunGet` | `s.app.GetCurationRun` | Retrieves status and package digest of a curation run |
| `POST /api/v1/runs/{id}/cancel` | `s.handleRunCancel` | `s.app.CancelCurationRun` | Cancels an in-progress or awaiting-decision curation run |
| `GET /api/v1/inbox` | `s.handleInboxList` | `s.app.ListInbox` | Returns paged list of pending and planned insight groups |
| `GET /api/v1/insights/{id}` | `s.handleInsightGet` | `s.app.GetInsightDetail` | Returns insight details, findings, comparisons, and history |
| `POST /api/v1/insights/{id}/decision` | `s.handleInsightDecide` | `s.app.DecideInsight` | Records decision (`plan`, `reject`, `reopen`, `obsolete`) |
| `POST /api/v1/insights/{id}/apply/preview` | `s.handleInsightApplyPreview` | `s.app.PreviewInsightApply` | Previews patch composition against `SKILL.md` |
| `POST /api/v1/insights/apply/confirm` | `s.handleInsightApplyConfirm` | `s.app.ConfirmInsightApply` | Confirms insight patch application with pins |

---

## 4. HTTP Status Mapping

Error responses follow the standard Skill Hub Result Envelope (`schema_version`, `status: "error"`, `error: { code, render: { ERROR, WHY, FIX } }`). The HTTP status code is derived from `app.ErrorCode` in [internal/delivery/web/errors.go](../../internal/delivery/web/errors.go):

| HTTP Status | Application Error Code | Typical Causes |
|---|---|---|
| `400 Bad Request` | `invalid_request`, `validation_failed`, `ambiguous_locator`, `ambiguous_ref`, `local_watch_unsupported`, `unsupported_schema`, `resource_limits_exceeded` | Malformed JSON, validation failure, local path given to remote tool |
| `401 Unauthorized` | Handled at transport layer | Missing or invalid Bearer token |
| `403 Forbidden` | `permission_denied` | Mutating operation blocked by security policy |
| `404 Not Found` | Handled at route/handler layer | Unknown skill ID, insight ID, run ID, or API route |
| `409 Conflict` | `skill_conflict`, `source_conflict`, `edit_conflict`, `stale_proposal`, `stale_context`, `source_changed`, `resource_content_unavailable` | Concurrent modification, duplicate ID, hash mismatch |
| `410 Gone` | `snapshot_expired` | Paged cursor expired due to underlying catalog changes |
| `421 Misdirected Request` | Handled at transport layer | Unallowed Host header (DNS rebinding protection) |
| `422 Unprocessable Entity` | `skill_selection_required`, `unknown_resolution`, `clarification_budget_exhausted` | Multi-skill repository requires explicit selection |
| `499 Client Closed Request` | `operation_cancelled` | Run or operation cancelled |
| `500 Internal Server Error` | `internal`, `resource_digest_mismatch`, `run_interrupted`, `partial_distill_failure` | Internal unrecoverable failure |
| `502 Bad Gateway` | `source_unavailable`, `resource_read_failed` | Remote Git repository unreachable or network failure |
| `503 Service Unavailable` | `workspace_invalid`, `recovery_required`, `index_stale` | Workspace requires recovery or canonical validation failed |

---

## 5. Schema and Fixture References

- Canonical JSON Schemas: [`schemas/`](../../schemas/)
- Normalized API Golden Responses: [`internal/delivery/web/testdata/golden/`](../../internal/delivery/web/testdata/golden/)
