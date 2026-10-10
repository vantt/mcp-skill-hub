import { useState } from 'react';
import { Link, useNavigate, useSearchParams } from 'react-router';
import { useSkills } from '../../api/queries';
import type { SkillListItem } from '../../api/types';
import { Skeleton } from '../../components/Skeleton';
import { StatusBadge } from '../../components/StatusBadge';
import { useT } from '../../i18n';

const ICON_SEARCH = '🔍';
const ICON_MORE = '⋯';
const OPT_UPSTREAM_ALL = 'All';
const OPT_UPSTREAM_UPDATES = 'Updates';
const OPT_UPSTREAM_MODIFIED = 'Modified locally';
const OPT_UPSTREAM_UNTRACKED = 'Not tracked';

export function SkillsScreen() {
  const t = useT();
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const [activeMenuId, setActiveMenuId] = useState<string | null>(null);

  const queryParam = searchParams.get('q') ?? '';
  const stateParam = searchParams.get('state') ?? 'all';
  const colParam = searchParams.get('collection') ?? 'all';
  const upstreamParam = searchParams.get('upstream') ?? 'all';

  const { data, isLoading, error } = useSkills(stateParam === 'all' ? undefined : stateParam);

  const allSkills = data?.skills ?? [];
  const updateCount = allSkills.filter(
    (s) => s.upstream_status === 'update_available' || s.upstream_status === 'diverged',
  ).length;
  const updatesBadgeText = `Updates (${updateCount})`;

  // Build collection options dynamically
  const collections = Array.from(new Set(allSkills.map((s) => s.collection).filter(Boolean))).sort();

  // Filter skills client-side by q and collection
  const filteredSkills = allSkills.filter((s) => {
    if (colParam !== 'all' && s.collection !== colParam) {
      return false;
    }
    if (upstreamParam === 'updates') {
      if (s.upstream_status !== 'update_available' && s.upstream_status !== 'diverged') {
        return false;
      }
    } else if (upstreamParam === 'modified') {
      if (s.upstream_status !== 'modified' && s.upstream_status !== 'diverged') {
        return false;
      }
    } else if (upstreamParam === 'untracked') {
      if (s.upstream_status !== 'untracked') {
        return false;
      }
    }
    if (queryParam) {
      const q = queryParam.toLowerCase();
      const matchId = s.id.toLowerCase().includes(q);
      const matchName = s.name.toLowerCase().includes(q);
      if (!matchId && !matchName) {
        return false;
      }
    }
    return true;
  });

  const updateSearch = (nextQ: string) => {
    const next = new URLSearchParams(searchParams);
    if (nextQ) {
      next.set('q', nextQ);
    } else {
      next.delete('q');
    }
    setSearchParams(next);
  };

  const updateStateFilter = (nextState: string) => {
    const next = new URLSearchParams(searchParams);
    if (nextState !== 'all') {
      next.set('state', nextState);
    } else {
      next.delete('state');
    }
    setSearchParams(next);
  };

  const updateColFilter = (nextCol: string) => {
    const next = new URLSearchParams(searchParams);
    if (nextCol !== 'all') {
      next.set('collection', nextCol);
    } else {
      next.delete('collection');
    }
    setSearchParams(next);
  };

  const updateUpstreamFilter = (nextUpstream: string) => {
    const next = new URLSearchParams(searchParams);
    if (nextUpstream !== 'all') {
      next.set('upstream', nextUpstream);
    } else {
      next.delete('upstream');
    }
    setSearchParams(next);
  };

  const clearFilters = () => {
    setSearchParams(new URLSearchParams());
  };

  const isFallback = Boolean(data?.summary && data.summary.toLowerCase().includes('fallback'));

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
      {/* Controls Bar */}
      <div
        style={{
          display: 'flex',
          flexWrap: 'wrap',
          gap: 'var(--space-3)',
          alignItems: 'center',
          justifyContent: 'space-between',
        }}
      >
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 'var(--space-3)', alignItems: 'flex-end', flex: 1 }}>
          <div className="app-field app-search" style={{ flex: '1 1 220px', maxWidth: '340px' }}>
            <label htmlFor="skills-search">{t('skills.search_label')}</label>
            <div className="fg-search">
              <span className="fg-search__glyph" aria-hidden="true">
                {ICON_SEARCH}
              </span>
              <input
                id="skills-search"
                type="search"
                className="fg-input"
                placeholder={t('skills.search_placeholder')}
                value={queryParam}
                onChange={(e) => updateSearch(e.target.value)}
              />
            </div>
          </div>

          <div className="app-field" style={{ width: '160px' }}>
            <label htmlFor="skills-filter-state">{t('skills.filter_lifecycle')}</label>
            <div className="fg-select">
            <select
              id="skills-filter-state"
              value={stateParam}
              onChange={(e) => updateStateFilter(e.target.value)}
            >
              <option value="all">{t('lifecycle.all')}</option>
              <option value="draft">{t('lifecycle_short.draft')}</option>
              <option value="active">{t('lifecycle_short.active')}</option>
              <option value="deprecated">{t('lifecycle_short.deprecated')}</option>
              <option value="archived">{t('lifecycle_short.archived')}</option>
            </select>
            <span className="fg-select__chev" aria-hidden="true">
              ▾
            </span>
            </div>
          </div>

          <div className="app-field" style={{ width: '170px' }}>
            <label htmlFor="skills-filter-collection">{t('skills.filter_collection')}</label>
            <div className="fg-select">
            <select
              id="skills-filter-collection"
              value={colParam}
              onChange={(e) => updateColFilter(e.target.value)}
            >
              <option value="all">{t('lifecycle.all')}</option>
              {collections.map((col) => (
                <option key={col} value={col}>
                  {col}
                </option>
              ))}
            </select>
            <span className="fg-select__chev" aria-hidden="true">
              ▾
            </span>
            </div>
          </div>

          <div className="app-field" style={{ width: '160px' }}>
            <label htmlFor="skills-filter-upstream">{t('skills.filter_upstream')}</label>
            <div className="fg-select">
            <select
              id="skills-filter-upstream"
              value={upstreamParam}
              onChange={(e) => updateUpstreamFilter(e.target.value)}
            >
              <option value="all">{OPT_UPSTREAM_ALL}</option>
              <option value="updates">{OPT_UPSTREAM_UPDATES}</option>
              <option value="modified">{OPT_UPSTREAM_MODIFIED}</option>
              <option value="untracked">{OPT_UPSTREAM_UNTRACKED}</option>
            </select>
            <span className="fg-select__chev" aria-hidden="true">
              ▾
            </span>
            </div>
          </div>

          {updateCount > 0 && (
            <button
              type="button"
              className="fg-chip fg-chip--warning"
              style={{ cursor: 'pointer', border: 'none' }}
              onClick={() => updateUpstreamFilter('updates')}
            >
              <span>{updatesBadgeText}</span>
            </button>
          )}
        </div>

        <div className="app-actions" style={{ display: 'flex', flexWrap: 'wrap', gap: 'var(--space-2)' }}>
          <Link to="/skills/add" className="fg-btn fg-btn--secondary" style={{ textDecoration: 'none' }}>
            <span>{t('action.add_from_github')}</span>
          </Link>
          <Link to="/skills/create" className="fg-btn fg-btn--primary" style={{ textDecoration: 'none' }}>
            <span>{t('action.create_skill')}</span>
          </Link>
        </div>
      </div>

      {/* Fallback Banner */}
      {isFallback && data?.summary && (
        <div className="fg-banner fg-banner--info" role="status">
          <span className="fg-banner__dot" />
          <span className="fg-banner__body">
            <span>{data.summary}</span>
          </span>
        </div>
      )}

      {/* Loading Skeleton */}
      {isLoading && (
        <div className="fg-card" style={{ display: 'flex', flexDirection: 'column', gap: '14px' }}>
          <Skeleton />
          <Skeleton />
          <Skeleton />
          <Skeleton />
        </div>
      )}

      {/* Error View */}
      {error && !isLoading && (
        <div className="fg-card fg-card--danger">
          <span className="t-body">
            <span>{t('common.error')}</span>
          </span>
        </div>
      )}

      {/* Empty State - No skills in workspace */}
      {!isLoading && !error && allSkills.length === 0 && (
        <div className="fg-card">
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
            <span className="t-body">
              <span>{t('skills.empty_none')}</span>
            </span>
            <Link to="/skills/add" className="fg-btn fg-btn--primary" style={{ textDecoration: 'none' }}>
              <span>{t('action.add_from_github')}</span>
            </Link>
          </div>
        </div>
      )}

      {/* Filtered Empty State */}
      {!isLoading && !error && allSkills.length > 0 && filteredSkills.length === 0 && (
        <div className="fg-card">
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
            <span className="t-body">
              <span>{t('skills.empty_filtered')}</span>
            </span>
            <button type="button" className="fg-btn fg-btn--secondary" onClick={clearFilters}>
              <span>{t('action.clear_filters')}</span>
            </button>
          </div>
        </div>
      )}

      {/* Table / Card List */}
      {!isLoading && !error && filteredSkills.length > 0 && (
        <div className="fg-card" style={{ padding: 0, overflow: 'visible' }}>
          <table className="fg-table fg-table--clickable app-skills" style={{ width: '100%' }}>
            <thead>
              <tr>
                <th>
                  <span>{t('skills.col_skill')}</span>
                </th>
                <th>
                  <span>{t('skills.col_collection')}</span>
                </th>
                <th>
                  <span>{t('skills.col_lifecycle')}</span>
                </th>
                <th>
                  <span>{t('skills.col_routing')}</span>
                </th>
                <th style={{ width: '48px' }}>
                  <span style={{ position: 'absolute', left: '-9999px' }}>
                    <span>{t('skills.col_actions')}</span>
                  </span>
                </th>
              </tr>
            </thead>
            <tbody>
              {filteredSkills.map((s: SkillListItem) => {
                const isMenuOpen = activeMenuId === s.id;
                const canDeprecate = s.lifecycle_state === 'active';
                const canArchive = s.lifecycle_state === 'deprecated';

                const tone =
                  s.lifecycle_state === 'active'
                    ? 'success'
                    : s.lifecycle_state === 'draft'
                      ? 'warning'
                      : s.lifecycle_state === 'deprecated'
                        ? 'danger'
                        : 'neutral';

                return (
                  <tr
                    key={s.id}
                    onClick={() => navigate(`/skills/${s.id}`)}
                    style={{ cursor: 'pointer' }}
                  >
                    <td>
                      <div className="t-ui">
                        <span>{s.name}</span>
                      </div>
                      <div style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', color: 'var(--color-text-muted)' }}>
                        <span>{s.id}</span>
                      </div>
                    </td>
                    <td className="t-body-sm" data-label={t('skills.col_collection')}>
                      <span>{s.collection}</span>
                    </td>
                    <td data-label={t('skills.col_lifecycle')}>
                      <div style={{ display: 'inline-flex', alignItems: 'center', gap: 'var(--space-1)', flexWrap: 'wrap' }}>
                        <StatusBadge
                          variant="chip"
                          label={t(`lifecycle_short.${s.lifecycle_state}`)}
                          tone={tone}
                        />
                        {s.upstream_status === 'update_available' && (
                          <StatusBadge variant="chip" tone="warning" label="Update available" />
                        )}
                        {s.upstream_status === 'diverged' && (
                          <StatusBadge variant="chip" tone="danger" label="Diverged" />
                        )}
                        {s.upstream_status === 'upstream_removed' && (
                          <StatusBadge variant="chip" tone="danger" label="Removed upstream" />
                        )}
                      </div>
                    </td>
                    <td className="t-body-sm" data-label={t('skills.col_routing')}>
                      <span style={{ color: s.routing_eligible ? 'var(--color-text)' : 'var(--color-text-muted)' }}>
                        <span>{s.routing_eligible ? t('routing.routable') : t('routing.not_routed')}</span>
                      </span>
                    </td>
                    <td
                      className="app-skills__actions"
                      onClick={(e) => {
                        e.stopPropagation();
                      }}
                    >
                      <button
                        type="button"
                        className="fg-icon-btn"
                        aria-label={`Actions for ${s.name}`}
                        aria-haspopup="menu"
                        aria-expanded={isMenuOpen}
                        onClick={() => setActiveMenuId(isMenuOpen ? null : s.id)}
                      >
                        {ICON_MORE}
                      </button>

                      {isMenuOpen && (
                        <div
                          className="fg-menu"
                          role="menu"
                          style={{
                            position: 'absolute',
                            right: '8px',
                            top: 'calc(100% - 4px)',
                            zIndex: 40,
                            display: 'flex',
                            flexDirection: 'column',
                            minWidth: '120px',
                            background: 'var(--color-surface-raised)',
                            border: '1px solid var(--color-border)',
                            borderRadius: 'var(--card-radius)',
                            boxShadow: 'var(--shadow-sm)',
                          }}
                        >
                          <Link
                            to={`/skills/${s.id}`}
                            className="fg-menu__item"
                            style={{ textDecoration: 'none', padding: '8px 12px', color: 'var(--color-text)' }}
                            onClick={() => setActiveMenuId(null)}
                          >
                            <span>{t('action.review')}</span>
                          </Link>

                          {canDeprecate && (
                            <Link
                              to={`/skills/${s.id}?tab=review`}
                              className="fg-menu__item"
                              style={{ textDecoration: 'none', padding: '8px 12px', color: 'var(--color-text)' }}
                              onClick={() => setActiveMenuId(null)}
                            >
                              <span>{t('action.deprecate')}</span>
                            </Link>
                          )}

                          {canArchive && (
                            <Link
                              to={`/skills/${s.id}?tab=review`}
                              className="fg-menu__item fg-menu__item--danger"
                              style={{ textDecoration: 'none', padding: '8px 12px', color: 'var(--color-danger)' }}
                              onClick={() => setActiveMenuId(null)}
                            >
                              <span>{t('action.archive')}</span>
                            </Link>
                          )}
                        </div>
                      )}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
