import type { ContentTrust } from '../../api/types';
import { CommandBlock } from '../../components/CommandBlock';
import { CopyButton } from '../../components/CopyButton';
import { StatusBadge } from '../../components/StatusBadge';

const TITLE_CONTENT_TRUST = 'Content trust';
const BADGE_APPROVED = 'Approved';
const BADGE_CHANGED = 'Changed since approval';
const BADGE_REVIEW_REQUIRED = 'Review required';

const LABEL_CONTENT_DIGEST = 'Content digest:';
const LABEL_MODIFIED = 'Modified:';
const LABEL_ADDED = 'Added:';
const LABEL_REMOVED = 'Removed:';
const LABEL_APPROVE_CONTENT = 'Approve content:';
const LABEL_SCRIPTS_CHANGED = 'scripts changed';
const LABEL_RUNTIME_CHANGED = 'runtime changed';
const LABEL_DEPENDENCIES_CHANGED = 'dependencies changed';

const MSG_WHAT_IS_TRUST =
  'Skills from other people can contain instructions that agents will follow. Content trust records that you read this content and approved it.';
const MSG_AGENTS_RECEIVE = "Agents receive this skill's content.";
const MSG_AGENTS_NO_CONTENT = 'Agents get no content and no files from this skill until it is approved.';
const MSG_CLI_ONLY =
  'Approval is CLI-only. Review the content, then run this command in your terminal. The WebUI and agents cannot approve content.';
const MSG_NOT_FOUND_IN_HISTORY =
  'The approved content could not be found in Git history; review the full skill.';
const MSG_NEVER_APPROVED = 'Never approved: review the full skill.';
const MSG_HISTORY_TRUNCATED = 'History was truncated; older commits are not shown.';

function formatItemList(prefix: string, items?: string[]) {
  if (!items || items.length === 0) return '';
  return `${prefix} ${items.join(', ')}`;
}

interface ContentTrustCardProps {
  trust: ContentTrust;
}

export function ContentTrustCard({ trust }: ContentTrustCardProps) {
  const badgeLabel = trust.approved
    ? BADGE_APPROVED
    : trust.changes_since_approval
      ? BADGE_CHANGED
      : BADGE_REVIEW_REQUIRED;

  const badgeTone = trust.approved ? 'success' : 'warning';
  const changes = trust.changes_since_approval;

  return (
    <section className="fg-card" style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' }}>
      <div className="fg-card__title">
        <span>{TITLE_CONTENT_TRUST}</span>
      </div>

      <span className="t-caption" style={{ color: 'var(--color-text-muted)' }}>
        <span>{MSG_WHAT_IS_TRUST}</span>
      </span>

      <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-2)' }}>
        <StatusBadge label={badgeLabel} tone={badgeTone} />
      </div>

      <div style={{ display: 'flex', flexDirection: 'column', gap: '4px' }}>
        <span className="t-caption" style={{ color: 'var(--color-text-muted)' }}>
          <span>{LABEL_CONTENT_DIGEST}</span>
        </span>
        <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 'var(--space-2)' }}>
          <code
            className="app-wrap"
            style={{
              fontFamily: 'var(--font-mono)',
              fontSize: '12px',
              wordBreak: 'break-all',
              backgroundColor: 'var(--color-surface-sunken)',
              padding: '2px 6px',
              borderRadius: '4px',
            }}
          >
            <span>{trust.content_digest}</span>
          </code>
          {trust.content_digest && <CopyButton text={trust.content_digest} />}
        </div>
      </div>

      <p className="t-body-sm" style={{ color: 'var(--color-text-muted)', margin: 0 }}>
        <span>{trust.approved ? MSG_AGENTS_RECEIVE : MSG_AGENTS_NO_CONTENT}</span>
      </p>

      {changes && changes.found && (
        <div
          style={{
            display: 'flex',
            flexDirection: 'column',
            gap: 'var(--space-2)',
            borderTop: '1px solid var(--color-border)',
            paddingTop: 'var(--space-2)',
          }}
        >
          <div style={{ display: 'flex', flexWrap: 'wrap', gap: 'var(--space-2)' }}>
            {changes.scripts_changed && <StatusBadge label={LABEL_SCRIPTS_CHANGED} tone="warning" />}
            {changes.runtime_changed && <StatusBadge label={LABEL_RUNTIME_CHANGED} tone="warning" />}
            {changes.dependencies_changed && <StatusBadge label={LABEL_DEPENDENCIES_CHANGED} tone="warning" />}
          </div>

          {changes.modified && changes.modified.length > 0 && (
            <div className="t-caption" style={{ color: 'var(--color-text-muted)' }}>
              <span>{formatItemList(LABEL_MODIFIED, changes.modified)}</span>
            </div>
          )}
          {changes.added && changes.added.length > 0 && (
            <div className="t-caption" style={{ color: 'var(--color-text-muted)' }}>
              <span>{formatItemList(LABEL_ADDED, changes.added)}</span>
            </div>
          )}
          {changes.removed && changes.removed.length > 0 && (
            <div className="t-caption" style={{ color: 'var(--color-text-muted)' }}>
              <span>{formatItemList(LABEL_REMOVED, changes.removed)}</span>
            </div>
          )}

          {changes.diff_command && (
            <div style={{ marginTop: '4px' }}>
              <CommandBlock command={changes.diff_command} />
            </div>
          )}

          {changes.history_truncated && (
            <span className="t-caption" style={{ color: 'var(--color-text-muted)' }}>
              <span>{MSG_HISTORY_TRUNCATED}</span>
            </span>
          )}
        </div>
      )}

      {changes && !changes.found && (
        <p className="t-body-sm" style={{ color: 'var(--color-warning)', margin: 0 }}>
          <span>{MSG_NOT_FOUND_IN_HISTORY}</span>
        </p>
      )}

      {!changes && !trust.approved && (
        <p className="t-body-sm" style={{ color: 'var(--color-text-muted)', margin: 0 }}>
          <span>{MSG_NEVER_APPROVED}</span>
        </p>
      )}

      {trust.approve_command && (
        <div
          style={{
            display: 'flex',
            flexDirection: 'column',
            gap: 'var(--space-2)',
            borderTop: '1px solid var(--color-border)',
            paddingTop: 'var(--space-2)',
          }}
        >
          <span className="t-caption" style={{ color: 'var(--color-text-muted)' }}>
            <span>{LABEL_APPROVE_CONTENT}</span>
          </span>
          <CommandBlock command={trust.approve_command} />
          <span className="t-caption" style={{ color: 'var(--color-text-muted)' }}>
            <span>{MSG_CLI_ONLY}</span>
          </span>
        </div>
      )}
    </section>
  );
}
