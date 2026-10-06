import { useState } from 'react';
import { Link, useNavigate, useParams } from 'react-router';
import { useQueryClient } from '@tanstack/react-query';
import {
  confirmInsightApply,
  previewInsightApply,
  useInsightDetail,
  useSession,
  useSkillDetail,
} from '../../api/queries';
import type {
  ConfirmationPins,
  InsightApplicationPreview,
  InsightApplicationResult,
  InsightDetailResult,
  SkillDetail,
} from '../../api/types';
import { ConflictDrawer } from '../../components/ConflictDrawer';
import { ProposalPreview } from '../../components/ProposalPreview';
import { Skeleton } from '../../components/Skeleton';
import { StatusBadge } from '../../components/StatusBadge';
import {
  addMapping,
  coverage,
  requiredObservations,
  type ConceptMapping,
} from '../../domain/evidence-set';
import {
  clearComposerDraft,
  loadComposerDraft,
  saveComposerDraft,
} from '../../state/composer-draft';

const TITLE_COMPOSER = 'Compose patch';
const LABEL_TARGET_FILE = 'Target file: ';
const LABEL_NOT_FOUND_TITLE = 'Insight not found';
const BTN_BACK_INBOX = 'Back to Inbox';
const BTN_RETRY = 'Retry';
const LABEL_EVIDENCE_MAPPING = 'Evidence mapping';
const LABEL_SAVE_LOCAL_DRAFT = 'Save local draft';
const LABEL_DRAFT_SAVED = 'Saved';
const LABEL_CANCEL = 'Cancel';
const BTN_PREVIEW_APPLY = 'Preview apply';
const BTN_PREVIEWING = 'Preparing preview…';
const LABEL_MAPPED_PREFIX = 'Mapped ';
const LABEL_MAPPED_OF = ' of ';
const LABEL_MAPPED_REQUIRED = ' required';
const LABEL_REASON_UNCHANGED = 'Change the skill content before previewing.';
const LABEL_REASON_UNMAPPED = 'Map all required observations before previewing.';
const LABEL_REBASE_WARNING =
  'Skill content was modified outside this draft. Please review conflicts before applying.';
const LABEL_STALE_EVIDENCE_WARNING =
  'Supporting evidence was updated since this draft was created.';
const LABEL_ADD_CONCEPT = '+ Add concept';
const LABEL_CONCEPT_PLACEHOLDER = 'Concept, e.g. retry limits';
const LABEL_APPLIED_SUCCESS = 'Insight patch applied successfully!';
const LABEL_APPLIED_DESC = 'The skill instructions have been updated with your changes.';
const BTN_VIEW_SKILL_REVIEW = 'View Skill Review →';
const LABEL_TAB_CONTENT = 'Content';
const LABEL_TAB_MAPPING = 'Mapping';
const LABEL_SLASH = '/';
const LABEL_LPAREN = ' (';
const LABEL_RPAREN = ')';
const CHAR_CLOSE = '×';

interface PatchComposerEditorProps {
  insightData: InsightDetailResult;
  skillData: SkillDetail;
  workspaceId: string;
}

