export interface HandoffBriefInput {
  sourceIds: string[];
  idempotencyKey: string;
}

export function buildHandoffBrief(input: HandoffBriefInput): string {
  const sourcesJSON = JSON.stringify(input.sourceIds);
  return `You are the Curator Agent for Skill Hub. Perform distillation on the selected learning sources.

1. Call curation_run_start with source_ids: ${sourcesJSON} and idempotency_key: "${input.idempotencyKey}".
2. For each prepared run, read items[].prepared.revision_package and the target-revision resources.
3. Analyze the changes to produce coverage, findings, comparisons, insights, and outstanding_decisions.
4. Call curation_run_submit for each run with your structured analysis.
5. Report per-source errors instead of skipping failed sources.
6. Return every run ID with its final state to the user.`;
}

export interface ResumeBriefInput {
  runId: string;
  sourceId: string;
  state: string;
  idempotencyKey?: string;
  decision?: string;
}

export function buildResumeBrief(input: ResumeBriefInput): string {
  const isRetryable = input.state === 'failed' || input.state === 'awaiting_decision';
  const retryClause = isRetryable
    ? `\n- Call curation_run_retry with run_id: "${input.runId}"${input.decision ? `, decision: "${input.decision}"` : ''}${input.idempotencyKey ? `, idempotency_key: "${input.idempotencyKey}"` : ''}.`
    : `\n- Inspect the run state with curation_run_get using run_id: "${input.runId}".`;

  return `You are resuming a Skill Hub distillation run for source "${input.sourceId}".

- Run ID: ${input.runId}
- Current state: ${input.state}${input.decision ? `\n- Human decision: ${input.decision}` : ''}${retryClause}
- Once unblocked, proceed with distillation analysis and call curation_run_submit.
- Return the run ID and its updated state to the user.`;
}
