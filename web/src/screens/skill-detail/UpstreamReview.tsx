import { useEffect, useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { ApiError } from '../../api/client';
import { confirmUpstreamUpdate, reviewSkillUpdate } from '../../api/queries';
import type { UpstreamFile, UpstreamResolution, UpstreamUpdatePreview } from '../../api/types';
import { DiffView } from '../../components/DiffView';
import { Skeleton } from '../../components/Skeleton';

interface UpstreamReviewProps {
  skillId: string;
  onClose: () => void;
  onConfirmed?: () => void;
}

export function UpstreamReview({ skillId, onClose, onConfirmed }: UpstreamReviewProps) {
  const queryClient = useQueryClient();
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [isConflict409, setIsConflict409] = useState(false);
  const [preview, setPreview] = useState<UpstreamUpdatePreview | null>(null);

  const [resolutions, setResolutions] = useState<Record<string, { action: string; content?: string }>>({});
  const [selectedFilePath, setSelectedFilePath] = useState<string>('');
  const [diffMode, setDiffMode] = useState<'result' | 'upstream' | 'local'>('result');
  const [applying, setApplying] = useState(false);
  const [confirming, setConfirming] = useState(false);

  const loadReview = async (customResolutions?: UpstreamResolution[]) => {
    setLoading(true);
    setError(null);
    setIsConflict409(false);
    try {
      const data = await reviewSkillUpdate(skillId, {
        resolutions: customResolutions,
      });
      setPreview(data);
      const firstFile = data.files[0];
      if (!selectedFilePath && firstFile) {
        setSelectedFilePath(firstFile.path);
      }
      // Populate default resolutions from preview
      const initial: Record<string, { action: string; content?: string }> = {};
      for (const f of data.files) {
        initial[f.path] = {
          action: f.action || f.default_action || 'upstream',
          content: f.merged_with_markers || '',
        };
      }
      setResolutions(initial);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : 'Failed to load update review';
      setError(msg);
      if (err instanceof ApiError && err.status === 409) {
        setIsConflict409(true);
      }
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void loadReview();
  }, [skillId]);

  const handleApplyChoices = async () => {
    setApplying(true);
    setError(null);
    try {
      const resList: UpstreamResolution[] = Object.entries(resolutions).map(([p, r]) => ({
        path: p,
        action: r.action,
        content: r.action === 'manual' ? r.content : undefined,
      }));
      const data = await reviewSkillUpdate(skillId, { resolutions: resList });
      setPreview(data);
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : 'Failed to apply choices');
    } finally {
      setApplying(false);
    }
  };

  const handleConfirmUpdate = async () => {
    if (!preview?.confirmation?.confirmation?.pins) return;
    setConfirming(true);
    setError(null);
    const pins = preview.confirmation.confirmation.pins;
    try {
      await confirmUpstreamUpdate(pins.proposal_id, {
        proposal_digest: pins.proposal_digest,
        base_version: pins.base_version,
      });
      // Invalidate queries including skill-runtime
      queryClient.invalidateQueries({ queryKey: ['skill-sources', skillId] });
      queryClient.invalidateQueries({ queryKey: ['skill-runtime', skillId] });
      queryClient.invalidateQueries({ queryKey: ['skill', skillId] });
      queryClient.invalidateQueries({ queryKey: ['skill-review', skillId] });
      queryClient.invalidateQueries({ queryKey: ['skills'] });
      onConfirmed?.();
      onClose();
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : 'Failed to confirm update');
    } finally {
      setConfirming(false);
    }
  };

  const formatChange = (status: string): string => {
    switch (status) {
      case 'upstream_only':
        return 'changed upstream';
      case 'local_only':
        return 'changed here';
      case 'both_changed':
        return 'changed here and upstream';
      case 'added_upstream':
        return 'added upstream';
      case 'added_local':
        return 'added here';
      case 'removed_upstream':
        return 'removed upstream';
      case 'removed_local':
        return 'removed here';
      case 'blocked':
        return 'blocked (contains conflict markers)';
      default:
        return status;
    }
  };

  if (loading) {
    return (
      <section className="fg-card" style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' }}>
        <Skeleton width="40%" height="24px" />
        <Skeleton height="150px" />
      </section>
    );
  }

  if (error) {
    return (
      <section className="fg-card" style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' }}>
        <div className="fg-banner fg-banner--danger">
          <span>{error}</span>
        </div>
        <div style={{ display: 'flex', gap: 'var(--space-2)' }}>
          {isConflict409 && (
            <button type="button" className="fg-btn fg-btn--primary" onClick={() => void loadReview()}>
              <span>Review again</span>
            </button>
          )}
          <button type="button" className="fg-btn fg-btn--secondary" onClick={onClose}>
            <span>Close</span>
          </button>
        </div>
      </section>
    );
  }

  if (!preview || preview.files.length === 0) {
    return (
      <section className="fg-card" style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' }}>
        <div className="fg-card__title">
          <span>Upstream is unchanged.</span>
        </div>
        <button type="button" className="fg-btn fg-btn--secondary" onClick={onClose}>
          <span>Close</span>
        </button>
      </section>
    );
  }

  const selectedFile: UpstreamFile =
    preview.files.find((f: UpstreamFile) => f.path === selectedFilePath) || preview.files[0]!;
  const selectedDiff =
    diffMode === 'upstream'
      ? selectedFile?.upstream_diff || ''
      : diffMode === 'local'
        ? selectedFile?.local_diff || ''
        : selectedFile?.result_diff || '';

  const hasUnresolved = preview.unresolved && preview.unresolved.length > 0;
  const commitDate = preview.target_committed_at ? preview.target_committed_at.slice(0, 10) : '';

  return (
    <section className="fg-card" style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: 'var(--space-2)' }}>
        <div>
          <h3 style={{ margin: 0, fontSize: '15px', fontWeight: 600 }}>
            Update {skillId} from {preview.base_commit?.slice(0, 7) || '-'} to {preview.target_commit?.slice(0, 7) || '-'}
            {commitDate ? `, upstream commit of ${commitDate}` : ''}
          </h3>
          <span style={{ fontSize: '12px', color: 'var(--color-text-subtle)' }}>
            Source: {preview.source_id}
          </span>
        </div>
        <button type="button" className="fg-btn fg-btn--secondary fg-btn--small" onClick={onClose}>
          <span>Close</span>
        </button>
      </div>

      {hasUnresolved && (
        <div className="fg-banner fg-banner--warning">
          <span>{preview.unresolved.length} file(s) need a decision before this update can be applied.</span>
        </div>
      )}

      {/* Files Table */}
      <div style={{ overflowX: 'auto' }}>
        <table className="fg-table" style={{ width: '100%', fontSize: '13px' }}>
          <thead>
            <tr>
              <th style={{ textAlign: 'left' }}>FILE</th>
              <th style={{ textAlign: 'left' }}>CHANGE</th>
              <th style={{ textAlign: 'left' }}>ACTION</th>
            </tr>
          </thead>
          <tbody>
            {preview.files.map((file: UpstreamFile) => {
              const currentRes = resolutions[file.path] || { action: file.action || file.default_action || 'upstream' };
              const isSelected = file.path === selectedFile?.path;
              return (
                <tr
                  key={file.path}
                  onClick={() => setSelectedFilePath(file.path)}
                  style={{
                    cursor: 'pointer',
                    backgroundColor: isSelected ? 'var(--color-surface-hover)' : undefined,
                  }}
                >
                  <td style={{ fontFamily: 'var(--font-mono)' }}>{file.path}</td>
                  <td>{formatChange(file.status)}</td>
                  <td onClick={(e) => e.stopPropagation()}>
                    <select
                      className="fg-select"
                      style={{ fontSize: '12px', padding: '2px 8px' }}
                      value={currentRes.action}
                      onChange={(e) => {
                        const newAction = e.target.value;
                        setResolutions((prev) => ({
                          ...prev,
                          [file.path]: {
                            ...prev[file.path],
                            action: newAction,
                          },
                        }));
                      }}
                    >
                      <option value="upstream">Take upstream</option>
                      <option value="local">Keep mine</option>
                      {file.conflicts === 0 && <option value="merged">Auto-merged</option>}
                      <option value="manual">Edit manually</option>
                    </select>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>

      <div style={{ display: 'flex', gap: 'var(--space-2)' }}>
        <button
          type="button"
          className="fg-btn fg-btn--secondary"
          onClick={() => void handleApplyChoices()}
          disabled={applying}
        >
          <span>{applying ? 'Applying choices…' : 'Apply choices'}</span>
        </button>
      </div>

      {/* Selected File Diff / Manual Editor */}
      {selectedFile && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-2)' }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <span style={{ fontWeight: 600, fontSize: '13px', fontFamily: 'var(--font-mono)' }}>
              {selectedFile.path}
            </span>
            <div style={{ display: 'flex', gap: 'var(--space-1)' }}>
              <button
                type="button"
                className={`fg-btn fg-btn--small ${diffMode === 'result' ? 'fg-btn--primary' : 'fg-btn--secondary'}`}
                onClick={() => setDiffMode('result')}
              >
                <span>Result diff</span>
              </button>
              <button
                type="button"
                className={`fg-btn fg-btn--small ${diffMode === 'upstream' ? 'fg-btn--primary' : 'fg-btn--secondary'}`}
                onClick={() => setDiffMode('upstream')}
              >
                <span>Upstream diff</span>
              </button>
              <button
                type="button"
                className={`fg-btn fg-btn--small ${diffMode === 'local' ? 'fg-btn--primary' : 'fg-btn--secondary'}`}
                onClick={() => setDiffMode('local')}
              >
                <span>Local diff</span>
              </button>
            </div>
          </div>

          {resolutions[selectedFile.path]?.action === 'manual' ? (
            <textarea
              className="fg-textarea"
              style={{
                fontFamily: 'var(--font-mono)',
                fontSize: '12px',
                minHeight: '200px',
                width: '100%',
                boxSizing: 'border-box',
              }}
              value={resolutions[selectedFile.path]?.content || ''}
              onChange={(e) => {
                const text = e.target.value;
                setResolutions((prev) => ({
                  ...prev,
                  [selectedFile.path]: {
                    action: prev[selectedFile.path]?.action || 'manual',
                    content: text,
                  },
                }));
              }}
            />
          ) : (
            <DiffView diff={selectedDiff} />
          )}
        </div>
      )}

      {/* Trust Notice and Confirm Footer */}
      {!hasUnresolved && preview.confirmation?.confirmation?.pins && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)', marginTop: 'var(--space-2)' }}>
          {preview.trust_impact?.review_required_after_apply && (
            <div className="fg-banner fg-banner--warning">
              <span>
                After applying, agents cannot use {skillId} until you approve the new content. Run skillhub skill review {skillId} after applying.
              </span>
            </div>
          )}
          <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 'var(--space-2)' }}>
            <button
              type="button"
              className="fg-btn fg-btn--primary"
              onClick={() => void handleConfirmUpdate()}
              disabled={confirming}
            >
              <span>{confirming ? 'Confirming…' : 'Confirm update'}</span>
            </button>
          </div>
        </div>
      )}
    </section>
  );
}
