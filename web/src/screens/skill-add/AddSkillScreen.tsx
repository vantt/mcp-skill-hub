import { useRef, useState } from 'react';
import { useNavigate } from 'react-router';
import { confirmSkillAdd, previewSkillAdd } from '../../api/queries';
import type { SkillAddProposal } from '../../api/types';
import { ApiError } from '../../api/client';
import { DiffView } from '../../components/DiffView';
import { StatusBadge } from '../../components/StatusBadge';
import { Toast } from '../../components/Toast';
import { useT } from '../../i18n';

const STEP_DISCOVER = 'Discover';
const STEP_REVIEW = 'Review';
const LABEL_COLLECTION = 'Collection';
const LABEL_TARGET_ID = 'Target ID';
const LABEL_SKILL_NAME_PATH = 'Skill name or path';
const LABEL_PREVIEW = 'Preview';
const LABEL_IMPORT_ALL = 'Import all';
const LABEL_DISCOVER_SKILLS = 'Discover skills';
const LABEL_CANCEL = 'Cancel';
const LABEL_BACK = 'Back';
const LABEL_CONFIRM_IMPORT = 'Confirm import';
const LABEL_TECH_DETAILS = 'Technical details';
const LABEL_RESOURCES = 'Resources';
const LABEL_IDENTITY = 'Identity';
const LABEL_ORIGIN = 'Origin';
const LABEL_DIFF = 'Diff';
const LABEL_ADVANCED = 'Advanced';
const HINT_GITHUB_URL = 'Public repository, subfolder or file URL.';
const HINT_DISCOVERING = 'Cloning and inspecting the pinned revision…';
const STEP_TWO = '2';
const LABEL_GITHUB_URL = 'GitHub URL ';
const REQ_STAR = '*';
const LABEL_SUMMARY = 'Summary';
const LABEL_TARGET_STATE = 'Target state';
const LABEL_LOCATOR = 'Locator';
const LABEL_ASSESSMENT = 'Assessment: public GitHub source repository.';
const LABEL_PROP_ID = 'proposal_id: ';
const LABEL_PROP_DIGEST = 'proposal_digest: ';
const LABEL_BASE_VERSION = 'base_version: ';
const LABEL_IMPORTING = 'Importing…';

