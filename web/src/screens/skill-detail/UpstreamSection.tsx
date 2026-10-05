import { useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { checkSkillUpstream } from '../../api/queries';
import type { SkillUpstream } from '../../api/types';
import { CommandBlock } from '../../components/CommandBlock';
import { StatusBadge } from '../../components/StatusBadge';

const TITLE_UPSTREAM = 'Upstream repository';
const BTN_CHECK_NOW = 'Check now';
const BTN_CHECKING = 'Checking…';
const BTN_REVIEW_UPDATE = 'Review update';
const LABEL_UNTRACKED_HINT =
  'This skill does not have an attached upstream source record. Run backfill to link it:';
const BANNER_UPSTREAM_REMOVED =
  'Skill removed in upstream repository. Your local copy continues working as a custom skill and no upstream updates can be applied.';
const LABEL_REPOSITORY = 'Repository';
const LABEL_PATH = 'Path';
const LABEL_CURRENT_COMMIT = 'Current commit';
const LABEL_LATEST_COMMIT = 'Latest commit';
const LABEL_FILES_CHANGED = 'Files changed';
const LABEL_LAST_CHECKED = 'Last checked';
const LABEL_CHANGED_UPSTREAM_PREFIX = 'Changed upstream (';
const LABEL_CHANGED_UPSTREAM_SUFFIX = '):';

interface UpstreamSectionProps {
  skillId: string;
  upstream: SkillUpstream | null;
  onReviewUpdate: () => void;
}

export function UpstreamSection({ skillId, upstream, onReviewUpdate }: UpstreamSectionProps) {
  const queryClient = useQueryClient();
  const [checking, setChecking] = useState(false);
  const [checkError, setCheckError] = useState<string | null>(null);

  if (!upstream) {
    return null;
  }

  const handleCheckNow = async () => {
    setChecking(true);
    setCheckError(null);
    try {
      await checkSkillUpstream(skillId);
      queryClient.invalidateQueries({ queryKey: ['skill-sources', skillId] });
      queryClient.invalidateQueries({ queryKey: ['skills'] });
    } catch (err) {
      setCheckError(err instanceof Error ? err.message : 'Failed to check upstream');
    } finally {
      setChecking(false);
    }
  };

  const getStatusProps = (status: string): { tone: 'success' | 'warning' | 'danger' | 'neutral'; label: string } => {
    switch (status) {
      case 'up_to_date':
        return { tone: 'success', label: 'Up to date' };
      case 'update_available':
        return { tone: 'warning', label: 'Update available' };
      case 'modified':
        return { tone: 'warning', label: 'Modified locally' };
      case 'unknown':
        return { tone: 'warning', label: 'Not checked' };
      case 'diverged':
        return { tone: 'danger', label: 'Diverged' };
      case 'upstream_removed':
        return { tone: 'danger', label: 'Removed upstream' };
      case 'unavailable':
        return { tone: 'danger', label: 'Unreachable' };
      case 'pinned':
        return { tone: 'neutral', label: 'Pinned' };
      case 'untracked':
        return { tone: 'neutral', label: 'Not tracked' };
      default:
        return { tone: 'neutral', label: status };
    }
  };

  const statusProps = getStatusProps(upstream.status);
  const canReview = upstream.status === 'update_available' || upstream.status === 'diverged';
  const shortBase = upstream.base_commit ? upstream.base_commit.slice(0, 12) : '-';
  const shortLatest = upstream.latest_commit ? upstream.latest_commit.slice(0, 12) : '-';
  const commitDate = upstream.latest_committed_at ? upstream.latest_committed_at.slice(0, 10) : '-';
  const repoRefText = `${upstream.repository}@${upstream.ref || 'main'}`;
  return (
    <section className="fg-card" style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: 'var(--space-2)' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-2)' }}>
          <span className="fg-card__title">{TITLE_UPSTREAM}</span>
          <StatusBadge tone={statusProps.tone} label={statusProps.label} />
        </div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-2)' }}>
          <button
            type="button"
            className="fg-btn fg-btn--secondary fg-btn--small"
            onClick={handleCheckNow}
            disabled={checking}
          >
            <span>{checking ? BTN_CHECKING : BTN_CHECK_NOW}</span>
          </button>
          <button
            type="button"
            className="fg-btn fg-btn--primary fg-btn--small"
            onClick={onReviewUpdate}
            disabled={!canReview}
          >
            <span>{BTN_REVIEW_UPDATE}</span>
          </button>
        </div>
      </div>

      {checkError && (
        <div className="fg-banner fg-banner--danger" style={{ fontSize: '13px' }}>
          <span>{checkError}</span>
        </div>
      )}

      {upstream.status === 'untracked' && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-2)' }}>
          <span style={{ fontSize: '13px', color: 'var(--color-text-muted)' }}>
            {LABEL_UNTRACKED_HINT}
          </span>
          <CommandBlock command={`skillhub source backfill --skill ${skillId}`} />
        </div>
      )}

      {upstream.status === 'upstream_removed' && (
        <div className="fg-banner fg-banner--warning" style={{ fontSize: '13px' }}>
          <span>{BANNER_UPSTREAM_REMOVED}</span>
        </div>
      )}

      <div className="fg-facts">
        <div className="fg-fact">
          <div className="fg-fact__label">
            <span>{LABEL_REPOSITORY}</span>
          </div>
          <div className="fg-fact__value" style={{ fontFamily: 'var(--font-mono)', fontSize: '13px' }}>
            <span>{repoRefText}</span>
          </div>
        </div>
        {upstream.path && (
          <div className="fg-fact">
            <div className="fg-fact__label">
              <span>{LABEL_PATH}</span>
            </div>
            <div className="fg-fact__value" style={{ fontFamily: 'var(--font-mono)', fontSize: '13px' }}>
              <span>{upstream.path}</span>
            </div>
          </div>
        )}
        <div className="fg-fact">
          <div className="fg-fact__label">
            <span>{LABEL_CURRENT_COMMIT}</span>
          </div>
          <div className="fg-fact__value" style={{ fontFamily: 'var(--font-mono)', fontSize: '13px' }}>
            <span>{shortBase}</span>
          </div>
        </div>
        <div className="fg-fact">
          <div className="fg-fact__label">
            <span>{LABEL_LATEST_COMMIT}</span>
          </div>
          <div className="fg-fact__value" style={{ fontFamily: 'var(--font-mono)', fontSize: '13px' }}>
            <span>{shortLatest} {commitDate !== '-' ? `(${commitDate})` : ''}</span>
          </div>
        </div>
        <div className="fg-fact">
          <div className="fg-fact__label">
            <span>{LABEL_FILES_CHANGED}</span>
          </div>
          <div className="fg-fact__value" style={{ fontSize: '13px' }}>
            <span>{upstream.changed_files >= 0 ? `${upstream.changed_files} file(s)` : '-'}</span>
          </div>
        </div>
        <div className="fg-fact">
          <div className="fg-fact__label">
            <span>{LABEL_LAST_CHECKED}</span>
          </div>
          <div className="fg-fact__value" style={{ fontSize: '13px' }}>
            <span>{upstream.checked_at || 'never'}</span>
          </div>
        </div>
      </div>

      {upstream.files && upstream.files.length > 0 && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-2)' }}>
          <div style={{ fontSize: '13px', fontWeight: 600 }}>
            <span>{LABEL_CHANGED_UPSTREAM_PREFIX}{upstream.files.length}{LABEL_CHANGED_UPSTREAM_SUFFIX}</span>
          </div>
          <ul style={{ margin: 0, paddingLeft: 'var(--space-4)', fontSize: '13px', fontFamily: 'var(--font-mono)' }}>
            {upstream.files.map((file) => {
              const statusBadgeText = `[${file.status}]`;
              return (
                <li key={file.path}>
                  <span style={{ color: 'var(--color-text-muted)', marginRight: 'var(--space-2)' }}>
                    <span>{statusBadgeText}</span>
                  </span>
                  <span>{file.path}</span>
                </li>
              );
            })}
          </ul>
        </div>
      )}

      {upstream.next_action && (
        <div style={{ fontSize: '13px', color: 'var(--color-text-muted)', marginTop: 'var(--space-1)' }}>
          <span>{upstream.next_action}</span>
        </div>
      )}
    </section>
  );
}
