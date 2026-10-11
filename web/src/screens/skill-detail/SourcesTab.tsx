import { useState } from 'react';
import { useSkillSources } from '../../api/queries';
import type { SkillSourcesResult } from '../../api/types';
import { ActionBar } from '../../components/ActionBar';
import { EmptyState } from '../../components/EmptyState';
import { Skeleton } from '../../components/Skeleton';
import { Toast } from '../../components/Toast';
import { AddSourceForm } from './AddSourceForm';
import { LearningSection } from './LearningSection';
import { UpstreamReview } from './UpstreamReview';
import { UpstreamSection } from './UpstreamSection';

const LABEL_RETRY = 'Retry';
const BTN_ADD_SOURCE = 'Add source';
const EMPTY_TITLE = 'No sources yet';
const EMPTY_DESCRIPTION =
  'A source is a Git repository tied to this skill in one of two roles: an upstream source is the repository the skill was copied from, and a learning reference is a repository your curator agent monitors for improvement ideas. Add a learning reference to start.';

// "owner/repo" for a GitHub URL, the address as given otherwise.
function repositoryLabel(repository: string): string {
  return repository.replace(/^https?:\/\/(www\.)?github\.com\//, '').replace(/\.git$/, '').replace(/\/$/, '');
}

export function sourcesStatusText(data: Pick<SkillSourcesResult, 'upstream' | 'learning'>): string {
  const parts: string[] = [];
  if (data.upstream) {
    const label = repositoryLabel(data.upstream.repository);
    parts.push(data.upstream.status === 'untracked' ? `Upstream: ${label} (not tracked)` : `Upstream: ${label}`);
  }
  const n = data.learning.length;
  if (n > 0) parts.push(`${n} learning ${n === 1 ? 'reference' : 'references'}`);
  return parts.length > 0 ? parts.join(' · ') : EMPTY_TITLE;
}

interface SourcesTabProps {
  skillId: string;
}

export function SourcesTab({ skillId }: SourcesTabProps) {
  const { data, isLoading, error, refetch } = useSkillSources(skillId);
  const [reviewOpen, setReviewOpen] = useState(false);
  const [addOpen, setAddOpen] = useState(false);
  const [toastMessage, setToastMessage] = useState<string | null>(null);

  if (isLoading) {
    return (
      <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
        <Skeleton width="40%" height="28px" />
        <Skeleton height="160px" />
        <Skeleton height="120px" />
      </div>
    );
  }

  if (error || !data) {
    return (
      <div className="fg-card" style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' }}>
        <div className="fg-banner fg-banner--danger">
          <span>{error instanceof Error ? error.message : 'Failed to load skill sources'}</span>
        </div>
        <button
          type="button"
          className="fg-btn fg-btn--secondary"
          style={{ width: 'fit-content' }}
          onClick={() => void refetch()}
        >
          <span>{LABEL_RETRY}</span>
        </button>
      </div>
    );
  }

  const isEmpty = !data.upstream && data.learning.length === 0;
  const openAdd = () => setAddOpen(true);
  const addButton = (
    <button type="button" className="fg-btn fg-btn--primary" onClick={openAdd} disabled={addOpen}>
      <span>{BTN_ADD_SOURCE}</span>
    </button>
  );

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
      <ActionBar status={<span>{sourcesStatusText(data)}</span>}>{addButton}</ActionBar>

      {addOpen && (
        <AddSourceForm
          skillId={skillId}
          linkedSourceIds={data.learning.map((ref) => ref.source_id)}
          upstreamSourceId={data.upstream?.source_id || undefined}
          onAdded={(message) => {
            setAddOpen(false);
            setToastMessage(message);
          }}
          onCancel={() => setAddOpen(false)}
        />
      )}

      {isEmpty ? (
        !addOpen && <EmptyState title={EMPTY_TITLE} description={EMPTY_DESCRIPTION} action={addButton} />
      ) : (
        <>
          {data.upstream &&
            (reviewOpen ? (
              <UpstreamReview skillId={skillId} onClose={() => setReviewOpen(false)} />
            ) : (
              <UpstreamSection
                skillId={skillId}
                upstream={data.upstream}
                onReviewUpdate={() => setReviewOpen(true)}
                onTracked={setToastMessage}
              />
            ))}
          <LearningSection skillId={skillId} learning={data.learning} />
        </>
      )}

      <Toast message={toastMessage} onClose={() => setToastMessage(null)} />
    </div>
  );
}
