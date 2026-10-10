import { useEffect, useRef, useState } from 'react';
import { DiffView } from './DiffView';
import { StatusBadge } from './StatusBadge';


const LABEL_STALE_PROPOSAL = 'This proposal is no longer current. The base changed after it was created.';
const LABEL_CREATE_NEW_PREVIEW = 'Create new preview';
const LABEL_AFFECTED_PATHS = 'Affected paths';
const LABEL_ROUTING_IMPACT = 'What changes';
const LABEL_TECH_DETAILS = 'Technical details';
const LABEL_PROP_ID = 'proposal_id: ';
const LABEL_PROP_DIGEST = 'proposal_digest: ';
const LABEL_BASE_VERSION = 'base_version: ';
const LABEL_COPIED = 'Copied';
const LABEL_COPY_PINS = '⧉ Copy pins';
const LABEL_CANCEL = 'Cancel';
const LABEL_APPLYING = 'Applying…';
const LABEL_RETRY = 'Retry';
export interface ProposalPreviewProps {
  open: boolean;
  title: string;
  target: string;
  fromState?: string;
  toState?: string;
  paths?: string[];
  impact?: string;
  warning?: string;
  diff?: string;
  stat?: string;
  proposalId: string;
  proposalDigest: string;
  baseVersion: string;
  confirmLabel?: string;
  dangerConfirm?: boolean;
  isStale?: boolean;
  isLoading?: boolean;
  networkError?: string | null;
  onConfirm: (pins: { proposalId: string; proposalDigest: string; baseVersion: string }) => void;
  onCancel: () => void;
  onNewPreview?: () => void;
}

