import { useT } from '../i18n';

export function LaterPhasePage() {
  const t = useT();
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
      }}
    >
      <div className="t-heading-sm">
        <span>{t('common.later_phase')}</span>
      </div>
    </div>
  );
}
