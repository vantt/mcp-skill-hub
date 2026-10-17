import { Link } from 'react-router';
import { useReviewQueue } from '../../api/queries';
import type { ReviewQueueItem } from '../../api/types';
import { EmptyState } from '../../components/EmptyState';
import { Skeleton } from '../../components/Skeleton';
import { StatusBadge } from '../../components/StatusBadge';

const TEXT = {
  loadFailed: 'The list of skills waiting for review could not be loaded.',
  title: 'Waiting for review',
  go: 'Review →',
  subtitle: 'Drafts and changed third-party content stay here until a person has looked at them. Approved skills leave the list.',
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
      <header style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-1)' }}>
        <h1 className="t-title" style={{ margin: 0, fontSize: '24px', fontWeight: 600 }}>
          {heading}
        </h1>
        <span className="t-body-sm" style={{ color: 'var(--color-text-muted)' }}>
          {TEXT.subtitle}
        </span>
      </header>
      {data.items.length === 0 ? (
        <EmptyState
          title="Nothing is waiting for review"
          description="New drafts and third-party skills whose content changed show up here."
        />
      ) : (
        <ul className="app-review-queue" aria-label="Skills waiting for review">
          {data.items.map((item) => (
            <li
              key={item.submission_id ?? item.skill_id}
              className={`app-review-queue__item app-review-queue__item--${item.kind}`}
            >
              <div className="app-review-queue__main">
                <Link to={reviewPath(item)} className="app-review-queue__title">
                  {item.title || item.skill_id}
                </Link>
                {item.origin && <span className="app-review-queue__origin app-wrap">{item.origin}</span>}
              </div>
              <div className="app-review-queue__meta">
                <StatusBadge label={KIND_LABEL[item.kind]} tone={item.kind === 'draft' ? 'neutral' : 'warning'} />
                {item.since && <span className="t-caption app-review-queue__since">{item.since}</span>}
              </div>
              <span className="app-review-queue__go" aria-hidden="true">
                {TEXT.go}
              </span>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
