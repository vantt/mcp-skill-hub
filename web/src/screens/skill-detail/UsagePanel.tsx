import { useState } from 'react';
import { useSkillUsage } from '../../api/queries';
import type { FunnelSince } from '../../api/types';
import { EmptyState } from '../../components/EmptyState';
import { Skeleton } from '../../components/Skeleton';

const TITLE_WINDOW = 'Time window';
const TITLE_RECOMMENDATIONS = 'Recommendations & Activations';
const TITLE_LOADS = 'Content loads';
const TITLE_OUTCOMES = 'Health & Feedback';
const TITLE_DOCTOR = 'Doctor checks';

const CAPTION_SERVER_OBSERVED = 'Basis: server-observed';
const CAPTION_HOST_REPORTED = 'Basis: host-reported';
const CAPTION_TERMINAL = 'Basis: terminal';

const LABEL_RECOMMENDED = 'Recommended';
const LABEL_ACTIVATED = 'Activated after recommendation';
const LABEL_ACCEPTANCE = 'Acceptance rate';
const LABEL_OVERRIDES = 'Overrides';
const LABEL_MISSES = 'Misses';
const LABEL_BLOCKED = 'Blocked by review';
const LABEL_SETUP_FAILURES = 'Setup failures';
const LABEL_NEGATIVE_FEEDBACK = 'Negative feedback';
const LABEL_NEGATIVE_AFTER_LOAD = 'Negative after load';
const LABEL_FAILURE_RATE = 'Doctor failure rate';
const LABEL_DOCTOR_READY = 'Ready';
const LABEL_DOCTOR_SETUP = 'Setup required';
const LABEL_DOCTOR_UNSUPPORTED = 'Unsupported platform';
const LABEL_DOCTOR_FAILED = 'Failed';

const LABEL_LOAD_ENTRYPOINT = 'Entrypoint';
const LABEL_LOAD_REFERENCE = 'Reference';
const LABEL_LOAD_SCRIPT = 'Script';
const LABEL_LOAD_ASSET = 'Asset';
const LABEL_LOAD_RESOURCE = 'Resource';

const EMPTY_TITLE = 'No usage recorded';
const EMPTY_DESCRIPTION =
  'No recommendations, activations, loads, or doctor checks observed for this skill in the selected time window.';

const WINDOW_OPTIONS: { label: string; value: FunnelSince }[] = [
  { label: '7 days', value: '7d' },
  { label: '30 days', value: '30d' },
  { label: '90 days', value: '90d' },
  { label: '180 days', value: '180d' },
];

function formatRate(rate: number | null | undefined): string {
  if (rate == null) {
    return 'N/A';
  }
  return `${(rate * 100).toFixed(1)}%`;
}

interface StatRowProps {
  label: string;
  value: string | number;
}

function StatRow({ label, value }: StatRowProps) {
  return (
    <div
      style={{
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        padding: '6px 0',
        borderBottom: '1px solid var(--color-border)',
      }}
    >
      <span className="t-body-sm" style={{ color: 'var(--color-text-muted)' }}>
        <span>{label}</span>
      </span>
      <span className="t-body-sm" style={{ fontWeight: 600 }}>
        <span>{value}</span>
      </span>
    </div>
  );
}

export interface UsagePanelProps {
  skillId: string;
}

