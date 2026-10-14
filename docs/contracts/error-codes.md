# Error codes and rendering

## Contract boundary

Errors are structured machine outcomes with a stable code and a concise,
user-facing recovery path. They are not substitutes for valid outcomes such as
protocol `no_skill`, nor do they authorize a repair or mutation. Protocol error
semantics are owned by [Agent–Hub Protocol](../design/02-agent-hub-protocol.md);
curation recovery and approval boundaries are owned by
[Curation UX and Lifecycle](../design/05-curation-lifecycle.md) and
[Source Learning and Distillation](../design/06-source-learning-and-distillation.md).

This registry records the initial V1 contract only. It makes no claim that a
command, schema, recovery service, or event log exists, and it adds no
cloud dependency or automatic application behavior; the web adapter ([Web API](web-api.md))
uses the same result envelope mapped to appropriate HTTP status codes.

## User-facing rendering

Every user-facing error renders exactly these labeled lines, in this order:

```text
ERROR: <what failed, in user language>
WHY: <known cause or evidence, without internal leakage>
FIX: <one concrete safe next action>
```

`ERROR` names the failed user goal, not an internal exception. `WHY` states only
known evidence and does not guess; it omits secrets, raw request content, paths
outside the authorized workspace, stack traces, and implementation details.
`FIX` supplies one actionable next step and preserves confirmation requirements.
When details are needed, they are available only through the progressive
disclosure levels in [UX fixtures](ux-fixtures.md). A machine result carries the
stable code in its `error` member as specified by the
[Result envelope](result-envelope.md).

## Error code registry

The machine-owned schema authority for error envelopes is [error-envelope.schema.json](../../schemas/error-envelope.schema.json).

