import { describe, expect, it } from 'vitest';
import { buildHandoffBrief, buildResumeBrief } from './handoff-brief';

describe('handoff-brief', () => {
  it('builds handoff brief with exact steps in order and matches inline snapshot', () => {
    const brief = buildHandoffBrief({
      sourceIds: ['source-a', 'source-b'],
      idempotencyKey: 'key-12345',
    });

    expect(brief).toContain('curation_run_start');
    expect(brief).toContain('["source-a","source-b"]');
    expect(brief).toContain('key-12345');
    expect(brief).toContain('items[].prepared.revision_package');
    expect(brief).toContain('coverage, findings, comparisons, insights, and outstanding_decisions');
    expect(brief).toContain('curation_run_submit');
    expect(brief).toContain('Report per-source errors instead of skipping failed sources.');
    expect(brief).toContain('Return every run ID with its final state to the user.');

    // Never contains token
    expect(brief).not.toContain('#token=');

    expect(brief).toMatchInlineSnapshot(`
      "You are the Curator Agent for Skill Hub. Perform distillation on the selected learning sources.

      1. Call curation_run_start with source_ids: ["source-a","source-b"] and idempotency_key: "key-12345".
      2. For each prepared run, read items[].prepared.revision_package and the target-revision resources.
      3. Analyze the changes to produce coverage, findings, comparisons, insights, and outstanding_decisions.
      4. Call curation_run_submit for each run with your structured analysis.
      5. Report per-source errors instead of skipping failed sources.
      6. Return every run ID with its final state to the user."
    `);
  });

  it('builds resume brief for failed or awaiting_decision with curation_run_retry and matches inline snapshot', () => {
    const retryBrief = buildResumeBrief({
      runId: 'RUN-FAIL1',
      sourceId: 'src-1',
      state: 'awaiting_decision',
      idempotencyKey: 'retry-key-1',
      decision: 'accept-finding-1',
    });

    expect(retryBrief).toContain('curation_run_retry');
    expect(retryBrief).toContain('RUN-FAIL1');
    expect(retryBrief).toContain('accept-finding-1');
    expect(retryBrief).not.toContain('#token=');

    expect(retryBrief).toMatchInlineSnapshot(`
      "You are resuming a Skill Hub distillation run for source "src-1".

      - Run ID: RUN-FAIL1
      - Current state: awaiting_decision
      - Human decision: accept-finding-1
      - Call curation_run_retry with run_id: "RUN-FAIL1", decision: "accept-finding-1", idempotency_key: "retry-key-1".
      - Once unblocked, proceed with distillation analysis and call curation_run_submit.
      - Return the run ID and its updated state to the user."
    `);

    // Non-retryable state (e.g. in_progress) does NOT mention curation_run_retry
    const inProgressBrief = buildResumeBrief({
      runId: 'RUN-INPROG',
      sourceId: 'src-2',
      state: 'in_progress',
    });

    expect(inProgressBrief).not.toContain('curation_run_retry');
    expect(inProgressBrief).toContain('curation_run_get');
    expect(inProgressBrief).not.toContain('#token=');
  });
});
