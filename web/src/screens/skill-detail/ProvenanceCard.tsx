import type { SkillProvenance } from '../../api/types';

const TITLE_PROVENANCE = 'Provenance';
const BTN_VIEW_SOURCES = 'View sources →';
const HELP_PROVENANCE = 'Where this skill came from.';
const LABEL_NO_PROVENANCE = 'Made on this machine (created here or added from a folder); no tracked source.';
const LABEL_SOURCE_LOCATOR = 'Source locator';
const LABEL_UPSTREAM_PATH = 'Upstream path';
const LABEL_SOURCE_REVISION = 'Source revision';
const LABEL_SOURCE_ID = 'Source ID';
const LABEL_CREATED_BY = 'Created by';
const LABEL_CREATED_AT = 'Created at';

interface ProvenanceCardProps {
  provenance?: SkillProvenance;
  onViewSources: () => void;
}

export function ProvenanceCard({ provenance, onViewSources }: ProvenanceCardProps) {
  const hasProvenance = Boolean(
    provenance &&
      (provenance.source_locator ||
        provenance.source_revision ||
        provenance.upstream_path ||
        provenance.source_id ||
        provenance.created_by ||
        provenance.created_at),
  );

  return (
    <section className="fg-card" style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' }}>
      <div style={{ display: 'flex', flexWrap: 'wrap', justifyContent: 'space-between', alignItems: 'center', gap: 'var(--space-2)' }}>
        <div className="fg-card__title">
          <span>{TITLE_PROVENANCE}</span>
        </div>
        <button
          type="button"
          className="fg-btn fg-btn--secondary fg-btn--small"
          onClick={onViewSources}
        >
          <span>{BTN_VIEW_SOURCES}</span>
        </button>
      </div>

      <span className="t-caption" style={{ color: 'var(--color-text-muted)' }}>
        <span>{HELP_PROVENANCE}</span>
      </span>

      {!hasProvenance ? (
        <div style={{ color: 'var(--color-text-muted)', fontSize: '13px' }}>
          <span>{LABEL_NO_PROVENANCE}</span>
        </div>
      ) : (
        <div className="fg-facts app-facts">
          <ProvenanceFact label={LABEL_SOURCE_LOCATOR} value={provenance?.source_locator} mono wide />
          <ProvenanceFact label={LABEL_UPSTREAM_PATH} value={provenance?.upstream_path} mono wide />
          <ProvenanceFact label={LABEL_SOURCE_REVISION} value={provenance?.source_revision} mono wide />
          <ProvenanceFact label={LABEL_SOURCE_ID} value={provenance?.source_id} mono />
          <ProvenanceFact label={LABEL_CREATED_BY} value={provenance?.created_by} />
          <ProvenanceFact label={LABEL_CREATED_AT} value={provenance?.created_at} />
        </div>
      )}
    </section>
  );
}

// One labelled value. Long identifiers take a full row and wrap, so they never run into a neighbour.
function ProvenanceFact({ label, value, mono, wide }: { label: string; value?: string; mono?: boolean; wide?: boolean }) {
  if (!value) return null;
  return (
    <div className={`fg-fact${wide ? ' fg-fact--wide' : ''}`}>
      <div className="fg-fact__label">
        <span>{label}</span>
      </div>
      <div className={`fg-fact__value${mono ? ' app-mono' : ''}`}>
        <span>{value}</span>
      </div>
    </div>
  );
}
