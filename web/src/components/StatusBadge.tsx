interface StatusBadgeProps {
  label: string;
  variant?: 'status' | 'chip';
  tone?: 'success' | 'warning' | 'danger' | 'info' | 'neutral';
  icon?: string;
}

export function StatusBadge({
  label,
  variant = 'status',
  tone = 'neutral',
  icon,
}: StatusBadgeProps) {
  if (variant === 'chip') {
    return (
      <span className={`fg-chip fg-chip--${tone}`} style={{ display: 'inline-flex', alignItems: 'center', gap: '4px' }}>
        {icon && <span aria-hidden="true">{icon}</span>}
        <span>{label}</span>
      </span>
    );
  }

  return (
    <span className={`fg-status fg-status--${tone}`}>
      <span className="fg-status__dot" />
      <span>{label}</span>
    </span>
  );
}