function PatchComposerEditor({ insightData, skillData, workspaceId }: PatchComposerEditorProps) {
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const insightId = insightData.insight.id;
  const skillId = skillData.skill_id;

  const currentContentDigest = skillData.content_digest || '';
  const currentEvidenceDigest = insightData.insight.evidence_digest || '';

  const initialDraft = loadComposerDraft(
    workspaceId,
    insightId,
    currentContentDigest,
    currentEvidenceDigest,
  );

  const [content, setContent] = useState(
    initialDraft ? initialDraft.data.content : (skillData.content || ''),
  );
  const [baselineContentDigest, setBaselineContentDigest] = useState(
    initialDraft ? initialDraft.data.contentDigest : currentContentDigest,
  );
  const [mappings, setMappings] = useState<ConceptMapping[]>(
    initialDraft ? initialDraft.data.mappings || [] : [],
  );
  const [newConceptInputs, setNewConceptInputs] = useState<Record<string, string>>({});
  const [draftSavedToast, setDraftSavedToast] = useState(false);
  const [needsRebase, setNeedsRebase] = useState(Boolean(initialDraft?.needsRebase));
  const [staleEvidence] = useState(Boolean(initialDraft?.staleEvidence));

  // ConflictDrawer state
  const [conflictOpen, setConflictOpen] = useState(false);
  const [conflictLatestContent, setConflictLatestContent] = useState('');
  const [conflictLatestDigest, setConflictLatestDigest] = useState('');

  // ProposalPreview state
  const [previewProposal, setPreviewProposal] = useState<InsightApplicationPreview | null>(null);
  const [previewLoading, setPreviewLoading] = useState(false);
  const [previewError, setPreviewError] = useState<string | null>(null);
  const [confirmLoading, setConfirmLoading] = useState(false);
  const [confirmError, setConfirmError] = useState<string | null>(null);
  const [appliedReceipt, setAppliedReceipt] = useState<InsightApplicationResult | null>(null);

  // Mobile / Narrow tabs
  const [activeTab, setActiveTab] = useState<'content' | 'mapping'>('content');

  const { refetch: refetchSkill } = useSkillDetail(skillId);

  const targetPath = skillData.path || `skills/${skillData.skill_id}/SKILL.md`;
  const required = requiredObservations(insightData);
  const cov = coverage(mappings, required);

  // Content diff check with normalized trailing newline
  const normCurrent = (skillData.content || '').replace(/\r\n/g, '\n').trimEnd() + '\n';
  const normEdited = content.replace(/\r\n/g, '\n').trimEnd() + '\n';
  const isContentChanged = normCurrent !== normEdited;
  const isCoverageComplete = cov.unmapped.length === 0;
  const canPreview = isContentChanged && isCoverageComplete;

  const previewDisabledReason = !isContentChanged
    ? LABEL_REASON_UNCHANGED
    : !isCoverageComplete
      ? LABEL_REASON_UNMAPPED
      : undefined;

  const handleManualSaveDraft = () => {
    saveComposerDraft(workspaceId, insightId, {
      content,
      mappings,
      contentDigest: baselineContentDigest,
      evidenceDigest: insightData.insight.evidence_digest || '',
    });
    setDraftSavedToast(true);
    setTimeout(() => setDraftSavedToast(false), 2000);
  };

  const handleAddConcept = (obsId: string) => {
    const conceptText = (newConceptInputs[obsId] || '').trim();
    if (!conceptText) return;
    setMappings((prev) => addMapping(prev, { observation_id: obsId, concept: conceptText }));
    setNewConceptInputs((prev) => ({ ...prev, [obsId]: '' }));
  };

  const handleRemoveConcept = (obsId: string, concept: string) => {
    setMappings((prev) =>
      prev.filter(
        (m) =>
          !(m.observation_id.trim() === obsId.trim() && m.concept.trim().toLowerCase() === concept.trim().toLowerCase()),
      ),
    );
  };

  const handlePreviewApply = async () => {
    if (!canPreview) return;
    setPreviewLoading(true);
    setPreviewError(null);

    try {
      // 1. Refetch skill detail and compare content_digest
      const refreshedSkill = await refetchSkill();
      const currentDigest = refreshedSkill.data?.content_digest || '';
      if (currentDigest && baselineContentDigest && currentDigest !== baselineContentDigest) {
        setConflictLatestContent(refreshedSkill.data?.content || '');
        setConflictLatestDigest(currentDigest);
        setConflictOpen(true);
        setPreviewLoading(false);
        return;
      }

      // 2. Request application preview
      const preview = await previewInsightApply(insightId, normEdited, mappings);
      setPreviewProposal(preview);
    } catch (err: unknown) {
      setPreviewError(err instanceof Error ? err.message : 'Failed to generate application preview');
    } finally {
      setPreviewLoading(false);
    }
  };

  const handleConfirmProposal = async (pins: ConfirmationPins) => {
    setConfirmLoading(true);
    setConfirmError(null);
    try {
      const result = await confirmInsightApply(pins);
      // Invalidate all affected queries per Requirement 5
      queryClient.invalidateQueries({ queryKey: ['skill', skillId] });
      queryClient.invalidateQueries({ queryKey: ['skill-review', skillId] });
      queryClient.invalidateQueries({ queryKey: ['skill-runtime', skillId] });
      queryClient.invalidateQueries({ queryKey: ['skill-sources', skillId] });
      queryClient.invalidateQueries({ queryKey: ['skills'] });
      queryClient.invalidateQueries({ queryKey: ['home'] });
      queryClient.invalidateQueries({ queryKey: ['inbox'] });
      queryClient.invalidateQueries({ queryKey: ['insight', insightId] });

      clearComposerDraft(workspaceId, insightId);
      setPreviewProposal(null);
      setAppliedReceipt(result);
    } catch (err: unknown) {
      setConfirmError(err instanceof Error ? err.message : 'Failed to apply insight patch');
    } finally {
      setConfirmLoading(false);
    }
  };

  if (appliedReceipt) {
    return (
      <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
        <h1 style={{ margin: 0, fontSize: '20px', fontWeight: 600 }}>
          <span>{TITLE_COMPOSER}</span>
        </h1>
        <div
          className="fg-card"
          style={{
            display: 'flex',
            flexDirection: 'column',
            gap: 'var(--space-3)',
            padding: 'var(--space-6) var(--space-4)',
            alignItems: 'center',
            textAlign: 'center',
          }}
        >
          <span style={{ fontSize: '18px', fontWeight: 600, color: 'var(--color-success, #16a34a)' }}>
            <span>{LABEL_APPLIED_SUCCESS}</span>
          </span>
          <p style={{ margin: 0, color: 'var(--color-text-muted)', fontSize: '14px' }}>
            <span>{LABEL_APPLIED_DESC}</span>
          </p>
          <Link
            to={`/skills/${encodeURIComponent(skillId)}?tab=review`}
            className="fg-btn fg-btn--primary"
            style={{ textDecoration: 'none', marginTop: 'var(--space-2)' }}
          >
            <span>{BTN_VIEW_SKILL_REVIEW}</span>
          </Link>
        </div>
      </div>
    );
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
      {/* Title & Target Path */}
      <div style={{ display: 'flex', flexDirection: 'column', gap: '4px' }}>
        <h1 style={{ margin: 0, fontSize: '20px', fontWeight: 600 }}>
          <span>{TITLE_COMPOSER}</span>
        </h1>
        <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12.5px', color: 'var(--color-text-muted)' }}>
          <span>{LABEL_TARGET_FILE}</span>
          <span>{targetPath}</span>
        </span>
      </div>

      {needsRebase && (
        <div className="fg-banner fg-banner--warning">
          <span>{LABEL_REBASE_WARNING}</span>
        </div>
      )}

      {staleEvidence && (
        <div className="fg-banner fg-banner--warning">
          <span>{LABEL_STALE_EVIDENCE_WARNING}</span>
        </div>
      )}

      {previewError && (
        <div className="fg-banner fg-banner--danger">
          <span>{previewError}</span>
        </div>
      )}

      {/* Tabs on mobile */}
      <div
        className="fg-tabs"
        style={{
          display: 'flex',
          gap: 'var(--space-2)',
          borderBottom: '1px solid var(--color-border)',
        }}
      >
        <button
          type="button"
          className={`fg-tab ${activeTab === 'content' ? 'fg-tab--active' : ''}`}
          onClick={() => setActiveTab('content')}
          style={{
            padding: 'var(--space-2) var(--space-3)',
            borderBottom: activeTab === 'content' ? '2px solid var(--color-primary)' : 'none',
            fontWeight: activeTab === 'content' ? 600 : 400,
            cursor: 'pointer',
            background: 'none',
            border: 'none',
          }}
        >
          <span>{LABEL_TAB_CONTENT}</span>
        </button>
        <button
          type="button"
          className={`fg-tab ${activeTab === 'mapping' ? 'fg-tab--active' : ''}`}
          onClick={() => setActiveTab('mapping')}
          style={{
            padding: 'var(--space-2) var(--space-3)',
            borderBottom: activeTab === 'mapping' ? '2px solid var(--color-primary)' : 'none',
            fontWeight: activeTab === 'mapping' ? 600 : 400,
            cursor: 'pointer',
            background: 'none',
            border: 'none',
          }}
        >
          <span>{LABEL_TAB_MAPPING}</span>
          <span>{LABEL_LPAREN}{cov.mapped}{LABEL_SLASH}{cov.required}{LABEL_RPAREN}</span>
        </button>
      </div>

      {/* Main Grid: Content Editor (left) & Mapping List (right) */}
      <div
        style={{
          display: 'grid',
          gridTemplateColumns: 'minmax(0, 1.2fr) minmax(0, 1fr)',
          gap: 'var(--space-4)',
          alignItems: 'start',
        }}
      >
        {/* Left Column: Full Replacement Textarea */}
        <section
          className="fg-card"
          style={{
            display: 'flex',
            flexDirection: 'column',
            gap: 'var(--space-3)',
            minWidth: 0,
          }}
        >
          <div className="fg-card__title" style={{ fontFamily: 'var(--font-mono)', fontSize: '13px' }}>
            <span>{targetPath}</span>
          </div>
          <textarea
            className="fg-input fg-input--area"
            aria-label="SKILL.md replacement content"
            spellCheck={false}
            style={{
              minHeight: '440px',
              fontFamily: 'var(--font-mono)',
              fontSize: '13px',
              lineHeight: 1.55,
              width: '100%',
              boxSizing: 'border-box',
            }}
            value={content}
            onChange={(e) => setContent(e.target.value)}
          />
        </section>

        {/* Right Column: Evidence Mapping */}
        <section
          className="fg-card"
          style={{
            display: 'flex',
            flexDirection: 'column',
            gap: 'var(--space-3)',
          }}
        >
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline' }}>
            <span className="fg-card__title" style={{ fontWeight: 600, fontSize: '15px' }}>
              <span>{LABEL_EVIDENCE_MAPPING}</span>
            </span>
            <span
              aria-live="polite"
              style={{
                fontFamily: 'var(--font-mono)',
                fontWeight: 600,
                fontSize: '13px',
                color: isCoverageComplete ? 'var(--color-success, #16a34a)' : 'var(--color-warning, #f59e0b)',
              }}
            >
              <span>{cov.mapped}</span>
              <span>{LABEL_SLASH}</span>
              <span>{cov.required}</span>
            </span>
          </div>

          <div style={{ display: 'flex', flexDirection: 'column' }}>
            {required.map((reqObs) => {
              const obsMappings = mappings.filter((m) => m.observation_id.trim() === reqObs.id.trim());
              const isMapped = obsMappings.length > 0;
              const sourceText = reqObs.sources
                .map((s) => (s.kind === 'direct' ? 'direct finding' : `comparison: ${s.subject || ''}`))
                .join(', ');

              return (
                <div
                  key={reqObs.id}
                  style={{
                    display: 'flex',
                    flexDirection: 'column',
                    gap: '6px',
                    padding: 'var(--space-3) 0',
                    borderTop: '1px solid var(--color-border)',
                  }}
                >
                  <div style={{ display: 'flex', justifyContent: 'space-between', gap: 'var(--space-2)', alignItems: 'flex-start' }}>
                    <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12px', wordBreak: 'break-all' }}>
                      <span>{reqObs.id}</span>
                    </span>
                    <StatusBadge
                      variant="chip"
                      tone={isMapped ? 'success' : 'neutral'}
                      label={isMapped ? 'mapped' : 'unmapped'}
                    />
                  </div>

                  <span className="t-caption" style={{ color: 'var(--color-text-subtle)', fontSize: '12px' }}>
                    <span>{sourceText}</span>
                  </span>

                  {reqObs.finding && (
                    <span style={{ fontSize: '12.5px', color: 'var(--color-text-muted)' }}>
                      <span>{reqObs.finding.what}</span>
                    </span>
                  )}

                  {/* List of existing concepts */}
                  {obsMappings.length > 0 && (
                    <div style={{ display: 'flex', flexWrap: 'wrap', gap: 'var(--space-1)', marginTop: '4px' }}>
                      {obsMappings.map((m) => (
                        <span
                          key={m.concept}
                          className="fg-chip fg-chip--info"
                          style={{ display: 'inline-flex', alignItems: 'center', gap: '4px', fontSize: '12px' }}
                        >
                          <span>{m.concept}</span>
                          <button
                            type="button"
                            onClick={() => handleRemoveConcept(reqObs.id, m.concept)}
                            style={{ all: 'unset', cursor: 'pointer', fontWeight: 'bold' }}
                            aria-label={`Remove concept ${m.concept} for ${reqObs.id}`}
                          >
                            <span>{CHAR_CLOSE}</span>
                          </button>
                        </span>
                      ))}
                    </div>
                  )}

                  {/* Add concept input */}
                  <div style={{ display: 'flex', gap: 'var(--space-2)', marginTop: '4px', alignItems: 'center' }}>
                    <input
                      className="fg-input"
                      placeholder={LABEL_CONCEPT_PLACEHOLDER}
                      value={newConceptInputs[reqObs.id] || ''}
                      onChange={(e) =>
                        setNewConceptInputs((prev) => ({ ...prev, [reqObs.id]: e.target.value }))
                      }
                      onKeyDown={(e) => {
                        if (e.key === 'Enter') {
                          e.preventDefault();
                          handleAddConcept(reqObs.id);
                        }
                      }}
                      aria-label={`Concept for ${reqObs.id}`}
                      style={{ fontSize: '12px', flex: 1 }}
                    />
                    <button
                      type="button"
                      className="fg-btn fg-btn--ghost"
                      style={{ alignSelf: 'flex-start', padding: '2px 8px', fontSize: '12px' }}
                      onClick={() => handleAddConcept(reqObs.id)}
                    >
                      <span>{LABEL_ADD_CONCEPT}</span>
                    </button>
                  </div>
                </div>
              );
            })}
          </div>
        </section>
      </div>

      {/* Sticky Bottom Bar */}
      <div
        style={{
          position: 'sticky',
          bottom: 0,
          display: 'flex',
          flexWrap: 'wrap',
          alignItems: 'center',
          gap: 'var(--space-3)',
          padding: 'var(--space-3) var(--space-4)',
          background: 'var(--color-surface)',
          border: '1px solid var(--color-border)',
          borderRadius: 'var(--card-radius)',
          zIndex: 10,
        }}
      >
        <button
          type="button"
          className="fg-btn fg-btn--ghost"
          onClick={handleManualSaveDraft}
        >
          <span>{draftSavedToast ? LABEL_DRAFT_SAVED : LABEL_SAVE_LOCAL_DRAFT}</span>
        </button>

        <span
          className="t-caption"
          style={{
            color: isCoverageComplete ? 'var(--color-success, #16a34a)' : 'var(--color-text-subtle)',
            flex: 1,
            fontSize: '13px',
          }}
        >
          <span>{LABEL_MAPPED_PREFIX}</span>
          <span>{cov.mapped}</span>
          <span>{LABEL_MAPPED_OF}</span>
          <span>{cov.required}</span>
          <span>{LABEL_MAPPED_REQUIRED}</span>
          {previewDisabledReason && !isCoverageComplete && (
            <span>{LABEL_LPAREN}{previewDisabledReason}{LABEL_RPAREN}</span>
          )}
        </span>

        <button
          type="button"
          className="fg-btn fg-btn--ghost"
          onClick={() => navigate(`/inbox/${encodeURIComponent(insightId)}`)}
        >
          <span>{LABEL_CANCEL}</span>
        </button>

        <button
          type="button"
          className="fg-btn fg-btn--primary"
          onClick={() => void handlePreviewApply()}
          disabled={!canPreview || previewLoading}
          title={previewDisabledReason}
        >
          <span>{previewLoading ? BTN_PREVIEWING : BTN_PREVIEW_APPLY}</span>
        </button>
      </div>

      {/* Conflict Drawer */}
      <ConflictDrawer
        open={conflictOpen}
        draftContent={content}
        latestContent={conflictLatestContent}
        expectedDigest={baselineContentDigest}
        latestDigest={conflictLatestDigest}
        onUseLatest={() => {
          setBaselineContentDigest(conflictLatestDigest);
          setContent(conflictLatestContent);
          saveComposerDraft(workspaceId, insightId, {
            content: conflictLatestContent,
            mappings,
            contentDigest: conflictLatestDigest,
            evidenceDigest: insightData.insight.evidence_digest || '',
          });
          setNeedsRebase(false);
          setConflictOpen(false);
        }}
        onDiscard={() => {
          clearComposerDraft(workspaceId, insightId);
          setContent(conflictLatestContent);
          setBaselineContentDigest(conflictLatestDigest);
          setNeedsRebase(false);
          setConflictOpen(false);
        }}
        onClose={() => setConflictOpen(false)}
      />

      {/* Proposal Preview Modal */}
      {previewProposal && (
        <ProposalPreview
          open={Boolean(previewProposal)}
          title="Apply insight patch"
          target={targetPath}
          fromState="current"
          toState="patched"
          proposalId={previewProposal.proposal_id}
          proposalDigest={previewProposal.proposal_digest}
          baseVersion={previewProposal.base_catalog_version}
          diff={previewProposal.diff}
          confirmLabel="Apply patch"
          isLoading={confirmLoading}
          networkError={confirmError}
          onConfirm={(pins) =>
            void handleConfirmProposal({
              proposal_id: pins.proposalId,
              proposal_digest: pins.proposalDigest,
              base_version: pins.baseVersion,
            })
          }
          onCancel={() => setPreviewProposal(null)}
        />
      )}
    </div>
  );
}

