import { Link } from 'react-router';
import { useHome } from '../../api/queries';
import { CopyButton } from '../../components/CopyButton';
import { Skeleton } from '../../components/Skeleton';
import { StatusBadge } from '../../components/StatusBadge';
import { resolveActionCta } from './action-cta';
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

  const formatCount = (count: number) => {
    return isHealthy ? String(count) : t('workspace.unavailable');
  };

  const getAvailabilityLabel = (availability: string) => {
    switch (availability) {
      case 'available':
        return t('availability.available');
      case 'not_configured':
        return t('availability.not_configured');
      default:
        return t('availability.unavailable');
    }
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-5)' }}>
      <section
        className="fg-card fg-card--rule"
        aria-labelledby="na-h"
        style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' }}
      >
        <span id="na-h" className="t-label" style={{ color: 'var(--color-text-subtle)' }}>
          <span>{t('home.next_action_title')}</span>
        </span>

        {topAction ? (
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
              <span>{topAction.summary}</span>
            </span>

            {cta.type === 'command' && (
              <div
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: '8px',
                  border: '1px solid var(--color-border)',
                  background: 'var(--color-surface-sunken)',
                  borderRadius: 'var(--input-radius)',
                  padding: '4px 4px 4px 12px',
                }}
              >
                <code style={{ fontFamily: 'var(--font-mono)', fontSize: '13px' }}>
                  {cta.command}
                </code>
                <CopyButton text={cta.command} label={t('action.copy_command')} />
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
                <span>{formatCount(cat.count)}</span>
              </span>
            </div>
          ))}
        </section>
      </div>

      <section
        className="fg-card"
        style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' }}
      >
        <div className="fg-card__title">
          <span>{t('home.action_categories')}</span>
        </div>
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 'var(--space-2)' }}>
          {(home.categories ?? []).map((cat) => (
            <span
              key={cat.kind}
              className={`fg-chip ${cat.count > 0 && isHealthy ? 'fg-chip--info' : 'fg-chip--neutral'}`}
              title={getAvailabilityLabel(cat.availability)}
              style={{ display: 'inline-flex', alignItems: 'center', gap: '6px' }}
            >
              <span>{cat.kind.replace(/_/g, ' ')}</span>
              <span style={{ fontFamily: 'var(--font-mono)' }}>
                <span>{formatCount(cat.count)}</span>
              </span>
            </span>
          ))}
        </div>
      </section>
    </div>
  );
}