export function AddSkillScreen() {
  const t = useT();
  const navigate = useNavigate();

  const [step, setStep] = useState<1 | 2>(1);
  const [locator, setLocator] = useState('https://github.com/acme/agent-skills/tree/main/skills');
  const [collection, setCollection] = useState('core');
  const [targetId, setTargetId] = useState('');
  const [selection, setSelection] = useState('');

  const [discovering, setDiscovering] = useState(false);
  const [selectionRequiredWhy, setSelectionRequiredWhy] = useState<string | null>(null);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);

  const [proposal, setProposal] = useState<SkillAddProposal | null>(null);
  const [confirming, setConfirming] = useState(false);
  const [receiptToast, setReceiptToast] = useState<string | null>(null);

  const abortControllerRef = useRef<AbortController | null>(null);

  const handleDiscover = async (importAll = false, customSelection?: string) => {
    setErrorMessage(null);
    setSelectionRequiredWhy(null);
    setDiscovering(true);

    abortControllerRef.current = new AbortController();

    try {
      const res = await previewSkillAdd(
        {
          locator: locator.trim(),
          collection: collection.trim() || undefined,
          target_id: targetId.trim() || undefined,
          selection: customSelection !== undefined ? customSelection : (selection.trim() || undefined),
          all: importAll,
        },
        abortControllerRef.current.signal,
      );
      setProposal(res);
      setStep(2);
    } catch (err) {
      if (err instanceof ApiError) {
        if (err.code === 'skill_selection_required') {
          setSelectionRequiredWhy(err.render.WHY || err.message);
        } else {
          setErrorMessage(err.render.WHY || err.render.ERROR || err.message);
        }
      } else if (err instanceof Error && err.name !== 'AbortError') {
        setErrorMessage(err.message);
      }
    } finally {
      setDiscovering(false);
      abortControllerRef.current = null;
    }
  };

  const handleCancelDiscover = () => {
    if (abortControllerRef.current) {
      abortControllerRef.current.abort();
      abortControllerRef.current = null;
    }
    setDiscovering(false);
  };

  const handleConfirm = async () => {
    if (!proposal || confirming) return;

    const pins = proposal.confirmation.confirmation?.pins ?? proposal.confirmation.pins;
    if (!pins) return;

    setConfirming(true);
    setErrorMessage(null);

    try {
      const res = await confirmSkillAdd(pins);
      setReceiptToast(`Imported · ${res.operation_id}`);
      setTimeout(() => {
        navigate('/skills');
      }, 1500);
    } catch (err) {
      if (err instanceof ApiError) {
        setErrorMessage(err.render.WHY || err.render.ERROR || err.message);
      } else if (err instanceof Error) {
        setErrorMessage(err.message);
      }
    } finally {
      setConfirming(false);
    }
  };

  const pins = proposal ? (proposal.confirmation.confirmation?.pins ?? proposal.confirmation.pins) : null;

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)', maxWidth: '960px' }}>
      {/* Stepper */}
      <div className="fg-stepper">
        <span className={`fg-step ${step === 1 ? 'fg-step--active' : 'fg-step--complete'}`}>
          <span className="fg-step__dot">{step === 1 ? '1' : '✓'}</span>
          <span>{STEP_DISCOVER}</span>
        </span>
        <span className="fg-stepper__rule" />
        <span className={`fg-step ${step === 2 ? 'fg-step--active' : ''}`}>
          <span className="fg-step__dot">{STEP_TWO}</span>
          <span>{STEP_REVIEW}</span>
        </span>
      </div>

      {errorMessage && (
        <div className="fg-banner fg-banner--danger" role="alert">
          <span className="fg-banner__dot" />
          <span className="fg-banner__body">
            <span>{errorMessage}</span>
          </span>
        </div>
      )}

      {/* Step 1: Discover */}
      {step === 1 && (
        <section
          className="fg-card"
          style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)', maxWidth: '760px' }}
        >
          <div className="fg-field">
            <label className="fg-field__label t-label" htmlFor="gh-url">
              <span>{LABEL_GITHUB_URL}</span>
              <span className="fg-field__req">{REQ_STAR}</span>
            </label>
            <input
              id="gh-url"
              className="fg-input"
              style={{ fontFamily: 'var(--font-mono)', fontSize: '13px' }}
              value={locator}
              onChange={(e) => setLocator(e.target.value)}
              placeholder="https://github.com/acme/agent-skills/tree/main/skills"
            />
            <span className="fg-field__hint">
              <span>{HINT_GITHUB_URL}</span>
            </span>
          </div>

          <details className="fg-acc">
            <summary style={{ cursor: 'pointer', fontWeight: 600 }}>
              <span>{LABEL_ADVANCED}</span>
            </summary>
            <div
              className="fg-acc__body"
              style={{
                display: 'grid',
                gridTemplateColumns: 'repeat(auto-fit, minmax(200px, 1fr))',
                gap: 'var(--space-4)',
                paddingTop: 'var(--space-3)',
              }}
            >
              <div className="fg-field">
                <label className="fg-field__label t-label" htmlFor="gh-col">
                  <span>{LABEL_COLLECTION}</span>
                </label>
                <input
                  id="gh-col"
                  className="fg-input"
                  value={collection}
                  onChange={(e) => setCollection(e.target.value)}
                />
              </div>

              <div className="fg-field">
                <label className="fg-field__label t-label" htmlFor="gh-target-id">
                  <span>{LABEL_TARGET_ID}</span>
                </label>
                <input
                  id="gh-target-id"
                  className="fg-input"
                  placeholder="Optional target ID"
                  value={targetId}
                  onChange={(e) => setTargetId(e.target.value)}
                />
              </div>
            </div>
          </details>

          {selectionRequiredWhy && (
            <div className="fg-caveat fg-caveat--info" role="status" style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' }}>
              <span className="t-body-sm" style={{ fontFamily: 'var(--font-mono)' }}>
                <span>{selectionRequiredWhy}</span>
              </span>
              <div style={{ display: 'flex', flexWrap: 'wrap', gap: 'var(--space-2)', alignItems: 'flex-end' }}>
                <div className="fg-field" style={{ flex: '1 1 220px' }}>
                  <label className="fg-field__label t-label" htmlFor="sel-path">
                    <span>{LABEL_SKILL_NAME_PATH}</span>
                  </label>
                  <input
                    id="sel-path"
                    className="fg-input"
                    value={selection}
                    onChange={(e) => setSelection(e.target.value)}
                    placeholder="e.g. pr-description"
                  />
                </div>
                <button
                  type="button"
                  className="fg-btn fg-btn--primary"
                  onClick={() => handleDiscover(false, selection)}
                  disabled={discovering}
                >
                  <span>{LABEL_PREVIEW}</span>
                </button>
                <button
                  type="button"
                  className="fg-btn fg-btn--secondary"
                  onClick={() => handleDiscover(true)}
                  disabled={discovering}
                >
                  <span>{LABEL_IMPORT_ALL}</span>
                </button>
              </div>
            </div>
          )}

          {discovering && (
            <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-3)' }} role="status">
              <span className="fg-spinner" />
              <span className="t-body-sm" style={{ color: 'var(--color-text-muted)' }}>
                <span>{HINT_DISCOVERING}</span>
              </span>
              <button type="button" className="fg-btn fg-btn--ghost" onClick={handleCancelDiscover}>
                <span>{LABEL_CANCEL}</span>
              </button>
            </div>
          )}

          <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 'var(--space-2)' }}>
            <button type="button" className="fg-btn fg-btn--ghost" onClick={() => navigate('/skills')}>
              <span>{LABEL_CANCEL}</span>
            </button>
            <button
              type="button"
              className="fg-btn fg-btn--primary"
              onClick={() => handleDiscover(false)}
              disabled={discovering || !locator.trim()}
            >
              <span>{LABEL_DISCOVER_SKILLS}</span>
            </button>
          </div>
        </section>
      )}

      {/* Step 2: Review */}
      {step === 2 && proposal && (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
          <div
            style={{
              display: 'grid',
              gridTemplateColumns: 'repeat(auto-fit, minmax(min(100%, 320px), 1fr))',
              gap: 'var(--space-4)',
              alignItems: 'start',
            }}
          >
            <section className="fg-card" style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' }}>
              <div className="fg-card__title">
                <span>{LABEL_IDENTITY}</span>
              </div>
              <div className="fg-facts">
                <div className="fg-fact">
                  <div className="fg-fact__label">
                    <span>{LABEL_SUMMARY}</span>
                  </div>
                  <div className="fg-fact__value">
                    <span>{proposal.summary}</span>
                  </div>
                </div>
                <div className="fg-fact">
                  <div className="fg-fact__label">
                    <span>{LABEL_TARGET_STATE}</span>
                  </div>
                  <div className="fg-fact__value">
                    <StatusBadge variant="chip" label={t('lifecycle_short.draft')} tone="warning" />
                  </div>
                </div>
              </div>
            </section>

            <section className="fg-card" style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' }}>
              <div className="fg-card__title">
                <span>{LABEL_ORIGIN}</span>
              </div>
              <div className="fg-facts" style={{ fontFamily: 'var(--font-mono)', fontSize: '13px' }}>
                <div className="fg-fact">
                  <div className="fg-fact__label">
                    <span>{LABEL_LOCATOR}</span>
                  </div>
                  <div className="fg-fact__value" style={{ wordBreak: 'break-all' }}>
                    <span>{locator}</span>
                  </div>
                </div>
              </div>
            </section>

            <section className="fg-card" style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-2)' }}>
              <div className="fg-card__title">
                <span>{LABEL_RESOURCES}</span>
              </div>
              <span className="t-caption" style={{ color: 'var(--color-text-subtle)' }}>
                <span>{LABEL_ASSESSMENT}</span>
              </span>
            </section>
          </div>

          {proposal.diff && (
            <section className="fg-card" style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)', padding: 'var(--space-4)' }}>
              <div className="fg-card__title">
                <span>{LABEL_DIFF}</span>
              </div>
              <DiffView diff={proposal.diff} />
            </section>
          )}

          {pins && (
            <details className="fg-acc">
              <summary style={{ cursor: 'pointer', fontWeight: 600, color: 'var(--color-text-muted)' }}>
                <span>{LABEL_TECH_DETAILS}</span>
              </summary>
              <div
                className="fg-acc__body"
                style={{
                  fontFamily: 'var(--font-mono)',
                  fontSize: '12.5px',
                  display: 'grid',
                  gap: '4px',
                  paddingTop: 'var(--space-2)',
                }}
              >
                <div>
                  <span style={{ color: 'var(--color-text-muted)' }}>{LABEL_PROP_ID}</span>
                  <span>{pins.proposal_id}</span>
                </div>
                <div>
                  <span style={{ color: 'var(--color-text-muted)' }}>{LABEL_PROP_DIGEST}</span>
                  <span>{pins.proposal_digest}</span>
                </div>
                <div>
                  <span style={{ color: 'var(--color-text-muted)' }}>{LABEL_BASE_VERSION}</span>
                  <span>{pins.base_version}</span>
                </div>
              </div>
            </details>
          )}

          <div
            style={{
              position: 'sticky',
              bottom: 0,
              display: 'flex',
              flexWrap: 'wrap',
              gap: 'var(--space-2)',
              justifyContent: 'flex-end',
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
              onClick={() => setStep(1)}
              disabled={confirming}
            >
              <span>{LABEL_BACK}</span>
            </button>
            <button
              type="button"
              className="fg-btn fg-btn--ghost"
              onClick={() => navigate('/skills')}
              disabled={confirming}
            >
              <span>{LABEL_CANCEL}</span>
            </button>
            <button
              type="button"
              className="fg-btn fg-btn--primary"
              onClick={handleConfirm}
              disabled={confirming}
            >
              <span>{confirming ? LABEL_IMPORTING : LABEL_CONFIRM_IMPORT}</span>
            </button>
          </div>
        </div>
      )}

      <Toast message={receiptToast} onClose={() => setReceiptToast(null)} />
    </div>
  );
}
