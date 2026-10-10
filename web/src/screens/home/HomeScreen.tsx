import { Link } from 'react-router';
import { useHome } from '../../api/queries';
import { Skeleton } from '../../components/Skeleton';
import { StatusBadge } from '../../components/StatusBadge';
import { actionHelp, resolveActionCta } from './action-cta';
import { CommandBlock } from '../../components/CommandBlock';
import { useT } from '../../i18n';

export function HomeScreen() {
  const t = useT();
  const { data: home, isLoading, error } = useHome();

  if (isLoading) {
    return (
      <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
        <div className="fg-card" style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
          <Skeleton width="120px" />
          <Skeleton width="60%" />
        </div>
        <div
          style={{
            display: 'grid',
            gridTemplateColumns: 'repeat(auto-fit, minmax(280px, 1fr))',
            gap: 'var(--space-4)',
          }}
        >
          <div className="fg-card" style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
            <Skeleton width="40%" />
            <Skeleton />
            <Skeleton />
            <Skeleton />
          </div>
          <div className="fg-card" style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
            <Skeleton width="40%" />
            <Skeleton />
            <Skeleton />
            <Skeleton />
          </div>
        </div>
      </div>
    );
  }

  if (error || !home) {
    return (
      <div className="fg-card fg-card--danger">
        <span className="t-body">
          <span>{t('common.error')}</span>
        </span>
      </div>
    );
  }

  const workspace = home.workspace;
  const isHealthy = workspace?.health === 'valid' && workspace?.index === 'current';
  const actions = home.actions ?? [];
  const topAction = actions[0];
  const cta = resolveActionCta(topAction);
  const help = actionHelp(topAction?.kind);

  const formatCount = (count: number, availability?: string) => {
    if (availability === 'not_configured') return t('home.not_configured');
    return isHealthy ? String(count) : t('workspace.unavailable');
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-5)' }}>
      <section
        className="fg-card fg-card--rule"
        aria-labelledby="na-h"
        style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' }}
      >
        <span id="na-h" className="t-label" style={{ color: 'var(--color-text-muted)' }}>
          <span>{t('home.next_action_title')}</span>
        </span>

        {topAction ? (
          <>
          <div
            style={{
              display: 'flex',
              flexWrap: 'wrap',
              alignItems: 'center',
              justifyContent: 'space-between',
              gap: 'var(--space-3) var(--space-4)',
            }}
          >
            <span className="t-subheading" style={{ flex: '1 1 280px' }}>
              <span>{help?.title ?? topAction.summary}</span>
            </span>

            {cta.type === 'command' && (
              <div style={{ flex: '1 1 320px', maxWidth: '520px' }}>
                <CommandBlock command={cta.command} />
              </div>
            )}

            {cta.type === 'link' && (
              <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                <Link
                  to={cta.to}
                  className="fg-btn fg-btn--primary"
                  style={{ textDecoration: 'none' }}
                >
                  <span>{cta.label}</span>
                </Link>
                {cta.note && (
                  <span className="t-caption" style={{ color: 'var(--color-text-subtle)' }}>
                    <span>{cta.note}</span>
                  </span>
                )}
              </div>
            )}
          </div>
          {help && (
            <span className="t-body-sm" style={{ color: 'var(--color-text-muted)' }}>
              <span>{help.why}</span>
            </span>
          )}
          </>
        ) : (
          <div className="t-body" style={{ color: 'var(--color-text-muted)' }}>
            <span>{t('home.nothing_needs_attention')}</span>
          </div>
        )}
      </section>

      <div
        style={{
          display: 'grid',
          gridTemplateColumns: 'repeat(auto-fit, minmax(min(100%, 300px), 1fr))',
          gap: 'var(--space-4)',
          alignItems: 'start',
        }}
      >
        <section
          className="fg-card"
          style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' }}
        >
          <div className="fg-card__title">
            <span>{t('workspace.title')}</span>
          </div>

          <div
            style={{
              display: 'flex',
              justifyContent: 'space-between',
              alignItems: 'center',
              padding: '6px 0',
              borderBottom: '1px solid var(--color-border)',
            }}
          >
            <span className="t-body-sm" style={{ color: 'var(--color-text-muted)' }}>
              <span>{t('workspace.row_health')}</span>
            </span>
            <StatusBadge
              label={
                workspace?.health === 'valid'
                  ? t('workspace.status_valid')
                  : t('workspace.status_invalid')
              }
              tone={workspace?.health === 'valid' ? 'success' : 'danger'}
            />
          </div>

          <div
            style={{
              display: 'flex',
              justifyContent: 'space-between',
              alignItems: 'center',
              padding: '6px 0',
              borderBottom: '1px solid var(--color-border)',
            }}
          >
            <span className="t-body-sm" style={{ color: 'var(--color-text-muted)' }}>
              <span>{t('workspace.row_index')}</span>
            </span>
            <StatusBadge
              label={
                workspace?.index === 'current'
                  ? t('workspace.index_current')
                  : t('workspace.index_stale')
              }
              tone={workspace?.index === 'current' ? 'success' : 'warning'}
            />
          </div>

          <div
            style={{
              display: 'flex',
              justifyContent: 'space-between',
              alignItems: 'center',
              padding: '6px 0',
            }}
          >
            <span className="t-body-sm" style={{ color: 'var(--color-text-muted)' }}>
              <span>{t('workspace.row_git')}</span>
            </span>
            <StatusBadge
              label={
                workspace?.git_dirty ? t('workspace.git_dirty') : t('workspace.git_clean')
              }
              tone={workspace?.git_dirty ? 'warning' : 'success'}
            />
          </div>
        </section>

        <section
          className="fg-card"
          style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-2)' }}
        >
          <div className="fg-card__title">
            <span>{t('home.summary_title')}</span>
          </div>

          {(home.categories ?? []).map((cat) => (
            <div
              key={cat.kind}
              style={{
                display: 'flex',
                justifyContent: 'space-between',
                alignItems: 'baseline',
                gap: 'var(--space-3)',
                padding: '5px 0',
                borderBottom: '1px solid var(--color-border)',
              }}
            >
              <span className="t-body-sm" style={{ color: 'var(--color-text-muted)' }}>
                <span>{cat.kind.replace(/_/g, ' ')}</span>
              </span>
              <span
                className="t-ui"
                style={{
                  fontFamily: 'var(--font-mono)',
                  fontVariantNumeric: 'tabular-nums',
                }}
              >
                <span>{formatCount(cat.count, cat.availability)}</span>
              </span>
            </div>
          ))}
        </section>
      </div>
    </div>
  );
}
