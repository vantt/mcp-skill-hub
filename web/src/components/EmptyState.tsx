import type { ReactNode } from 'react';

interface EmptyStateProps {
  title: string;
  description?: string;
  action?: ReactNode;
}

export function EmptyState({ title, description, action }: EmptyStateProps) {
  return (
    <div
      style={{
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'center',
        justifyContent: 'center',
        padding: 'var(--space-8) var(--space-4)',
        textAlign: 'center',
        gap: 'var(--space-3)',
        border: '1px dashed var(--color-border)',
        borderRadius: 'var(--card-radius)',
        background: 'var(--color-surface-sunken)',
      }}
    >
      <div className="t-heading-sm" style={{ margin: 0 }}>
        <span>{title}</span>
      </div>
      {description && (
        <div className="t-ui" style={{ color: 'var(--color-text-subtle)', maxWidth: '480px' }}>
          <span>{description}</span>
        </div>
      )}
      {action && <div style={{ marginTop: 'var(--space-2)' }}>{action}</div>}
    </div>
  );
}