export function PatchComposerScreen() {
  const { id: rawId } = useParams<{ id: string }>();
  const insightId = rawId || '';

  const { data: session } = useSession();
  const workspaceId = session?.workspace_id || 'default';

  const {
    data: insightData,
    isLoading: insightLoading,
    error: insightError,
    refetch: refetchInsight,
  } = useInsightDetail(insightId);

  const skillId = insightData?.insight?.skill_id || '';
  const {
    data: skillData,
    isLoading: skillLoading,
    error: skillError,
    refetch: refetchSkill,
  } = useSkillDetail(skillId);

  if (insightLoading || (skillLoading && skillId)) {
    return (
      <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
        <h1 style={{ margin: 0, fontSize: '20px', fontWeight: 600 }}>
          <span>{TITLE_COMPOSER}</span>
        </h1>
        <div className="fg-card" style={{ display: 'flex', flexDirection: 'column', gap: '14px' }}>
          <Skeleton width="40%" height="24px" />
          <Skeleton height="300px" />
        </div>
      </div>
    );
  }

  const isNotFound =
    insightError instanceof Error &&
    (insightError.message.includes('404') ||
      insightError.message.includes('not found') ||
      (insightError as { status?: number }).status === 404);

  if (isNotFound) {
    return (
      <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
        <h1 style={{ margin: 0, fontSize: '20px', fontWeight: 600 }}>
          <span>{TITLE_COMPOSER}</span>
        </h1>
        <div
          className="fg-card"
          style={{
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
            gap: 'var(--space-3)',
            padding: 'var(--space-6) var(--space-4)',
          }}
        >
          <span style={{ fontSize: '16px', fontWeight: 600 }}>{LABEL_NOT_FOUND_TITLE}</span>
          <Link to="/inbox" className="fg-btn fg-btn--primary" style={{ textDecoration: 'none' }}>
            <span>{BTN_BACK_INBOX}</span>
          </Link>
        </div>
      </div>
    );
  }

  if (insightError || skillError || !insightData || !skillData) {
    const err = insightError || skillError;
    return (
      <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
        <h1 style={{ margin: 0, fontSize: '20px', fontWeight: 600 }}>
          <span>{TITLE_COMPOSER}</span>
        </h1>
        <div className="fg-banner fg-banner--danger">
          <span>{err instanceof Error ? err.message : 'Failed to load composer data'}</span>
        </div>
        <div style={{ display: 'flex', gap: 'var(--space-2)' }}>
          <button
            type="button"
            className="fg-btn fg-btn--secondary"
            onClick={() => {
              void refetchInsight();
              void refetchSkill();
            }}
          >
            <span>{BTN_RETRY}</span>
          </button>
          <Link to="/inbox" className="fg-btn fg-btn--ghost" style={{ textDecoration: 'none' }}>
            <span>{BTN_BACK_INBOX}</span>
          </Link>
        </div>
      </div>
    );
  }

  return (
    <PatchComposerEditor
      key={insightId}
      insightData={insightData}
      skillData={skillData}
      workspaceId={workspaceId}
    />
  );
}
