import { useEffect, useRef, useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { confirmSkillMutation, previewSkillUpdate } from '../../api/queries';
import type { SkillDetail, SkillProposal } from '../../api/types';
import { ApiError, apiFetch } from '../../api/client';
import { proposalPatch, proposalPaths, proposalWarning } from '../../api/proposal-view';
import { ConflictDrawer } from '../../components/ConflictDrawer';
import { Markdown } from '../../components/Markdown';
import { ProposalPreview } from '../../components/ProposalPreview';
import { Toast } from '../../components/Toast';
import { clearDraft, loadDraft, saveDraft } from '../../state/drafts';
import { ROUTING_HELP, SCOPE_OPTIONS } from '../routing-hints';
import {
  EDIT_FIELD_LABEL,
  changedFields,
  describeChanges,
  fieldsFromSkill,
  mergeOntoLatest,
  splitList,
  type EditFields,
} from './edit-fields';

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
const LABEL_REQUIRED_SCOPE = 'Required before activation.';
const HINT_NAME = 'The title people see in lists.';
const HINT_DESCRIPTION = 'One or two sentences on what the skill does. Agents read this to decide whether to load it.';
const MSG_DRAFT_SAVED = 'Draft saved in this browser';
const MSG_NO_CHANGES = 'Nothing changed yet. Edit a field, then preview the change before it is saved.';
const MSG_PREVIEW_NOTE = 'Preview shows what will change. Nothing is saved until you confirm.';
const REQ_STAR = '*';

// Where each field of the Review checklist lives in this form.
const FOCUS_TARGET: Record<string, string> = {
  content: 'edit-content',
  triggers: 'edit-trigs',
  operations: 'edit-ops',
  scope: 'edit-scope',
};

interface EditorTabProps {
  skill: SkillDetail;
  workspaceId: string;
  focusField?: string | null;
}

interface ConflictInfo {
  // The version the person opened; the saved skill prop moves on once the latest is fetched.
  base: EditFields;
  latest: SkillDetail;
  mine: string[];
  theirs: string[];
  both: string[];
}

export function EditorTab({ skill, workspaceId, focusField }: EditorTabProps) {
  const queryClient = useQueryClient();

  const initialDraft = loadDraft<EditFields>(workspaceId, 'skill', skill.skill_id, skill.content_digest);
  const initial: EditFields = initialDraft ? { ...fieldsFromSkill(skill), ...initialDraft.value } : fieldsFromSkill(skill);

  const [content, setContent] = useState(initial.content);
  const [name, setName] = useState(initial.name);
  const [description, setDescription] = useState(initial.description);
  const [operations, setOperations] = useState(initial.operations);
  const [triggers, setTriggers] = useState(initial.triggers);
  const [notFor, setNotFor] = useState(initial.notFor);
  const [minScope, setMinScope] = useState(initial.minScope);

  const [activeView, setActiveView] = useState<'edit' | 'preview'>('edit');
  const [isWide, setIsWide] = useState(false);
  const [lastSaved, setLastSaved] = useState<number | null>(initialDraft ? initialDraft.savedAt : null);

  const [loading, setLoading] = useState(false);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const [conflict, setConflict] = useState<ConflictInfo | null>(null);
  const [proposal, setProposal] = useState<SkillProposal | null>(null);
  const [previewChanges, setPreviewChanges] = useState<string[]>([]);
  const [previewOpen, setPreviewOpen] = useState(false);
  const [toastMessage, setToastMessage] = useState<string | null>(null);
  const focusedFor = useRef<string | null>(null);

  const mine: EditFields = { name, description, content, operations, triggers, notFor, minScope };
  const base = fieldsFromSkill(skill);
  const dirty = changedFields(base, mine);

  useEffect(() => {
    const handleResize = () => setIsWide(window.innerWidth >= 1280);
    handleResize();
    window.addEventListener('resize', handleResize);
    return () => window.removeEventListener('resize', handleResize);
  }, []);

  // "Go to field" from the Review tab lands here: put the cursor in that field.
  useEffect(() => {
    if (!focusField || focusedFor.current === focusField) return;
    const id = FOCUS_TARGET[focusField] ?? focusField;
    const el = document.getElementById(id);
    if (!el) return;
    focusedFor.current = focusField;
    el.scrollIntoView?.({ block: 'center' });
    el.focus();
  }, [focusField, isWide]);

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

  const showError = (err: unknown) => {
    if (err instanceof ApiError) {
      setErrorMessage(err.render.WHY || err.render.ERROR || err.message);
    } else if (err instanceof Error) {
      setErrorMessage(err.message);
    }
  };

  const requestPreview = async (against: SkillDetail, fields: EditFields) => {
    const prop = await previewSkillUpdate(skill.skill_id, {
      expected_content_digest: against.content_digest,
      name: fields.name !== against.name ? fields.name : undefined,
      description: fields.description !== against.description ? fields.description : undefined,
      content: fields.content !== against.content ? fields.content : undefined,
      routing: {
        operations: splitList(fields.operations),
        triggers: splitList(fields.triggers),
        not_for: splitList(fields.notFor),
        min_scope: fields.minScope,
      },
    });
    setPreviewChanges(describeChanges(fieldsFromSkill(against), fields));
    setProposal(prop);
    setPreviewOpen(true);
  };

  const fetchLatest = () =>
    queryClient.fetchQuery({
      queryKey: ['skill', skill.skill_id],
      queryFn: () => apiFetch<SkillDetail>(`/skills/${encodeURIComponent(skill.skill_id)}`),
      staleTime: 0,
    });

  const handlePreviewChanges = async () => {
    setLoading(true);
    setErrorMessage(null);
    try {
      await requestPreview(skill, mine);
    } catch (err) {
      if (err instanceof ApiError && err.code === 'edit_conflict') {
        try {
          const latest = await fetchLatest();
          const theirs = changedFields(base, fieldsFromSkill(latest));
          const bothFields = dirty.filter((f) => theirs.includes(f)).map((f) => EDIT_FIELD_LABEL[f]);
          setConflict({
            base,
            latest,
            mine: describeChanges(base, mine),
            theirs: describeChanges(base, fieldsFromSkill(latest)),
            both: bothFields,
          });
        } catch (fetchErr) {
          showError(fetchErr);
        }
      } else {
        showError(err);
      }
    } finally {
      setLoading(false);
    }
  };

  const handleConfirm = async (pins: { proposalId: string; proposalDigest: string; baseVersion: string }) => {
    try {
      await confirmSkillMutation(pins.proposalId, pins);
      setPreviewOpen(false);
      clearDraft(workspaceId, 'skill', skill.skill_id);
      setToastMessage(`Saved ${previewChanges.length === 1 ? '1 change' : `${previewChanges.length} changes`} to ${skill.skill_id}.`);
      queryClient.invalidateQueries({ queryKey: ['skill', skill.skill_id] });
      queryClient.invalidateQueries({ queryKey: ['skill-review', skill.skill_id] });
      queryClient.invalidateQueries({ queryKey: ['skills'] });
      queryClient.invalidateQueries({ queryKey: ['home'] });
      // Saving rewrites SKILL.md (its frontmatter follows name and description), so show the saved
      // text; otherwise the form would still count the old frontmatter as an unsaved change.
      try {
        applyFields(fieldsFromSkill(await fetchLatest()));
      } catch {
        // The saved result is already confirmed; a failed refresh only leaves the form as typed.
      }
    } catch (err) {
      showError(err);
    }
  };

  const applyFields = (f: EditFields) => {
    setContent(f.content);
    setName(f.name);
    setDescription(f.description);
    setOperations(f.operations);
    setTriggers(f.triggers);
    setNotFor(f.notFor);
    setMinScope(f.minScope);
  };

  const handleDiscardConflict = () => {
    const latest = conflict?.latest ?? skill;
    clearDraft(workspaceId, 'skill', skill.skill_id);
    applyFields(fieldsFromSkill(latest));
    setConflict(null);
    queryClient.invalidateQueries({ queryKey: ['skill', skill.skill_id] });
  };

  // Puts the person's edits on top of the saved version. A field they did not touch keeps the saved
  // value, so an edit made elsewhere is not undone.
  const handleUseLatestAsBase = async () => {
    try {
      setLoading(true);
      const refreshed = conflict?.latest ?? (await fetchLatest());
      const merged = mergeOntoLatest(conflict?.base ?? base, mine, fieldsFromSkill(refreshed));
      applyFields(merged);
      saveDraft(workspaceId, 'skill', skill.skill_id, refreshed.content_digest, merged);
      setConflict(null);
      await requestPreview(refreshed, merged);
    } catch (err) {
      showError(err);
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
                id="edit-content"
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
              id="edit-content"
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
            <span className="fg-field__hint">
              <span>{HINT_NAME}</span>
            </span>
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
            <span className="fg-field__hint">
              <span>{HINT_DESCRIPTION}</span>
            </span>
          </div>

          <div className="fg-field">
            <label className="fg-field__label t-label" htmlFor="edit-ops">
              <span>{LABEL_OPERATIONS}</span>
            </label>
            <input
              id="edit-ops"
              className="fg-input"
              placeholder={ROUTING_HELP.operations.example}
              value={operations}
              onChange={(e) => setOperations(e.target.value)}
            />
            <span className="fg-field__hint">
              <span>{ROUTING_HELP.operations.hint}</span>
            </span>
          </div>

          <div className="fg-field">
            <label className="fg-field__label t-label" htmlFor="edit-trigs">
              <span>{LABEL_TRIGGERS} </span>
              <span className="fg-field__req">{REQ_STAR}</span>
            </label>
            <input
              id="edit-trigs"
              className="fg-input"
              placeholder={ROUTING_HELP.triggers.example}
              value={triggers}
              onChange={(e) => setTriggers(e.target.value)}
            />
            <span className="fg-field__hint">
              <span>{ROUTING_HELP.triggers.hint}</span>
            </span>
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
              placeholder={ROUTING_HELP.notFor.example}
              value={notFor}
              onChange={(e) => setNotFor(e.target.value)}
            />
            <span className="fg-field__hint">
              <span>{ROUTING_HELP.notFor.hint}</span>
            </span>
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
                {SCOPE_OPTIONS.map((sc) => (
                  <option key={sc.value} value={sc.value}>
                    {sc.label}
                  </option>
                ))}
              </select>
              <span className="fg-select__chev" aria-hidden="true">
                ▾
              </span>
            </div>
            <span className="fg-field__hint">
              <span>{ROUTING_HELP.minScope.hint}</span>
            </span>
            {!minScope && (
              <span className="fg-field__hint" style={{ color: 'var(--color-warning)' }}>
                <span>{LABEL_REQUIRED_SCOPE}</span>
              </span>
            )}
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
          <span>
            {dirty.length === 0 ? MSG_NO_CHANGES : lastSaved ? `${MSG_DRAFT_SAVED}. ${MSG_PREVIEW_NOTE}` : MSG_PREVIEW_NOTE}
          </span>
        </span>
        <button
          type="button"
          className="fg-btn fg-btn--primary"
          onClick={handlePreviewChanges}
          disabled={loading || dirty.length === 0}
        >
          <span>{loading ? 'Validating…' : LABEL_PREVIEW_CHANGES}</span>
        </button>
      </div>

      {/* Proposal Preview Modal */}
      {previewOpen && proposal && (
        <ProposalPreview
          open={previewOpen}
          title={`Save changes to ${skill.skill_id}?`}
          target={skill.skill_id}
          toState={skill.lifecycle_state}
          paths={proposalPaths(proposal)}
          changes={previewChanges}
          warning={proposalWarning(proposal)}
          diff={proposalPatch(proposal)}
          proposalId={pins?.proposal_id || ''}
          proposalDigest={pins?.proposal_digest || ''}
          baseVersion={pins?.base_version || ''}
          confirmLabel="Save changes"
          onConfirm={handleConfirm}
          onCancel={() => setPreviewOpen(false)}
        />
      )}

      {/* Conflict Drawer */}
      <ConflictDrawer
        open={conflict !== null}
        draftContent={content}
        latestContent={conflict?.latest.content ?? ''}
        expectedDigest={skill.content_digest}
        latestDigest={conflict?.latest.content_digest ?? ''}
        mine={conflict?.mine}
        theirs={conflict?.theirs}
        bothFields={conflict?.both}
        onUseLatest={handleUseLatestAsBase}
        onDiscard={handleDiscardConflict}
        onClose={() => setConflict(null)}
      />

      <Toast message={toastMessage} onClose={() => setToastMessage(null)} />
    </div>
  );
}
