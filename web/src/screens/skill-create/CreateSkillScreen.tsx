import { useState } from 'react';
import { useNavigate } from 'react-router';
import { confirmSkillMutation, previewSkillCreate } from '../../api/queries';
import type { SkillProposal } from '../../api/types';
import { ApiError } from '../../api/client';
import { ProposalPreview } from '../../components/ProposalPreview';
import { Toast } from '../../components/Toast';
import { useT } from '../../i18n';

const LABEL_IDENTITY = 'Identity';
const LABEL_ROUTING = 'Routing';
const LABEL_INSTRUCTIONS = 'Instructions';
const LABEL_SKILL_ID = 'Skill ID';
const LABEL_DISPLAY_NAME = 'Display name';
const LABEL_DESCRIPTION = 'Description';
const LABEL_COLLECTION = 'Collection';
const LABEL_OPERATIONS = 'Operations';
const LABEL_TRIGGERS = 'Triggers';
const LABEL_NOT_FOR = 'Not for / Rationale';
const LABEL_MIN_SCOPE = 'Min scope';
const LABEL_SCAFFOLD = 'Default scaffold';
const LABEL_UPLOAD = 'Upload Markdown';
const LABEL_WRITE = 'Write';
const LABEL_CANCEL = 'Cancel';
const LABEL_PREVIEW_DRAFT = 'Preview draft';
const REQ_STAR = '*';
const COLLECTIONS = ['core', 'default', 'engineering', 'operations', 'docs'];
const SCOPES = [
  { value: '', label: 'Select scope' },
  { value: 'file', label: 'file' },
  { value: 'change', label: 'change' },
  { value: 'repository', label: 'repository' },
];
const MSG_FILE_LOADED = 'File loaded successfully.';

const HINT_SKILL_ID = 'Lowercase, hyphenated (e.g. release-checklist).';
const HINT_SCAFFOLD = 'A default scaffold will be used. An untouched scaffold cannot be activated.';
const HINT_UPLOAD = 'Drop a .md file or click to browse. Max size 1 MB.';

