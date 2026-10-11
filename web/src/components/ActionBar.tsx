import type { ReactNode } from 'react';

interface ActionBarProps {
  /** Optional status text shown on the left. */
  status?: ReactNode;
  /** The action buttons, aligned to the right. */
  children: ReactNode;
}

/**
 * The status and primary-action bar of a page. It sits at the top of the
 * page content in the normal flow, so it never covers the fields below it.
 */
export function ActionBar({ status, children }: ActionBarProps) {
  return (
    <div
      style={{
        display: 'flex',
        flexWrap: 'wrap',
        alignItems: 'center',
        justifyContent: 'space-between',
        gap: 'var(--space-3)',
        padding: 'var(--space-3) var(--space-4)',
        background: 'var(--color-surface)',
        border: '1px solid var(--color-border)',
        borderRadius: 'var(--card-radius)',
      }}
    >
      {status !== undefined && (
        <span className="t-caption" style={{ color: 'var(--color-text-subtle)' }}>
          {status}
        </span>
      )}
      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 'var(--space-2)', marginLeft: 'auto' }}>{children}</div>
    </div>
  );
}
