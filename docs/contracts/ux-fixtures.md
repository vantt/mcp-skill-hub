# UX fixtures

## Contract boundary

Fixtures are deterministic, machine-readable acceptance inputs for the local,
agent-facing Curation UX and its CLI fallback. They describe intended V1
outcomes; they neither establish an implementation nor add a web UI, cloud
service, automatic skill application, or canonical event log. The product UX
and command boundary are owned by [Curation UX and Lifecycle](../design/05-curation-lifecycle.md);
source-learning terms and safety invariants are owned by
[Source Learning and Distillation](../design/06-source-learning-and-distillation.md).

## Fixture shape

Each YAML fixture has this compact shape:

```yaml
scenario: <stable scenario name>
given: <deterministic workspace and request conditions>
when:
  application_command: <one application command name>
expect:
  status: <expected outcome>
  summary: <L0 user-facing summary>
  suggested_actions:
    - <the one recommended next action>
  confirmation:
    policy_revision: <opaque policy revision>
    action_class: <one policy-schema action class>
    application_command: <the governed application command>
    confirmation:
      required: <boolean>
      mode: <policy-schema interaction mode>
      pins: <semantic preview pins when action_class is semantic>
  progressive_disclosure:
    L0: <counts and one recommendation>
    L1: <item summary and impact>
    L2: <evidence, comparison, or diff>
    L3: <raw canonical artifacts and diagnostics>
```

`scenario`, `given`, `when`, and every listed `expect` member are required.
A fixture represents one user journey and one application command. A mutation
uses the command named by the lifecycle mapping, not an adapter or a guessed
CLI spelling. Read-only journeys may name their corresponding read command.
Opaque IDs, digests, revisions, snapshots, and policy revisions are synthetic
strings; fixtures must not encode their structure or use real credentials,
absolute paths, or secrets.

## Disclosure contract

Fixtures assert all four levels, even when an interaction defaults to L0:

- **L0** is normal output: counts plus exactly one recommended next action.
- **L1** identifies a relevant item and its impact.
- **L2** exposes supporting evidence, a cross-source comparison, or a proposed
  diff only on request.
- **L3** exposes raw canonical artifacts and technical diagnostics only on
  request.

Normal copy uses the user vocabulary in the lifecycle design (for example,
“finding” rather than “observation”). It must not require a user to understand
cursor, snapshot, revision, or other storage terminology. Technical details
remain available at L2 or L3 when needed for review or recovery.

## Mutations, confirmation, and errors

`expect.confirmation` uses the complete shape in
`action-confirmation-policy.schema.json`, including `policy_revision`,
`action_class`, `application_command`, and its nested `confirmation` object.
The scenario's `when.intent` supplies the explicit request for mechanical or
network operations; semantic actions instead require preview-and-approval
pins named `proposal_id`, `proposal_digest`, and `base_version`. The initial classes are:

| Class | Fixture expectation |
|---|---|
| `read-only` | Runs immediately. |
| `mechanical` | An explicit task or batch intent is sufficient; no per-item prompt. |
| `network` | An explicit check/batch intent is sufficient; otherwise ask once before network access. |
| `semantic` | Requires preview plus explicit approval pinned to proposal identity, digest, and base version. |
| `destructive` | Requires impact preview and confirmation. |
| `git-commit` | Requires an explicit user request. |
| `git-push` | Never automatic in V1. |

A mutation fixture records its confirmation expectation. Destructive actions
and routing changes follow the approval boundary in
[Curation UX and Lifecycle](../design/05-curation-lifecycle.md). A fixture must
never model automatic application of an insight or automatic Git push.

A fixture that expects an error includes an L0 rendering with these labeled
parts:

```text
ERROR: what failed
WHY: the known reason or evidence
FIX: one concrete next action
```

The code and rendering rules are owned by [Error codes](error-codes.md). A
partial batch failure remains an exception summary rather than evidence that
successful work was discarded.

## Required journey coverage

The fixture suite covers healthy and invalid/recovery-required Curation Home;
due, changed, and unavailable sources; high-value insight review; stale
proposal and dirty-Git-after-apply paths; and interrupted or partially failed
distillation. It preserves the status-first, offline nature of Curation Home:
network access is represented only by an explicit source-check action.
