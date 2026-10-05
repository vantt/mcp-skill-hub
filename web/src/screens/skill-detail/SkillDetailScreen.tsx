import { useState } from 'react';
import { useParams, useSearchParams } from 'react-router';
import { useQueryClient } from '@tanstack/react-query';
import {
  confirmSkillMutation,
  previewSkillTransition,
  useSession,
  useSkillDetail,
  useSkillReview,
} from '../../api/queries';
import type { SkillProposal } from '../../api/types';
import { ApiError } from '../../api/client';
import { ConfirmDialog } from '../../components/ConfirmDialog';
import { NotFoundPage } from '../../components/NotFoundPage';
import { ProposalPreview } from '../../components/ProposalPreview';
import { Skeleton } from '../../components/Skeleton';
import { StatusBadge } from '../../components/StatusBadge';
import { Toast } from '../../components/Toast';
import { EditorTab } from './EditorTab';
import { ResourcesTab } from './ResourcesTab';
import { ReviewTab } from './ReviewTab';
import { UsagePanel } from './UsagePanel';
import { useT } from '../../i18n';

const LABEL_TAB_REVIEW = 'Review';
const LABEL_TAB_EDITOR = 'Editor';
const LABEL_TAB_RESOURCES = 'Resources';
const LABEL_TAB_USAGE = 'Usage';
const LABEL_ACTIVATE = 'Activate skill';
const LABEL_DEPRECATE = 'Deprecate';
const LABEL_ARCHIVE = 'Archive';
const LABEL_ARCHIVE_SKILL = 'Archive skill';
const LABEL_COPIED = 'Copied';
const LABEL_ARCHIVED_READONLY = 'Read-only — no transitions from archived.';
const TITLE_ARCHIVE_CONFIRM = 'Archive skill?';
const BODY_ARCHIVE_CONFIRM =
  'Archiving removes this skill from agent routing. Agents will no longer be able to select or invoke it.';
const MSG_MISSING_REQ = 'Missing activation requirements';

