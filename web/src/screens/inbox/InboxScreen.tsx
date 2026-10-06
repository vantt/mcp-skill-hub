import { useState } from 'react';
import { useNavigate, useSearchParams } from 'react-router';
import { useQuery } from '@tanstack/react-query';
import { fetchInboxPage } from '../../api/queries';
import type { InboxPageResult, InsightInboxGroup, InsightInboxItem } from '../../api/types';
import { Skeleton } from '../../components/Skeleton';
import { StatusBadge } from '../../components/StatusBadge';

const TITLE_INBOX = 'Inbox';
const LABEL_STATUS = 'Status';
const LABEL_CATEGORY = 'Category';
const LABEL_PRIORITY = 'Priority';
const LABEL_STATUS_ALL = 'Status: All';
const LABEL_CATEGORY_ALL = 'Category: All';
const LABEL_PRIORITY_ALL = 'Priority: All';
const LABEL_STATUS_PENDING = 'Pending';
const LABEL_STATUS_PLANNED = 'Planned';
const LABEL_STATUS_REJECTED = 'Rejected';
const LABEL_STATUS_OBSOLETE = 'Obsolete';
const LABEL_PRIORITY_CRITICAL = 'Critical';
const LABEL_PRIORITY_HIGH = 'High';
const LABEL_PRIORITY_MEDIUM = 'Medium';
const LABEL_PRIORITY_LOW = 'Low';
const LABEL_FILTERING_PREFIX = 'Filtering ';
const LABEL_FILTERING_SUFFIX = ' loaded groups';
const LABEL_EMPTY_INBOX = 'No pending or planned insights.';
const LABEL_NO_FILTER_MATCH = 'No matches in the loaded groups.';
const BTN_CLEAR_FILTERS = 'Clear filters';
const LABEL_DEGRADED_BODY = 'Data changed since this page loaded. Reloading from the first page.';
const BTN_RELOAD = 'Reload';
const BTN_LOAD_MORE = 'Load more';
const BTN_LOADING_MORE = 'Loading more…';
const LABEL_RETRY = 'Retry';
const LABEL_DOT = ' · ';
const LABEL_OPEN_ARROW = 'Open →';
const LABEL_STALE_EVIDENCE = '⚠ Stale evidence';
const LABEL_SCORE = 'Score ';
const LABEL_IMPACT = ' · Impact ';
const LABEL_FINDINGS = ' findings';
const LABEL_CHEVRON_DOWN = '▾';
const ICON_ENVELOPE = '✉';

