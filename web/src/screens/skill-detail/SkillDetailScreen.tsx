import { useState } from 'react';
import { useParams, useSearchParams } from 'react-router';
import { useQueryClient } from '@tanstack/react-query';
import {
  confirmSkillMutation,
  previewSkillTransition,
  useSession,
  useSkillDetail,
  useSkillReview,
  useSkillSources,
} from '../../api/queries';
import type { SkillProposal } from '../../api/types';
import { ApiError } from '../../api/client';
import { ConfirmDialog } from '../../components/ConfirmDialog';
import { NotFoundPage } from '../../components/NotFoundPage';
import { ProposalPreview } from '../../components/ProposalPreview';
import { proposalPatch, proposalPaths, proposalWarning } from '../../api/proposal-view';
import { Skeleton } from '../../components/Skeleton';
import { StatusBadge } from '../../components/StatusBadge';
import { Toast } from '../../components/Toast';
import { EditorTab } from './EditorTab';
import { ResourcesTab } from './ResourcesTab';
import { ReviewTab } from './ReviewTab';
import { UsagePanel } from './UsagePanel';
import { RuntimeTab } from './RuntimeTab';
import { SourcesTab } from './SourcesTab';
import { DistillTab } from './DistillTab';
import { activationChecklist } from './activation-checklist';
import { useT } from '../../i18n';

const LABEL_TAB_REVIEW = 'Review';
const LABEL_TAB_EDITOR = 'Editor';
const LABEL_TAB_RESOURCES = 'Resources';
const LABEL_TAB_USAGE = 'Usage';
const LABEL_TAB_RUNTIME = 'Runtime';
const LABEL_TAB_SOURCES = 'Sources';
const LABEL_TAB_DISTILL = 'Distill';
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
const LABEL_GO_TO_FIELDS = 'Fill in';
const MSG_READY_TO_ACTIVATE =
  'Ready to activate. Once active, agents can select this skill. You review what changes before it is applied.';

const TRANSITION_COPY: Record<'active' | 'deprecated' | 'archived', { verb: string; impact: string }> = {
  active: {
    verb: 'Activate',
    impact: 'Agents will be able to find and use this skill. Its files do not change.',
  },
  deprecated: {
    verb: 'Deprecate',
    impact: 'Agents will stop choosing this skill for new work. Its files stay in Git.',
  },
  archived: {
    verb: 'Archive',
    impact: 'The skill is removed from agent routing and becomes read-only. Its files stay in Git.',
  },
};

