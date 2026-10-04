import { useEffect, useRef, useState } from 'react';
import { ConfirmDialog } from './ConfirmDialog';

const TITLE_CONFLICT = 'SKILL.md changed since you opened it';
const BANNER_CONFLICT =
  'Another edit was applied (digest differs). Merge your draft onto the latest content and preview again. Your draft is kept.';
const LABEL_YOUR_DRAFT = 'Your draft';
const LABEL_LATEST_CANONICAL = 'Latest canonical';
const LABEL_TECH_DETAILS = 'Technical details';
const LABEL_EXPECTED = 'expected: ';
const LABEL_LATEST = 'latest: ';
const LABEL_DOWNLOAD_DRAFT = 'Download draft (.md)';
const LABEL_COPY_DRAFT = 'Copy draft';
const LABEL_COPIED = 'Copied';
const LABEL_DISCARD_RELOAD = 'Discard draft and reload';
const LABEL_USE_LATEST = 'Use latest as base';

export interface ConflictDrawerProps {
  open: boolean;
  draftContent: string;
  latestContent: string;
  expectedDigest: string;
  latestDigest: string;
  onUseLatest: () => void;
  onDiscard: () => void;
  onClose: () => void;
}

export function ConflictDrawer({
  open,
  draftContent,
  latestContent,
  expectedDigest,
  latestDigest,
  onUseLatest,
  onDiscard,
  onClose,
}: ConflictDrawerProps) {
  const [confirmDiscardOpen, setConfirmDiscardOpen] = useState(false);
  const [copiedDraft, setCopiedDraft] = useState(false);
  const drawerRef = useRef<HTMLElement>(null);
  const triggerRef = useRef<Element | null>(null);

  useEffect(() => {
    if (open) {
      triggerRef.current = document.activeElement;
      drawerRef.current?.focus();
    } else {
      if (triggerRef.current && 'focus' in triggerRef.current) {
        (triggerRef.current as HTMLElement).focus();
      }
    }
  }, [open]);

  useEffect(() => {
    if (!open) return;
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && !confirmDiscardOpen) {
        onClose();
      }
    };
    document.addEventListener('keydown', handleKeyDown);
    return () => document.removeEventListener('keydown', handleKeyDown);
  }, [open, confirmDiscardOpen, onClose]);

  if (!open) return null;

  const handleCopyDraft = async () => {
    try {
      await navigator.clipboard.writeText(draftContent);
      setCopiedDraft(true);
      setTimeout(() => setCopiedDraft(false), 2000);
    } catch {
      // Ignore clipboard write error
    }
  };

  const handleDownloadDraft = () => {
    const blob = new Blob([draftContent], { type: 'text/markdown;charset=utf-8' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = 'draft.md';
    a.click();
    URL.revokeObjectURL(url);
  };

  return (
    <>
      <div
        className="fg-scrim"
        style={{
          position: 'fixed',
          inset: 0,
          backgroundColor: 'rgba(0, 0, 0, 0.5)',
          zIndex: 80,
        }}
        onClick={onClose}
      />

      <aside
        ref={drawerRef}
        className="fg-drawer"
        role="dialog"
        aria-modal="true"
        aria-labelledby="cd-t"
        tabIndex={-1}
        style={{
          position: 'fixed',
          top: 0,
          right: 0,
          bottom: 0,
          width: '640px',
          maxWidth: '100%',
          display: 'flex',
          flexDirection: 'column',
          zIndex: 90,
          background: 'var(--color-surface)',
          boxShadow: 'var(--shadow-xl)',
          outline: 'none',
        }}
      >
        <div
          className="fg-drawer__head"
          style={{
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            padding: 'var(--space-4)',
            borderBottom: '1px solid var(--color-border)',
          }}
        >
          <span id="cd-t" className="t-heading-sm" style={{ fontWeight: 600 }}>
            <span>{TITLE_CONFLICT}</span>
          </span>
          <button type="button" className="fg-icon-btn" aria-label="Close" onClick={onClose}>
            ✕
          </button>
        </div>

        <div
          className="fg-drawer__body"
          style={{
            overflowY: 'auto',
            flex: 1,
            padding: 'var(--space-4)',
            display: 'flex',
            flexDirection: 'column',
            gap: 'var(--space-4)',
          }}
        >
          <div className="fg-banner fg-banner--warning" role="alert">
            <span className="fg-banner__dot" />
            <span className="fg-banner__body">
              <span>{BANNER_CONFLICT}</span>
            </span>
          </div>

          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(240px, 1fr))', gap: 'var(--space-3)' }}>
            <div style={{ display: 'flex', flexDirection: 'column', gap: '6px', minWidth: 0 }}>
              <span className="t-label" style={{ color: 'var(--color-text-muted)' }}>
                <span>{LABEL_YOUR_DRAFT}</span>
              </span>
              <pre
                style={{
                  margin: 0,
                  padding: 'var(--space-3)',
                  background: 'var(--color-surface-sunken)',
                  border: '1px solid var(--color-border)',
                  borderRadius: 'var(--radius-sm)',
                  fontFamily: 'var(--font-mono)',
                  fontSize: '12px',
                  whiteSpace: 'pre-wrap',
                  maxHeight: '260px',
                  overflow: 'auto',
                }}
              >
                {draftContent}
              </pre>
            </div>

            <div style={{ display: 'flex', flexDirection: 'column', gap: '6px', minWidth: 0 }}>
              <span className="t-label" style={{ color: 'var(--color-text-muted)' }}>
                <span>{LABEL_LATEST_CANONICAL}</span>
              </span>
              <pre
                style={{
                  margin: 0,
                  padding: 'var(--space-3)',
                  background: 'var(--color-surface-sunken)',
                  border: '1px solid var(--color-border)',
                  borderRadius: 'var(--radius-sm)',
                  fontFamily: 'var(--font-mono)',
                  fontSize: '12px',
                  whiteSpace: 'pre-wrap',
                  maxHeight: '260px',
                  overflow: 'auto',
                }}
              >
                {latestContent}
              </pre>
            </div>
          </div>

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
                <span style={{ color: 'var(--color-text-muted)' }}>{LABEL_EXPECTED}</span>
                <span>{expectedDigest}</span>
              </div>
              <div>
                <span style={{ color: 'var(--color-text-muted)' }}>{LABEL_LATEST}</span>
                <span>{latestDigest}</span>
              </div>
            </div>
          </details>
        </div>

        <div
          className="fg-drawer__foot"
          style={{
            display: 'flex',
            flexWrap: 'wrap',
            justifyContent: 'flex-end',
            gap: 'var(--space-2)',
            padding: 'var(--space-3) var(--space-4)',
            borderTop: '1px solid var(--color-border)',
            background: 'var(--color-surface-sunken)',
          }}
        >
          <button type="button" className="fg-btn fg-btn--ghost" onClick={handleDownloadDraft}>
            <span>{LABEL_DOWNLOAD_DRAFT}</span>
          </button>
          <button type="button" className="fg-btn fg-btn--ghost" onClick={handleCopyDraft}>
            <span>{copiedDraft ? LABEL_COPIED : LABEL_COPY_DRAFT}</span>
          </button>
          <button
            type="button"
            className="fg-btn fg-btn--danger"
            onClick={() => setConfirmDiscardOpen(true)}
          >
            <span>{LABEL_DISCARD_RELOAD}</span>
          </button>
          <button type="button" className="fg-btn fg-btn--primary" onClick={onUseLatest}>
            <span>{LABEL_USE_LATEST}</span>
          </button>
        </div>
      </aside>

      <ConfirmDialog
        open={confirmDiscardOpen}
        title="Discard draft?"
        body="Your uncommitted draft changes will be permanently discarded and replaced with the latest canonical content."
        confirmLabel="Discard draft"
        danger
        onConfirm={() => {
          setConfirmDiscardOpen(false);
          onDiscard();
        }}
        onCancel={() => setConfirmDiscardOpen(false)}
      />
    </>
  );
}
