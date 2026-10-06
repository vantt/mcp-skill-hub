import { useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router';
import { useQueryClient } from '@tanstack/react-query';
import { decideInsight, useInsightDetail } from '../../api/queries';
import type { Comparison, Observation } from '../../api/types';
import { ConfirmDialog } from '../../components/ConfirmDialog';
import { Skeleton } from '../../components/Skeleton';
import { StatusBadge } from '../../components/StatusBadge';
import { isInsightStale } from '../../domain/insight-staleness';

const TITLE_INSIGHT = 'Insight';
const LABEL_NOT_FOUND_TITLE = 'Insight not found';
const LABEL_NOT_FOUND_PREFIX = 'The requested insight (';
const LABEL_NOT_FOUND_SUFFIX = ') could not be found.';
const BTN_BACK_INBOX = 'Back to Inbox';
const BTN_RETRY = 'Retry';
const LABEL_FINDINGS_TITLE = 'Findings';
const LABEL_COMPARISONS_TITLE = 'Comparisons';
const LABEL_DECISION_HISTORY = 'Decision history';
const LABEL_STATUS = 'Status';
const BTN_COMPOSE_PATCH = 'Compose patch';
const BTN_PLAN = 'Plan';
const BTN_REJECT = 'Reject';
const BTN_REOPEN = 'Reopen';
const BTN_OBSOLETE = 'Obsolete';
const LABEL_READ_ONLY = 'Read-only.';
const LABEL_SIDEBAR_HINT =
  'Evidence is current. Decisions apply immediately; rationale is required.';
const LABEL_RATIONALE = 'Rationale';
const LABEL_ASTERISK = ' *';
const LABEL_CANCEL = 'Cancel';
const BTN_SUBMITTING = 'Submitting…';
const REASON_STALE_EVIDENCE =
  'Supporting evidence is stale. Re-run distillation or inspect findings before composing.';
const LABEL_STALE_COMPARISON = '⚠ Stale comparison';
const LABEL_SUBJECT = 'Subject';
const LABEL_VERDICT = 'Verdict';
const LABEL_TRADEOFFS = 'Trade-offs';
const TITLE_OBSOLETE_DIALOG = 'Obsolete insight';
const BODY_OBSOLETE_DIALOG =
  'Marking this insight obsolete indicates it is no longer valid or relevant. Rationale is required.';
const LABEL_STALE_BADGE = '⚠ Stale';
const LABEL_INSIGHT_SUFFIX = ' insight';

export function InsightDetailScreen() {
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const { id: rawId } = useParams<{ id: string }>();
  const insightId = rawId || '';

  const { data, isLoading, error, refetch } = useInsightDetail(insightId);

  // Decision modal state
  const [modalDecision, setModalDecision] = useState<'plan' | 'reject' | 'reopen' | null>(null);
  const [modalRationale, setModalRationale] = useState('');
  const [submittingDecision, setSubmittingDecision] = useState(false);
  const [decisionError, setDecisionError] = useState<string | null>(null);

  // Obsolete confirm dialog state
  const [obsoleteOpen, setObsoleteOpen] = useState(false);

  if (isLoading) {
    return (
      <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
        <h1 style={{ margin: 0, fontSize: '20px', fontWeight: 600 }}>
          <span>{TITLE_INSIGHT}</span>
        </h1>
        <div className="fg-card" style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' }}>
          <Skeleton width="40%" height="24px" />
          <Skeleton height="100px" />
          <Skeleton height="80px" />
        </div>
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
          <span>{TITLE_INSIGHT}</span>
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
          <span style={{ fontSize: '16px', fontWeight: 600 }}>{LABEL_NOT_FOUND_TITLE}</span>
          <span style={{ color: 'var(--color-text-muted)', fontSize: '13px' }}>
            <span>{LABEL_NOT_FOUND_PREFIX}</span>
            <span>{insightId}</span>
            <span>{LABEL_NOT_FOUND_SUFFIX}</span>
          </span>
          <Link to="/inbox" className="fg-btn fg-btn--primary" style={{ textDecoration: 'none' }}>
            <span>{BTN_BACK_INBOX}</span>
          </Link>
        </div>
      </div>
    );
  }

  if (error || !data) {
    return (
      <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
        <h1 style={{ margin: 0, fontSize: '20px', fontWeight: 600 }}>
          <span>{TITLE_INSIGHT}</span>
        </h1>
        <div className="fg-banner fg-banner--danger">
          <span>{error instanceof Error ? error.message : 'Failed to load insight'}</span>
        </div>
        <div style={{ display: 'flex', gap: 'var(--space-2)' }}>
          <button
            type="button"
            className="fg-btn fg-btn--secondary"
            onClick={() => void refetch()}
          >
            <span>{BTN_RETRY}</span>
          </button>
          <Link to="/inbox" className="fg-btn fg-btn--ghost" style={{ textDecoration: 'none' }}>
            <span>{BTN_BACK_INBOX}</span>
          </Link>
        </div>
      </div>
    );
  }

  const insight = data.insight;
  const findings = data.findings || [];
  const comparisons = data.comparisons || [];
  const stale = isInsightStale(data);
  const status = insight.status;

  const canCompose = status === 'pending' || status === 'planned';
  const canPlan = status === 'pending';
  const canReject = status === 'pending' || status === 'planned';
  const canReopen = status === 'rejected' || status === 'obsolete';
  const canObsolete = status === 'pending' || status === 'planned';
  const isReadOnly = status === 'withdrawn';

  const handleDecisionSubmit = async (decision: string, rationale: string) => {
    setSubmittingDecision(true);
    setDecisionError(null);
    try {
      await decideInsight(insight.id, decision, rationale);
      // Invalidate relevant queries per Requirement 5
      queryClient.invalidateQueries({ queryKey: ['inbox'] });
      queryClient.invalidateQueries({ queryKey: ['insight', insight.id] });
      queryClient.invalidateQueries({ queryKey: ['home'] });
      queryClient.invalidateQueries({ queryKey: ['sources'] });
      queryClient.invalidateQueries({ queryKey: ['skill-sources', insight.skill_id] });
      setModalDecision(null);
      setModalRationale('');
      setObsoleteOpen(false);
    } catch (err: unknown) {
      setDecisionError(err instanceof Error ? err.message : 'Failed to record decision');
    } finally {
      setSubmittingDecision(false);
    }
  };

  const handleOpenDecisionModal = (decision: 'plan' | 'reject' | 'reopen') => {
    setModalDecision(decision);
    setModalRationale('');
    setDecisionError(null);
  };
  const findingsTitle = `${LABEL_FINDINGS_TITLE} · ${findings.length}`;
  const comparisonsTitle = `${LABEL_COMPARISONS_TITLE} · ${comparisons.length}`;

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
      {decisionError && (
        <div className="fg-banner fg-banner--danger">
          <span>{decisionError}</span>
        </div>
      )}

      <div
        style={{
          display: 'grid',
          gridTemplateColumns: 'minmax(0, 1fr) 260px',
          gap: 'var(--space-4)',
          alignItems: 'start',
        }}
      >
        {/* Main Content Column */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)', minWidth: 0 }}>
          {/* Header Card */}
          <section className="fg-card" style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' }}>
            <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12.5px', color: 'var(--color-text-muted)' }}>
              <span>{insight.id}</span>
            </span>
            <span className="t-title" style={{ fontSize: '18px', fontWeight: 600, textWrap: 'pretty' }}>
              <span>{insight.recommendation}</span>
            </span>
            <p className="t-body" style={{ margin: 0, color: 'var(--color-text-muted)', textWrap: 'pretty', fontSize: '14px', lineHeight: 1.5 }}>
              <span>{insight.rationale}</span>
            </p>
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 'var(--space-2)' }}>
              <StatusBadge
                variant="chip"
                tone={
                  insight.priority === 'critical'
                    ? 'danger'
                    : insight.priority === 'high'
                      ? 'warning'
                        : 'neutral'
                }
                label={insight.priority}
              />
              <span className="fg-chip fg-chip--neutral">{insight.category}</span>
              <span className="fg-chip fg-chip--neutral" style={{ fontFamily: 'var(--font-mono)' }}>
                {insight.skill_id}
              </span>
              {stale && <span className="fg-chip fg-chip--warning">{LABEL_STALE_BADGE}</span>}
            </div>
          </section>

          {/* Findings Card */}
          <section className="fg-card" style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-2)' }}>
            <div className="fg-card__title">
              <span style={{ fontWeight: 600, fontSize: '14px' }}>
                <span>{findingsTitle}</span>
              </span>
            </div>
            <div style={{ display: 'flex', flexDirection: 'column' }}>
              {findings.map((o: Observation) => (
                <div
                  key={o.id}
                  style={{
                    display: 'flex',
                    flexDirection: 'column',
                    gap: '2px',
                    padding: '8px 0',
                    borderBottom: '1px solid var(--color-border)',
                  }}
                >
                  <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--color-text-muted)' }}>
                    <span>{o.id}</span>
                  </span>
                  <span className="t-body-sm" style={{ fontSize: '13px' }}>
                    <span>{o.what}</span>
                  </span>
                </div>
              ))}
            </div>
          </section>

          {/* Comparisons Card */}
          {comparisons.length > 0 && (
            <section className="fg-card" style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-2)' }}>
              <div className="fg-card__title">
                <span style={{ fontWeight: 600, fontSize: '14px' }}>
                  <span>{comparisonsTitle}</span>
                </span>
              </div>
              <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' }}>
                {comparisons.map((c: Comparison) => (
                  <div
                    key={c.id}
                    style={{
                      display: 'flex',
                      flexDirection: 'column',
                      gap: 'var(--space-2)',
                      padding: 'var(--space-2) 0',
                      borderBottom: '1px solid var(--color-border)',
                    }}
                  >
                    <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                      <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--color-text-muted)' }}>
                        <span>{c.id}</span>
                      </span>
                      {c.stale && (
                        <span style={{ fontSize: '12px', color: 'var(--color-warning)' }}>
                          <span>{LABEL_STALE_COMPARISON}</span>
                        </span>
                      )}
                    </div>
                    <div
                      style={{
                        display: 'grid',
                        gridTemplateColumns: 'repeat(auto-fit, minmax(160px, 1fr))',
                        gap: 'var(--space-2)',
                        fontSize: '13px',
                      }}
                    >
                      <div>
                        <div style={{ fontSize: '11px', color: 'var(--color-text-muted)' }}>{LABEL_SUBJECT}</div>
                        <div style={{ fontWeight: 500 }}>{c.subject}</div>
                      </div>
                      <div>
                        <div style={{ fontSize: '11px', color: 'var(--color-text-muted)' }}>{LABEL_VERDICT}</div>
                        <div style={{ fontWeight: 500 }}>{c.verdict}</div>
                      </div>
                      <div>
                        <div style={{ fontSize: '11px', color: 'var(--color-text-muted)' }}>{LABEL_TRADEOFFS}</div>
                        <div>{c.tradeoffs || 'none'}</div>
                      </div>
                    </div>
                  </div>
                ))}
              </div>
            </section>
          )}

          {/* Decision History Card */}
          {insight.decision_history && insight.decision_history.length > 0 && (
            <section className="fg-card" style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-2)' }}>
              <div className="fg-card__title">
                <span style={{ fontWeight: 600, fontSize: '14px' }}>{LABEL_DECISION_HISTORY}</span>
              </div>
              <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-2)' }}>
                {insight.decision_history.map((d, index) => (
                  <div
                    key={index}
                    style={{
                      display: 'flex',
                      flexDirection: 'column',
                      gap: '2px',
                      padding: 'var(--space-2) 0',
                      borderBottom: '1px solid var(--color-border)',
                      fontSize: '13px',
                    }}
                  >
                    <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                      <span style={{ fontWeight: 600, textTransform: 'capitalize' }}>{d.decision}</span>
                      <span style={{ fontSize: '11px', color: 'var(--color-text-muted)' }}>{d.decided_at}</span>
                    </div>
                    <span style={{ color: 'var(--color-text-muted)' }}>{d.rationale}</span>
                  </div>
                ))}
              </div>
            </section>
          )}
        </div>

        {/* Sidebar Actions Column */}
        <aside
          className="fg-card"
          style={{
            display: 'flex',
            flexDirection: 'column',
            gap: 'var(--space-3)',
            position: 'sticky',
            top: 0,
          }}
        >
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <span className="t-label" style={{ color: 'var(--color-text-muted)', fontSize: '13px' }}>
              <span>{LABEL_STATUS}</span>
            </span>
            <StatusBadge
              variant="chip"
              tone={
                status === 'planned'
                  ? 'success'
                  : status === 'rejected'
                    ? 'danger'
                    : 'neutral'
              }
              label={status}
            />
          </div>

          <div style={{ height: '1px', backgroundColor: 'var(--color-border)' }} />

          {/* Action buttons */}
          {canCompose && (
            <button
              type="button"
              className="fg-btn fg-btn--primary"
              disabled={stale}
              title={stale ? REASON_STALE_EVIDENCE : undefined}
              onClick={() => navigate(`/inbox/${encodeURIComponent(insight.id)}/apply`)}
            >
              <span>{BTN_COMPOSE_PATCH}</span>
            </button>
          )}

          {canPlan && (
            <button
              type="button"
              className="fg-btn fg-btn--secondary"
              onClick={() => handleOpenDecisionModal('plan')}
            >
              <span>{BTN_PLAN}</span>
            </button>
          )}

          {canReject && (
            <button
              type="button"
              className="fg-btn fg-btn--secondary"
              onClick={() => handleOpenDecisionModal('reject')}
            >
              <span>{BTN_REJECT}</span>
            </button>
          )}

          {canReopen && (
            <button
              type="button"
              className="fg-btn fg-btn--secondary"
              onClick={() => handleOpenDecisionModal('reopen')}
            >
              <span>{BTN_REOPEN}</span>
            </button>
          )}

          {canObsolete && (
            <button
              type="button"
              className="fg-btn fg-btn--ghost"
              style={{ color: 'var(--color-danger, #dc2626)' }}
              onClick={() => setObsoleteOpen(true)}
            >
              <span>{BTN_OBSOLETE}</span>
            </button>
          )}

          {isReadOnly && (
            <span className="t-caption" style={{ color: 'var(--color-text-muted)' }}>
              <span>{LABEL_READ_ONLY}</span>
            </span>
          )}

          <span className="t-caption" style={{ color: 'var(--color-text-muted)', fontSize: '12px' }}>
            <span>{LABEL_SIDEBAR_HINT}</span>
          </span>
        </aside>
      </div>

      {/* Decision Rationale Modal */}
      {modalDecision && (
        <div
          role="dialog"
          aria-modal="true"
          style={{
            position: 'fixed',
            inset: 0,
            backgroundColor: 'rgba(0, 0, 0, 0.5)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            padding: 'var(--space-4)',
            zIndex: 1000,
          }}
        >
          <div
            className="fg-card"
            style={{
              maxWidth: '480px',
              width: '100%',
              display: 'flex',
              flexDirection: 'column',
              gap: 'var(--space-3)',
            }}
          >
            <div className="fg-card__title" style={{ fontSize: '16px', fontWeight: 600 }}>
              <span style={{ textTransform: 'capitalize' }}>{modalDecision}</span>
              <span>{LABEL_INSIGHT_SUFFIX}</span>
            </div>

            <div className="fg-field">
              <label
                className="fg-field__label t-label"
                htmlFor="decision-rationale"
                style={{ fontWeight: 600, fontSize: '13px' }}
              >
                <span>{LABEL_RATIONALE}</span>
                <span style={{ color: 'var(--color-danger, #dc2626)' }}>{LABEL_ASTERISK}</span>
              </label>
              <textarea
                id="decision-rationale"
                className="fg-input fg-input--area"
                style={{ minHeight: '80px', width: '100%', fontSize: '13px' }}
                value={modalRationale}
                onChange={(e) => setModalRationale(e.target.value)}
                placeholder="Explain the reason for this decision…"
              />
            </div>

            <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 'var(--space-2)' }}>
              <button
                type="button"
                className="fg-btn fg-btn--secondary"
                onClick={() => setModalDecision(null)}
              >
                <span>{LABEL_CANCEL}</span>
              </button>
              <button
                type="button"
                className="fg-btn fg-btn--primary"
                disabled={submittingDecision || !modalRationale.trim()}
                onClick={() => void handleDecisionSubmit(modalDecision, modalRationale.trim())}
              >
                <span>{submittingDecision ? BTN_SUBMITTING : 'Confirm ' + modalDecision}</span>
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Obsolete ConfirmDialog */}
      {obsoleteOpen && (
        <ConfirmDialog
          open={obsoleteOpen}
          title={TITLE_OBSOLETE_DIALOG}
          body={BODY_OBSOLETE_DIALOG}
          confirmLabel={submittingDecision ? BTN_SUBMITTING : BTN_OBSOLETE}
          danger
          needsRationale
          onConfirm={(rationale) => void handleDecisionSubmit('obsolete', (rationale || '').trim())}
          onCancel={() => setObsoleteOpen(false)}
        />
      )}
    </div>
  );
}
