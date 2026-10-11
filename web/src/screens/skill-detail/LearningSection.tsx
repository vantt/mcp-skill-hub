import { useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { confirmSourceProposal, previewDetachSource } from '../../api/queries';
import type { LearningReference } from '../../api/types';
import { ConfirmDialog } from '../../components/ConfirmDialog';
import { StatusBadge } from '../../components/StatusBadge';

const TITLE_LEARNING = 'Learning references';
const LABEL_NO_LEARNING = 'No learning references yet.';
const INTRO_LEARNING =
  'A learning reference is a repository whose ideas should improve this skill. Skill Hub checks it for new commits; when it is ready, hand it to your curator agent from the Sources page. Use Add source above to link one.';
const LABEL_LAST_CHECKED = 'Last checked: ';
const BTN_UNLINK = 'Unlink';
const BTN_UNLINKING = 'Unlinking…';

interface LearningSectionProps {
  skillId: string;
  learning: LearningReference[];
}

export function LearningSection({ skillId, learning }: LearningSectionProps) {
  const queryClient = useQueryClient();
  const [unlinkingSourceId, setUnlinkingSourceId] = useState<string | null>(null);
  const [unlinking, setUnlinking] = useState(false);
  const [unlinkError, setUnlinkError] = useState<string | null>(null);

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
        <span>{TITLE_LEARNING}</span>
      </div>

      <p style={{ margin: 0, fontSize: '13px', color: 'var(--color-text-muted)', maxWidth: '72ch' }}>
        <span>{INTRO_LEARNING}</span>
      </p>

      {unlinkError && (
        <div className="fg-banner fg-banner--danger" style={{ fontSize: '13px' }}>
          <span>{unlinkError}</span>
        </div>
      )}

      {learning.length === 0 ? (
        <div style={{ color: 'var(--color-text-muted)', fontSize: '13px', padding: 'var(--space-2) 0' }}>
          <span>{LABEL_NO_LEARNING}</span>
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
                  <span style={{ fontSize: '12px', fontFamily: 'var(--font-mono)', color: 'var(--color-text-muted)' }}>
                    {ref.locator}
                  </span>
                  <div style={{ display: 'flex', gap: 'var(--space-3)', fontSize: '12px', color: 'var(--color-text-muted)', marginTop: '2px' }}>
                    <span>{LABEL_LAST_CHECKED}{lastChecked}</span>
                  </div>
                </div>

                <button
                  type="button"
                  className="fg-btn fg-btn--secondary fg-btn--small"
                  onClick={() => setUnlinkingSourceId(ref.source_id)}
                >
                  <span>{BTN_UNLINK}</span>
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
          confirmLabel={unlinking ? BTN_UNLINKING : BTN_UNLINK}
          danger
          onConfirm={() => void handleConfirmUnlink()}
          onCancel={() => setUnlinkingSourceId(null)}
        />
      )}
    </section>
  );
}
