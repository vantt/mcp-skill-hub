import type { ReactNode } from 'react';
import type {
  LearningReference,
  SkillDetail,
  SkillReviewResult,
  SkillSourcesResult,
  SkillUpstream,
} from '../../api/types';
import { Skeleton } from '../../components/Skeleton';
import { StatusBadge } from '../../components/StatusBadge';
import { useT } from '../../i18n';
import { ProvenanceCard } from './ProvenanceCard';
import { resourceSummaryText, upstreamStatusBadge } from './detail-format';

const TITLE_ABOUT = 'About';
const TITLE_ROUTING = 'Routing';
const TITLE_SOURCES = 'Sources';
const HELP_ABOUT = 'What this skill is and where it lives.';
const HELP_ROUTING = 'When the resolver offers this skill to an agent.';
const HELP_SOURCES = 'Repositories this skill comes from or learns from.';
const LABEL_NAME = 'Name';
const LABEL_DESCRIPTION = 'Description';
const LABEL_COLLECTION = 'Collection';
const LABEL_LIFECYCLE = 'Lifecycle state';
const LABEL_FILES = 'Files';
const LABEL_MIN_SCOPE = 'Min scope';
const LABEL_OPERATIONS = 'Operations';
const LABEL_TRIGGERS = 'Triggers';
const LABEL_NOT_FOR = 'Not for';
const LABEL_NONE = 'None';
const LABEL_UNKNOWN = '—';
const LABEL_NO_DESCRIPTION = 'No description yet.';
const LABEL_UPSTREAM = 'Upstream';
const LABEL_LEARNING = 'Learning reference';
const LABEL_PATH = 'Path: ';
const LABEL_LAST_CHECKED = 'Last checked: ';
const LABEL_NEVER = 'never';
const LABEL_NO_SOURCES = 'No sources yet.';
const LABEL_SOURCES_ERROR = 'Could not load sources. Open the Sources tab to retry.';
const LABEL_SOURCES_LOADING = 'Loading sources';
const BTN_EDIT = 'Edit';
const BTN_MANAGE_SOURCES = 'Manage sources';