export function UsagePanel({ skillId }: UsagePanelProps) {
  const [since, setSince] = useState<FunnelSince>('30d');
  const { data: report, isLoading, error } = useSkillUsage(skillId, since);

  const skill = report?.skill;
  const windowSummary = report?.window
    ? `${report.window.since} to ${report.window.until} (${report.window.days} days)`
    : '';
  const isZero =
    !skill ||
    (skill.recommended_primary === 0 &&
      skill.recommended_supporting === 0 &&
      skill.total_activations === 0 &&
      skill.blocked_by_review === 0 &&
      skill.total_loads === 0 &&
      skill.total_doctor === 0 &&
      skill.setup_failed === 0 &&
      skill.negative_feedback === 0);

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
      <section
        className="fg-card"
        style={{
          display: 'flex',
          flexWrap: 'wrap',
          alignItems: 'center',
          justifyContent: 'space-between',
          gap: 'var(--space-3)',
        }}
      >
        <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-1)' }}>
          <span className="fg-card__title">
            <span>{TITLE_WINDOW}</span>
          </span>
          {report?.window && (
            <span className="t-caption" style={{ color: 'var(--color-text-subtle)' }}>
              <span>{windowSummary}</span>
            </span>
          )}
        </div>
        <div
          role="group"
          aria-label="Select usage time window"
          style={{ display: 'flex', gap: 'var(--space-2)' }}
        >
          {WINDOW_OPTIONS.map((opt) => (
            <button
              key={opt.value}
              type="button"
              className={`fg-btn ${since === opt.value ? 'fg-btn--primary' : 'fg-btn--ghost'}`}
              style={{ padding: '4px 10px', fontSize: '13px' }}
              onClick={() => setSince(opt.value)}
            >
              <span>{opt.label}</span>
            </button>
          ))}
        </div>
      </section>

      {/* Loading Skeleton */}
      {isLoading && (
        <div
          style={{
            display: 'grid',
            gridTemplateColumns: 'repeat(auto-fit, minmax(min(100%, 300px), 1fr))',
            gap: 'var(--space-4)',
          }}
        >
          <Skeleton height="180px" />
          <Skeleton height="180px" />
          <Skeleton height="180px" />
          <Skeleton height="180px" />
        </div>
      )}

      {/* Error state */}
      {error && !isLoading && (
        <EmptyState
          title="Failed to load usage"
          description={error instanceof Error ? error.message : 'An error occurred.'}
        />
      )}

      {/* Empty State */}
      {!isLoading && !error && isZero && (
        <EmptyState title={EMPTY_TITLE} description={EMPTY_DESCRIPTION} />
      )}

      {/* Content Groups */}
      {!isLoading && !error && !isZero && skill && (
        <div
          style={{
            display: 'grid',
            gridTemplateColumns: 'repeat(auto-fit, minmax(min(100%, 300px), 1fr))',
            gap: 'var(--space-4)',
            alignItems: 'start',
          }}
        >
          {/* Group 1: Recommendations & Activations */}
          <section
            className="fg-card"
            style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-2)' }}
          >
            <div style={{ display: 'flex', flexDirection: 'column', gap: '2px' }}>
              <div className="fg-card__title">
                <span>{TITLE_RECOMMENDATIONS}</span>
              </div>
              <span className="t-caption" style={{ color: 'var(--color-text-subtle)' }}>
                <span>{CAPTION_SERVER_OBSERVED}</span>
              </span>
            </div>
            <StatRow label={LABEL_RECOMMENDED} value={skill.recommended_primary} />
            <StatRow
              label={LABEL_ACTIVATED}
              value={skill.activations['recommended'] || 0}
            />
            <StatRow label={LABEL_ACCEPTANCE} value={formatRate(skill.acceptance_rate)} />
            <StatRow label={LABEL_OVERRIDES} value={skill.overrides} />
            <StatRow label={LABEL_MISSES} value={skill.misses} />
            <StatRow label={LABEL_BLOCKED} value={skill.blocked_by_review} />
          </section>

          {/* Group 2: Content Loads */}
          <section
            className="fg-card"
            style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-2)' }}
          >
            <div style={{ display: 'flex', flexDirection: 'column', gap: '2px' }}>
              <div className="fg-card__title">
                <span>{TITLE_LOADS}</span>
              </div>
              <span className="t-caption" style={{ color: 'var(--color-text-subtle)' }}>
                <span>{CAPTION_SERVER_OBSERVED}</span>
              </span>
            </div>
            <StatRow label={LABEL_LOAD_ENTRYPOINT} value={skill.loads['entrypoint'] || 0} />
            <StatRow label={LABEL_LOAD_REFERENCE} value={skill.loads['reference'] || 0} />
            <StatRow label={LABEL_LOAD_SCRIPT} value={skill.loads['script'] || 0} />
            <StatRow label={LABEL_LOAD_ASSET} value={skill.loads['asset'] || 0} />
            <StatRow label={LABEL_LOAD_RESOURCE} value={skill.loads['resource'] || 0} />
          </section>

          {/* Group 3: Health & Feedback */}
          <section
            className="fg-card"
            style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-2)' }}
          >
            <div style={{ display: 'flex', flexDirection: 'column', gap: '2px' }}>
              <div className="fg-card__title">
                <span>{TITLE_OUTCOMES}</span>
              </div>
              <span className="t-caption" style={{ color: 'var(--color-text-subtle)' }}>
                <span>{CAPTION_HOST_REPORTED}</span>
              </span>
            </div>
            <StatRow
              label={LABEL_SETUP_FAILURES}
              value={`${skill.setup_failed}${skill.setup_failed_rate != null ? ` (${formatRate(skill.setup_failed_rate)})` : ''}`}
            />
            <StatRow label={LABEL_NEGATIVE_FEEDBACK} value={skill.negative_feedback} />
            <StatRow label={LABEL_NEGATIVE_AFTER_LOAD} value={skill.negative_after_load} />
          </section>

          {/* Group 4: Doctor Checks */}
          <section
            className="fg-card"
            style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-2)' }}
          >
            <div style={{ display: 'flex', flexDirection: 'column', gap: '2px' }}>
              <div className="fg-card__title">
                <span>{TITLE_DOCTOR}</span>
              </div>
              <span className="t-caption" style={{ color: 'var(--color-text-subtle)' }}>
                <span>{CAPTION_TERMINAL}</span>
              </span>
            </div>
            <StatRow label={LABEL_DOCTOR_READY} value={skill.doctor['ready'] || 0} />
            <StatRow
              label={LABEL_DOCTOR_SETUP}
              value={skill.doctor['setup_required'] || 0}
            />
            <StatRow
              label={LABEL_DOCTOR_UNSUPPORTED}
              value={skill.doctor['unsupported_platform'] || 0}
            />
            <StatRow label={LABEL_DOCTOR_FAILED} value={skill.doctor['failed'] || 0} />
            <StatRow
              label={LABEL_FAILURE_RATE}
              value={formatRate(skill.doctor_failure_rate)}
            />
          </section>
        </div>
      )}
    </div>
  );
}
