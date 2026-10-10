import { useSearchParams } from 'react-router';
import type { SkillDetail, SkillReviewResult } from '../../api/types';
import { CommandBlock } from '../../components/CommandBlock';
import { StatusBadge } from '../../components/StatusBadge';
import { ContentTrustCard } from './ContentTrustCard';
import { ProvenanceCard } from './ProvenanceCard';
import { activationChecklist } from './activation-checklist';
const TITLE_VALIDITY = 'Validity';
const TITLE_READINESS = 'Activation readiness';
const TITLE_RESOURCES = 'Resources status';
const TITLE_CANONICAL_SERVED = 'In Git vs what agents see';
const TITLE_GIT = 'Git';
const LABEL_STRUCTURALLY_VALID = 'Structurally valid';
const LABEL_NOT_VALID = 'Not valid';
const HELP_VALIDITY = 'Whether the skill file follows the required format.';
const HELP_VALID_OK = 'No format problems found.';
const HELP_VALID_BAD = 'The skill file has format problems. Fix them in the Editor tab.';
const HELP_READINESS = 'What a draft needs before agents can use it. A skill with a missing item cannot be activated.';
const HELP_RESOURCES = 'Files that ship with the skill.';
const HELP_CANONICAL_SERVED =
  'In Git files is the lifecycle stored in the skill files. Agents see is what the search index gives them. They can differ until the index is rebuilt.';
const LABEL_MISSING = 'Missing';
const LABEL_GO_TO_FIELD = 'Go to field';
const LABEL_CANONICAL = 'In Git files';
const LABEL_SERVED = 'Agents see';
const WARNING_DIVERGED = 'What agents see differs from the skill files. Rebuild the index to publish the files.';
const LABEL_GIT_NOTICE = 'The WebUI never commits or pushes.';
const CMD_GIT_STATUS = 'git status';

interface ReviewTabProps {
  skill: SkillDetail;
  review?: SkillReviewResult;
  onGoToEditor: (field?: string) => void;
  onViewSources?: () => void;
}

export function ReviewTab({ skill, review, onGoToEditor, onViewSources }: ReviewTabProps) {
  const [, setSearchParams] = useSearchParams();
  const handleViewSources = () => {
    if (onViewSources) {
      onViewSources();
    } else {
      setSearchParams((prev) => {
        const next = new URLSearchParams(prev);
        next.set('tab', 'sources');
        return next;
      });
    }
  };
  const checklist = activationChecklist(skill, review);

  const totalBytes = review?.resource_status?.total_bytes ?? skill.resources?.reduce((acc, r) => acc + r.size_bytes, 0) ?? 0;
  const resourceCount = review?.resource_status?.resource_count ?? skill.resources?.length ?? 1;
  const resourceSummaryText = `${resourceCount} ${resourceCount === 1 ? 'file' : 'files'} · ${totalBytes} bytes`;

  return (
    <div
      style={{
        display: 'grid',
        gridTemplateColumns: 'repeat(auto-fit, minmax(min(100%, 320px), 1fr))',
        gap: 'var(--space-4)',
        alignItems: 'start',
      }}
    >
      {/* Content Trust Card (Third-party only) */}
      {review?.content_trust?.third_party && (
        <ContentTrustCard trust={review.content_trust} />
      )}

      {/* Validity Card */}
      <section className="fg-card" style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-2)' }}>
        <div className="fg-card__title">
          <span>{TITLE_VALIDITY}</span>
        </div>
        <span className="t-caption" style={{ color: 'var(--color-text-muted)' }}>
          <span>{HELP_VALIDITY}</span>
        </span>
        <StatusBadge
          label={review && !review.valid ? LABEL_NOT_VALID : LABEL_STRUCTURALLY_VALID}
          tone={review && !review.valid ? 'danger' : 'success'}
        />
        <span className="t-body-sm" style={{ color: 'var(--color-text-muted)' }}>
          <span>{review && !review.valid ? HELP_VALID_BAD : HELP_VALID_OK}</span>
        </span>
      </section>

      {/* Activation Readiness Card */}
      <section className="fg-card" style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-2)' }}>
        <div className="fg-card__title">
          <span>{TITLE_READINESS}</span>
        </div>
        <span className="t-caption" style={{ color: 'var(--color-text-muted)' }}>
          <span>{HELP_READINESS}</span>
        </span>
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
        <span className="t-caption" style={{ color: 'var(--color-text-muted)' }}>
          <span>{HELP_RESOURCES}</span>
        </span>
        <span className="t-body-sm">
          <span>{resourceSummaryText}</span>
        </span>
      </section>

      {/* Canonical vs Served Card */}
      <section className="fg-card" style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' }}>
        <div className="fg-card__title">
          <span>{TITLE_CANONICAL_SERVED}</span>
        </div>
        <span className="t-caption" style={{ color: 'var(--color-text-muted)' }}>
          <span>{HELP_CANONICAL_SERVED}</span>
        </span>
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
      <ProvenanceCard provenance={review?.provenance} onViewSources={handleViewSources} />

      {/* Git Card */}
      <section className="fg-card" style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' }}>
        <div className="fg-card__title">
          <span>{TITLE_GIT}</span>
        </div>
        <CommandBlock command={CMD_GIT_STATUS} />
        <span className="t-caption" style={{ color: 'var(--color-text-subtle)' }}>
          <span>{LABEL_GIT_NOTICE}</span>
        </span>
      </section>
    </div>
  );
}
