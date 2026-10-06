import { useEffect, useRef, useState, type ReactNode } from 'react';
import { Link, useLocation } from 'react-router';
import { AppearanceMenu } from './AppearanceMenu';
import { Banner } from './Banner';
import { NavRail } from './NavRail';
import { SessionExpired } from './SessionExpired';
import { onSessionExpired } from '../state/session';
import { useHome } from '../api/queries';
import { useT } from '../i18n';

interface AppShellProps {
  children: ReactNode;
}

export function AppShell({ children }: AppShellProps) {
  const t = useT();
  const location = useLocation();
  const [expired, setExpired] = useState(false);
  const headingRef = useRef<HTMLHeadingElement>(null);
  const { data: home, refetch: refetchHome } = useHome();

  useEffect(() => {
    return onSessionExpired(() => {
      setExpired(true);
    });
  }, []);

  const path = location.pathname;

  let pageTitle = t('nav.home');
  let parentLabel: string | null = null;
  let parentPath: string | null = null;

  if (path === '/skills') {
    pageTitle = t('nav.skills');
  } else if (path === '/skills/add') {
    pageTitle = t('action.add_from_github');
    parentLabel = t('nav.skills');
    parentPath = '/skills';
  } else if (path === '/skills/create') {
    pageTitle = t('action.create_skill');
    parentLabel = t('nav.skills');
    parentPath = '/skills';
  } else if (path.startsWith('/skills/')) {
    const id = path.split('/')[2] ?? '';
    pageTitle = id;
    parentLabel = t('nav.skills');
    parentPath = '/skills';
  } else if (path === '/sources') {
    pageTitle = t('nav.sources');
  } else if (path === '/sources/distill') {
    pageTitle = 'Distill with Curator Agent';
    parentLabel = t('nav.sources');
    parentPath = '/sources';
  } else if (path.startsWith('/sources/runs/')) {
    pageTitle = 'Run';
    parentLabel = t('nav.sources');
    parentPath = '/sources';
  } else if (path === '/inbox') {
    pageTitle = t('nav.inbox');
  } else if (path.startsWith('/inbox/')) {
    if (path.endsWith('/apply')) {
      const insightId = path.split('/')[2] ?? '';
      pageTitle = 'Compose patch';
      parentLabel = 'Insight';
      parentPath = `/inbox/${encodeURIComponent(insightId)}`;
    } else {
      pageTitle = 'Insight';
      parentLabel = t('nav.inbox');
      parentPath = '/inbox';
    }
  }

  useEffect(() => {
    document.title = `${pageTitle} · Skill Hub`;
    headingRef.current?.focus();
  }, [pageTitle, path]);

  const isDegraded = home?.workspace?.index === 'stale';

  const handleCopyRebuild = async () => {
    try {
      await navigator.clipboard.writeText('skillhub rebuild');
    } catch {
      // Ignore clipboard write error
    }
  };

  return (
    <div style={{ height: '100vh', display: 'flex', flexDirection: 'column' }}>
      <a
        href="#main"
        style={{
          position: 'absolute',
          left: '-9999px',
          top: '8px',
          zIndex: 100,
          padding: '8px 12px',
          background: 'var(--color-surface)',
          color: 'var(--color-action)',
        }}
      >
        <span>{t('nav.skip_to_main')}</span>
      </a>

      <header
        className="fg-shell__header"
        style={{
          height: '56px',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          padding: '0 var(--space-4)',
          borderBottom: '1px solid var(--color-border)',
          background: 'var(--color-surface)',
          zIndex: 20,
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-3)', minWidth: 0, flex: 1 }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: '8px', width: '200px' }}>
            <span style={{ color: 'var(--color-action)', fontSize: '18px' }}>◆</span>
            <span className="t-heading-sm" style={{ fontWeight: 600 }}>
              <span>{t('app.title')}</span>
            </span>
          </div>

          <div style={{ display: 'flex', flexDirection: 'column', minWidth: 0, flex: 1 }}>
            {parentLabel && parentPath && (
              <div className="fg-breadcrumb" style={{ fontSize: '12px', display: 'flex', alignItems: 'center', gap: '4px' }}>
                <Link to={parentPath} style={{ color: 'var(--color-text-subtle)', textDecoration: 'none' }}>
                  <span>{parentLabel}</span>
                </Link>
                <span className="fg-breadcrumb__sep" style={{ color: 'var(--color-text-subtle)' }}>
                  /
                </span>
                <span className="fg-breadcrumb__here" style={{ color: 'var(--color-text)' }}>
                  <span>{pageTitle}</span>
                </span>
              </div>
            )}
            <h1
              ref={headingRef}
              tabIndex={-1}
              className="t-heading"
              style={{
                margin: 0,
                fontSize: '18px',
                fontWeight: 600,
                outline: 'none',
                whiteSpace: 'nowrap',
                overflow: 'hidden',
                textOverflow: 'ellipsis',
              }}
            >
              <span>{pageTitle}</span>
            </h1>
          </div>
        </div>

        <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-2)' }}>
          <AppearanceMenu />
        </div>
      </header>

      <div style={{ display: 'flex', flex: 1, minHeight: 0 }}>
        <NavRail />

        <main
          id="main"
          tabIndex={-1}
          style={{
            flex: 1,
            overflowY: 'auto',
            padding: 'var(--space-6) var(--space-6) var(--space-10)',
            display: 'flex',
            flexDirection: 'column',
            gap: 'var(--space-5)',
            outline: 'none',
            background: 'var(--color-surface)',
          }}
        >
          {isDegraded && (
            <Banner
              variant="danger"
              actions={
                <>
                  <button
                    type="button"
                    className="fg-btn fg-btn--secondary"
                    onClick={handleCopyRebuild}
                  >
                    <span>{t('action.copy_command')}</span>
                  </button>
                  <button
                    type="button"
                    className="fg-btn fg-btn--ghost"
                    onClick={() => refetchHome()}
                  >
                    <span>{t('action.reload')}</span>
                  </button>
                </>
              }
            >
              <strong>
                <span>{t('home.degraded_title')}</span>
              </strong>{' '}
              <span>{t('home.degraded_body')}</span>
            </Banner>
          )}

          {expired ? <SessionExpired /> : children}
        </main>
      </div>
    </div>
  );
}
