import { useState } from 'react';
import { Link, useSearchParams } from 'react-router';
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
import { distillLabel } from '../../domain/source-distill';

const TITLE_SOURCES = 'Sources';
const BTN_CHECK_ALL = 'Check all';
const BTN_CHECKING_ALL = 'Checking all…';
const BTN_CHECK_DUE = 'Check due sources';
const BTN_CHECKING_DUE = 'Checking due…';
const BTN_DISTILL_CURATOR = 'Distill with Curator Agent';
const INTRO_SOURCES =
  'A source is a repository your skills learn from or track for updates. Check finds new commits; a source marked ready has commits no skill has learned from yet. Tick it and hand it to your curator agent.';
const BTN_ADD_FROM_GITHUB = 'Add skills from GitHub';
const LABEL_NO_SOURCES = 'No sources yet. Link a repository whose ideas should improve your skills.';
const LABEL_NO_SOURCES_WHY =
  'A source is a repository your skills track or learn from. Skill Hub checks it for new commits, and your curator agent turns what it finds into lessons for the linked skills. Add skills from GitHub, or open a skill and add a learning reference on its Sources tab.';
const LABEL_NO_READY_SOURCES = 'No sources ready to distill.';
const LABEL_SKILLS_PREFIX = 'Skills:';
const LABEL_NO_SKILLS = 'no skills';
const LABEL_LAST_CHECKED = 'Last checked: ';
const LABEL_WATCH = 'Watch: ';
const BTN_CHECK_NOW = 'Check now';
const BTN_CHECKING = 'Checking…';
const BTN_IMPORT_MORE = 'Import more';
const BTN_UNWATCH = 'Unwatch';
const BTN_UNWATCHING = 'Unwatching…';
const TITLE_IMPORT_SKILLS_FROM = 'Import skills from ';
const BTN_CLOSE = 'Close';
const BTN_CANCEL = 'Cancel';
const BTN_IMPORT = 'Import';
const BTN_IMPORTING = 'Importing…';
const LABEL_RETRY = 'Retry';
const LABEL_LPAREN = ' (';
const LABEL_RPAREN = ')';
const LABEL_DISCOVERED = 'Discovered ';
const LABEL_SKILLS_COUNT = ' skill(s). ';
const LABEL_IMPORTABLE = ' importable.';
const LABEL_IMPORTABLE_SKILLS = 'Importable skills';
const LABEL_ALREADY_IMPORTED_TITLE = 'Already imported / conflicting';
const LABEL_ALREADY_IMPORTED = 'already imported';

