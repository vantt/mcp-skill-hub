import { useEffect, useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { confirmSkillMutation, previewSkillUpdate } from '../../api/queries';
import type { SkillDetail, SkillProposal } from '../../api/types';
import { ApiError, apiFetch } from '../../api/client';
import { ConflictDrawer } from '../../components/ConflictDrawer';
import { Markdown } from '../../components/Markdown';
import { ProposalPreview } from '../../components/ProposalPreview';
import { Toast } from '../../components/Toast';
import { clearDraft, loadDraft, saveDraft } from '../../state/drafts';
import { useT } from '../../i18n';

const TITLE_ROUTING_META = 'Routing & metadata';
const LABEL_NAME = 'Name';
const LABEL_DESCRIPTION = 'Description';
const LABEL_OPERATIONS = 'Operations';
const LABEL_TRIGGERS = 'Triggers';
const LABEL_NOT_FOR = 'Not for / Rationale';
const LABEL_MIN_SCOPE = 'Min scope';
const LABEL_EDIT = 'Edit';
const LABEL_PREVIEW = 'Preview';
const LABEL_PREVIEW_CHANGES = 'Preview changes';
const LABEL_REQUIRED_TRIGGER = 'Required before activation.';
const REQ_STAR = '*';
const SCOPES = [
  { value: '', label: 'Select scope' },
  { value: 'single_step', label: 'single_step' },
  { value: 'multi_step', label: 'multi_step' },
  { value: 'project', label: 'project' },
];
const MSG_DRAFT_SAVED = 'Draft saved in this browser';

interface SkillDraftData {
  content: string;
  name: string;
  description: string;
  operations: string;
  triggers: string;
  notFor: string;
  minScope: string;
}

interface EditorTabProps {
  skill: SkillDetail;
  workspaceId: string;
  focusField?: string | null;
}

export function EditorTab({ skill, workspaceId }: EditorTabProps) {
  const t = useT();
  const queryClient = useQueryClient();

  const initialDraft = loadDraft<SkillDraftData>(workspaceId, 'skill', skill.skill_id, skill.content_digest);

  const [content, setContent] = useState(initialDraft ? initialDraft.value.content : (skill.content || ''));
  const [name, setName] = useState(initialDraft ? initialDraft.value.name : (skill.name || ''));
  const [description, setDescription] = useState(initialDraft ? initialDraft.value.description : (skill.description || ''));
  const [operations, setOperations] = useState(initialDraft ? initialDraft.value.operations : (skill.routing?.operations || []).join(', '));
  const [triggers, setTriggers] = useState(initialDraft ? initialDraft.value.triggers : (skill.routing?.triggers || []).join(', '));
  const [notFor, setNotFor] = useState(initialDraft ? initialDraft.value.notFor : (skill.routing?.not_for || []).join(', '));
  const [minScope, setMinScope] = useState(initialDraft ? initialDraft.value.minScope : (skill.routing?.min_scope || ''));

  const [activeView, setActiveView] = useState<'edit' | 'preview'>('edit');
  const [isWide, setIsWide] = useState(false);
  const [lastSaved, setLastSaved] = useState<number | null>(initialDraft ? initialDraft.savedAt : null);

  const [loading, setLoading] = useState(false);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const [conflictDrawerOpen, setConflictDrawerOpen] = useState(false);
  const [proposal, setProposal] = useState<SkillProposal | null>(null);
  const [previewOpen, setPreviewOpen] = useState(false);
  const [toastMessage, setToastMessage] = useState<string | null>(null);

  useEffect(() => {
    const handleResize = () => setIsWide(window.innerWidth >= 1280);
    handleResize();
    window.addEventListener('resize', handleResize);
    return () => window.removeEventListener('resize', handleResize);
  }, []);


  // Autosave draft on change
  useEffect(() => {
    const timer = setTimeout(() => {
      saveDraft(workspaceId, 'skill', skill.skill_id, skill.content_digest, {
        content,
        name,
        description,
        operations,
        triggers,
        notFor,
        minScope,
      });
      setLastSaved(Date.now());
    }, 500);

    return () => clearTimeout(timer);
  }, [content, name, description, operations, triggers, notFor, minScope, workspaceId, skill.skill_id, skill.content_digest]);

  const handlePreviewChanges = async () => {
    setLoading(true);
    setErrorMessage(null);

    const ops = operations.split(',').map((s) => s.trim()).filter(Boolean);
    const trigs = triggers.split(',').map((s) => s.trim()).filter(Boolean);
    const nots = notFor.split(',').map((s) => s.trim()).filter(Boolean);

    try {
      const prop = await previewSkillUpdate(skill.skill_id, {
        expected_content_digest: skill.content_digest,
        name: name !== skill.name ? name : undefined,
        description: description !== skill.description ? description : undefined,
        content: content !== skill.content ? content : undefined,
        routing: {
          operations: ops,
          triggers: trigs,
          not_for: nots,
          min_scope: minScope,
        },
      });
      setProposal(prop);
      setPreviewOpen(true);
    } catch (err) {
      if (err instanceof ApiError && err.code === 'edit_conflict') {
        setConflictDrawerOpen(true);
      } else if (err instanceof ApiError) {
        setErrorMessage(err.render.WHY || err.render.ERROR || err.message);
      } else if (err instanceof Error) {
        setErrorMessage(err.message);
      }
    } finally {
      setLoading(false);
    }
  };

  const handleConfirm = async (pins: { proposalId: string; proposalDigest: string; baseVersion: string }) => {
    try {
      const res = await confirmSkillMutation(pins.proposalId, pins);
      setPreviewOpen(false);
      clearDraft(workspaceId, 'skill', skill.skill_id);
      setToastMessage(`Applied · ${res.operation_id}`);
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

  const handleDiscardConflict = () => {
    clearDraft(workspaceId, 'skill', skill.skill_id);
    setContent(skill.content || '');
    setName(skill.name || '');
    setDescription(skill.description || '');
    setConflictDrawerOpen(false);
    queryClient.invalidateQueries({ queryKey: ['skill', skill.skill_id] });
  };

  const handleUseLatestAsBase = async () => {
    try {
      setLoading(true);
      const refreshed = await queryClient.fetchQuery({
        queryKey: ['skill', skill.skill_id],
        queryFn: () => apiFetch<SkillDetail>(`/skills/${encodeURIComponent(skill.skill_id)}`),
      });
      saveDraft(workspaceId, 'skill', skill.skill_id, refreshed.content_digest, {
        content,
        name,
        description,
        operations,
        triggers,
        notFor,
        minScope,
      });
      setConflictDrawerOpen(false);

      const ops = operations.split(',').map((s) => s.trim()).filter(Boolean);
      const trigs = triggers.split(',').map((s) => s.trim()).filter(Boolean);
      const nots = notFor.split(',').map((s) => s.trim()).filter(Boolean);

      const prop = await previewSkillUpdate(skill.skill_id, {
        expected_content_digest: refreshed.content_digest,
        name: name !== refreshed.name ? name : undefined,
        description: description !== refreshed.description ? description : undefined,
        content: content !== refreshed.content ? content : undefined,
        routing: {
          operations: ops,
          triggers: trigs,
          not_for: nots,
          min_scope: minScope,
        },
      });
      setProposal(prop);
      setPreviewOpen(true);
    } catch (err) {
      if (err instanceof ApiError) {
        setErrorMessage(err.render.WHY || err.render.ERROR || err.message);
      } else if (err instanceof Error) {
        setErrorMessage(err.message);
      }
    } finally {
      setLoading(false);
    }
  };

  const pins = proposal?.confirmation?.confirmation?.pins ?? proposal?.confirmation?.pins ?? null;

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

      <div
        style={{
          display: 'grid',
          gridTemplateColumns: isWide ? '1fr 340px' : '1fr',
          gap: 'var(--space-4)',
          alignItems: 'start',
        }}
      >
        {/* Left Pane: Content Editor */}
        <section className="fg-card" style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)', minWidth: 0 }}>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: 'var(--space-2)' }}>
            <span className="fg-card__title" style={{ fontFamily: 'var(--font-mono)', fontSize: '13px' }}>
              <span>{skill.path}</span>
            </span>

            {!isWide && (
              <div className="fg-seg">
                <button
                  type="button"
                  className={`fg-seg__btn ${activeView === 'edit' ? 'fg-seg__btn--active' : ''}`}
                  onClick={() => setActiveView('edit')}
                >
                  <span>{LABEL_EDIT}</span>
                </button>
                <button
                  type="button"
                  className={`fg-seg__btn ${activeView === 'preview' ? 'fg-seg__btn--active' : ''}`}
                  onClick={() => setActiveView('preview')}
                >
                  <span>{LABEL_PREVIEW}</span>
                </button>
              </div>
            )}
          </div>

          {isWide ? (
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 'var(--space-3)' }}>
              <textarea
                className="fg-input fg-input--area"
                style={{
                  minHeight: '440px',
                  fontFamily: 'var(--font-mono)',
                  fontSize: '13px',
                  lineHeight: '1.55',
                  resize: 'vertical',
                }}
                value={content}
                onChange={(e) => setContent(e.target.value)}
                aria-label="SKILL.md content"
              />
              <div
                style={{
                  minHeight: '440px',
                  padding: 'var(--space-3) var(--space-4)',
                  border: '1px solid var(--color-border)',
                  borderRadius: 'var(--input-radius)',
                  background: 'var(--color-surface-sunken)',
                  overflow: 'auto',
                }}
              >
                <Markdown content={content} />
              </div>
            </div>
          ) : activeView === 'edit' ? (
            <textarea
              className="fg-input fg-input--area"
              style={{
                minHeight: '380px',
                fontFamily: 'var(--font-mono)',
                fontSize: '13px',
                lineHeight: '1.55',
                resize: 'vertical',
              }}
              value={content}
              onChange={(e) => setContent(e.target.value)}
              aria-label="SKILL.md content"
            />
          ) : (
            <div
              style={{
                minHeight: '380px',
                padding: 'var(--space-3) var(--space-4)',
                border: '1px solid var(--color-border)',
                borderRadius: 'var(--input-radius)',
                background: 'var(--color-surface-sunken)',
                overflow: 'auto',
              }}
            >
              <Markdown content={content} />
            </div>
          )}
        </section>

        {/* Right Pane: Routing & Metadata */}
        <section className="fg-card" style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
          <div className="fg-card__title">
            <span>{TITLE_ROUTING_META}</span>
          </div>

          <div className="fg-field">
            <label className="fg-field__label t-label" htmlFor="edit-name">
              <span>{LABEL_NAME}</span>
            </label>
            <input
              id="edit-name"
              className="fg-input"
              value={name}
              onChange={(e) => setName(e.target.value)}
            />
          </div>

          <div className="fg-field">
            <label className="fg-field__label t-label" htmlFor="edit-desc">
              <span>{LABEL_DESCRIPTION}</span>
            </label>
            <textarea
              id="edit-desc"
              className="fg-input fg-input--area"
              style={{ minHeight: '64px' }}
              value={description}
              onChange={(e) => setDescription(e.target.value)}
            />
          </div>

          <div className="fg-field">
            <label className="fg-field__label t-label" htmlFor="edit-ops">
              <span>{LABEL_OPERATIONS}</span>
            </label>
            <input
              id="edit-ops"
              className="fg-input"
              value={operations}
              onChange={(e) => setOperations(e.target.value)}
            />
          </div>

          <div className="fg-field">
            <label className="fg-field__label t-label" htmlFor="edit-trigs">
              <span>{LABEL_TRIGGERS} </span>
              <span className="fg-field__req">{REQ_STAR}</span>
            </label>
            <input
              id="edit-trigs"
              className="fg-input"
              value={triggers}
              onChange={(e) => setTriggers(e.target.value)}
            />
            {!triggers.trim() && (
              <span className="fg-field__hint" style={{ color: 'var(--color-warning)' }}>
                <span>{LABEL_REQUIRED_TRIGGER}</span>
              </span>
            )}
          </div>

          <div className="fg-field">
            <label className="fg-field__label t-label" htmlFor="edit-notfor">
              <span>{LABEL_NOT_FOR}</span>
            </label>
            <input
              id="edit-notfor"
              className="fg-input"
              value={notFor}
              onChange={(e) => setNotFor(e.target.value)}
            />
          </div>

          <div className="fg-field">
            <label className="fg-field__label t-label" htmlFor="edit-scope">
              <span>{LABEL_MIN_SCOPE} </span>
              <span className="fg-field__req">{REQ_STAR}</span>
            </label>
            <div className="fg-select">
              <select
                id="edit-scope"
                value={minScope}
                onChange={(e) => setMinScope(e.target.value)}
              >
                {SCOPES.map((sc) => (
                  <option key={sc.value} value={sc.value}>
                    {sc.label}
                  </option>
                ))}
              </select>
              <span className="fg-select__chev" aria-hidden="true">
                ▾
              </span>
            </div>
          </div>
        </section>
      </div>

      {/* Sticky Footer */}
      <div
        style={{
          position: 'sticky',
          bottom: 0,
          display: 'flex',
          flexWrap: 'wrap',
          alignItems: 'center',
          justifyContent: 'space-between',
          gap: 'var(--space-3)',
          padding: 'var(--space-3) var(--space-4)',
          background: 'var(--color-surface)',
          border: '1px solid var(--color-border)',
          borderRadius: 'var(--card-radius)',
          zIndex: 10,
        }}
      >
        <span className="t-caption" style={{ color: 'var(--color-text-subtle)' }}>
          <span>{lastSaved ? MSG_DRAFT_SAVED : ''}</span>
        </span>
        <button
          type="button"
          className="fg-btn fg-btn--primary"
          onClick={handlePreviewChanges}
          disabled={loading}
        >
          <span>{loading ? 'Validating…' : LABEL_PREVIEW_CHANGES}</span>
        </button>
      </div>

      {/* Proposal Preview Modal */}
      {previewOpen && proposal && (
        <ProposalPreview
          open={previewOpen}
          title={proposal.summary || 'Update skill'}
          target={skill.skill_id}
          fromState={skill.lifecycle_state}
          toState={skill.lifecycle_state}
          paths={proposal.paths}
          impact={proposal.impact}
          warning={proposal.warning}
          diff={proposal.diff}
          stat={proposal.stat}
          proposalId={pins?.proposal_id || ''}
          proposalDigest={pins?.proposal_digest || ''}
          baseVersion={pins?.base_version || ''}
          confirmLabel={t('action.review')}
          onConfirm={handleConfirm}
          onCancel={() => setPreviewOpen(false)}
        />
      )}

      {/* Conflict Drawer */}
      <ConflictDrawer
        open={conflictDrawerOpen}
        draftContent={content}
        latestContent={skill.content || ''}
        expectedDigest={skill.content_digest}
        latestDigest="latest"
        onUseLatest={handleUseLatestAsBase}
        onDiscard={handleDiscardConflict}
        onClose={() => setConflictDrawerOpen(false)}
      />

      <Toast message={toastMessage} onClose={() => setToastMessage(null)} />
    </div>
  );
}
