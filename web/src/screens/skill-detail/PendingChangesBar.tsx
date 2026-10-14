const TEXT = {
  agentsKeep: ' · agents still get the current version',
  discard: 'Discard',
  review: 'Review →',
};

interface PendingChangesBarProps {
  count: number;
  onDiscard: () => void;
  onReview: () => void;
}

// Says that edits are waiting and that agents still get the committed version. It renders nothing
// while there are no changes.
export function PendingChangesBar({ count, onDiscard, onReview }: PendingChangesBarProps) {
  if (count === 0) return null;
  const headline = `${count} ${count === 1 ? 'change' : 'changes'} not committed`;
  return (
    <div className="app-pending-bar" role="region" aria-label="Pending changes">
      <span className="t-body-sm">
        <strong>{headline}</strong>
        <span>{TEXT.agentsKeep}</span>
      </span>
      <span className="app-pending-bar__actions">
        <button type="button" className="fg-btn fg-btn--secondary" onClick={onDiscard}>
          <span>{TEXT.discard}</span>
        </button>
        <button type="button" className="fg-btn fg-btn--primary" onClick={onReview}>
          <span>{TEXT.review}</span>
        </button>
      </span>
    </div>
  );
}