export function ProposalPreview({
  open,
  title,
  target,
  fromState,
  toState,
  paths = [],
  impact,
  warning,
  diff = '',
  stat,
  proposalId,
  proposalDigest,
  baseVersion,
  confirmLabel = 'Confirm',
  dangerConfirm = false,
  isStale = false,
  isLoading = false,
  networkError,
  onConfirm,
  onCancel,
  onNewPreview,
}: ProposalPreviewProps) {
  const [submitting, setSubmitting] = useState(false);
  const [copiedPins, setCopiedPins] = useState(false);
  const dialogRef = useRef<HTMLDivElement>(null);
  const triggerRef = useRef<Element | null>(null);

  useEffect(() => {
    if (open) {
      triggerRef.current = document.activeElement;
      dialogRef.current?.focus();
    } else {
      if (triggerRef.current && 'focus' in triggerRef.current) {
        (triggerRef.current as HTMLElement).focus();
      }
    }
  }, [open]);

  useEffect(() => {
    if (!open) return;
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        onCancel();
      }
    };
    document.addEventListener('keydown', handleKeyDown);
    return () => document.removeEventListener('keydown', handleKeyDown);
  }, [open, onCancel]);

  if (!open) return null;

  const handleConfirmClick = () => {
    if (submitting || isLoading || isStale) return;
    setSubmitting(true);
    onConfirm({
      proposalId,
      proposalDigest,
      baseVersion,
    });
  };

  const handleCopyPins = async () => {
    const text = `proposal_id: ${proposalId}\nproposal_digest: ${proposalDigest}\nbase_version: ${baseVersion}`;
    try {
      await navigator.clipboard.writeText(text);
      setCopiedPins(true);
      setTimeout(() => setCopiedPins(false), 2000);
    } catch {
      // Ignore clipboard write error
    }
  };

  const hasDiff = Array.isArray(diff) ? diff.length > 0 : typeof diff === 'string' && diff.trim() !== '';

  const stateTone = (state?: string) => {
    switch (state) {
      case 'active':
        return 'success';
      case 'draft':
        return 'warning';
      case 'deprecated':
        return 'danger';
      default:
        return 'neutral';
    }
  };

  return (
    <div
      className="fg-scrim"
      style={{
        position: 'fixed',
        inset: 0,
        backgroundColor: 'rgba(0, 0, 0, 0.5)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        zIndex: 90,
        padding: 'var(--space-4)',
      }}
      onClick={onCancel}
    >
      <div
        ref={dialogRef}
        className="fg-modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby="pp-t"
        tabIndex={-1}
        onClick={(e) => e.stopPropagation()}
        style={{
          width: '960px',
          maxWidth: '100%',
          maxHeight: 'calc(100vh - 48px)',
          display: 'flex',
          flexDirection: 'column',
          background: 'var(--color-surface)',
          borderRadius: 'var(--card-radius)',
          boxShadow: 'var(--shadow-xl)',
          overflow: 'hidden',
          outline: 'none',
        }}
      >
        {/* Header */}
        <div
          className="fg-modal__head"
          style={{
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'flex-start',
            padding: 'var(--space-4)',
            borderBottom: '1px solid var(--color-border)',
          }}
        >
          <div style={{ display: 'flex', flexDirection: 'column', gap: '6px', minWidth: 0 }}>
            <div id="pp-t" className="fg-modal__title" style={{ fontSize: '18px', fontWeight: 600 }}>
              <span>{title}</span>
            </div>
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: '8px', alignItems: 'center' }}>
              <span style={{ fontFamily: 'var(--font-mono)', fontSize: '12.5px', color: 'var(--color-text-muted)' }}>
                <span>{target}</span>
              </span>
              {fromState && (
                <>
                  <StatusBadge variant="chip" label={fromState} tone={stateTone(fromState)} />
                  <span aria-hidden="true">→</span>
                </>
              )}
              {toState && <StatusBadge variant="chip" label={toState} tone={stateTone(toState)} />}
            </div>
          </div>
          <button type="button" className="fg-icon-btn" aria-label="Close" onClick={onCancel}>
            ✕
          </button>
        </div>

        {/* Body */}
        <div
          className="fg-modal__body"
          style={{
            overflowY: 'auto',
            flex: 1,
            padding: 'var(--space-4)',
            display: 'flex',
            flexDirection: 'column',
            gap: 'var(--space-4)',
          }}
        >
          {isStale && (
            <div className="fg-banner fg-banner--danger" role="alert" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                <span className="fg-banner__dot" />
                <span className="fg-banner__body">
                  <span>{LABEL_STALE_PROPOSAL}</span>
                </span>
              </div>
              {onNewPreview && (
                <button type="button" className="fg-btn fg-btn--secondary" onClick={onNewPreview}>
                  <span>{LABEL_CREATE_NEW_PREVIEW}</span>
                </button>
              )}
            </div>
          )}

          {networkError && (
            <div className="fg-banner fg-banner--warning" role="alert">
              <span className="fg-banner__dot" />
              <span className="fg-banner__body">
                <span>{networkError}</span>
              </span>
            </div>
          )}

          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))', gap: 'var(--space-4)' }}>
            {Array.isArray(paths) && paths.length > 0 && (
              <div style={{ display: 'flex', flexDirection: 'column', gap: '6px' }}>
                <span className="t-label" style={{ color: 'var(--color-text-muted)' }}>
                  <span>{LABEL_AFFECTED_PATHS}</span>
                </span>
                {paths.map((p, idx) => (
                  <span key={idx} style={{ fontFamily: 'var(--font-mono)', fontSize: '12.5px', wordBreak: 'break-all' }}>
                    <span>{p}</span>
                  </span>
                ))}
              </div>
            )}

            {impact && (
              <div style={{ display: 'flex', flexDirection: 'column', gap: '6px' }}>
                <span className="t-label" style={{ color: 'var(--color-text-muted)' }}>
                  <span>{LABEL_ROUTING_IMPACT}</span>
                </span>
                <span className="t-body-sm" style={{ color: 'var(--color-text)' }}>
                  <span>{impact}</span>
                </span>
              </div>
            )}
          </div>

          {warning && (
            <div
              className="fg-caveat fg-caveat--warn"
              style={{
                padding: 'var(--space-2) var(--space-3)',
                background: 'var(--color-warning-tint)',
                borderLeft: '3px solid var(--color-warning)',
                borderRadius: 'var(--radius-xs)',
              }}
            >
              <span>{warning}</span>
            </div>
          )}

          {(hasDiff || Boolean(stat)) && <DiffView diff={diff} stat={stat} />}

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
                <span className="app-wrap" data-testid="pin-proposal-id">{proposalId}</span>
              </div>
              <div>
                <span style={{ color: 'var(--color-text-muted)' }}>{LABEL_PROP_DIGEST}</span>
                <span className="app-wrap" data-testid="pin-proposal-digest">{proposalDigest}</span>
              </div>
              <div>
                <span style={{ color: 'var(--color-text-muted)' }}>{LABEL_BASE_VERSION}</span>
                <span className="app-wrap" data-testid="pin-base-version">{baseVersion}</span>
              </div>
              <button
                type="button"
                className="fg-btn fg-btn--ghost"
                style={{ justifySelf: 'start', padding: '2px 8px', marginTop: '4px' }}
                onClick={handleCopyPins}
              >
                <span>{copiedPins ? LABEL_COPIED : LABEL_COPY_PINS}</span>
              </button>
            </div>
          </details>
        </div>

        {/* Actions */}
        <div
          className="fg-modal__actions"
          style={{
            display: 'flex',
            justifyContent: 'flex-end',
            gap: 'var(--space-2)',
            padding: 'var(--space-3) var(--space-4)',
            borderTop: '1px solid var(--color-border)',
            background: 'var(--color-surface-sunken)',
          }}
        >
          <button
            type="button"
            className="fg-btn fg-btn--ghost"
            onClick={() => {
              setSubmitting(false);
              onCancel();
            }}
            disabled={submitting || isLoading}
          >
            <span>{LABEL_CANCEL}</span>
          </button>
          <button
            type="button"
            className={`fg-btn ${dangerConfirm ? 'fg-btn--danger' : 'fg-btn--primary'}`}
            onClick={handleConfirmClick}
            disabled={isStale || submitting || isLoading}
          >
            <span>{isLoading || submitting ? LABEL_APPLYING : (networkError ? LABEL_RETRY : confirmLabel)}</span>
          </button>
        </div>
      </div>
    </div>
  );
}