export function InboxScreen() {
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();

  const statusFilter = searchParams.get('status') || 'all';
  const categoryFilter = searchParams.get('category') || 'all';
  const priorityFilter = searchParams.get('priority') || 'all';

  const {
    data: initialPage,
    isLoading: loading,
    error: queryError,
    refetch,
  } = useQuery({
    queryKey: ['inbox'],
    queryFn: () => fetchInboxPage(25),
  });

  const [morePages, setMorePages] = useState<InboxPageResult[]>([]);
  const [loadingMore, setLoadingMore] = useState(false);
  const [loadMoreError, setLoadMoreError] = useState<string | null>(null);
  const [snapshotExpired, setSnapshotExpired] = useState(false);

  const pages = initialPage ? [initialPage, ...morePages] : [];

  const handleReload = () => {
    setMorePages([]);
    setSnapshotExpired(false);
    setLoadMoreError(null);
    void refetch();
  };

  const handleLoadMore = async () => {
    const lastPage = pages[pages.length - 1];
    if (!lastPage || !lastPage.has_more || !lastPage.next_cursor || loadingMore) {
      return;
    }
    setLoadingMore(true);
    setLoadMoreError(null);
    try {
      const nextPage = await fetchInboxPage(25, lastPage.next_cursor);
      setMorePages((prev) => [...prev, nextPage]);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : '';
      if (msg.includes('snapshot_expired') || (err as { status?: number }).status === 410) {
        setSnapshotExpired(true);
      } else {
        setLoadMoreError(msg || 'Failed to load more insights');
      }
    } finally {
      setLoadingMore(false);
    }
  };

  const handleStatusChange = (val: string) => {
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev);
      if (val === 'all') next.delete('status');
      else next.set('status', val);
      return next;
    });
  };

  const handleCategoryChange = (val: string) => {
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev);
      if (val === 'all') next.delete('category');
      else next.set('category', val);
      return next;
    });
  };

  const handlePriorityChange = (val: string) => {
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev);
      if (val === 'all') next.delete('priority');
      else next.set('priority', val);
      return next;
    });
  };

  const handleClearFilters = () => {
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev);
      next.delete('status');
      next.delete('category');
      next.delete('priority');
      return next;
    });
  };

  if (loading) {
    return (
      <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
        <h1 style={{ margin: 0, fontSize: '20px', fontWeight: 600 }}>
          <span>{TITLE_INBOX}</span>
        </h1>
        <div className="fg-card" style={{ display: 'flex', flexDirection: 'column', gap: '14px' }}>
          <Skeleton width="30%" height="24px" />
          <Skeleton height="80px" />
          <Skeleton height="80px" />
        </div>
      </div>
    );
  }

  const error =
    loadMoreError ||
    (queryError instanceof Error ? queryError.message : queryError ? 'Failed to load inbox' : null);

  if (error && pages.length === 0) {
    return (
      <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
        <h1 style={{ margin: 0, fontSize: '20px', fontWeight: 600 }}>
          <span>{TITLE_INBOX}</span>
        </h1>
        <div className="fg-banner fg-banner--danger">
          <span>{error}</span>
        </div>
        <button
          type="button"
          className="fg-btn fg-btn--secondary"
          style={{ width: 'fit-content' }}
          onClick={handleReload}
        >
          <span>{LABEL_RETRY}</span>
        </button>
      </div>
    );
  }

  const allGroups = pages.flatMap((p) => p.groups);
  const categories = Array.from(new Set(allGroups.map((g) => g.category))).filter(Boolean);

  const filteredGroups = allGroups
    .map((g) => {
      if (categoryFilter !== 'all' && g.category.toLowerCase() !== categoryFilter.toLowerCase()) {
        return null;
      }
      const filteredItems = g.items.filter((item) => {
        const ins = item.insight;
        if (statusFilter !== 'all' && ins.status.toLowerCase() !== statusFilter.toLowerCase()) {
          return false;
        }
        if (priorityFilter !== 'all' && ins.priority.toLowerCase() !== priorityFilter.toLowerCase()) {
          return false;
        }
        return true;
      });
      if (filteredItems.length === 0) {
        return null;
      }
      return {
        ...g,
        items: filteredItems,
      };
    })
    .filter((g): g is InsightInboxGroup => g !== null);

  const lastPage = pages[pages.length - 1];
  const hasMore = Boolean(lastPage?.has_more);
  const filterCaption = `${LABEL_FILTERING_PREFIX}${filteredGroups.length}${LABEL_FILTERING_SUFFIX}`;
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
      {/* Title & Filters */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: 'var(--space-3)' }}>
        <h1 style={{ margin: 0, fontSize: '20px', fontWeight: 600 }}>
          <span>{TITLE_INBOX}</span>
        </h1>
      </div>

      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 'var(--space-2)', alignItems: 'center' }}>
        {/* Status Filter */}
        <div className="fg-select" style={{ width: '150px' }}>
          <select
            aria-label={LABEL_STATUS}
            value={statusFilter}
            onChange={(e) => handleStatusChange(e.target.value)}
          >
            <option value="all">{LABEL_STATUS_ALL}</option>
            <option value="pending">{LABEL_STATUS_PENDING}</option>
            <option value="planned">{LABEL_STATUS_PLANNED}</option>
            <option value="rejected">{LABEL_STATUS_REJECTED}</option>
            <option value="obsolete">{LABEL_STATUS_OBSOLETE}</option>
          </select>
          <span className="fg-select__chev">{LABEL_CHEVRON_DOWN}</span>
        </div>

        {/* Category Filter */}
        <div className="fg-select" style={{ width: '170px' }}>
          <select
            aria-label={LABEL_CATEGORY}
            value={categoryFilter}
            onChange={(e) => handleCategoryChange(e.target.value)}
          >
            <option value="all">{LABEL_CATEGORY_ALL}</option>
            {categories.map((c) => (
              <option key={c} value={c}>{c}</option>
            ))}
          </select>
          <span className="fg-select__chev">{LABEL_CHEVRON_DOWN}</span>
        </div>

        {/* Priority Filter */}
        <div className="fg-select" style={{ width: '150px' }}>
          <select
            aria-label={LABEL_PRIORITY}
            value={priorityFilter}
            onChange={(e) => handlePriorityChange(e.target.value)}
          >
            <option value="all">{LABEL_PRIORITY_ALL}</option>
            <option value="critical">{LABEL_PRIORITY_CRITICAL}</option>
            <option value="high">{LABEL_PRIORITY_HIGH}</option>
            <option value="medium">{LABEL_PRIORITY_MEDIUM}</option>
            <option value="low">{LABEL_PRIORITY_LOW}</option>
          </select>
          <span className="fg-select__chev">{LABEL_CHEVRON_DOWN}</span>
        </div>

        <span className="t-caption" style={{ color: 'var(--color-text-muted)' }}>
          <span>{filterCaption}</span>
        </span>
      </div>

      {snapshotExpired && (
        <div className="fg-banner fg-banner--info" role="status">
          <span className="fg-banner__dot" />
          <span className="fg-banner__body">{LABEL_DEGRADED_BODY}</span>
          <button
            type="button"
            className="fg-btn fg-btn--ghost fg-btn--small"
            onClick={handleReload}
          >
            <span>{BTN_RELOAD}</span>
          </button>
        </div>
      )}

      {allGroups.length === 0 ? (
        <div className="fg-card">
          <div className="fg-empty" style={{ padding: 'var(--space-6)', display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 'var(--space-2)' }}>
            <span style={{ fontSize: '24px' }}>{ICON_ENVELOPE}</span>
            <span className="t-body" style={{ color: 'var(--color-text-muted)' }}>{LABEL_EMPTY_INBOX}</span>
          </div>
        </div>
      ) : filteredGroups.length === 0 ? (
        <div className="fg-card">
          <div className="fg-empty" style={{ padding: 'var(--space-6)', display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 'var(--space-3)' }}>
            <span className="t-body" style={{ color: 'var(--color-text-muted)' }}>{LABEL_NO_FILTER_MATCH}</span>
            <button
              type="button"
              className="fg-btn fg-btn--secondary"
              onClick={handleClearFilters}
            >
              <span>{BTN_CLEAR_FILTERS}</span>
            </button>
          </div>
        </div>
      ) : (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
          {filteredGroups.map((g) => (
            <section key={`${g.skill_id}-${g.category}`} style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-2)' }}>
              <div
                className="fg-group-head"
                style={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: 'var(--space-2)',
                  fontSize: '13px',
                  fontWeight: 600,
                  color: 'var(--color-text-muted)',
                  padding: 'var(--space-1) 0',
                }}
              >
                <span style={{ fontFamily: 'var(--font-mono)' }}>{g.skill_id}</span>
                <span>{LABEL_DOT}</span>
                <span>{g.category}</span>
                <span className="fg-chip fg-chip--neutral" style={{ fontSize: '11px', padding: '1px 6px' }}>
                  {g.items.length}
                </span>
              </div>

              <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-2)' }}>
                {g.items.map((item: InsightInboxItem) => {
                  const ins = item.insight;
                  const rank = item.rank;
                  const itemCaption = `${LABEL_SCORE}${rank?.score ?? 0}${LABEL_IMPACT}${rank?.impact ?? 'low'}${LABEL_DOT}${ins.observation_ids?.length || 0}${LABEL_FINDINGS}`;
                  return (
                    <button
                      key={ins.id}
                      type="button"
                      className="fg-card"
                      onClick={() => navigate(`/inbox/${encodeURIComponent(ins.id)}`)}
                      style={{
                        all: 'unset',
                        boxSizing: 'border-box',
                        cursor: 'pointer',
                        display: 'flex',
                        flexWrap: 'wrap',
                        gap: 'var(--space-2) var(--space-4)',
                        alignItems: 'center',
                        padding: 'var(--space-4)',
                        border: '1px solid var(--color-border)',
                        borderRadius: 'var(--card-radius)',
                        background: 'var(--color-surface)',
                      }}
                    >
                      <StatusBadge
                        variant="chip"
                        tone={
                          ins.priority === 'critical'
                            ? 'danger'
                            : ins.priority === 'high'
                              ? 'warning'
                              : 'neutral'
                        }
                        label={ins.priority}
                      />

                      <div style={{ display: 'flex', flexDirection: 'column', gap: '4px', flex: '1 1 280px', minWidth: 0 }}>
                        <span className="t-ui" style={{ fontWeight: 600, fontSize: '14px', textWrap: 'pretty' }}>
                          {ins.recommendation}
                        </span>
                        <span className="t-caption" style={{ color: 'var(--color-text-muted)', fontFamily: 'var(--font-mono)' }}>
                          <span>{itemCaption}</span>
                          {rank?.stale && (
                            <span style={{ color: 'var(--color-warning)', marginLeft: '8px' }}>
                              {LABEL_STALE_EVIDENCE}
                            </span>
                          )}
                        </span>
                      </div>

                      <StatusBadge
                        variant="chip"
                        tone={ins.status === 'planned' ? 'success' : 'neutral'}
                        label={ins.status}
                      />

                      <span className="t-ui" style={{ color: 'var(--color-primary)', fontWeight: 500, fontSize: '13px' }}>
                        {LABEL_OPEN_ARROW}
                      </span>
                    </button>
                  );
                })}
              </div>
            </section>
          ))}

          {hasMore && (
            <button
              type="button"
              className="fg-btn fg-btn--secondary"
              style={{ alignSelf: 'center', marginTop: 'var(--space-2)' }}
              onClick={() => void handleLoadMore()}
              disabled={loadingMore}
            >
              <span>{loadingMore ? BTN_LOADING_MORE : BTN_LOAD_MORE}</span>
            </button>
          )}
        </div>
      )}
    </div>
  );
}
