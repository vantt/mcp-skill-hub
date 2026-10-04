import { Link, useLocation } from 'react-router';
import { useHome } from '../api/queries';
import { useT } from '../i18n';

interface NavItem {
  id: string;
  path: string;
  labelKey: string;
  icon: string;
  showBadge?: boolean;
}

export function NavRail() {
  const t = useT();
  const location = useLocation();
  const { data: home } = useHome();

  const isHealthy = home?.workspace?.health === 'valid' && home?.workspace?.index === 'current';
  const pendingCount = isHealthy
    ? (home?.categories?.find((c) => c.kind === 'pending_insights')?.count ?? 0)
    : 0;

  const navItems: NavItem[] = [
    { id: 'home', path: '/', labelKey: 'nav.home', icon: '⌂' },
    { id: 'skills', path: '/skills', labelKey: 'nav.skills', icon: '◆' },
    { id: 'sources', path: '/sources', labelKey: 'nav.sources', icon: '⊞' },
    { id: 'inbox', path: '/inbox', labelKey: 'nav.inbox', icon: '✉', showBadge: true },
  ];

  const currentPath = location.pathname;

  return (
    <aside
      className="fg-shell__rail fg-shell__rail--left"
      style={{
        display: 'flex',
        flexDirection: 'column',
        justifyContent: 'space-between',
        gap: 'var(--space-4)',
        overflowY: 'auto',
        overflowX: 'hidden',
        boxSizing: 'border-box',
        borderRight: '1px solid var(--color-border)',
        background: 'var(--color-surface)',
      }}
    >
      <nav className="fg-nav" aria-label="Primary" style={{ width: '100%', minWidth: 0, boxSizing: 'border-box' }}>
        {navItems.map((item) => {
          const isActive =
            item.path === '/'
              ? currentPath === '/'
              : currentPath === item.path || currentPath.startsWith(`${item.path}/`);
          const label = t(item.labelKey);

          return (
            <Link
              key={item.id}
              to={item.path}
              className={`fg-nav__item ${isActive ? 'fg-nav__item--on' : ''}`}
              title={label}
              aria-label={label}
              aria-current={isActive ? 'page' : undefined}
              style={{
                textDecoration: 'none',
              }}
            >
              <span className="fg-nav__icon" aria-hidden="true" style={{ fontSize: '16px', width: '20px', textAlign: 'center' }}>
                {item.icon}
              </span>
              <span className="fg-nav__label" style={{ flex: 1, whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>
                <span>{label}</span>
              </span>
              {item.showBadge && pendingCount > 0 && (
                <span
                  className="fg-nav__badge"
                  style={{
                    padding: '2px 6px',
                    borderRadius: '999px',
                    fontSize: '11px',
                    fontWeight: 600,
                    background: 'var(--color-action)',
                    color: 'var(--color-on-action)',
                  }}
                >
                  <span>{String(pendingCount)}</span>
                </span>
              )}
            </Link>
          );
        })}
      </nav>

      {home?.workspace && (
        <div
          style={{
            borderTop: '1px solid var(--color-border)',
            padding: 'var(--space-3)',
            display: 'flex',
            flexDirection: 'column',
            gap: 'var(--space-2)',
          }}
        >
          <span className="t-label" style={{ color: 'var(--color-text-muted)' }}>
            <span>{t('workspace.title')}</span>
          </span>
          <div style={{ display: 'flex', flexDirection: 'column', gap: '4px' }}>
            <span
              className={`fg-status fg-status--${home.workspace.health === 'valid' ? 'success' : 'danger'}`}
              style={{ fontSize: '12px' }}
            >
              <span className="fg-status__dot" />
              <span>
                {home.workspace.health === 'valid'
                  ? t('workspace.status_valid')
                  : t('workspace.status_invalid')}
              </span>
            </span>
            <span
              className={`fg-status fg-status--${home.workspace.index === 'current' ? 'success' : 'warning'}`}
              style={{ fontSize: '12px' }}
            >
              <span className="fg-status__dot" />
              <span>
                {home.workspace.index === 'current'
                  ? t('workspace.index_current')
                  : t('workspace.index_stale')}
              </span>
            </span>
          </div>
        </div>
      )}
    </aside>
  );
}