// Splits `text` on backtick pairs so commands in a sentence show as code, not raw backticks.
function InlineCode({ text }: { text: string }) {
  return (
    <>
      {text.split('`').map((part, i) =>
        i % 2 === 1 ? (
          <code key={i} className="app-wrap" style={{ fontFamily: 'var(--font-mono)' }}>
            {part}
          </code>
        ) : (
          <span key={i}>{part}</span>
        ),
      )}
    </>
  );
}

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
  const { data: sourcesData } = useSkillSources(skillId);
  const hasSourcesUpdate =
    sourcesData?.upstream?.status === 'update_available' ||
    sourcesData?.upstream?.status === 'diverged';
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
    next.delete('field');
    setSearchParams(next);
  };

  // Opens the Editor with the cursor in the field the Review tab pointed at.
  const openEditor = (field?: string) => {
    const next = new URLSearchParams(searchParams);
    next.set('tab', 'editor');
    if (field) next.set('field', field);
    else next.delete('field');
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
  const missingItems = activationChecklist(skill, review).filter((item) => !item.valid);
  const isDraft = skill.lifecycle_state === 'draft';
  const missingLead = `${MSG_MISSING_REQ}: `;
  const missingList = `${missingItems.map((item) => item.label).join(', ')}.`;
  const headline = isDraft
    ? isReady
      ? MSG_READY_TO_ACTIVATE
      : MSG_MISSING_REQ
    : review?.next_action || 'Skill is up to date.';

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

          {review?.collection && (
            <span className="fg-chip fg-chip--neutral">
              <span>{review.collection}</span>
            </span>
          )}

          <StatusBadge variant="chip" label={t(`lifecycle_short.${skill.lifecycle_state}`)} tone={tone} />
        </div>

        <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 'var(--space-3)' }}>
          <span className="t-body-sm" style={{ color: 'var(--color-text-muted)', flex: '1 1 260px' }}>
            {isDraft && !isReady && !reviewLoading ? (
              <>
                <span style={{ color: 'var(--color-warning)', fontWeight: 600 }}>{missingLead}</span>
                <span>{missingList}</span>{' '}
                <button
                  type="button"
                  className="fg-btn fg-btn--ghost"
                  style={{ padding: '2px 6px', fontSize: '12px' }}
                  onClick={() => openEditor(missingItems[0]?.id)}
                >
                  <span>{LABEL_GO_TO_FIELDS}</span>
                </button>
              </>
            ) : (
              <InlineCode text={headline} />
            )}
          </span>

          {isDraft && (
            <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
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
      <div className="fg-tabs app-tabs" role="tablist" style={{ overflowX: 'auto' }}>
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
        <button
          type="button"
          className={`fg-tab ${activeTab === 'runtime' ? 'fg-tab--active' : ''}`}
          role="tab"
          aria-selected={activeTab === 'runtime'}
          onClick={() => setTab('runtime')}
        >
          <span>{LABEL_TAB_RUNTIME}</span>
        </button>
        <button
          type="button"
          className={`fg-tab ${activeTab === 'sources' ? 'fg-tab--active' : ''}`}
          role="tab"
          aria-selected={activeTab === 'sources'}
          aria-label={hasSourcesUpdate ? 'Sources, update available' : LABEL_TAB_SOURCES}
          onClick={() => setTab('sources')}
        >
          <span>{LABEL_TAB_SOURCES}</span>
          {hasSourcesUpdate && (
            <span
              className="fg-tab__badge"
              style={{
                width: '8px',
                height: '8px',
                borderRadius: '50%',
                backgroundColor: 'var(--color-warning)',
                display: 'inline-block',
                marginLeft: 'var(--space-1)',
              }}
              aria-hidden="true"
            />
          )}
        </button>
        <button
          type="button"
          className={`fg-tab ${activeTab === 'distill' ? 'fg-tab--active' : ''}`}
          role="tab"
          aria-selected={activeTab === 'distill'}
          onClick={() => setTab('distill')}
        >
          <span>{LABEL_TAB_DISTILL}</span>
        </button>
      </div>

      {/* Tab Panels */}
      {activeTab === 'review' && (
        <ReviewTab
          skill={skill}
          review={review}
          onGoToEditor={openEditor}
        />
      )}

      {activeTab === 'editor' && (
        <EditorTab skill={skill} workspaceId={workspaceId} focusField={searchParams.get('field')} />
      )}

      {activeTab === 'resources' && <ResourcesTab skill={skill} />}

      {activeTab === 'usage' && <UsagePanel skillId={skill.skill_id} />}

      {activeTab === 'runtime' && (
        <RuntimeTab skillId={skill.skill_id} runtimeHints={review?.runtime_hints} />
      )}
      {activeTab === 'sources' && (
        <SourcesTab skillId={skill.skill_id} />
      )}
      {activeTab === 'distill' && (
        <DistillTab skillId={skill.skill_id} />
      )}
      {/* Transition Proposal Preview */}
      {proposalOpen && transitionProposal && (
        <ProposalPreview
          open={proposalOpen}
          title={
            transitionTarget
              ? `${TRANSITION_COPY[transitionTarget].verb} ${skill.skill_id}?`
              : transitionProposal.summary || 'Skill transition'
          }
          target={skill.skill_id}
          fromState={skill.lifecycle_state}
          toState={transitionTarget || undefined}
          impact={transitionTarget ? TRANSITION_COPY[transitionTarget].impact : undefined}
          paths={proposalPaths(transitionProposal)}
          warning={proposalWarning(transitionProposal)}
          diff={proposalPatch(transitionProposal)}
          proposalId={pins?.proposal_id || ''}
          proposalDigest={pins?.proposal_digest || ''}
          baseVersion={pins?.base_version || ''}
          confirmLabel={transitionTarget ? `${TRANSITION_COPY[transitionTarget].verb} skill` : 'Confirm'}
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
