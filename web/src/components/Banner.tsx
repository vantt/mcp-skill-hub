import type { ReactNode } from 'react';

interface BannerProps {
  children: ReactNode;
  variant?: 'danger' | 'warning' | 'info' | 'success';
  actions?: ReactNode;
  role?: 'alert' | 'status';
}

export function Banner({ children, variant = 'info', actions, role = 'alert' }: BannerProps) {
  return (
    <div
      className={`fg-banner fg-banner--${variant}`}
      role={role}
      style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', flexWrap: 'wrap', gap: 'var(--space-3)' }}
    >
      <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-2)' }}>
        <span className="fg-banner__dot" />
        <span className="fg-banner__body">{children}</span>
      </div>
      {actions && <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-2)' }}>{actions}</div>}
    </div>
  );
}
