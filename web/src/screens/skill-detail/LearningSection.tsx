import { useState } from 'react';
import { Link } from 'react-router';
import { useQueryClient } from '@tanstack/react-query';
import {
  confirmSourceProposal,
  previewAttachSource,
  previewDetachSource,
} from '../../api/queries';
import type { LearningReference } from '../../api/types';
import { ConfirmDialog } from '../../components/ConfirmDialog';
import { StatusBadge } from '../../components/StatusBadge';

interface LearningSectionProps {
  skillId: string;
  learning: LearningReference[];
}

export function LearningSection({ skillId, learning }: LearningSectionProps) {
  const queryClient = useQueryClient();
  const [locator, setLocator] = useState('');
  const [adding, setAdding] = useState(false);
  const [addError, setAddError] = useState<string | null>(null);

  const [unlinkingSourceId, setUnlinkingSourceId] = useState<string | null>(null);
  const [unlinking, setUnlinking] = useState(false);
  const [unlinkError, setUnlinkError] = useState<string | null>(null);

  const handleAddReference = async (e: React.FormEvent) => {
    e.preventDefault();
    const trimmed = locator.trim();
    if (!trimmed) return;

    setAdding(true);
    setAddError(null);
    try {
      const prop = await previewAttachSource(skillId, { locator: trimmed });
      const pins = prop.confirmation?.confirmation?.pins;
      if (pins) {
        await confirmSourceProposal(pins.proposal_id, {
          proposal_digest: pins.proposal_digest,
          base_version: pins.base_version,
        });
      }
      setLocator('');
      queryClient.invalidateQueries({ queryKey: ['skill-sources', skillId] });
    } catch (err: unknown) {
      setAddError(err instanceof Error ? err.message : 'Failed to attach source');
    } finally {
      setAdding(false);
    }
  };

  const handleConfirmUnlink = async () => {
    if (!unlinkingSourceId) return;
    setUnlinking(true);
    setUnlinkError(null);
    try {
      const prop = await previewDetachSource(skillId, unlinkingSourceId);
      const pins = prop.confirmation?.confirmation?.pins;
      if (pins) {
        await confirmSourceProposal(pins.proposal_id, {
          proposal_digest: pins.proposal_digest,
          base_version: pins.base_version,
        });
      }
      queryClient.invalidateQueries({ queryKey: ['skill-sources', skillId] });
      setUnlinkingSourceId(null);
    } catch (err: unknown) {
      setUnlinkError(err instanceof Error ? err.message : 'Failed to detach source');
    } finally {
      setUnlinking(false);
    }
  };

  return (
    <section className="fg-card" style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' }}>
      <div className="fg-card__title">
        <span>Learning references</span>
      </div>

      {/* Add Reference Form */}
      <form onSubmit={handleAddReference} style={{ display: 'flex', gap: 'var(--space-2)', flexWrap: 'wrap' }}>
        <input
          type="text"
          className="fg-input"
          style={{ flex: 1, minWidth: '240px' }}
          placeholder="https://github.com/owner/repo"
          value={locator}
          onChange={(e) => setLocator(e.target.value)}
          disabled={adding}
        />
        <button
          type="submit"
          className="fg-btn fg-btn--secondary"
          disabled={adding || !locator.trim()}
        >
          <span>{adding ? 'Adding…' : 'Add learning reference'}</span>
        </button>
      </form>

      {addError && (
        <div className="fg-banner fg-banner--danger" style={{ fontSize: '13px' }}>
          <span>{addError}</span>
        </div>
      )}
      {unlinkError && (
        <div className="fg-banner fg-banner--danger" style={{ fontSize: '13px' }}>
          <span>{unlinkError}</span>
        </div>
      )}

      {learning.length === 0 ? (
        <div style={{ color: 'var(--color-text-subtle)', fontSize: '13px', padding: 'var(--space-2) 0' }}>
          <span>No learning references yet. Link a repository or document whose ideas should improve this skill.</span>
        </div>
      ) : (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-2)' }}>
          {learning.map((ref) => {
            const lastChecked = ref.last_checked_at ? ref.last_checked_at.slice(0, 10) : 'never';
            return (
              <div
                key={ref.source_id}
                style={{
                  display: 'flex',
                  justifyContent: 'space-between',
                  alignItems: 'center',
                  padding: 'var(--space-2)',
                  border: '1px solid var(--color-border)',
                  borderRadius: 'var(--radius-sm)',
                  flexWrap: 'wrap',
                  gap: 'var(--space-2)',
                }}
              >
                <div style={{ display: 'flex', flexDirection: 'column', gap: '2px' }}>
                  <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-2)' }}>
                    <span style={{ fontWeight: 600, fontSize: '13px' }}>{ref.source_id}</span>
                    <StatusBadge variant="chip" tone="neutral" label={ref.role} />
                    {ref.availability === 'unavailable' && (
                      <StatusBadge variant="chip" tone="danger" label="Unavailable" />
                    )}
                  </div>
                  <span style={{ fontSize: '12px', fontFamily: 'var(--font-mono)', color: 'var(--color-text-subtle)' }}>
                    {ref.locator}
                  </span>
                  <div style={{ display: 'flex', gap: 'var(--space-3)', fontSize: '12px', color: 'var(--color-text-subtle)', marginTop: '2px' }}>
                    <span>Last checked: {lastChecked}</span>
                    {ref.pending_insights > 0 && (
                      <Link
                        to="/inbox"
                        style={{ color: 'var(--color-primary)', textDecoration: 'underline' }}
                      >
                        {ref.pending_insights} pending insight(s)
                      </Link>
                    )}
                  </div>
                </div>

                <button
                  type="button"
                  className="fg-btn fg-btn--secondary fg-btn--small"
                  onClick={() => setUnlinkingSourceId(ref.source_id)}
                >
                  <span>Unlink</span>
                </button>
              </div>
            );
          })}
        </div>
      )}

      {unlinkingSourceId && (
        <ConfirmDialog
          open={Boolean(unlinkingSourceId)}
          title={`Unlink ${unlinkingSourceId}`}
          body={`Are you sure you want to unlink ${unlinkingSourceId} from this skill?`}
          confirmLabel={unlinking ? 'Unlinking…' : 'Unlink'}
          danger
          onConfirm={() => void handleConfirmUnlink()}
          onCancel={() => setUnlinkingSourceId(null)}
        />
      )}
    </section>
  );
}
