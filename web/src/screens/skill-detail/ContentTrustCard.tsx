import type { ContentTrust } from '../../api/types';
import { CommandBlock } from '../../components/CommandBlock';
import { CopyButton } from '../../components/CopyButton';
import { StatusBadge } from '../../components/StatusBadge';

const TITLE_CONTENT_TRUST = 'Content trust';
const BADGE_APPROVED = 'Approved';
const BADGE_CHANGED = 'Changed since approval';
const BADGE_REVIEW_REQUIRED = 'Review required';
const LABEL_DIGEST = 'Content digest';
const TEXT_NEVER_APPROVED = 'Never approved: review the full skill.';
const TEXT_NOT_FOUND_GIT =
  'The approved content could not be found in Git history; review the full skill.';
const TEXT_CLI_ONLY =
  'Approval is CLI-only. Review the content, then run this command in your terminal. The WebUI and agents cannot approve content.';
const IMPACT_APPROVED = "Agents receive this skill's content.";
const IMPACT_BLOCKED =
  'Agents get no content and no files from this skill until it is approved.';
const NOTE_TRUNCATED = 'Commit history was truncated during diff walk.';
const LABEL_MODIFIED_FILES = 'Modified files: ';
const LABEL_ADDED_FILES = 'Added files: ';
const LABEL_REMOVED_FILES = 'Removed files: ';

export interface ContentTrustCardProps {
  trust: ContentTrust;
}

export function ContentTrustCard({ trust }: ContentTrustCardProps) {
  if (!trust.third_party) {
    return null;
  }

  let badgeLabel = BADGE_REVIEW_REQUIRED;
  let badgeTone: 'success' | 'warning' = 'warning';
  if (trust.approved) {
    badgeLabel = BADGE_APPROVED;
    badgeTone = 'success';
  } else if (trust.changes_since_approval) {
    badgeLabel = BADGE_CHANGED;
    badgeTone = 'warning';
  }

  const changes = trust.changes_since_approval;

  return (
    <section
      className="fg-card"
      style={{
        display: 'flex',
        flexDirection: 'column',
        gap: 'var(--space-3)',
        gridColumn: '1 / -1',
      }}
    >
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          gap: 'var(--space-2)',
        }}
      >
        <div className="fg-card__title">
          <span>{TITLE_CONTENT_TRUST}</span>
        </div>
        <StatusBadge label={badgeLabel} tone={badgeTone} />
      </div>

      {/* Content Digest */}
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          gap: 'var(--space-2)',
          padding: '6px 0',
          borderBottom: '1px solid var(--color-border)',
        }}
      >
        <span className="t-body-sm" style={{ color: 'var(--color-text-muted)' }}>
          <span>{LABEL_DIGEST}</span>
        </span>
        <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-2)' }}>
          <code style={{ fontSize: '12px' }}>{trust.content_digest}</code>
          <CopyButton text={trust.content_digest} />
        </div>
      </div>

      {/* Changes Since Approval */}
      {changes && changes.found && (
        <div
          style={{
            display: 'flex',
            flexDirection: 'column',
            gap: 'var(--space-2)',
            padding: 'var(--space-2) 0',
          }}
        >
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: 'var(--space-2)' }}>
            {changes.scripts_changed && (
              <StatusBadge label="scripts changed" tone="warning" />
            )}
            {changes.runtime_changed && (
              <StatusBadge label="runtime changed" tone="warning" />
            )}
            {changes.dependencies_changed && (
              <StatusBadge label="dependencies changed" tone="warning" />
            )}
          </div>

          {changes.diff_command && <CommandBlock command={changes.diff_command} />}

          {changes.modified && changes.modified.length > 0 && (
            <div className="t-body-sm">
              <span style={{ color: 'var(--color-text-muted)' }}>{LABEL_MODIFIED_FILES}</span>
              <span>{changes.modified.join(', ')}</span>
            </div>
          )}
          {changes.added && changes.added.length > 0 && (
            <div className="t-body-sm">
              <span style={{ color: 'var(--color-text-muted)' }}>{LABEL_ADDED_FILES}</span>
              <span>{changes.added.join(', ')}</span>
            </div>
          )}
          {changes.removed && changes.removed.length > 0 && (
            <div className="t-body-sm">
              <span style={{ color: 'var(--color-text-muted)' }}>{LABEL_REMOVED_FILES}</span>
              <span>{changes.removed.join(', ')}</span>
            </div>
          )}

          {changes.history_truncated && (
            <span className="t-caption" style={{ color: 'var(--color-warning)' }}>
              <span>{NOTE_TRUNCATED}</span>
            </span>
          )}
        </div>
      )}

      {changes && !changes.found && (
        <div className="t-body-sm" style={{ color: 'var(--color-text-muted)' }}>
          <span>{TEXT_NOT_FOUND_GIT}</span>
        </div>
      )}

      {!changes && !trust.approved && (
        <div className="t-body-sm" style={{ color: 'var(--color-text-muted)' }}>
          <span>{TEXT_NEVER_APPROVED}</span>
        </div>
      )}

      {/* Approval Command */}
      {trust.approve_command && (
        <div
          style={{
            display: 'flex',
            flexDirection: 'column',
            gap: 'var(--space-2)',
            padding: 'var(--space-2) 0',
          }}
        >
          <span className="t-body-sm" style={{ color: 'var(--color-text-muted)' }}>
            <span>{TEXT_CLI_ONLY}</span>
          </span>
          <CommandBlock command={trust.approve_command} />
        </div>
      )}

      {/* Agent Impact */}
      <div
        className="t-body-sm"
        style={{
          color: trust.approved ? 'var(--color-success)' : 'var(--color-warning)',
          fontWeight: 500,
        }}
      >
        <span>{trust.approved ? IMPACT_APPROVED : IMPACT_BLOCKED}</span>
      </div>
    </section>
  );
}
