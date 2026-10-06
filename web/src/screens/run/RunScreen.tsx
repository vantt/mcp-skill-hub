import { useEffect, useRef, useState } from 'react';
import { Link, useParams } from 'react-router';
import { useQueryClient } from '@tanstack/react-query';
import { cancelDistillRun, useDistillRun, useSession } from '../../api/queries';
import { ConfirmDialog } from '../../components/ConfirmDialog';
import { Skeleton } from '../../components/Skeleton';
import { StatusBadge } from '../../components/StatusBadge';
import { buildResumeBrief } from '../../domain/handoff-brief';
import { addRecentRun, listRecentRuns, updateRecentRun } from '../../state/recent-runs';

const TITLE_RUN = 'Run';
const BTN_REFRESH_STATUS = 'Refresh status';
const BTN_COPY_RESUME = 'Copy resume handoff';
const BTN_CANCEL_RUN = 'Cancel run';
const BTN_CANCELLING_RUN = 'Cancelling…';
const TITLE_CANCEL_DIALOG = 'Cancel run';
const BODY_CANCEL_DIALOG =
  'Cancelling this run will stop processing. The source cursor does not advance, and an agent working on this run will fail to submit.';
const CAVEAT_NO_KEY =
  "This browser does not have the run's handoff key, so resume is unavailable. Cancel the run, then create a new handoff.";

const LABEL_RUN_NOT_FOUND = 'Run not found';
const LABEL_RUN_NOT_FOUND_PREFIX = 'The requested run (';
const LABEL_RUN_NOT_FOUND_SUFFIX = ') could not be found.';
const BTN_BACK_SOURCES = 'Back to Sources';
const BTN_RETRY = 'Retry';
const LABEL_ATTEMPT = ' · Attempt ';
const LABEL_ARROW = ' → ';
const LABEL_REVISION = 'Revision';
const LABEL_PACKAGE_DIGEST = 'Package digest';
const LABEL_CHANGED_RESOURCES = 'Changed resources';
const LABEL_CHANGED_RESOURCES_LIST = 'Changed resources list (';
const LABEL_RPAREN = ')';
const LABEL_PREPARED = 'Prepared';
const LABEL_PREPARED_DESC = 'Waiting for Curator Agent to start the run.';
const LABEL_IN_PROGRESS = 'In progress';
const LABEL_IN_PROGRESS_DESC = 'The Curator Agent is analyzing changes and producing findings.';
const LABEL_OUTSTANDING_DECISION = 'Outstanding decision';
const LABEL_DECISION_GUIDANCE = 'The agent needs guidance before proceeding.';
const LABEL_DECISION = 'Decision';
const LABEL_ASTERISK = ' *';
const LABEL_REQUIRED_HINT = 'Required before copying the resume handoff.';
const LABEL_FINALIZED = 'Finalized';
const LABEL_FINDINGS = 'Findings';
const LABEL_COMPARISONS = 'Comparisons';
const LABEL_INSIGHTS = 'Insights';
const BTN_OPEN_INBOX = 'Open Inbox →';
const LABEL_RUN_FAILED = 'Run failed';
const LABEL_CANCELLED = 'Cancelled';
const LABEL_CANCELLED_DESC = 'This run was cancelled. The source cursor was not advanced.';
const LABEL_CANCELLED_AT_PREFIX = ' (Cancelled at: ';
const LABEL_COPIED = 'Copied';
const CHAR_COPY = ' ⧉';
const CHAR_CHECK = ' ✓';
const TITLE_CLICK_COPY = 'Click to copy run ID';
const PLACEHOLDER_DECISION = 'e.g. Intentional removal. Drop nitpicks guidance.';
const FALLBACK_FAILED_EXEC = 'The curation run failed during execution.';
const FALLBACK_LOAD_FAILED = 'Failed to load run';
const FALLBACK_CANCEL_FAILED = 'Failed to cancel run';

