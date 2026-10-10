import { Link } from 'react-router';
import { useT } from '../i18n';

export function NotFoundPage() {
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
        <span>{t('common.not_found')}</span>
      </div>
      <div className="t-body" style={{ color: 'var(--color-text-muted)', maxWidth: '56ch' }}>
        <span>{t('common.not_found_hint')}</span>
      </div>
      <div style={{ display: 'flex', gap: 'var(--space-2)', flexWrap: 'wrap', justifyContent: 'center' }}>
        <Link to="/skills" className="fg-btn fg-btn--primary" style={{ textDecoration: 'none' }}>
          <span>{t('nav.skills')}</span>
        </Link>
        <Link to="/sources" className="fg-btn fg-btn--secondary" style={{ textDecoration: 'none' }}>
          <span>{t('nav.sources')}</span>
        </Link>
        <Link to="/" className="fg-btn fg-btn--secondary" style={{ textDecoration: 'none' }}>
          <span>{t('nav.home')}</span>
        </Link>
      </div>
    </div>
  );
}