const CARD_STYLE = { display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' } as const;
const CARD_HEADER_STYLE = {
  display: 'flex',
  flexWrap: 'wrap',
  justifyContent: 'space-between',
  alignItems: 'center',
  gap: 'var(--space-2)',
} as const;
const HELP_STYLE = { color: 'var(--color-text-muted)' } as const;
const MUTED_SMALL_STYLE = { fontSize: '12px', color: 'var(--color-text-muted)', overflowWrap: 'anywhere' } as const;
const MONO_STYLE = { fontFamily: 'var(--font-mono)', fontSize: '13px', overflowWrap: 'anywhere' } as const;
const ROW_STYLE = {
  display: 'flex',
  flexDirection: 'column',
  gap: '2px',
  padding: 'var(--space-2)',
  border: '1px solid var(--color-border)',
  borderRadius: 'var(--radius-sm)',
  minWidth: 0,
} as const;

export interface OverviewTabProps {
  skill: SkillDetail;
  review?: SkillReviewResult;
  sources?: SkillSourcesResult;
  sourcesLoading: boolean;
  sourcesError: unknown;
  onOpenTab: (tab: 'sources' | 'editor') => void;
}

export function OverviewTab({ skill, review, sources, sourcesLoading, sourcesError, onOpenTab }: OverviewTabProps) {
  const t = useT();
  const description = skill.description.trim();
  const totalBytes =
    review?.resource_status?.total_bytes ?? skill.resources?.reduce((acc, r) => acc + r.size_bytes, 0) ?? 0;
  const resourceCount = review?.resource_status?.resource_count ?? skill.resources?.length ?? 0;
  const routing = skill.routing;

  return (
    <div
      style={{
        display: 'grid',
        gridTemplateColumns: 'repeat(auto-fit, minmax(min(100%, 320px), 1fr))',
        gap: 'var(--space-4)',
        alignItems: 'start',
      }}
    >
      <section className="fg-card" style={CARD_STYLE} aria-label={TITLE_ABOUT}>
        <div style={CARD_HEADER_STYLE}>
          <div className="fg-card__title">
            <span>{TITLE_ABOUT}</span>
          </div>
          <button type="button" className="fg-btn fg-btn--secondary fg-btn--small" onClick={() => onOpenTab('editor')}>
            <span>{BTN_EDIT}</span>
          </button>
        </div>
        <span className="t-caption" style={HELP_STYLE}>
          <span>{HELP_ABOUT}</span>
        </span>
        <div className="fg-facts">
          <Fact label={LABEL_NAME}>{skill.name || skill.skill_id}</Fact>
          <Fact label={LABEL_DESCRIPTION}>
            <p className="app-wrap" style={{ margin: 0, ...(description ? undefined : HELP_STYLE) }}>
              {description || LABEL_NO_DESCRIPTION}
            </p>
          </Fact>
          <Fact label={LABEL_COLLECTION}>{review?.collection || LABEL_UNKNOWN}</Fact>
          <Fact label={LABEL_LIFECYCLE}>{t(`lifecycle_short.${skill.lifecycle_state}`)}</Fact>
          <Fact label={LABEL_FILES}>{resourceSummaryText(resourceCount, totalBytes)}</Fact>
        </div>
      </section>

      <section className="fg-card" style={CARD_STYLE} aria-label={TITLE_ROUTING}>
        <div className="fg-card__title">
          <span>{TITLE_ROUTING}</span>
        </div>
        <span className="t-caption" style={HELP_STYLE}>
          <span>{HELP_ROUTING}</span>
        </span>
        <div className="fg-facts">
          <Fact label={LABEL_MIN_SCOPE}>{routing?.min_scope || LABEL_NONE}</Fact>
          <Fact label={LABEL_OPERATIONS}>
            <ValueList items={routing?.operations} />
          </Fact>
          <Fact label={LABEL_TRIGGERS}>
            <ValueList items={routing?.triggers} />
          </Fact>
          <Fact label={LABEL_NOT_FOR}>
            <ValueList items={routing?.not_for} />
          </Fact>
        </div>
      </section>

      <ProvenanceCard provenance={review?.provenance} onViewSources={() => onOpenTab('sources')} />

      <section className="fg-card" style={CARD_STYLE} aria-label={TITLE_SOURCES}>
        <div style={CARD_HEADER_STYLE}>
          <div className="fg-card__title">
            <span>{TITLE_SOURCES}</span>
          </div>
          <button type="button" className="fg-btn fg-btn--secondary fg-btn--small" onClick={() => onOpenTab('sources')}>
            <span>{BTN_MANAGE_SOURCES}</span>
          </button>
        </div>
        <span className="t-caption" style={HELP_STYLE}>
          <span>{HELP_SOURCES}</span>
        </span>
        <SourcesSummary sources={sources} loading={sourcesLoading} error={sourcesError} />
      </section>
    </div>
  );
}

function Fact({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="fg-fact">
      <div className="fg-fact__label">
        <span>{label}</span>
      </div>
      <div className="fg-fact__value app-wrap" style={{ overflowWrap: 'anywhere', minWidth: 0 }}>
        {children}
      </div>
    </div>
  );
}

function ValueList({ items }: { items?: string[] }) {
  if (!items || items.length === 0) {
    return <span style={HELP_STYLE}>{LABEL_NONE}</span>;
  }
  return (
    <ul style={{ margin: 0, paddingLeft: 'var(--space-4)' }}>
      {items.map((item) => (
        <li key={item}>{item}</li>
      ))}
    </ul>
  );
}

function SourcesSummary({
  sources,
  loading,
  error,
}: {
  sources?: SkillSourcesResult;
  loading: boolean;
  error: unknown;
}) {
  if (loading) {
    return (
      <div role="status" aria-label={LABEL_SOURCES_LOADING} style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-2)' }}>
        <Skeleton height={48} />
        <Skeleton height={48} />
      </div>
    );
  }
  if (error) {
    return (
      <div className="fg-banner fg-banner--danger" style={{ fontSize: '13px' }}>
        <span>{LABEL_SOURCES_ERROR}</span>
      </div>
    );
  }
  const upstream = sources?.upstream ?? null;
  const learning = sources?.learning ?? [];
  if (!upstream && learning.length === 0) {
    return (
      <div style={{ color: 'var(--color-text-muted)', fontSize: '13px' }}>
        <span>{LABEL_NO_SOURCES}</span>
      </div>
    );
  }
  return (
    <ul style={{ listStyle: 'none', margin: 0, padding: 0, display: 'flex', flexDirection: 'column', gap: 'var(--space-2)' }}>
      {upstream && <UpstreamRow upstream={upstream} />}
      {learning.map((ref) => (
        <LearningRow key={ref.source_id} reference={ref} />
      ))}
    </ul>
  );
}

function UpstreamRow({ upstream }: { upstream: SkillUpstream }) {
  const badge = upstreamStatusBadge(upstream.status);
  const location = upstream.ref ? `${upstream.repository}@${upstream.ref}` : upstream.repository;
  return (
    <li style={ROW_STYLE}>
      <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 'var(--space-2)' }}>
        <span style={{ fontWeight: 600, fontSize: '13px' }}>{LABEL_UPSTREAM}</span>
        <StatusBadge tone={badge.tone} label={badge.label} />
      </div>
      <span style={MONO_STYLE}>{location}</span>
      {upstream.path && (
        <span style={MUTED_SMALL_STYLE}>
          {LABEL_PATH}
          {upstream.path}
        </span>
      )}
    </li>
  );
}

function LearningRow({ reference }: { reference: LearningReference }) {
  const lastChecked = reference.last_checked_at ? reference.last_checked_at.slice(0, 10) : LABEL_NEVER;
  return (
    <li style={ROW_STYLE}>
      <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 'var(--space-2)' }}>
        <span style={{ fontWeight: 600, fontSize: '13px', overflowWrap: 'anywhere' }}>{reference.source_id}</span>
        <StatusBadge variant="chip" tone="neutral" label={LABEL_LEARNING} />
      </div>
      <span style={MONO_STYLE}>{reference.locator}</span>
      <span style={MUTED_SMALL_STYLE}>
        {LABEL_LAST_CHECKED}
        {lastChecked}
      </span>
    </li>
  );
}
