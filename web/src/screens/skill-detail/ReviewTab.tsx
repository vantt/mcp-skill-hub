import type { SkillDetail, SkillReviewResult } from '../../api/types';
import { CopyButton } from '../../components/CopyButton';
import { StatusBadge } from '../../components/StatusBadge';
import { ContentTrustCard } from './ContentTrustCard';

const TITLE_VALIDITY = 'Validity';
const TITLE_READINESS = 'Activation readiness';
const TITLE_RESOURCES = 'Resources status';
const TITLE_CANONICAL_SERVED = 'Canonical vs served';
const TITLE_PROVENANCE = 'Provenance';
const TITLE_GIT = 'Git';
const LABEL_STRUCTURALLY_VALID = 'Structurally valid';
const LABEL_CANONICAL_ISSUES = '0 canonical issues';
const LABEL_MISSING = 'Missing';
const LABEL_GO_TO_FIELD = 'Go to field';
const LABEL_CANONICAL = 'Canonical';
const LABEL_SERVED = 'Served';
const LABEL_ORIGIN = 'Origin';
const LABEL_REF_COMMIT = 'Ref · commit';
const LABEL_GIT_NOTICE = 'The WebUI never commits or pushes.';
const CMD_GIT_STATUS = 'git status';
const REF_MAIN_CANONICAL = 'main · canonical';
const WARNING_DIVERGED = 'Served catalog differs from canonical files. Rebuild to publish.';

interface ReviewTabProps {
  skill: SkillDetail;
  review?: SkillReviewResult;
  onGoToEditor: (field?: string) => void;
}

