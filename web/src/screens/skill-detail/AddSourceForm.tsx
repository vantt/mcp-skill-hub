import { useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { ApiError } from '../../api/client';
import { confirmSourceProposal, previewAttachSource, useSources } from '../../api/queries';
import type { ConfirmationPins, SourceProposal } from '../../api/types';
import { ProposalPreview } from '../../components/ProposalPreview';

const TITLE_ADD = 'Add a learning reference';
const INTRO_ADD =
  'A learning reference is a repository your curator agent monitors for ideas that should improve this skill. You see the exact change before anything is written.';
const MODE_WATCHED = 'Link a watched source';
const MODE_URL = 'Watch a GitHub repository';
const LABEL_SOURCE = 'Watched source';
const LABEL_URL = 'GitHub repository URL';
const LABEL_REF = 'Branch or tag';
const LABEL_PATH = 'Folder path';
const HINT_OPTIONAL = 'Optional.';
const REQ_STAR = '*';
const PLACEHOLDER_URL = 'https://github.com/owner/repo';
const PLACEHOLDER_REF = 'main';
const PLACEHOLDER_PATH = 'docs/guides';
const MSG_LOADING_SOURCES = 'Loading watched sources…';
const MSG_NO_WATCHED =
  'Every watched source is already linked to this skill. Watch a GitHub repository instead.';
const MSG_SOURCES_FAILED = 'Could not load watched sources. Watch a GitHub repository instead.';
const BTN_PREVIEW = 'Preview';
const BTN_PREVIEWING = 'Preparing preview…';
const BTN_CHECKING_REPO = 'Checking repository…';
const BTN_CANCEL = 'Cancel';
const BTN_CONFIRM = 'Add source';

type Mode = 'watched' | 'url';

// The message to show for a failed API call: the server's reason first, then its headline.
export function sourceErrorText(err: unknown, fallback: string): string {
  if (err instanceof ApiError) return err.render.WHY || err.render.ERROR || err.message;
  return err instanceof Error ? err.message : fallback;
}

export function proposalPins(p: SourceProposal): ConfirmationPins | null {
  const pins = p.confirmation?.confirmation?.pins;
  return pins && pins.proposal_id ? pins : null;
}

interface AddSourceFormProps {
  skillId: string;
  // Sources already linked to the skill as learning references.
  linkedSourceIds: string[];
  // The skill's upstream source record, which cannot also be a learning reference.
  upstreamSourceId?: string;
  onAdded: (message: string) => void;
  onCancel: () => void;
}

export function AddSourceForm({ skillId, linkedSourceIds, upstreamSourceId, onAdded, onCancel }: AddSourceFormProps) {
  const queryClient = useQueryClient();
  const sources = useSources();

  const excluded = new Set(linkedSourceIds);
  if (upstreamSourceId) excluded.add(upstreamSourceId);
  const options = (sources.data?.sources ?? [])
    .filter((item) => !excluded.has(item.record.id) && !item.skills.includes(skillId))
    .map((item) => item.record);

  const [chosenMode, setChosenMode] = useState<Mode | null>(null);
  const mode: Mode = chosenMode ?? (options.length > 0 ? 'watched' : 'url');
  const [sourceId, setSourceId] = useState('');
  const [locator, setLocator] = useState('');
  const [ref, setRef] = useState('');
  const [path, setPath] = useState('');

  const [previewing, setPreviewing] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [info, setInfo] = useState<string | null>(null);
  const [proposal, setProposal] = useState<SourceProposal | null>(null);
  const [confirming, setConfirming] = useState(false);
  const [confirmError, setConfirmError] = useState<string | null>(null);
  const [confirmAttempt, setConfirmAttempt] = useState(0);

  const selectedSourceId = options.some((o) => o.id === sourceId) ? sourceId : (options[0]?.id ?? '');
  const canPreview = mode === 'watched' ? selectedSourceId !== '' : locator.trim() !== '';

  const handlePreview = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!canPreview || previewing) return;
    setPreviewing(true);
    setError(null);
    setInfo(null);
    try {
      const body =
        mode === 'watched'
          ? { source_id: selectedSourceId }
          : {
              locator: locator.trim(),
              ...(ref.trim() ? { ref: ref.trim() } : {}),
              ...(path.trim() ? { path: path.trim() } : {}),
            };
      const res = await previewAttachSource(skillId, body);
      if (proposalPins(res)) {
        setConfirmError(null);
        setProposal(res);
      } else {
        setInfo(res.summary);
      }
    } catch (err) {
      setError(sourceErrorText(err, 'Failed to preview the source'));
    } finally {
      setPreviewing(false);
    }
  };

  const handleConfirm = async (pins: { proposalId: string; proposalDigest: string; baseVersion: string }) => {
    setConfirming(true);
    setConfirmError(null);
    try {
      const res = await confirmSourceProposal(pins.proposalId, {
        proposal_digest: pins.proposalDigest,
        base_version: pins.baseVersion,
      });
      void queryClient.invalidateQueries({ queryKey: ['skill-sources', skillId] });
      void queryClient.invalidateQueries({ queryKey: ['sources'] });
      setProposal(null);
      onAdded(res.summary || `Added a learning reference to ${skillId}.`);
    } catch (err) {
      setConfirmError(sourceErrorText(err, 'Failed to add the source'));
      setConfirmAttempt((n) => n + 1);
    } finally {
      setConfirming(false);
    }
  };

  const pins = proposal ? proposalPins(proposal) : null;
  const proposalSourceId =
    proposal?.source?.id || (mode === 'watched' ? selectedSourceId : locator.trim());
  const busyLabel = mode === 'url' ? BTN_CHECKING_REPO : BTN_PREVIEWING;

  let watchedNotice: string | null = null;
  if (sources.isLoading) watchedNotice = MSG_LOADING_SOURCES;
  else if (sources.error) watchedNotice = MSG_SOURCES_FAILED;
  else if (options.length === 0) watchedNotice = MSG_NO_WATCHED;

  return (
    <section
      className="fg-card"
      aria-labelledby="add-source-title"
      style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}
    >
      <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-1)' }}>
        <div id="add-source-title" className="fg-card__title">
          <span>{TITLE_ADD}</span>
        </div>
        <p style={{ margin: 0, fontSize: '13px', color: 'var(--color-text-muted)', maxWidth: '72ch' }}>
          <span>{INTRO_ADD}</span>
        </p>
      </div>

      <div className="fg-seg" role="radiogroup" aria-label="How to add the source" style={{ width: 'fit-content' }}>
        {(['watched', 'url'] as const).map((m) => (
          <button
            key={m}
            type="button"
            role="radio"
            aria-checked={mode === m}
            className={`fg-seg__btn ${mode === m ? 'fg-seg__btn--active' : ''}`}
            onClick={() => {
              setChosenMode(m);
              setError(null);
              setInfo(null);
            }}
            disabled={previewing}
          >
            <span>{m === 'watched' ? MODE_WATCHED : MODE_URL}</span>
          </button>
        ))}
      </div>

      <form onSubmit={handlePreview} style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
        {mode === 'watched' ? (
          <div className="fg-field" style={{ maxWidth: '480px' }}>
            <label className="fg-field__label t-label" htmlFor="add-source-id">
              <span>{LABEL_SOURCE}</span>
            </label>
            <div className="fg-select">
              <select
                id="add-source-id"
                value={selectedSourceId}
                onChange={(e) => setSourceId(e.target.value)}
                disabled={previewing || options.length === 0}
              >
                {options.map((o) => (
                  <option key={o.id} value={o.id}>
                    {o.locator.repository || o.locator.url ? `${o.id} · ${o.locator.repository || o.locator.url}` : o.id}
                  </option>
                ))}
              </select>
              <span className="fg-select__chev" aria-hidden="true">
                ▾
              </span>
            </div>
            {watchedNotice && (
              <span className="fg-field__hint">
                <span>{watchedNotice}</span>
              </span>
            )}
          </div>
        ) : (
          <>
            <div className="fg-field" style={{ maxWidth: '640px' }}>
              <label className="fg-field__label t-label" htmlFor="add-source-url">
                <span>{LABEL_URL}</span>
                <span className="fg-field__req">{REQ_STAR}</span>
              </label>
              <input
                id="add-source-url"
                className="fg-input"
                style={{ fontFamily: 'var(--font-mono)', fontSize: '13px' }}
                placeholder={PLACEHOLDER_URL}
                value={locator}
                onChange={(e) => setLocator(e.target.value)}
                disabled={previewing}
                required
              />
            </div>
            <div
              style={{
                display: 'grid',
                gridTemplateColumns: 'repeat(auto-fit, minmax(200px, 1fr))',
                gap: 'var(--space-4)',
                maxWidth: '640px',
              }}
            >
              <div className="fg-field">
                <label className="fg-field__label t-label" htmlFor="add-source-ref">
                  <span>{LABEL_REF}</span>
                </label>
                <input
                  id="add-source-ref"
                  className="fg-input"
                  style={{ fontFamily: 'var(--font-mono)', fontSize: '13px' }}
                  placeholder={PLACEHOLDER_REF}
                  value={ref}
                  onChange={(e) => setRef(e.target.value)}
                  disabled={previewing}
                />
                <span className="fg-field__hint">
                  <span>{HINT_OPTIONAL}</span>
                </span>
              </div>
              <div className="fg-field">
                <label className="fg-field__label t-label" htmlFor="add-source-path">
                  <span>{LABEL_PATH}</span>
                </label>
                <input
                  id="add-source-path"
                  className="fg-input"
                  style={{ fontFamily: 'var(--font-mono)', fontSize: '13px' }}
                  placeholder={PLACEHOLDER_PATH}
                  value={path}
                  onChange={(e) => setPath(e.target.value)}
                  disabled={previewing}
                />
                <span className="fg-field__hint">
                  <span>{HINT_OPTIONAL}</span>
                </span>
              </div>
            </div>
          </>
        )}

        {error && (
          <div className="fg-banner fg-banner--danger" role="alert">
            <span className="fg-banner__dot" />
            <span className="fg-banner__body">
              <span>{error}</span>
            </span>
          </div>
        )}
        {info && (
          <div className="fg-banner fg-banner--info" role="status">
            <span className="fg-banner__dot" />
            <span className="fg-banner__body">
              <span>{info}</span>
            </span>
          </div>
        )}

        <div style={{ display: 'flex', gap: 'var(--space-2)', flexWrap: 'wrap' }}>
          <button type="submit" className="fg-btn fg-btn--primary" disabled={!canPreview || previewing}>
            <span>{previewing ? busyLabel : BTN_PREVIEW}</span>
          </button>
          <button type="button" className="fg-btn fg-btn--ghost" onClick={onCancel} disabled={previewing}>
            <span>{BTN_CANCEL}</span>
          </button>
        </div>
      </form>

      {proposal && pins && (
        <ProposalPreview
          key={confirmAttempt}
          open
          title={`Add source ${proposalSourceId}?`}
          target={skillId}
          impact={proposal.summary}
          paths={[...(proposal.diff?.added ?? []), ...(proposal.diff?.modified ?? [])]}
          warning={proposal.warnings?.[0]?.summary}
          proposalId={pins.proposal_id}
          proposalDigest={pins.proposal_digest}
          baseVersion={pins.base_version}
          confirmLabel={BTN_CONFIRM}
          isLoading={confirming}
          networkError={confirmError}
          onConfirm={(p) => void handleConfirm(p)}
          onCancel={() => setProposal(null)}
        />
      )}
    </section>
  );
}
