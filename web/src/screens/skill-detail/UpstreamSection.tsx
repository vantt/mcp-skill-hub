import { useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { checkSkillUpstream } from '../../api/queries';
import type { SkillUpstream } from '../../api/types';
import { CommandBlock } from '../../components/CommandBlock';
import { StatusBadge } from '../../components/StatusBadge';

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

  return (
    <section className="fg-card" style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: 'var(--space-2)' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-2)' }}>
          <span className="fg-card__title">Upstream repository</span>
          <StatusBadge tone={statusProps.tone} label={statusProps.label} />
        </div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-2)' }}>
          <button
            type="button"
            className="fg-btn fg-btn--secondary fg-btn--small"
            onClick={handleCheckNow}
            disabled={checking}
          >
            <span>{checking ? 'Checking…' : 'Check now'}</span>
          </button>
          <button
            type="button"
            className="fg-btn fg-btn--primary fg-btn--small"
            onClick={onReviewUpdate}
            disabled={!canReview}
          >
            <span>Review update</span>
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
          <span style={{ fontSize: '13px', color: 'var(--color-text-subtle)' }}>
            This skill does not have an attached upstream source record. Run backfill to link it:
          </span>
          <CommandBlock command={`skillhub source backfill --skill ${skillId}`} />
        </div>
      )}

      {upstream.status === 'upstream_removed' && (
        <div className="fg-banner fg-banner--warning" style={{ fontSize: '13px' }}>
          <span>
            Skill removed in upstream repository. Your local copy continues working as a custom skill and no upstream updates can be applied.
          </span>
        </div>
      )}

      <div className="fg-facts">
        <div className="fg-fact">
          <div className="fg-fact__label">
            <span>Repository</span>
          </div>
          <div className="fg-fact__value" style={{ fontFamily: 'var(--font-mono)', fontSize: '13px' }}>
            <span>{upstream.repository}@{upstream.ref || 'main'}</span>
          </div>
        </div>
        {upstream.path && (
          <div className="fg-fact">
            <div className="fg-fact__label">
              <span>Path</span>
            </div>
            <div className="fg-fact__value" style={{ fontFamily: 'var(--font-mono)', fontSize: '13px' }}>
              <span>{upstream.path}</span>
            </div>
          </div>
        )}
        <div className="fg-fact">
          <div className="fg-fact__label">
            <span>Current commit</span>
          </div>
          <div className="fg-fact__value" style={{ fontFamily: 'var(--font-mono)', fontSize: '13px' }}>
            <span>{shortBase}</span>
          </div>
        </div>
        <div className="fg-fact">
          <div className="fg-fact__label">
            <span>Latest commit</span>
          </div>
          <div className="fg-fact__value" style={{ fontFamily: 'var(--font-mono)', fontSize: '13px' }}>
            <span>{shortLatest} {commitDate !== '-' ? `(${commitDate})` : ''}</span>
          </div>
        </div>
        <div className="fg-fact">
          <div className="fg-fact__label">
            <span>Files changed</span>
          </div>
          <div className="fg-fact__value" style={{ fontSize: '13px' }}>
            <span>{upstream.changed_files >= 0 ? `${upstream.changed_files} file(s)` : '-'}</span>
          </div>
        </div>
        <div className="fg-fact">
          <div className="fg-fact__label">
            <span>Last checked</span>
          </div>
          <div className="fg-fact__value" style={{ fontSize: '13px' }}>
            <span>{upstream.checked_at || 'never'}</span>
          </div>
        </div>
      </div>

      {upstream.files && upstream.files.length > 0 && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-2)' }}>
          <div style={{ fontSize: '13px', fontWeight: 600 }}>
            <span>Changed upstream ({upstream.files.length}):</span>
          </div>
          <ul style={{ margin: 0, paddingLeft: 'var(--space-4)', fontSize: '13px', fontFamily: 'var(--font-mono)' }}>
            {upstream.files.map((file) => (
              <li key={file.path}>
                <span style={{ color: 'var(--color-text-subtle)', marginRight: 'var(--space-2)' }}>[{file.status}]</span>
                <span>{file.path}</span>
              </li>
            ))}
          </ul>
        </div>
      )}

      {upstream.next_action && (
        <div style={{ fontSize: '13px', color: 'var(--color-text-subtle)', marginTop: 'var(--space-1)' }}>
          <span>{upstream.next_action}</span>
        </div>
      )}
    </section>
  );
}