export function RunScreen() {
  const queryClient = useQueryClient();
  const { id: rawRunId } = useParams<{ id: string }>();
  const runId = rawRunId || '';

  const { data: session } = useSession();
  const workspaceId = session?.workspace_id || 'default';

  const { data, isLoading, error, refetch } = useDistillRun(runId, (query) => {
    const state = query.state.data?.run?.state;
    return state === 'prepared' || state === 'in_progress' ? 5000 : false;
  });

  const [copiedId, setCopiedId] = useState(false);
  const [copiedResume, setCopiedResume] = useState(false);
  const [decision, setDecision] = useState('');
  const [cancelOpen, setCancelOpen] = useState(false);
  const [cancelling, setCancelling] = useState(false);
  const [actionError, setActionError] = useState<string | null>(null);

  // Polite live region tracking
  const [liveAnnouncement, setLiveAnnouncement] = useState('');
  const prevLiveStateRef = useRef<string | null>(null);

  const run = data?.run;


  // Manage live announcement on state changes
  useEffect(() => {
    if (run?.state) {
      if (prevLiveStateRef.current !== null && prevLiveStateRef.current !== run.state) {
        setLiveAnnouncement(`Run state changed to ${run.state}`);
      }
      prevLiveStateRef.current = run.state;
    }
  }, [run?.state]);

  // Update recent runs on load
  useEffect(() => {
    if (run) {
      const list = listRecentRuns(workspaceId);
      const existing = list.find((r) => r.runId === run.id);
      if (existing) {
        if (existing.state !== run.state) {
          updateRecentRun(workspaceId, run.id, run.state);
        }
      } else {
        addRecentRun(workspaceId, {
          runId: run.id,
          sourceId: run.source_id,
          state: run.state,
          openedAt: Date.now(),
        });
      }
    }
  }, [run, workspaceId]);

  if (isLoading) {
    return (
      <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
        <Skeleton width="40%" height="32px" />
        <Skeleton height="140px" />
        <Skeleton height="180px" />
      </div>
    );
  }

  const isNotFound =
    error instanceof Error &&
    (error.message.includes('404') ||
      error.message.includes('not found') ||
      (error as { status?: number }).status === 404);

  if (isNotFound) {
    return (
      <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
        <h1 style={{ margin: 0, fontSize: '20px', fontWeight: 600 }}>
          <span>{TITLE_RUN}</span>
        </h1>
        <div
          className="fg-card"
          style={{
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
            gap: 'var(--space-3)',
            padding: 'var(--space-6) var(--space-4)',
          }}
        >
          <span style={{ fontSize: '16px', fontWeight: 600 }}>{LABEL_RUN_NOT_FOUND}</span>
          <span style={{ color: 'var(--color-text-muted)', fontSize: '13px' }}>
            {LABEL_RUN_NOT_FOUND_PREFIX}{runId}{LABEL_RUN_NOT_FOUND_SUFFIX}
          </span>
          <Link to="/sources" className="fg-btn fg-btn--primary" style={{ textDecoration: 'none' }}>
            <span>{BTN_BACK_SOURCES}</span>
          </Link>
        </div>
      </div>
    );
  }

  if (error || !run) {
    return (
      <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
        <div className="fg-banner fg-banner--danger">
          <span>{error instanceof Error ? error.message : FALLBACK_LOAD_FAILED}</span>
        </div>
        <div style={{ display: 'flex', gap: 'var(--space-2)' }}>
          <button
            type="button"
            className="fg-btn fg-btn--secondary"
            onClick={() => void refetch()}
          >
            <span>{BTN_RETRY}</span>
          </button>
          <Link to="/sources" className="fg-btn fg-btn--ghost" style={{ textDecoration: 'none' }}>
            <span>{BTN_BACK_SOURCES}</span>
          </Link>
        </div>
      </div>
    );
  }

  // Retrieve stored idempotency key
  const recentList = listRecentRuns(workspaceId);
  const currentRecent = recentList.find((r) => r.runId === run.id);
  const storedKey = currentRecent?.idempotencyKey;
  const hasKey = Boolean(storedKey);

  const canResume =
    hasKey &&
    (run.state === 'failed' || run.state === 'awaiting_decision');
  const resumeDisabled = run.state === 'awaiting_decision' && !decision.trim();

  const canCancel = run.state !== 'finalized' && run.state !== 'cancelled';

  const handleCopyId = async () => {
    try {
      await navigator.clipboard.writeText(run.id);
      setCopiedId(true);
      setTimeout(() => setCopiedId(false), 2000);
    } catch {
      // Fallback
    }
  };

  const handleCopyResume = async () => {
    if (!canResume) return;
    const brief = buildResumeBrief({
      runId: run.id,
      sourceId: run.source_id,
      state: run.state,
      idempotencyKey: storedKey,
      decision: run.state === 'awaiting_decision' ? decision.trim() : undefined,
    });
    try {
      await navigator.clipboard.writeText(brief);
      setCopiedResume(true);
      setTimeout(() => setCopiedResume(false), 2000);
    } catch {
      // Fallback
    }
  };

  const handleConfirmCancel = async () => {
    setCancelling(true);
    setActionError(null);
    try {
      await cancelDistillRun(run.id);
      updateRecentRun(workspaceId, run.id, 'cancelled');
      queryClient.invalidateQueries({ queryKey: ['distill-run', run.id] });
      setCancelOpen(false);
    } catch (err: unknown) {
      setActionError(err instanceof Error ? err.message : FALLBACK_CANCEL_FAILED);
    } finally {
      setCancelling(false);
    }
  };

  const fromRev = run.from_revision?.value || 'none';
  const toRev = run.to_revision.value;
  const changedResources = run.changed_resources || [];

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
      {/* Polite live region for screen readers */}
      <div
        aria-live="polite"
        style={{
          position: 'absolute',
          width: '1px',
          height: '1px',
          padding: 0,
          margin: '-1px',
          overflow: 'hidden',
          clip: 'rect(0, 0, 0, 0)',
          whiteSpace: 'nowrap',
          border: 0,
        }}
      >
        <span>{liveAnnouncement}</span>
      </div>

      {actionError && (
        <div className="fg-banner fg-banner--danger">
          <span>{actionError}</span>
        </div>
      )}

      {/* Header */}
      <section style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 'var(--space-2) var(--space-3)' }}>
        <button
          type="button"
          onClick={() => void handleCopyId()}
          title={TITLE_CLICK_COPY}
          style={{ all: 'unset', cursor: 'pointer', fontFamily: 'var(--font-mono)', fontSize: '15px', fontWeight: 600 }}
        >
          <span>{run.id}</span>
          <span>{copiedId ? CHAR_CHECK : CHAR_COPY}</span>
        </button>
        <StatusBadge
          variant="chip"
          tone={
            run.state === 'finalized'
              ? 'success'
              : run.state === 'failed'
                ? 'danger'
                : run.state === 'in_progress'
                  ? 'info'
                  : 'neutral'
          }
          label={run.state}
        />
        <span style={{ fontSize: '13px', color: 'var(--color-text-muted)' }}>
          <span>{run.source_id}</span>
          <span>{LABEL_ATTEMPT}</span>
          <span>{run.attempt}</span>
        </span>
      </section>

      {/* Facts Card */}
      <section className="fg-card" style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' }}>
        <div
          style={{
            display: 'grid',
            gridTemplateColumns: 'repeat(auto-fit, minmax(180px, 1fr))',
            gap: 'var(--space-3)',
          }}
        >
          <div>
            <div style={{ fontSize: '12px', color: 'var(--color-text-muted)', marginBottom: '4px' }}>
              <span>{LABEL_REVISION}</span>
            </div>
            <div style={{ fontFamily: 'var(--font-mono)', fontSize: '13px' }}>
              <span>{fromRev}</span>
              <span>{LABEL_ARROW}</span>
              <span>{toRev}</span>
            </div>
          </div>
          <div>
            <div style={{ fontSize: '12px', color: 'var(--color-text-muted)', marginBottom: '4px' }}>
              <span>{LABEL_PACKAGE_DIGEST}</span>
            </div>
            <div style={{ fontFamily: 'var(--font-mono)', fontSize: '13px', wordBreak: 'break-all' }}>
              <span>{run.package_digest}</span>
            </div>
          </div>
          <div>
            <div style={{ fontSize: '12px', color: 'var(--color-text-muted)', marginBottom: '4px' }}>
              <span>{LABEL_CHANGED_RESOURCES}</span>
            </div>
            <div style={{ fontSize: '13px', fontWeight: 600 }}>
              <span>{changedResources.length}</span>
            </div>
          </div>
        </div>

        {changedResources.length > 0 && (
          <details style={{ marginTop: 'var(--space-2)' }}>
            <summary style={{ cursor: 'pointer', fontSize: '13px', color: 'var(--color-primary)' }}>
              <span>{LABEL_CHANGED_RESOURCES_LIST}</span>
              <span>{changedResources.length}</span>
              <span>{LABEL_RPAREN}</span>
            </summary>
            <div
              style={{
                display: 'grid',
                gap: '6px',
                fontFamily: 'var(--font-mono)',
                fontSize: '12.5px',
                marginTop: 'var(--space-2)',
                padding: 'var(--space-2)',
                backgroundColor: 'var(--color-surface-sunken)',
                borderRadius: 'var(--radius-sm)',
              }}
            >
              {changedResources.map((res) => (
                <div key={res.path} style={{ display: 'flex', justifyContent: 'space-between' }}>
                  <span>{res.path}</span>
                  <span style={{ color: 'var(--color-text-muted)' }}>{res.status}</span>
                </div>
              ))}
            </div>
          </details>
        )}
      </section>

      {/* State Panels */}
      {run.state === 'prepared' && (
        <section className="fg-card" style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-2)' }}>
          <div className="fg-card__title">
            <span style={{ fontWeight: 600 }}>{LABEL_PREPARED}</span>
          </div>
          <span style={{ fontSize: '13px', color: 'var(--color-text-muted)' }}>
            {LABEL_PREPARED_DESC}
          </span>
        </section>
      )}

      {run.state === 'in_progress' && (
        <section className="fg-card" style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-2)' }}>
          <div className="fg-card__title">
            <span style={{ fontWeight: 600 }}>{LABEL_IN_PROGRESS}</span>
          </div>
          <span style={{ fontSize: '13px', color: 'var(--color-text-muted)' }}>
            {LABEL_IN_PROGRESS_DESC}
          </span>
        </section>
      )}

      {run.state === 'awaiting_decision' && (
        <section
          className="fg-card"
          style={{
            display: 'flex',
            flexDirection: 'column',
            gap: 'var(--space-3)',
            borderLeft: '4px solid var(--color-warning, #f59e0b)',
          }}
        >
          <div className="fg-card__title">
            <span style={{ fontWeight: 600 }}>{LABEL_OUTSTANDING_DECISION}</span>
          </div>

          {run.outstanding_decisions && run.outstanding_decisions.length > 0 ? (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-2)' }}>
              {run.outstanding_decisions.map((od, i) => (
                <div key={i} style={{ display: 'flex', flexDirection: 'column', gap: '4px', fontSize: '13px' }}>
                  <div style={{ display: 'flex', gap: 'var(--space-2)', alignItems: 'center' }}>
                    <StatusBadge variant="chip" tone="warning" label={od.category} />
                  </div>
                  <p style={{ margin: 0 }}>
                    <span>{od.detail}</span>
                  </p>
                </div>
              ))}
            </div>
          ) : (
            <span style={{ fontSize: '13px', color: 'var(--color-text-muted)' }}>
              {LABEL_DECISION_GUIDANCE}
            </span>
          )}

          <div className="fg-field">
            <label className="fg-field__label t-label" htmlFor="run-decision" style={{ fontWeight: 600, fontSize: '13px' }}>
              <span>{LABEL_DECISION}</span>
              <span style={{ color: 'var(--color-danger, #dc2626)' }}>{LABEL_ASTERISK}</span>
            </label>
            <textarea
              id="run-decision"
              className="fg-input fg-input--area"
              style={{ minHeight: '80px', width: '100%', fontFamily: 'inherit', fontSize: '13px' }}
              value={decision}
              onChange={(e) => setDecision(e.target.value)}
              placeholder={PLACEHOLDER_DECISION}
            />
            <span className="fg-field__hint" style={{ fontSize: '12px', color: 'var(--color-text-muted)' }}>
              {LABEL_REQUIRED_HINT}
            </span>
          </div>
        </section>
      )}

      {run.state === 'finalized' && (
        <section className="fg-card" style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' }}>
          <div className="fg-card__title">
            <span style={{ fontWeight: 600 }}>{LABEL_FINALIZED}</span>
          </div>
          <div
            style={{
              display: 'grid',
              gridTemplateColumns: 'repeat(auto-fit, minmax(140px, 1fr))',
              gap: 'var(--space-3)',
            }}
          >
            <div>
              <div style={{ fontSize: '12px', color: 'var(--color-text-muted)' }}>
                <span>{LABEL_FINDINGS}</span>
              </div>
              <div style={{ fontSize: '14px', fontWeight: 600 }}>
                <span>{run.finding_ids?.length || 0}</span>
              </div>
            </div>
            <div>
              <div style={{ fontSize: '12px', color: 'var(--color-text-muted)' }}>
                <span>{LABEL_COMPARISONS}</span>
              </div>
              <div style={{ fontSize: '14px', fontWeight: 600 }}>
                <span>{run.comparison_ids?.length || 0}</span>
              </div>
            </div>
            <div>
              <div style={{ fontSize: '12px', color: 'var(--color-text-muted)' }}>
                <span>{LABEL_INSIGHTS}</span>
              </div>
              <div style={{ fontSize: '14px', fontWeight: 600 }}>
                <span>{run.insight_ids?.length || 0}</span>
              </div>
            </div>
          </div>
          <Link
            to="/inbox"
            className="fg-btn fg-btn--primary"
            style={{ alignSelf: 'flex-start', textDecoration: 'none', marginTop: 'var(--space-2)' }}
          >
            <span>{BTN_OPEN_INBOX}</span>
          </Link>
        </section>
      )}

      {run.state === 'failed' && (
        <div className="fg-banner fg-banner--danger" style={{ display: 'flex', flexDirection: 'column', gap: '4px' }}>
          <span style={{ fontWeight: 600 }}>{LABEL_RUN_FAILED}</span>
          <span>{run.failure || FALLBACK_FAILED_EXEC}</span>
        </div>
      )}

      {run.state === 'cancelled' && (
        <section className="fg-card" style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-2)' }}>
          <div className="fg-card__title">
            <span style={{ fontWeight: 600 }}>{LABEL_CANCELLED}</span>
          </div>
          <span style={{ fontSize: '13px', color: 'var(--color-text-muted)' }}>
            <span>{LABEL_CANCELLED_DESC}</span>
            {run.cancelled_at && (
              <>
                <span>{LABEL_CANCELLED_AT_PREFIX}</span>
                <span>{run.cancelled_at}</span>
                <span>{LABEL_RPAREN}</span>
              </>
            )}
          </span>
        </section>
      )}

      {/* Advisory if no key on browser */}
      {!hasKey && (
        <div
          className="fg-banner fg-banner--info"
          style={{
            padding: 'var(--space-3)',
            backgroundColor: 'var(--color-surface-sunken)',
            borderRadius: 'var(--radius-sm)',
            border: '1px solid var(--color-border)',
            fontSize: '13px',
          }}
        >
          <span>{CAVEAT_NO_KEY}</span>
        </div>
      )}

      {/* Actions toolbar */}
      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 'var(--space-2)', alignItems: 'center' }}>
        <button
          type="button"
          className="fg-btn fg-btn--secondary"
          onClick={() => void refetch()}
        >
          <span>{BTN_REFRESH_STATUS}</span>
        </button>
        {canResume && (
          <button
            type="button"
            className="fg-btn fg-btn--primary"
            onClick={() => void handleCopyResume()}
            disabled={resumeDisabled}
          >
            <span>{copiedResume ? LABEL_COPIED : BTN_COPY_RESUME}</span>
          </button>
        )}
        <span style={{ flex: 1 }} />
        {canCancel && (
          <button
            type="button"
            className="fg-btn fg-btn--danger"
            onClick={() => setCancelOpen(true)}
          >
            <span>{BTN_CANCEL_RUN}</span>
          </button>
        )}
      </div>

      {/* Cancel Confirm Dialog */}
      {cancelOpen && (
        <ConfirmDialog
          open={cancelOpen}
          title={TITLE_CANCEL_DIALOG}
          body={BODY_CANCEL_DIALOG}
          confirmLabel={cancelling ? BTN_CANCELLING_RUN : BTN_CANCEL_RUN}
          danger
          onConfirm={() => void handleConfirmCancel()}
          onCancel={() => setCancelOpen(false)}
        />
      )}
    </div>
  );
}