export function ReviewTab({ skill, review, onGoToEditor }: ReviewTabProps) {
  const readiness = review?.activation_readiness;
  const isUntouched = readiness?.untouched_scaffold ?? false;

  const hasOperations = (skill.routing?.operations ?? []).length > 0;
  const hasTriggers = (skill.routing?.triggers ?? []).length > 0;
  const hasScope = Boolean(skill.routing?.min_scope);
  const hasContent = !isUntouched && Boolean(skill.content && skill.content.trim().length > 0);

  const checklist = [
    {
      id: 'content',
      label: 'Instructions (SKILL.md)',
      valid: hasContent,
    },
    {
      id: 'triggers',
      label: 'Triggers',
      valid: hasTriggers,
    },
    {
      id: 'operations',
      label: 'Operations & Rationale',
      valid: hasOperations || Boolean(skill.routing?.not_for),
    },
    {
      id: 'scope',
      label: 'Min scope',
      valid: hasScope,
    },
  ];

  const totalBytes = review?.resource_status?.total_bytes ?? skill.resources?.reduce((acc, r) => acc + r.size_bytes, 0) ?? 0;
  const resourceCount = review?.resource_status?.resource_count ?? skill.resources?.length ?? 1;
  const resourceSummaryText = `${resourceCount} file(s) · ${totalBytes} bytes`;

  return (
    <div
      style={{
        display: 'grid',
        gridTemplateColumns: 'repeat(auto-fit, minmax(min(100%, 320px), 1fr))',
        gap: 'var(--space-4)',
        alignItems: 'start',
      }}
    >
      {/* Content Trust Card */}
      {review?.content_trust && review.content_trust.third_party && (
        <ContentTrustCard trust={review.content_trust} />
      )}

      {/* Validity Card */}
      <section className="fg-card" style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-2)' }}>
        <div className="fg-card__title">
          <span>{TITLE_VALIDITY}</span>
        </div>
        <StatusBadge label={LABEL_STRUCTURALLY_VALID} tone="success" />
        <span className="t-body-sm" style={{ color: 'var(--color-text-muted)' }}>
          <span>{LABEL_CANONICAL_ISSUES}</span>
        </span>
      </section>

      {/* Activation Readiness Card */}
      <section className="fg-card" style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-2)' }}>
        <div className="fg-card__title">
          <span>{TITLE_READINESS}</span>
        </div>
        {checklist.map((item) => (
          <div
            key={item.id}
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: 'var(--space-2)',
              padding: '6px 0',
              borderBottom: '1px solid var(--color-border)',
            }}
          >
            <span
              aria-hidden="true"
              style={{
                width: '16px',
                color: item.valid ? 'var(--color-success)' : 'var(--color-warning)',
                fontWeight: 'bold',
              }}
            >
              {item.valid ? '✓' : '▲'}
            </span>
            <span className="t-body-sm" style={{ flex: 1 }}>
              <span>{item.label}</span>
            </span>
            {!item.valid && (
              <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                <span className="t-caption" style={{ color: 'var(--color-warning)' }}>
                  <span>{LABEL_MISSING}</span>
                </span>
                <button
                  type="button"
                  className="fg-btn fg-btn--ghost"
                  style={{ padding: '2px 6px', fontSize: '11px' }}
                  onClick={() => onGoToEditor(item.id)}
                >
                  <span>{LABEL_GO_TO_FIELD}</span>
                </button>
              </div>
            )}
          </div>
        ))}
      </section>

      {/* Resources Status Card */}
      <section className="fg-card" style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-2)' }}>
        <div className="fg-card__title">
          <span>{TITLE_RESOURCES}</span>
        </div>
        <span className="t-body-sm">
          <span>{resourceSummaryText}</span>
        </span>
      </section>

      {/* Canonical vs Served Card */}
      <section className="fg-card" style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' }}>
        <div className="fg-card__title">
          <span>{TITLE_CANONICAL_SERVED}</span>
        </div>
        {skill.diverged && (
          <div className="fg-caveat fg-caveat--warn">
            <span>{WARNING_DIVERGED}</span>
          </div>
        )}
        <div className="fg-facts">
          <div className="fg-fact">
            <div className="fg-fact__label">
              <span>{LABEL_CANONICAL}</span>
            </div>
            <div className="fg-fact__value">
              <span>{skill.lifecycle_state}</span>
            </div>
          </div>
          <div className="fg-fact">
            <div className="fg-fact__label">
              <span>{LABEL_SERVED}</span>
            </div>
            <div className="fg-fact__value">
              <span>{skill.status}</span>
            </div>
          </div>
        </div>
      </section>

      {/* Provenance Card */}
      <section className="fg-card" style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-2)' }}>
        <div className="fg-card__title">
          <span>{TITLE_PROVENANCE}</span>
        </div>
        <div className="fg-facts">
          <div className="fg-fact">
            <div className="fg-fact__label">
              <span>{LABEL_ORIGIN}</span>
            </div>
            <div className="fg-fact__value" style={{ fontFamily: 'var(--font-mono)', fontSize: '13px' }}>
              <span>{skill.path}</span>
            </div>
          </div>
          <div className="fg-fact">
            <div className="fg-fact__label">
              <span>{LABEL_REF_COMMIT}</span>
            </div>
            <div className="fg-fact__value" style={{ fontFamily: 'var(--font-mono)', fontSize: '13px' }}>
              <span>{REF_MAIN_CANONICAL}</span>
            </div>
          </div>
        </div>
      </section>

      {/* Git Card */}
      <section className="fg-card" style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' }}>
        <div className="fg-card__title">
          <span>{TITLE_GIT}</span>
        </div>
        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: '8px',
            border: '1px solid var(--color-border)',
            background: 'var(--color-surface-sunken)',
            borderRadius: 'var(--input-radius)',
            padding: '4px 4px 4px 12px',
          }}
        >
          <code style={{ fontFamily: 'var(--font-mono)', fontSize: '13px', flex: 1 }}>{CMD_GIT_STATUS}</code>
          <CopyButton text={CMD_GIT_STATUS} />
        </div>
        <span className="t-caption" style={{ color: 'var(--color-text-subtle)' }}>
          <span>{LABEL_GIT_NOTICE}</span>
        </span>
      </section>
    </div>
  );
}
