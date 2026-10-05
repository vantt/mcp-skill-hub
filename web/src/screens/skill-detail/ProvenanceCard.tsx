import type { SkillProvenance } from '../../api/types';

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
          <span>Provenance</span>
        </div>
        <button
          type="button"
          className="fg-btn fg-btn--secondary fg-btn--small"
          onClick={onViewSources}
        >
          <span>View sources →</span>
        </button>
      </div>

      {!hasProvenance ? (
        <div style={{ color: 'var(--color-text-subtle)', fontSize: '13px' }}>
          <span>Created in this workspace</span>
        </div>
      ) : (
        <div className="fg-facts">
          {provenance?.source_locator && (
            <div className="fg-fact">
              <div className="fg-fact__label">
                <span>Source locator</span>
              </div>
              <div className="fg-fact__value" style={{ fontFamily: 'var(--font-mono)', fontSize: '13px' }}>
                <span>{provenance.source_locator}</span>
              </div>
            </div>
          )}
          {provenance?.upstream_path && (
            <div className="fg-fact">
              <div className="fg-fact__label">
                <span>Upstream path</span>
              </div>
              <div className="fg-fact__value" style={{ fontFamily: 'var(--font-mono)', fontSize: '13px' }}>
                <span>{provenance.upstream_path}</span>
              </div>
            </div>
          )}
          {provenance?.source_revision && (
            <div className="fg-fact">
              <div className="fg-fact__label">
                <span>Source revision</span>
              </div>
              <div className="fg-fact__value" style={{ fontFamily: 'var(--font-mono)', fontSize: '13px' }}>
                <span>{provenance.source_revision}</span>
              </div>
            </div>
          )}
          {provenance?.source_id && (
            <div className="fg-fact">
              <div className="fg-fact__label">
                <span>Source ID</span>
              </div>
              <div className="fg-fact__value" style={{ fontFamily: 'var(--font-mono)', fontSize: '13px' }}>
                <span>{provenance.source_id}</span>
              </div>
            </div>
          )}
          {provenance?.created_by && (
            <div className="fg-fact">
              <div className="fg-fact__label">
                <span>Created by</span>
              </div>
              <div className="fg-fact__value" style={{ fontSize: '13px' }}>
                <span>{provenance.created_by}</span>
              </div>
            </div>
          )}
          {provenance?.created_at && (
            <div className="fg-fact">
              <div className="fg-fact__label">
                <span>Created at</span>
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
