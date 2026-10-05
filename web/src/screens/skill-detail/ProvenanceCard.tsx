import type { SkillProvenance } from '../../api/types';

const TITLE_PROVENANCE = 'Provenance';
const BTN_VIEW_SOURCES = 'View sources →';
const LABEL_NO_PROVENANCE = 'Created in this workspace';
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
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
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

      {!hasProvenance ? (
        <div style={{ color: 'var(--color-text-muted)', fontSize: '13px' }}>
          <span>{LABEL_NO_PROVENANCE}</span>
        </div>
      ) : (
        <div className="fg-facts">
          {provenance?.source_locator && (
            <div className="fg-fact">
              <div className="fg-fact__label">
                <span>{LABEL_SOURCE_LOCATOR}</span>
              </div>
              <div className="fg-fact__value" style={{ fontFamily: 'var(--font-mono)', fontSize: '13px' }}>
                <span>{provenance.source_locator}</span>
              </div>
            </div>
          )}
          {provenance?.upstream_path && (
            <div className="fg-fact">
              <div className="fg-fact__label">
                <span>{LABEL_UPSTREAM_PATH}</span>
              </div>
              <div className="fg-fact__value" style={{ fontFamily: 'var(--font-mono)', fontSize: '13px' }}>
                <span>{provenance.upstream_path}</span>
              </div>
            </div>
          )}
          {provenance?.source_revision && (
            <div className="fg-fact">
              <div className="fg-fact__label">
                <span>{LABEL_SOURCE_REVISION}</span>
              </div>
              <div className="fg-fact__value" style={{ fontFamily: 'var(--font-mono)', fontSize: '13px' }}>
                <span>{provenance.source_revision}</span>
              </div>
            </div>
          )}
          {provenance?.source_id && (
            <div className="fg-fact">
              <div className="fg-fact__label">
                <span>{LABEL_SOURCE_ID}</span>
              </div>
              <div className="fg-fact__value" style={{ fontFamily: 'var(--font-mono)', fontSize: '13px' }}>
                <span>{provenance.source_id}</span>
              </div>
            </div>
          )}
          {provenance?.created_by && (
            <div className="fg-fact">
              <div className="fg-fact__label">
                <span>{LABEL_CREATED_BY}</span>
              </div>
              <div className="fg-fact__value" style={{ fontSize: '13px' }}>
                <span>{provenance.created_by}</span>
              </div>
            </div>
          )}
          {provenance?.created_at && (
            <div className="fg-fact">
              <div className="fg-fact__label">
                <span>{LABEL_CREATED_AT}</span>
              </div>
              <div className="fg-fact__value" style={{ fontSize: '13px' }}>
                <span>{provenance.created_at}</span>
              </div>
            </div>
          )}
        </div>
      )}
    </section>
  );
}