export function SkillDetailScreen() {
  const t = useT();
  const queryClient = useQueryClient();
  const { id } = useParams<{ id: string }>();
  const [searchParams, setSearchParams] = useSearchParams();

  const activeTab = searchParams.get('tab') || 'review';
  const skillId = id || '';

  const { data: session } = useSession();
  const workspaceId = session?.workspace_id || 'default';

  const { data: skill, isLoading: skillLoading, error: skillError } = useSkillDetail(skillId);
  const { data: review, isLoading: reviewLoading } = useSkillReview(skillId);

  const [copiedId, setCopiedId] = useState(false);
  const [transitionProposal, setTransitionProposal] = useState<SkillProposal | null>(null);
  const [transitionTarget, setTransitionTarget] = useState<'active' | 'deprecated' | 'archived' | ''>('');
  const [proposalOpen, setProposalOpen] = useState(false);
  const [confirmArchiveOpen, setConfirmArchiveOpen] = useState(false);
  const [toastMessage, setToastMessage] = useState<string | null>(null);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);

  if (skillError instanceof ApiError && skillError.status === 404) {
    return <NotFoundPage />;
  }

  if (skillLoading || !skill) {
    return (
      <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
        <Skeleton width="40%" height="32px" />
        <Skeleton width="60%" />
        <Skeleton height="300px" />
      </div>
    );
  }

  const setTab = (tab: string) => {
    const next = new URLSearchParams(searchParams);
    next.set('tab', tab);
    setSearchParams(next);
  };

  const handleCopyId = async () => {
    try {
      await navigator.clipboard.writeText(skill.skill_id);
      setCopiedId(true);
      setTimeout(() => setCopiedId(false), 2000);
    } catch {
      // Ignore clipboard error
    }
  };

  const handleTransition = async (target: 'active' | 'deprecated' | 'archived') => {
    setErrorMessage(null);
    setTransitionTarget(target);
    try {
      const prop = await previewSkillTransition(skill.skill_id, target);
      setTransitionProposal(prop);
      setProposalOpen(true);
    } catch (err) {
      if (err instanceof ApiError) {
        setErrorMessage(err.render.WHY || err.render.ERROR || err.message);
      } else if (err instanceof Error) {
        setErrorMessage(err.message);
      }
    }
  };

  const handleConfirmTransition = async (pins: {
    proposalId: string;
    proposalDigest: string;
    baseVersion: string;
  }) => {
    try {
      const res = await confirmSkillMutation(pins.proposalId, pins);
      setProposalOpen(false);
      setToastMessage(`Transitioned · ${res.operation_id}`);
      queryClient.invalidateQueries({ queryKey: ['skill', skill.skill_id] });
      queryClient.invalidateQueries({ queryKey: ['skill-review', skill.skill_id] });
      queryClient.invalidateQueries({ queryKey: ['skills'] });
      queryClient.invalidateQueries({ queryKey: ['home'] });
    } catch (err) {
      if (err instanceof ApiError) {
        setErrorMessage(err.render.WHY || err.render.ERROR || err.message);
      } else if (err instanceof Error) {
        setErrorMessage(err.message);
      }
    }
  };

  const readiness = review?.activation_readiness;
  const isReady = readiness?.ready ?? false;

  const tone =
    skill.lifecycle_state === 'active'
      ? 'success'
      : skill.lifecycle_state === 'draft'
        ? 'warning'
        : skill.lifecycle_state === 'deprecated'
          ? 'danger'
          : 'neutral';

  const pins =
    transitionProposal?.confirmation?.confirmation?.pins ?? transitionProposal?.confirmation?.pins ?? null;

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
      {errorMessage && (
        <div className="fg-banner fg-banner--danger" role="alert">
          <span className="fg-banner__dot" />
          <span className="fg-banner__body">
            <span>{errorMessage}</span>
          </span>
        </div>
      )}

      {/* Header */}
      <section style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' }}>
        <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 'var(--space-2) var(--space-3)' }}>
          <span className="t-title" style={{ fontSize: '24px', fontWeight: 600 }}>
            <span>{skill.name}</span>
          </span>

          <button
            type="button"
            onClick={handleCopyId}
            title="Copy ID"
            style={{
              all: 'unset',
              cursor: 'pointer',
              fontFamily: 'var(--font-mono)',
              fontSize: '13px',
              color: 'var(--color-text-muted)',
              padding: '2px 6px',
              border: '1px solid var(--color-border)',
              borderRadius: 'var(--radius-xs)',
            }}
          >
            <span>{skill.skill_id}</span>
            <span> </span>
            <span aria-hidden="true">{copiedId ? LABEL_COPIED : '⧉'}</span>
          </button>

          <span className="fg-chip fg-chip--neutral">
            <span>{skill.routing?.operations?.[0] || 'core'}</span>
          </span>

          <StatusBadge variant="chip" label={t(`lifecycle_short.${skill.lifecycle_state}`)} tone={tone} />
        </div>

        <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 'var(--space-3)' }}>
          <span className="t-body-sm" style={{ color: 'var(--color-text-muted)', flex: '1 1 260px' }}>
            <span>{review?.next_action || 'Skill is up to date.'}</span>
          </span>

          {skill.lifecycle_state === 'draft' && (
            <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
              {!isReady && !reviewLoading && (
                <span className="t-caption" style={{ color: 'var(--color-warning)' }}>
                  <span>{MSG_MISSING_REQ}</span>
                </span>
              )}
              <button
                type="button"
                className="fg-btn fg-btn--primary"
                disabled={!isReady || reviewLoading}
                onClick={() => handleTransition('active')}
              >
                <span>{LABEL_ACTIVATE}</span>
              </button>
            </div>
          )}

          {skill.lifecycle_state === 'active' && (
            <button
              type="button"
              className="fg-btn fg-btn--secondary"
              onClick={() => handleTransition('deprecated')}
            >
              <span>{LABEL_DEPRECATE}</span>
            </button>
          )}

          {skill.lifecycle_state === 'deprecated' && (
            <button
              type="button"
              className="fg-btn fg-btn--danger"
              onClick={() => setConfirmArchiveOpen(true)}
            >
              <span>{LABEL_ARCHIVE}</span>
            </button>
          )}

          {skill.lifecycle_state === 'archived' && (
            <span className="t-caption" style={{ color: 'var(--color-text-subtle)' }}>
              <span>{LABEL_ARCHIVED_READONLY}</span>
            </span>
          )}
        </div>
      </section>

      {/* Tabs */}
      <div className="fg-tabs" role="tablist" style={{ overflowX: 'auto' }}>
        <button
          type="button"
          className={`fg-tab ${activeTab === 'review' ? 'fg-tab--active' : ''}`}
          role="tab"
          aria-selected={activeTab === 'review'}
          onClick={() => setTab('review')}
        >
          <span>{LABEL_TAB_REVIEW}</span>
        </button>
        <button
          type="button"
          className={`fg-tab ${activeTab === 'editor' ? 'fg-tab--active' : ''}`}
          role="tab"
          aria-selected={activeTab === 'editor'}
          onClick={() => setTab('editor')}
        >
          <span>{LABEL_TAB_EDITOR}</span>
        </button>
        <button
          type="button"
          className={`fg-tab ${activeTab === 'resources' ? 'fg-tab--active' : ''}`}
          role="tab"
          aria-selected={activeTab === 'resources'}
          onClick={() => setTab('resources')}
        >
          <span>{LABEL_TAB_RESOURCES}</span>
        </button>
        <button
          type="button"
          className={`fg-tab ${activeTab === 'usage' ? 'fg-tab--active' : ''}`}
          role="tab"
          aria-selected={activeTab === 'usage'}
          onClick={() => setTab('usage')}
        >
          <span>{LABEL_TAB_USAGE}</span>
        </button>
      </div>

      {/* Tab Panels */}
      {activeTab === 'review' && (
        <ReviewTab
          skill={skill}
          review={review}
          onGoToEditor={() => {
            setTab('editor');
          }}
        />
      )}

      {activeTab === 'editor' && (
        <EditorTab skill={skill} workspaceId={workspaceId} />
      )}

      {activeTab === 'resources' && <ResourcesTab skill={skill} />}

      {activeTab === 'usage' && <UsagePanel skillId={skill.skill_id} />}
      {/* Transition Proposal Preview */}
      {proposalOpen && transitionProposal && (
        <ProposalPreview
          open={proposalOpen}
          title={transitionProposal.summary || 'Skill Transition'}
          target={skill.skill_id}
          fromState={transitionProposal.from_state || skill.lifecycle_state}
          toState={transitionTarget || undefined}
          impact={transitionProposal.impact}
          warning={transitionProposal.warning}
          diff={transitionProposal.diff}
          stat={transitionProposal.stat}
          proposalId={pins?.proposal_id || ''}
          proposalDigest={pins?.proposal_digest || ''}
          baseVersion={pins?.base_version || ''}
          confirmLabel={`Confirm ${transitionTarget || 'transition'}`}
          dangerConfirm={transitionTarget === 'archived'}
          onConfirm={handleConfirmTransition}
          onCancel={() => setProposalOpen(false)}
        />
      )}

      {/* Archive Confirmation Dialog */}
      <ConfirmDialog
        open={confirmArchiveOpen}
        title={TITLE_ARCHIVE_CONFIRM}
        body={BODY_ARCHIVE_CONFIRM}
        confirmLabel={LABEL_ARCHIVE_SKILL}
        danger
        onConfirm={() => {
          setConfirmArchiveOpen(false);
          handleTransition('archived');
        }}
        onCancel={() => setConfirmArchiveOpen(false)}
      />

      <Toast message={toastMessage} onClose={() => setToastMessage(null)} />
    </div>
  );
}
