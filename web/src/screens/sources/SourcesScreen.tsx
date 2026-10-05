import { useState } from 'react';
import { Link } from 'react-router';
import { useQueryClient } from '@tanstack/react-query';
import {
  checkSources,
  confirmSourceImport,
  confirmSourceProposal,
  previewSourceImport,
  previewUnwatchSource,
  useSources,
} from '../../api/queries';
import type { DiscoveredImportSkill, SourceImportProposal, SourceSummary } from '../../api/types';
import { ConfirmDialog } from '../../components/ConfirmDialog';
import { Skeleton } from '../../components/Skeleton';
import { StatusBadge } from '../../components/StatusBadge';

export function SourcesScreen() {
  const queryClient = useQueryClient();
  const { data, isLoading, error, refetch } = useSources();

  const [checkingAll, setCheckingAll] = useState(false);
  const [checkingSourceId, setCheckingSourceId] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);

  const [unwatchSourceId, setUnwatchSourceId] = useState<string | null>(null);
  const [unwatching, setUnwatching] = useState(false);

  // Import more state
  const [importSourceId, setImportSourceId] = useState<string | null>(null);
  const [importProposal, setImportProposal] = useState<SourceImportProposal | null>(null);
  const [selectedSkills, setSelectedSkills] = useState<string[]>([]);
  const [loadingImport, setLoadingImport] = useState(false);
  const [confirmingImport, setConfirmingImport] = useState(false);

  const handleCheckAll = async () => {
    setCheckingAll(true);
    setActionError(null);
    try {
      await checkSources({ all: true });
      queryClient.invalidateQueries({ queryKey: ['sources'] });
    } catch (err: unknown) {
      setActionError(err instanceof Error ? err.message : 'Failed to check all sources');
    } finally {
      setCheckingAll(false);
    }
  };

  const handleCheckSource = async (sourceId: string) => {
    setCheckingSourceId(sourceId);
    setActionError(null);
    try {
      await checkSources({ source_ids: [sourceId] });
      queryClient.invalidateQueries({ queryKey: ['sources'] });
    } catch (err: unknown) {
      setActionError(err instanceof Error ? err.message : 'Failed to check source');
    } finally {
      setCheckingSourceId(null);
    }
  };

  const handleConfirmUnwatch = async () => {
    if (!unwatchSourceId) return;
    setUnwatching(true);
    setActionError(null);
    try {
      const prop = await previewUnwatchSource(unwatchSourceId);
      const pins = prop.confirmation?.confirmation?.pins;
      if (pins) {
        await confirmSourceProposal(pins.proposal_id, {
          proposal_digest: pins.proposal_digest,
          base_version: pins.base_version,
        });
      }
      queryClient.invalidateQueries({ queryKey: ['sources'] });
      setUnwatchSourceId(null);
    } catch (err: unknown) {
      setActionError(err instanceof Error ? err.message : 'Failed to unwatch source');
    } finally {
      setUnwatching(false);
    }
  };

  const handleOpenImportMore = async (sourceId: string) => {
    setImportSourceId(sourceId);
    setLoadingImport(true);
    setActionError(null);
    try {
      const prop = await previewSourceImport(sourceId);
      setImportProposal(prop);
      setSelectedSkills(prop.importable.map((s) => s.target_id));
    } catch (err: unknown) {
      setActionError(err instanceof Error ? err.message : 'Failed to inspect source skills');
      setImportSourceId(null);
    } finally {
      setLoadingImport(false);
    }
  };

  const handleConfirmImportMore = async () => {
    if (!importProposal?.confirmation?.confirmation?.pins) return;
    setConfirmingImport(true);
    setActionError(null);
    try {
      await confirmSourceImport(importProposal.confirmation.confirmation.pins);
      queryClient.invalidateQueries({ queryKey: ['sources'] });
      queryClient.invalidateQueries({ queryKey: ['skills'] });
      setImportSourceId(null);
      setImportProposal(null);
    } catch (err: unknown) {
      setActionError(err instanceof Error ? err.message : 'Failed to import skills');
    } finally {
      setConfirmingImport(false);
    }
  };

  if (isLoading) {
    return (
      <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
        <Skeleton width="30%" height="32px" />
        <Skeleton height="120px" />
        <Skeleton height="120px" />
      </div>
    );
  }

  if (error) {
    return (
      <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
        <div className="fg-banner fg-banner--danger">
          <span>{error instanceof Error ? error.message : 'Failed to load sources'}</span>
        </div>
        <button
          type="button"
          className="fg-btn fg-btn--secondary"
          style={{ width: 'fit-content' }}
          onClick={() => void refetch()}
        >
          <span>Retry</span>
        </button>
      </div>
    );
  }

  const groups = data?.groups || [];
  const allSources = data?.sources || [];
  const sourcesMap = new Map<string, (typeof allSources)[number]['record']>();
  for (const s of allSources) {
    sourcesMap.set(s.record.id, s.record);
  }

  const isEmpty = groups.length === 0 && allSources.length === 0;

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
      {/* Toolbar */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: 'var(--space-3)' }}>
        <div>
          <h1 style={{ margin: 0, fontSize: '20px', fontWeight: 600 }}>Sources</h1>
        </div>
        <div style={{ display: 'flex', gap: 'var(--space-2)' }}>
          <button
            type="button"
            className="fg-btn fg-btn--secondary"
            onClick={() => void handleCheckAll()}
            disabled={checkingAll || isEmpty}
          >
            <span>{checkingAll ? 'Checking all…' : 'Check all'}</span>
          </button>
          <Link to="/skills/add" className="fg-btn fg-btn--primary" style={{ textDecoration: 'none' }}>
            <span>Add skills from GitHub</span>
          </Link>
        </div>
      </div>

      {actionError && (
        <div className="fg-banner fg-banner--danger">
          <span>{actionError}</span>
        </div>
      )}

      {isEmpty ? (
        <div className="fg-card" style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 'var(--space-3)', padding: 'var(--space-6) var(--space-4)' }}>
          <span style={{ color: 'var(--color-text-muted)', fontSize: '14px', textAlign: 'center' }}>
            No sources yet. Link a repository whose ideas should improve your skills.
          </span>
          <Link to="/skills/add" className="fg-btn fg-btn--primary" style={{ textDecoration: 'none' }}>
            <span>Add skills from GitHub</span>
          </Link>
        </div>
      ) : (
        groups.map((group) => (
          <section key={group.repository} className="fg-card" style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' }}>
            <div className="fg-card__title">
              <span style={{ fontFamily: 'var(--font-mono)', fontSize: '14px' }}>{group.repository}</span>
            </div>

            <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' }}>
              {group.sources.map((src: SourceSummary) => {
                const rec = sourcesMap.get(src.id);
                const isChecking = checkingSourceId === src.id;
                const isOrphan = src.referencing_skills.length === 0;
                const lastChecked = src.last_checked_at ? src.last_checked_at.slice(0, 10) : 'never';
                const cadence = rec?.monitoring?.cadence || 'weekly';

                return (
                  <div
                    key={src.id}
                    style={{
                      display: 'flex',
                      justifyContent: 'space-between',
                      alignItems: 'center',
                      padding: 'var(--space-3)',
                      border: '1px solid var(--color-border)',
                      borderRadius: 'var(--radius-sm)',
                      flexWrap: 'wrap',
                      gap: 'var(--space-3)',
                    }}
                  >
                    <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-1)', flex: 1, minWidth: '260px' }}>
                      <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-2)', flexWrap: 'wrap' }}>
                        <span style={{ fontWeight: 600, fontSize: '14px' }}>{src.id}</span>
                        {rec?.locator?.ref && (
                          <span style={{ fontSize: '12px', color: 'var(--color-text-muted)', fontFamily: 'var(--font-mono)' }}>
                            @{rec.locator.ref}
                          </span>
                        )}
                        <StatusBadge variant="chip" tone="neutral" label={src.role} />
                        {isOrphan && (
                          <StatusBadge variant="chip" tone="warning" label="No linked skills" />
                        )}
                      </div>

                      <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-2)', flexWrap: 'wrap', fontSize: '13px' }}>
                        <span style={{ color: 'var(--color-text-muted)' }}>Skills:</span>
                        {isOrphan ? (
                          <span style={{ color: 'var(--color-text-muted)', fontStyle: 'italic' }}>no skills</span>
                        ) : (
                          src.referencing_skills.map((skillId) => (
                            <Link
                              key={skillId}
                              to={`/skills/${encodeURIComponent(skillId)}`}
                              style={{ color: 'var(--color-primary)', textDecoration: 'none', fontWeight: 500 }}
                            >
                              <span>{skillId}</span>
                            </Link>
                          ))
                        )}
                      </div>

                      <div style={{ display: 'flex', gap: 'var(--space-3)', fontSize: '12px', color: 'var(--color-text-muted)' }}>
                        <span>Last checked: {lastChecked}</span>
                        <span>Watch: {cadence}</span>
                      </div>
                    </div>

                    <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-2)' }}>
                      <button
                        type="button"
                        className="fg-btn fg-btn--secondary fg-btn--small"
                        onClick={() => void handleCheckSource(src.id)}
                        disabled={isChecking}
                      >
                        <span>{isChecking ? 'Checking…' : 'Check now'}</span>
                      </button>
                      <button
                        type="button"
                        className="fg-btn fg-btn--secondary fg-btn--small"
                        onClick={() => void handleOpenImportMore(src.id)}
                      >
                        <span>Import more</span>
                      </button>
                      <button
                        type="button"
                        className="fg-btn fg-btn--secondary fg-btn--small"
                        onClick={() => setUnwatchSourceId(src.id)}
                      >
                        <span>Unwatch</span>
                      </button>
                    </div>
                  </div>
                );
              })}
            </div>
          </section>
        ))
      )}

      {/* Unwatch Confirm Dialog */}
      {unwatchSourceId && (
        <ConfirmDialog
          open={Boolean(unwatchSourceId)}
          title={`Unwatch ${unwatchSourceId}`}
          body={`Are you sure you want to stop watching ${unwatchSourceId}?`}
          confirmLabel={unwatching ? 'Unwatching…' : 'Unwatch'}
          danger
          onConfirm={() => void handleConfirmUnwatch()}
          onCancel={() => setUnwatchSourceId(null)}
        />
      )}

      {/* Import More Dialog */}
      {importSourceId && (
        <div
          role="dialog"
          aria-modal="true"
          style={{
            position: 'fixed',
            inset: 0,
            backgroundColor: 'rgba(0, 0, 0, 0.5)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            padding: 'var(--space-4)',
            zIndex: 1000,
          }}
        >
          <div
            className="fg-card"
            style={{
              maxWidth: '560px',
              width: '100%',
              display: 'flex',
              flexDirection: 'column',
              gap: 'var(--space-3)',
              maxHeight: '90vh',
              overflowY: 'auto',
            }}
          >
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              <span className="fg-card__title">Import skills from {importSourceId}</span>
              <button
                type="button"
                className="fg-btn fg-btn--secondary fg-btn--small"
                onClick={() => setImportSourceId(null)}
              >
                <span>Close</span>
              </button>
            </div>

            {loadingImport ? (
              <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-2)' }}>
                <Skeleton height="30px" />
                <Skeleton height="60px" />
              </div>
            ) : importProposal ? (
              <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' }}>
                <span>{importProposal.summary}</span>

                <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-2)' }}>
                  {importProposal.discovered.map((skill: DiscoveredImportSkill) => {
                    const isImported = skill.imported || skill.conflict;
                    return (
                      <label
                        key={skill.target_id}
                        style={{
                          display: 'flex',
                          alignItems: 'center',
                          gap: 'var(--space-2)',
                          fontSize: '13px',
                          color: isImported ? 'var(--color-text-muted)' : 'var(--color-text)',
                        }}
                      >
                        <input
                          type="checkbox"
                          disabled={isImported}
                          checked={isImported ? false : selectedSkills.includes(skill.target_id)}
                          onChange={(e) => {
                            if (e.target.checked) {
                              setSelectedSkills([...selectedSkills, skill.target_id]);
                            } else {
                              setSelectedSkills(selectedSkills.filter((id) => id !== skill.target_id));
                            }
                          }}
                        />
                        <span style={{ fontWeight: 600 }}>{skill.name}</span>
                        <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px' }}>({skill.target_id})</span>
                        {isImported && (
                          <span style={{ fontStyle: 'italic', fontSize: '12px' }}>
                            — {skill.skip_reason || 'already imported'}
                          </span>
                        )}
                      </label>
                    );
                  })}
                </div>

                <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 'var(--space-2)', marginTop: 'var(--space-2)' }}>
                  <button
                    type="button"
                    className="fg-btn fg-btn--secondary"
                    onClick={() => setImportSourceId(null)}
                  >
                    <span>Cancel</span>
                  </button>
                  <button
                    type="button"
                    className="fg-btn fg-btn--primary"
                    disabled={confirmingImport || importProposal.importable.length === 0}
                    onClick={() => void handleConfirmImportMore()}
                  >
                    <span>{confirmingImport ? 'Importing…' : 'Import'}</span>
                  </button>
                </div>
              </div>
            ) : null}
          </div>
        </div>
      )}
    </div>
  );
}