export function SourcesScreen() {
  const queryClient = useQueryClient();
  const [searchParams] = useSearchParams();
  const filterReady = searchParams.get('filter') === 'ready';


  const { data, isLoading, error, refetch } = useSources();

  const [checkingAll, setCheckingAll] = useState(false);
  const [checkingDue, setCheckingDue] = useState(false);
  const [checkingSourceId, setCheckingSourceId] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);

  const [selectedSources, setSelectedSources] = useState<string[]>([]);

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

  const handleCheckDue = async () => {
    setCheckingDue(true);
    setActionError(null);
    try {
      await checkSources({ due: true });
      queryClient.invalidateQueries({ queryKey: ['sources'] });
    } catch (err: unknown) {
      setActionError(err instanceof Error ? err.message : 'Failed to check due sources');
    } finally {
      setCheckingDue(false);
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
      const pins = importProposal.confirmation.confirmation.pins;
      await confirmSourceImport({
        proposal_id: pins.proposal_id,
        proposal_digest: pins.proposal_digest,
        base_version: pins.base_version,
      });
      queryClient.invalidateQueries({ queryKey: ['sources'] });
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
          <span>{LABEL_RETRY}</span>
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

  const resolveSummary = (src: SourceSummary): SourceSummary => {
    const rec = sourcesMap.get(src.id);
    const isUpstreamOnly =
      src.upstream_only ??
      (rec?.purpose === 'upstream' ||
        src.role === 'upstream' ||
        (src.skills_vendored_count > 0 && src.referencing_skills.length === 0));
    const isNeverDistilled = !rec?.distilled_revision || !rec?.distilled_revision.value;
    const isChangedOrPending = rec?.status === 'changed' || rec?.status === 'distill_pending';
    const isReadyToDistill =
      src.ready_to_distill ?? (!isUpstreamOnly && (isChangedOrPending || isNeverDistilled));

    return {
      ...src,
      status: rec?.status || src.status,
      current_revision: rec?.current_revision || src.current_revision,
      distilled_revision: rec?.distilled_revision || src.distilled_revision,
      ready_to_distill: isReadyToDistill,
      upstream_only: isUpstreamOnly,
    };
  };

  // Filter groups if filterReady is true
  const displayedGroups = filterReady
    ? groups
        .map((g) => ({
          ...g,
          sources: g.sources.filter((s) => distillLabel(resolveSummary(s)).selectable),
        }))
        .filter((g) => g.sources.length > 0)
    : groups;

  const distillQuery = selectedSources.map((id) => `source=${encodeURIComponent(id)}`).join('&');
  const distillHref = distillQuery ? `/sources/distill?${distillQuery}` : '/sources/distill';

  const importModalTitle = `${TITLE_IMPORT_SKILLS_FROM}${importSourceId || ''}`;
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
      {/* Toolbar */}
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: 'var(--space-3)' }}>
        <div>
          <h1 style={{ margin: 0, fontSize: '20px', fontWeight: 600 }}>
            <span>{TITLE_SOURCES}</span>
          </h1>
        </div>
        <div style={{ display: 'flex', gap: 'var(--space-2)', flexWrap: 'wrap', alignItems: 'center' }}>
          <button
            type="button"
            className="fg-btn fg-btn--secondary"
            onClick={() => void handleCheckDue()}
            disabled={checkingDue || isEmpty}
          >
            <span>{checkingDue ? BTN_CHECKING_DUE : BTN_CHECK_DUE}</span>
          </button>
          <button
            type="button"
            className="fg-btn fg-btn--secondary"
            onClick={() => void handleCheckAll()}
            disabled={checkingAll || isEmpty}
          >
            <span>{checkingAll ? BTN_CHECKING_ALL : BTN_CHECK_ALL}</span>
          </button>
          <Link to={distillHref} className="fg-btn fg-btn--primary" style={{ textDecoration: 'none' }}>
            <span>
              {BTN_DISTILL_CURATOR}
              {selectedSources.length > 0 ? `${LABEL_LPAREN}${selectedSources.length}${LABEL_RPAREN}` : ''}
            </span>
          </Link>
          <Link to="/skills/add" className="fg-btn fg-btn--secondary" style={{ textDecoration: 'none' }}>
            <span>{BTN_ADD_FROM_GITHUB}</span>
          </Link>
        </div>
      </div>

      <p style={{ margin: 0, fontSize: '14px', color: 'var(--color-text-muted)', maxWidth: '72ch' }}>
        <span>{INTRO_SOURCES}</span>
      </p>

      {actionError && (
        <div className="fg-banner fg-banner--danger">
          <span>{actionError}</span>
        </div>
      )}

      {isEmpty ? (
        <div className="fg-card" style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 'var(--space-3)', padding: 'var(--space-6) var(--space-4)' }}>
          <span style={{ color: 'var(--color-text-muted)', fontSize: '14px', textAlign: 'center' }}>
            {LABEL_NO_SOURCES}
          </span>
          <span style={{ color: 'var(--color-text-muted)', fontSize: '13px', textAlign: 'center', maxWidth: '60ch' }}>
            {LABEL_NO_SOURCES_WHY}
          </span>
          <Link to="/skills/add" className="fg-btn fg-btn--primary" style={{ textDecoration: 'none' }}>
            <span>{BTN_ADD_FROM_GITHUB}</span>
          </Link>
        </div>
      ) : filterReady && displayedGroups.length === 0 ? (
        <div className="fg-card" style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 'var(--space-3)', padding: 'var(--space-6) var(--space-4)' }}>
          <span style={{ color: 'var(--color-text-muted)', fontSize: '14px', textAlign: 'center' }}>
            {LABEL_NO_READY_SOURCES}
          </span>
        </div>
      ) : (
        displayedGroups.map((group) => (
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
                const refText = rec?.locator?.ref ? `@${rec.locator.ref}` : '';
                const distill = distillLabel(resolveSummary(src));

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
                    <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-3)', flex: 1, minWidth: '260px' }}>
                      {distill.selectable && (
                        <input
                          type="checkbox"
                          checked={selectedSources.includes(src.id)}
                          onChange={(e) => {
                            if (e.target.checked) {
                              setSelectedSources((prev) => [...prev, src.id]);
                            } else {
                              setSelectedSources((prev) => prev.filter((id) => id !== src.id));
                            }
                          }}
                          aria-label={`Select ${src.id} for distillation`}
                          style={{ cursor: 'pointer' }}
                        />
                      )}
                      <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-1)', flex: 1 }}>
                        <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-2)', flexWrap: 'wrap' }}>
                          <span style={{ fontWeight: 600, fontSize: '14px' }}>{src.id}</span>
                          {refText ? (
                            <span style={{ fontSize: '12px', color: 'var(--color-text-muted)', fontFamily: 'var(--font-mono)' }}>
                              <span>{refText}</span>
                            </span>
                          ) : null}
                          <StatusBadge
                            variant="chip"
                            tone={distill.selectable ? 'warning' : 'neutral'}
                            label={distill.label}
                          />
                          {isOrphan && (
                            <StatusBadge variant="chip" tone="warning" label="No linked skills" />
                          )}
                        </div>

                        <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-2)', flexWrap: 'wrap', fontSize: '13px' }}>
                          <span style={{ color: 'var(--color-text-muted)' }}>{LABEL_SKILLS_PREFIX}</span>
                          {isOrphan ? (
                            <span style={{ color: 'var(--color-text-muted)', fontStyle: 'italic' }}>{LABEL_NO_SKILLS}</span>
                          ) : (
                            src.referencing_skills.map((skillId) => (
                              <Link
                                key={skillId}
                                to={`/skills/${encodeURIComponent(skillId)}`}
                                style={{ color: 'var(--color-link)', textDecoration: 'none', fontWeight: 500 }}
                              >
                                <span>{skillId}</span>
                              </Link>
                            ))
                          )}
                        </div>

                        <div style={{ display: 'flex', gap: 'var(--space-3)', fontSize: '12px', color: 'var(--color-text-muted)' }}>
                          <span>{LABEL_LAST_CHECKED}{lastChecked}</span>
                          <span>{LABEL_WATCH}{cadence}</span>
                        </div>
                      </div>
                    </div>

                    <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-2)', flexWrap: 'wrap' }}>
                      <button
                        type="button"
                        className="fg-btn fg-btn--secondary fg-btn--small"
                        onClick={() => void handleCheckSource(src.id)}
                        disabled={isChecking}
                      >
                        <span>{isChecking ? BTN_CHECKING : BTN_CHECK_NOW}</span>
                      </button>
                      <button
                        type="button"
                        className="fg-btn fg-btn--secondary fg-btn--small"
                        onClick={() => void handleOpenImportMore(src.id)}
                      >
                        <span>{BTN_IMPORT_MORE}</span>
                      </button>
                      <button
                        type="button"
                        className="fg-btn fg-btn--secondary fg-btn--small"
                        onClick={() => setUnwatchSourceId(src.id)}
                      >
                        <span>{BTN_UNWATCH}</span>
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
          confirmLabel={unwatching ? BTN_UNWATCHING : BTN_UNWATCH}
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
              maxHeight: '80vh',
              display: 'flex',
              flexDirection: 'column',
              gap: 'var(--space-3)',
              overflow: 'hidden',
            }}
          >
            <div className="fg-card__title" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              <span style={{ fontWeight: 600, fontSize: '16px' }}>{importModalTitle}</span>
              <button
                type="button"
                className="fg-btn fg-btn--ghost fg-btn--small"
                onClick={() => {
                  setImportSourceId(null);
                  setImportProposal(null);
                }}
              >
                <span>{BTN_CLOSE}</span>
              </button>
            </div>

            {loadingImport ? (
              <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-2)', padding: 'var(--space-3)' }}>
                <Skeleton height="32px" />
                <Skeleton height="32px" />
              </div>
            ) : importProposal ? (
              <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)', overflowY: 'auto' }}>
                <div style={{ fontSize: '13px', color: 'var(--color-text-muted)' }}>
                  <span>{LABEL_DISCOVERED}{importProposal.discovered.length}{LABEL_SKILLS_COUNT}{importProposal.importable.length}{LABEL_IMPORTABLE}</span>
                </div>

                {importProposal.importable.length > 0 && (
                  <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-2)' }}>
                    <span style={{ fontWeight: 600, fontSize: '13px' }}>{LABEL_IMPORTABLE_SKILLS}</span>
                    {importProposal.importable.map((skill: DiscoveredImportSkill) => (
                      <label
                        key={skill.target_id}
                        style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-2)', fontSize: '13px', cursor: 'pointer' }}
                      >
                        <input
                          type="checkbox"
                          checked={selectedSkills.includes(skill.target_id)}
                          onChange={(e) => {
                            if (e.target.checked) {
                              setSelectedSkills((prev) => [...prev, skill.target_id]);
                            } else {
                              setSelectedSkills((prev) => prev.filter((id) => id !== skill.target_id));
                            }
                          }}
                        />
                        <span style={{ fontFamily: 'var(--font-mono)' }}>{skill.target_id}</span>
                        {skill.name && <span style={{ color: 'var(--color-text-muted)' }}>{LABEL_LPAREN}{skill.name}{LABEL_RPAREN}</span>}
                      </label>
                    ))}
                  </div>
                )}

                {importProposal.skipped.length > 0 && (
                  <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-2)' }}>
                    <span style={{ fontWeight: 600, fontSize: '13px', color: 'var(--color-text-muted)' }}>{LABEL_ALREADY_IMPORTED_TITLE}</span>
                    {importProposal.skipped.map((skill: DiscoveredImportSkill) => (
                      <div key={skill.target_id} style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-2)', fontSize: '13px', color: 'var(--color-text-muted)' }}>
                        <span style={{ fontFamily: 'var(--font-mono)' }}>{skill.target_id}</span>
                        {skill.skip_reason && <span style={{ fontStyle: 'italic', fontSize: '12px' }}>{LABEL_LPAREN}{skill.skip_reason === 'already_imported' ? LABEL_ALREADY_IMPORTED : skill.skip_reason}{LABEL_RPAREN}</span>}
                      </div>
                    ))}
                  </div>
                )}
              </div>
            ) : null}

            <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 'var(--space-2)', marginTop: 'var(--space-2)' }}>
              <button
                type="button"
                className="fg-btn fg-btn--secondary"
                onClick={() => {
                  setImportSourceId(null);
                  setImportProposal(null);
                }}
              >
                <span>{BTN_CANCEL}</span>
              </button>
              <button
                type="button"
                className="fg-btn fg-btn--primary"
                onClick={() => void handleConfirmImportMore()}
                disabled={confirmingImport || selectedSkills.length === 0}
              >
                <span>{confirmingImport ? BTN_IMPORTING : BTN_IMPORT}</span>
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
