# Result envelope

## Contract boundary

This is the stable result boundary shared by command and tool adapters. It
keeps a compact human result alongside structured automation data; adapters do
not define separate domain semantics. The ownership boundary is described in
[Curation UX and Lifecycle](../design/05-curation-lifecycle.md), while resolver
responses and their protocol-level outcomes are defined in
[Agent–Hub Protocol](../design/02-agent-hub-protocol.md).

The envelope is a V1 contract, not evidence that a schema, command, CLI, or MCP
tool has been implemented. It does not introduce a web UI, cloud dependency,
automatic insight application, or canonical event log.

## Stable public envelope

Every result has these public members:

```json
{
  "status": "<machine outcome>",
  "summary": "<concise user-facing outcome>",
  "items": [],
  "suggested_actions": [],
  "warnings": [],
  "error": null
}
```

`status`, `summary`, `items`, `suggested_actions`, `warnings`, and `error` are
stable public fields. They are always present, with an empty array or `null`
when no value applies. `status` is a command-specific, machine-readable
outcome; it is not a substitute for the resolver statuses defined by the
protocol. `summary` is concise user-facing text. `items` provides compact
structured item summaries, and `suggested_actions` provides actionable next
steps. `warnings` describes non-fatal exceptions. `error` is `null` for a
non-error result and otherwise carries a stable error code and user-safe
rendering from [Error codes](error-codes.md).

The default result is L0: counts and one recommended next action. Evidence,
comparisons, diffs, raw artifacts, and diagnostics are requested through the
progressive-disclosure boundary, not embedded by default. See
[UX fixtures](ux-fixtures.md).

Public identifiers that a caller must resubmit (such as an insight, proposal,
or operation identifier) are opaque strings. In the interactive CLI, users can
confirm proposals by short ID (`skillhub skill confirm <proposal-id>`), while MCP
and automation endpoints require all three pins: `proposal_id`, `proposal_digest`,
and `base_version` as governed by [action-confirmation-policy.schema.json](../../schemas/action-confirmation-policy.schema.json).

CLI JSON includes `suggested_actions[].cli`, the runnable shell counterpart of
the unchanged `command`. Semantic previews that require confirmation also
include top-level `cli`: the full confirm command with proposal ID, digest,
base version, resolved workspace, and JSON output. Run that command only after
the user approves the reviewed preview. A stale proposal fails rather than
regenerating a different change. MCP responses omit these CLI-only fields.
Commands requiring additional intent (for example, an unavailable curation-run
tool) point to the existing CLI inspection route, not an invented mutation.

`skillhub eval routing --json` uses this envelope with its complete evaluation
report under `metrics`, not as bare root-level metric fields. A failed quality
threshold retains the metrics, returns `action_required` with
`routing_threshold_failed` warnings and a null operational `error`, and exits 1.
Successful evaluation exits 0; invalid requests still use an error envelope.

Every applied change to the hub becomes one Git commit that holds only that
change's files and its operation receipt. The result carries it as `commit`
(see the schema): `status` is `committed`, `skipped`, `failed` or `deferred`,
`hash` and `message` name the commit, `reach` says who gets the new version
(`this_hub_now`, and `push_needed` with the remote's `push_command`), and a
`failed` or `skipped` commit gives its `reason` and the exact `manual_command`
that finishes it. The files stay written when a commit fails: signing, a Git
hook or a missing identity can stop it, and nothing is rolled back. `commit` is
absent when the caller did not ask for a commit (`--no-commit` on the command
line). The CLI, the MCP curation tools and the web all commit; only the CLI has
`--no-commit`. A change is refused before anything is written when its files
already differ from Git; see `edit_conflict` in [Error codes](error-codes.md).

In a terminal, a hub-changing CLI command asks `Apply this change? [y/N]` under
its preview and applies exactly the previewed change on `y`. Without a terminal,
with `--json`, or for an agent, the preview ends with `Show the diff: <command>
--diff` and `Apply: <command> --yes`, and nothing is written until `--yes`.
`--diff` fills `full_diff` in the preview, as the former `--full-diff` did.

`skillhub skill checkout <id>` and `skillhub skill commit <id|folder>` use this
envelope too. A checkout result adds `skill_id`, `state`, `path` (the project
folder, relative to the project root), `file_count`, `base_version` and, when
the skill had no uncommitted edits, `base_commit`; it changes nothing on the
hub. A commit preview adds `case` (`update`, `merge`, `new` or `replace`),
`files[]` (`path`, `change` as `A`, `M` or `D`, `added`, `removed`, `size`,
and for a merge `status` in the words of the person committing),
`excluded[]` (`path`, `reason`), `merge[]` (`path`, `status`, `action`),
`conflicts[]` (the files that still need a decision), `routing_suggestions`,
`routing_lint[]`, `trust` (`third_party`, `origin`, `content_digest`), `checkout`
(`path`, `base_version`) and `nothing_to_commit`; applying it returns the
result with `approved`, `activated`, `changed_paths` and the usual `commit`.
A third-party skill's new content is applied only together with its approval.
Without a terminal that needs `--approve-content <content_digest>`, otherwise the
command is refused with `content_review_required`. These fields are additive.
`skillhub status` run inside a project adds `open_checkout` actions to `actions`
(see [the curation home schema](../../schemas/curation-home-v1.schema.json)).

State-reporting results explicitly disclose state basis: canonical facts
derived from the local Git working tree versus served facts from the active SQLite
catalog generation (`servable`, `catalog_snapshot`, `generation`). Unconfirmed editor
sessions store bounded 24-hour recovery artifacts in `runtime/edits/` to prevent data loss.
## Field classifications

| Classification | Fields | Compatibility rule |
|---|---|---|
| **Public** | `status`, `summary`, `items`, `suggested_actions`, `warnings`, `error`, and caller-resubmitted opaque identifiers | Present and backward-compatible within V1. Consumers may rely only on documented semantics, not ID formats or presentation wording. |
| **Experimental** | Optional additive fields explicitly marked `experimental`; optional detail handles or pagination/capability metadata when introduced | May change or disappear within V1. Consumers must tolerate absence and must not make safety decisions from them. |
| **Internal** | Correlation/tracing data, raw diagnostics, storage/index state, implementation-specific snapshots, policy internals, and transport-only metadata not classified public | Not a consumer contract. It is omitted from normal user output and may change without compatibility notice. |

An unclassified field is internal. Experimental fields cannot alter confirmation,
authorization, integrity, or mutation semantics; those decisions remain with
the application and Agent Host. Protocol-specific public fields such as
`schema_version`, request/resolution correlation, and resolution validity are
owned by [Agent–Hub Protocol](../design/02-agent-hub-protocol.md), rather than
silently added to this common curation envelope.

## Result rules

- Do not expose raw request content, secrets, personal data, chain-of-thought,
  or unrequested diagnostics.
- Preserve warnings when a batch isolates an individual failure; do not turn a
  partial failure into a false all-or-nothing result.
- A valid abstention such as protocol `no_skill` is an outcome, not an error.
- Output intended for automation is structured; automation must not parse
  decorative terminal output.
- A result never implies that active skill content changed unless an approved
  semantic mutation completed.
