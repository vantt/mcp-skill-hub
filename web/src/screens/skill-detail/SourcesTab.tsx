import { useState } from 'react';
import { useSkillSources } from '../../api/queries';
import { Skeleton } from '../../components/Skeleton';
import { LearningSection } from './LearningSection';
import { UpstreamReview } from './UpstreamReview';
import { UpstreamSection } from './UpstreamSection';

interface SourcesTabProps {
  skillId: string;
}

export function SourcesTab({ skillId }: SourcesTabProps) {
  const { data, isLoading, error, refetch } = useSkillSources(skillId);
  const [reviewOpen, setReviewOpen] = useState(false);

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
          <span>Retry</span>
        </button>
      </div>
    );
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
      {reviewOpen ? (
        <UpstreamReview
          skillId={skillId}
          onClose={() => setReviewOpen(false)}
        />
      ) : (
        <UpstreamSection
          skillId={skillId}
          upstream={data.upstream}
          onReviewUpdate={() => setReviewOpen(true)}
        />
      )}

      <LearningSection skillId={skillId} learning={data.learning} />
    </div>
  );
}
