import { Link } from 'react-router';
import { useReviewQueue } from '../../api/queries';
import type { ReviewQueueItem } from '../../api/types';
import { EmptyState } from '../../components/EmptyState';
import { Skeleton } from '../../components/Skeleton';
import { StatusBadge } from '../../components/StatusBadge';

const TEXT = {
  loadFailed: 'The list of skills waiting for review could not be loaded.',
  title: 'Waiting for review',
};


const KIND_LABEL: Record<ReviewQueueItem['kind'], string> = {
  draft: 'New draft',
  content_changed: 'Content changed · needs approval',
  submission: 'Submitted · needs approval',
};

function reviewPath(item: ReviewQueueItem): string {
  if (item.submission_id) return `/review/submissions/${encodeURIComponent(item.submission_id)}`;
  return `/skills/${encodeURIComponent(item.skill_id)}/review-content`;
}

// What needs a person before agents get it: new drafts, third-party skills whose content changed
// after it was approved, and skills other machines submitted. Each row opens the full review.
export function WaitingForReviewScreen() {
  const { data, isLoading, error } = useReviewQueue();

  if (isLoading) {
    return (
      <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' }}>
        <Skeleton width="40%" height="32px" />
        <Skeleton height="160px" />
      </div>
    );
  }
  if (error || !data) {
    return (
      <div className="fg-banner fg-banner--danger" role="alert">
        <span>{TEXT.loadFailed}</span>
      </div>
    );
  }

  const heading = `${TEXT.title} (${data.items.length})`;

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
      <h1 className="t-title" style={{ margin: 0, fontSize: '24px', fontWeight: 600 }}>
        {heading}
      </h1>
      {data.items.length === 0 ? (
        <EmptyState
          title="Nothing is waiting for review"
          description="New drafts and third-party skills whose content changed show up here."
        />
      ) : (
        <ul className="app-review-queue" aria-label="Skills waiting for review">
          {data.items.map((item) => (
            <li key={item.submission_id ?? item.skill_id} className="fg-card app-review-queue__item">
              <div style={{ display: 'flex', flexDirection: 'column', gap: '2px', minWidth: 0 }}>
                <Link to={reviewPath(item)} className="t-body" style={{ fontWeight: 600 }}>
                  {item.title || item.skill_id}
                </Link>
                {item.origin && (
                  <span className="t-caption app-wrap" style={{ color: 'var(--color-text-muted)' }}>
                    {item.origin}
                  </span>
                )}
              </div>
              <StatusBadge label={KIND_LABEL[item.kind]} tone={item.kind === 'draft' ? 'neutral' : 'warning'} />
              <span className="t-caption" style={{ color: 'var(--color-text-muted)' }}>
                {item.since}
              </span>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
