import { CommandBlock } from './CommandBlock';
import { useT } from '../i18n';

export function SessionExpired() {
  const t = useT();

  return (
    <div
      style={{
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'center',
        justifyContent: 'center',
        minHeight: '60vh',
        padding: 'var(--space-6) var(--space-4)',
        textAlign: 'center',
        gap: 'var(--space-4)',
      }}
    >
      <div
        className="fg-card"
        style={{
          maxWidth: '520px',
          width: '100%',
          display: 'flex',
          flexDirection: 'column',
          gap: 'var(--space-4)',
          textAlign: 'left',
        }}
      >
        <div className="t-heading-sm">
          <span>{t('common.session_expired_title')}</span>
        </div>
        <div className="t-ui" style={{ color: 'var(--color-text-subtle)' }}>
          <span>{t('common.session_expired_desc')}</span>
        </div>
        <CommandBlock command="skillhub serve web" />
      </div>
    </div>
  );
}