| Code | Retry posture | Required ERROR / WHY / FIX meaning |
|---|---|---|
| `invalid_request` | Do not retry unchanged input | The request cannot be accepted; identify the invalid field or limit; correct the request. |
| `unsupported_schema` | Do not retry unchanged | The requested schema major is unsupported; state the supported contract path; use a compatible schema. |
| `stale_context` | Retry with current context | The request no longer matches current context; state that it changed; resubmit a current compact snapshot. |
| `unknown_resolution` | Resolve again when appropriate | The prior resolution cannot be used; state that it is unavailable; start a new resolution. |
| `clarification_budget_exhausted` | Do not loop | Resolution cannot safely ask again; state the unresolved discriminator; narrow the task or continue without a skill. |
| `resolution_retry_exhausted` | Do not loop for the same scope | Resolution retries are exhausted; state that the scope remained unresolved; change the task scope before retrying. |
| `index_stale` | Retry after repair | Derived search state does not match canonical files; state that canonical data remains intact; run `skillhub doctor --fix` or `skillhub maintain rebuild`. |
| `snapshot_expired` | Resolve again | The resolved resource version is unavailable; state that the selection expired; resolve again before loading content. |
| `resource_digest_mismatch` | Never load automatically | Integrity verification failed; state that content was not used; inspect or repair the resource before retrying. |
| `permission_denied` | Retry only after an authorized policy change | The requested action lacks permission; state the denied capability without exposing policy internals; obtain appropriate authorization. |
| `internal_error` | Limited retry | An unexpected failure occurred; state only a safe correlation reference when available; retry later or inspect diagnostics. |
| `workspace_invalid` | Retry after validation or repair | The workspace cannot safely serve the requested action; state the validation failure category; run `skillhub maintain validate` or supported recovery path. |
| `recovery_required` | Retry after the presented recovery choice | Earlier work needs recovery before optional work; state that active skills were not changed; choose the offered recovery action. |
| `run_interrupted` | Explicit retry, defer, or cancel | Analysis stopped before completion; state that the analyzed position was not advanced; retry, defer the affected work, or cancel. |
| `partial_distill_failure` | Retry failed scope only when appropriate | Part of a batch failed while other work was isolated; identify the exception summary; review the failed scope or retry it. |
| `source_unavailable` | Retry or change monitoring by policy | A source could not be reached; state that its last known revision remains valid and curated skills were not changed; retry, pause monitoring, or inspect the error. |
| `stale_proposal` | Regenerate and review | Nothing was applied because the reviewed proposal or its base changed; state that mismatch; regenerate and review a new preview. |
| `resource_read_failed` | Retry only when the source/resource can be read safely | A requested resource could not be read; state the safe failure category without exposing content; inspect the source or retry the authorized read. |
| `operation_cancelled` | Do not retry | The operation was cancelled before completion; state that no canonical changes were applied; initiate a new operation when needed. |
| `ambiguous_locator` | Correct locator or specify path/ref | The locator matched multiple potential repositories, refs, or paths; provide an explicit subpath or reference. |
| `ambiguous_ref` | Specify branch, tag, or commit | The remote reference could not be uniquely resolved; specify the intended branch or commit. |
| `skill_selection_required` | Select a skill or use `--all` | The source repository contains multiple discovered skills; select one with `--skill <name>` or import all with `--all`. |
| `skill_conflict` | Rename or use a different ID | A skill with the target ID already exists in the workspace; choose a distinct ID or edit the existing skill. `skillhub skill checkout` returns it when `.claude/skills/<id>/` already exists in the project; the FIX says to commit or edit that folder. |
| `source_conflict` | Use a different source ID | A monitored source with the specified identifier already exists; choose a distinct source ID. |
| `resource_limits_exceeded` | Reduce file count or size | The skill or companion assets exceed size limits; remove large files or split the skill. The network listener answers a submission over its limits with this code and HTTP 413 (see [Submissions on the network listener](#submissions-on-the-network-listener)). |
| `source_changed` | Re-run check or preview | The remote source revision changed since preview generation; generate a fresh preview against the new revision. |
| `edit_conflict` | Review recovery file and retry | The canonical skill was modified while the external editor was open; original edits were saved to a 24-hour recovery file in `runtime/edits/`. It also refuses a change whose files already differ from Git ("has edits that were not committed"), before anything is written: commit or discard those files, or re-run on the command line with `--no-commit`. |
| `validation_failed` | Correct canonical errors | Canonical workspace files failed schema or consistency validation; inspect the reported file and line errors. |
| `local_watch_unsupported` | Use a remote Git repository | Watching a local directory is not supported; watching applies only to remote Git repositories. |
| `resource_content_unavailable` | Re-add or re-import the skill | Required companion resource content is missing or corrupted and historical bytes are never guessed; re-add the skill or restore the resource. |
| `content_review_required` | Do not retry automatically | The requested skill is from a third-party source and its content has not been approved; the agent must not use the skill and must tell the user to run `skillhub skill review <id>` to review and approve it. `skillhub skill commit` returns it too, without a terminal and without `--approve-content`: new content of a third-party skill is never applied unapproved, because that would stop agents from getting the skill. A person runs the commit in a terminal and answers the approval question. |

## Submissions on the network listener

`POST /mcp/submissions` on the hub's network listener (port 7420) takes a skill a
machine sends for review. Like `GET /mcp/bundle`, it sits behind the Host check,
the bearer token and the rate limiter, and its refusals are a small JSON body
`{"code", "message"}` rather than the full result envelope, with these statuses:

| Status | `code` | Meaning |
|---|---|---|
| 202 | | Queued: `{"submission_id": "SUB-…", "status": "waiting"}`. Nothing on the hub changed. |
| 400 | `invalid_request` | The archive is malformed, a path leaves the skill folder or has a control or direction-changing character, two paths differ only in letter case, or it holds hub-managed or never-imported files (`.meta/`, the checkout marker, secrets). |
| 401 / 403 | | No or wrong token (401), or a Host or Origin the listener does not accept (403). |
| 404 | | The path is not served: a hub from before submissions. |
| 405 / 415 | `method_not_allowed`, `invalid_request` | Not a `POST`, or not `application/vnd.skillhub.bundle+gzip`. |
| 408 | `request_timeout` | The archive took longer than 2 minutes to arrive; nothing was queued. |
| 413 | `resource_limits_exceeded` | More than 512 files, more than 16 MiB of files, or a larger archive than a skill may be. |
| 429 | `submission_queue_full` | The queue already holds 50 waiting submissions or 256 MiB, counting the declared size of uploads still arriving; this is decided from the archive's metadata, before its files are written. The shared rate limiter also answers 429, with `Retry-After` and no body code. |
| 500 | `internal_error` | The submission could not be queued; the hub logs a correlation id. |
| 503 | `submission_busy` | Four submissions are already arriving; send again in a moment. |

`git_dirty_after_apply` is deliberately not an error code: after a successful
approved apply, uncommitted Git changes are a user-facing status or warning.
The result must say whether the change was committed (`commit`), and when it was
not, that it is active locally and uncommitted; it must not imply that a push
occurred.

## Classification and extension

Registry codes and the public `error.code` are public V1 fields defined in [error-envelope.schema.json](../../schemas/error-envelope.schema.json). Safe rendering
text is public behavior but may use equivalent wording so long as its meaning
and the ERROR/WHY/FIX order remain intact. Correlation, raw diagnostics, and
implementation-specific causes are internal. New codes are additive only when
they preserve the documented recovery boundary; unknown codes must be rendered
as a safe `internal_error`-style message rather than exposing internals.