export function CreateSkillScreen() {
  const t = useT();
  const navigate = useNavigate();

  const [id, setId] = useState('');
  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [collection, setCollection] = useState('core');

  const [operations, setOperations] = useState('');
  const [triggers, setTriggers] = useState('');
  const [notFor, setNotFor] = useState('');
  const [minScope, setMinScope] = useState('');

  const [instructionMode, setInstructionMode] = useState<'scaffold' | 'upload' | 'write'>('scaffold');
  const [customContent, setCustomContent] = useState('');

  const [errors, setErrors] = useState<Record<string, string>>({});
  const [loading, setLoading] = useState(false);
  const [apiError, setApiError] = useState<string | null>(null);

  const [proposal, setProposal] = useState<SkillProposal | null>(null);
  const [previewOpen, setPreviewOpen] = useState(false);
  const [receiptToast, setReceiptToast] = useState<string | null>(null);

  const validate = () => {
    const errs: Record<string, string> = {};
    const trimmedId = id.trim();
    if (!trimmedId) {
      errs.id = 'Skill ID is required.';
    } else if (!/^[a-z0-9]+(-[a-z0-9]+)*$/.test(trimmedId)) {
      errs.id = 'Skill ID must be lowercase alphanumeric with hyphens.';
    }

    if (!name.trim()) {
      errs.name = 'Display name is required.';
    }

    setErrors(errs);
    return Object.keys(errs).length === 0;
  };

  const handleFileUpload = async (file: File) => {
    if (!file.name.endsWith('.md')) {
      setErrors((prev) => ({ ...prev, upload: 'Only .md Markdown files are accepted.' }));
      return;
    }
    if (file.size > 1024 * 1024) {
      setErrors((prev) => ({ ...prev, upload: 'File exceeds 1 MB limit.' }));
      return;
    }
    try {
      const text = await file.text();
      setCustomContent(text);
      setErrors((prev) => {
        const next = { ...prev };
        delete next.upload;
        return next;
      });
    } catch {
      setErrors((prev) => ({ ...prev, upload: 'Could not read file.' }));
    }
  };

  const getContent = () => {
    if (instructionMode === 'write') {
      return customContent;
    }
    if (instructionMode === 'upload') {
      return customContent;
    }
    return `---\nname: ${name || id || 'New Skill'}\ndescription: ${description}\n---\n\n# ${name || id || 'New Skill'}\n\n<!-- scaffold: replace with instructions -->\n`;
  };

  const handlePreview = async () => {
    if (!validate()) return;

    setLoading(true);
    setApiError(null);

    const ops = operations
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean);
    const trigs = triggers
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean);
    const nots = notFor
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean);

    try {
      const prop = await previewSkillCreate({
        id: id.trim(),
        name: name.trim(),
        description: description.trim(),
        collection: collection.trim() || 'core',
        content: getContent(),
        routing:
          ops.length > 0 || trigs.length > 0 || nots.length > 0 || minScope
            ? {
                operations: ops,
                triggers: trigs,
                not_for: nots,
                min_scope: minScope,
              }
            : undefined,
      });
      setProposal(prop);
      setPreviewOpen(true);
    } catch (err) {
      if (err instanceof ApiError) {
        setApiError(err.render.WHY || err.render.ERROR || err.message);
      } else if (err instanceof Error) {
        setApiError(err.message);
      }
    } finally {
      setLoading(false);
    }
  };

  const handleConfirm = async (pins: { proposalId: string; proposalDigest: string; baseVersion: string }) => {
    try {
      const res = await confirmSkillMutation(pins.proposalId, pins);
      setPreviewOpen(false);
      setReceiptToast(`Created · ${res.operation_id}`);
      setTimeout(() => {
        navigate(`/skills/${id.trim()}`);
      }, 1200);
    } catch (err) {
      if (err instanceof ApiError) {
        setApiError(err.render.WHY || err.render.ERROR || err.message);
      } else if (err instanceof Error) {
        setApiError(err.message);
      }
    }
  };

  const pins = proposal ? (proposal.confirmation.confirmation?.pins ?? proposal.confirmation.pins) : null;

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)', maxWidth: '760px', width: '100%' }}>
      {apiError && (
        <div className="fg-banner fg-banner--danger" role="alert">
          <span className="fg-banner__dot" />
          <span className="fg-banner__body">
            <span>{apiError}</span>
          </span>
        </div>
      )}

      {/* Identity Card */}
      <section className="fg-card" style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
        <div className="fg-card__title">
          <span>{LABEL_IDENTITY}</span>
        </div>

        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(200px, 1fr))', gap: 'var(--space-4)' }}>
          <div className="fg-field">
            <label className="fg-field__label t-label" htmlFor="create-id">
              <span>{LABEL_SKILL_ID} </span>
              <span className="fg-field__req">{REQ_STAR}</span>
            </label>
            <input
              id="create-id"
              className="fg-input"
              style={{ fontFamily: 'var(--font-mono)' }}
              placeholder="e.g. release-checklist"
              value={id}
              onChange={(e) => setId(e.target.value)}
              onBlur={validate}
              aria-invalid={Boolean(errors.id)}
            />
            {errors.id ? (
              <span className="fg-field__hint" style={{ color: 'var(--color-danger)' }}>
                <span>{errors.id}</span>
              </span>
            ) : (
              <span className="fg-field__hint">
                <span>{HINT_SKILL_ID}</span>
              </span>
            )}
          </div>

          <div className="fg-field">
            <label className="fg-field__label t-label" htmlFor="create-name">
              <span>{LABEL_DISPLAY_NAME} </span>
              <span className="fg-field__req">{REQ_STAR}</span>
            </label>
            <input
              id="create-name"
              className="fg-input"
              placeholder="Release Checklist"
              value={name}
              onChange={(e) => setName(e.target.value)}
              onBlur={validate}
              aria-invalid={Boolean(errors.name)}
            />
            {errors.name && (
              <span className="fg-field__hint" style={{ color: 'var(--color-danger)' }}>
                <span>{errors.name}</span>
              </span>
            )}
          </div>
        </div>

        <div className="fg-field">
          <label className="fg-field__label t-label" htmlFor="create-desc">
            <span>{LABEL_DESCRIPTION}</span>
          </label>
          <textarea
            id="create-desc"
            className="fg-input fg-input--area"
            style={{ minHeight: '64px' }}
            value={description}
            onChange={(e) => setDescription(e.target.value)}
          />
        </div>

        <div className="fg-field" style={{ maxWidth: '240px' }}>
          <label className="fg-field__label t-label" htmlFor="create-col">
            <span>{LABEL_COLLECTION}</span>
          </label>
          <div className="fg-select">
            <select id="create-col" value={collection} onChange={(e) => setCollection(e.target.value)}>
              {COLLECTIONS.map((col) => (
                <option key={col} value={col}>
                  {col}
                </option>
              ))}
            </select>
            <span className="fg-select__chev" aria-hidden="true">
              ▾
            </span>
          </div>
        </div>
      </section>

      {/* Routing Card */}
      <section className="fg-card" style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
        <div className="fg-card__title">
          <span>{LABEL_ROUTING}</span>
        </div>

        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(200px, 1fr))', gap: 'var(--space-4)' }}>
          <div className="fg-field">
            <label className="fg-field__label t-label" htmlFor="create-ops">
              <span>{LABEL_OPERATIONS}</span>
            </label>
            <input
              id="create-ops"
              className="fg-input"
              placeholder="write, review"
              value={operations}
              onChange={(e) => setOperations(e.target.value)}
            />
          </div>

          <div className="fg-field">
            <label className="fg-field__label t-label" htmlFor="create-trigs">
              <span>{LABEL_TRIGGERS}</span>
            </label>
            <input
              id="create-trigs"
              className="fg-input"
              placeholder="prepare a release"
              value={triggers}
              onChange={(e) => setTriggers(e.target.value)}
            />
          </div>

          <div className="fg-field">
            <label className="fg-field__label t-label" htmlFor="create-notfor">
              <span>{LABEL_NOT_FOR}</span>
            </label>
            <input
              id="create-notfor"
              className="fg-input"
              value={notFor}
              onChange={(e) => setNotFor(e.target.value)}
            />
          </div>

          <div className="fg-field">
            <label className="fg-field__label t-label" htmlFor="create-scope">
              <span>{LABEL_MIN_SCOPE}</span>
            </label>
            <div className="fg-select">
              <select id="create-scope" value={minScope} onChange={(e) => setMinScope(e.target.value)}>
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
        </div>
      </section>

      {/* Instructions Card */}
      <section className="fg-card" style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
        <div className="fg-card__title">
          <span>{LABEL_INSTRUCTIONS}</span>
        </div>

        <div className="fg-seg" style={{ alignSelf: 'flex-start' }}>
          <button
            type="button"
            className={`fg-seg__btn ${instructionMode === 'scaffold' ? 'fg-seg__btn--active' : ''}`}
            onClick={() => setInstructionMode('scaffold')}
          >
            <span>{LABEL_SCAFFOLD}</span>
          </button>
          <button
            type="button"
            className={`fg-seg__btn ${instructionMode === 'upload' ? 'fg-seg__btn--active' : ''}`}
            onClick={() => setInstructionMode('upload')}
          >
            <span>{LABEL_UPLOAD}</span>
          </button>
          <button
            type="button"
            className={`fg-seg__btn ${instructionMode === 'write' ? 'fg-seg__btn--active' : ''}`}
            onClick={() => setInstructionMode('write')}
          >
            <span>{LABEL_WRITE}</span>
          </button>
        </div>

        {instructionMode === 'scaffold' && (
          <div className="fg-caveat fg-caveat--info">
            <span>{HINT_SCAFFOLD}</span>
          </div>
        )}

        {instructionMode === 'upload' && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-2)' }}>
            <label
              className="fg-upload"
              htmlFor="upload-input"
              style={{
                display: 'flex',
                flexDirection: 'column',
                alignItems: 'center',
                justifyContent: 'center',
                padding: 'var(--space-6)',
                border: '1px dashed var(--color-border)',
                borderRadius: 'var(--card-radius)',
                background: 'var(--color-surface-sunken)',
                cursor: 'pointer',
              }}
            >
              <span className="t-body-sm">
                <span>{HINT_UPLOAD}</span>
              </span>
              <input
                id="upload-input"
                type="file"
                accept=".md"
                style={{ display: 'none' }}
                onChange={(e) => {
                  const file = e.target.files?.[0];
                  if (file) handleFileUpload(file);
                }}
              />
            </label>
            {errors.upload && (
              <span className="fg-field__hint" style={{ color: 'var(--color-danger)' }}>
                <span>{errors.upload}</span>
              </span>
            )}
            {customContent && (
              <span className="t-caption" style={{ color: 'var(--color-success)' }}>
                <span>{MSG_FILE_LOADED}</span>
              </span>
            )}
          </div>
        )}

        {instructionMode === 'write' && (
          <textarea
            className="fg-input fg-input--area"
            style={{ minHeight: '220px', fontFamily: 'var(--font-mono)', fontSize: '13px' }}
            placeholder="# Release Checklist\n\nInstructions..."
            value={customContent}
            onChange={(e) => setCustomContent(e.target.value)}
          />
        )}
      </section>

      {/* Sticky Footer */}
      <div
        style={{
          position: 'sticky',
          bottom: 0,
          display: 'flex',
          justifyContent: 'flex-end',
          gap: 'var(--space-2)',
          padding: 'var(--space-3) var(--space-4)',
          background: 'var(--color-surface)',
          border: '1px solid var(--color-border)',
          borderRadius: 'var(--card-radius)',
          zIndex: 10,
        }}
      >
        <button type="button" className="fg-btn fg-btn--ghost" onClick={() => navigate('/skills')}>
          <span>{LABEL_CANCEL}</span>
        </button>
        <button
          type="button"
          className="fg-btn fg-btn--primary"
          onClick={handlePreview}
          disabled={loading}
        >
          <span>{loading ? 'Validating…' : LABEL_PREVIEW_DRAFT}</span>
        </button>
      </div>

      {previewOpen && proposal && pins && (
        <ProposalPreview
          open={previewOpen}
          title={proposal.summary || 'Create draft skill'}
          target={id.trim()}
          fromState="none"
          toState="draft"
          paths={proposal.paths}
          impact={proposal.impact}
          warning={proposal.warning}
          diff={proposal.diff}
          stat={proposal.stat}
          proposalId={pins.proposal_id}
          proposalDigest={pins.proposal_digest}
          baseVersion={pins.base_version}
          confirmLabel={t('action.create_skill')}
          onConfirm={handleConfirm}
          onCancel={() => setPreviewOpen(false)}
        />
      )}

      <Toast message={receiptToast} onClose={() => setReceiptToast(null)} />
    </div>
  );
}
