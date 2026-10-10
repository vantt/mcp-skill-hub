import { useEffect, useRef, useState } from 'react';
import { ConfirmDialog } from './ConfirmDialog';

const TITLE_CONFLICT = 'SKILL.md changed since you opened it';
const BANNER_CONFLICT =
  'This skill was changed somewhere else after you opened it, for example with the command line. Nothing was saved, and your edits are still here.';
const LABEL_MINE = 'What you changed';
const LABEL_THEIRS = 'What changed meanwhile';
const LABEL_NONE = 'Nothing in the fields you can edit here.';
const LABEL_BOTH_PREFIX = 'Both changes touch: ';
const LABEL_BOTH_SUFFIX =
  '. If you continue, your version of these replaces the other one. The preview shows exactly what would be saved, and nothing is written until you confirm it.';
const LABEL_NEXT = 'What you can do';
const LABEL_NEXT_KEEP = 'Keep my edits on the latest version: your edits are applied on top of what is saved now, then you preview the result.';
const LABEL_NEXT_DISCARD = 'Discard my edits: your changes are dropped and the editor shows what is saved now.';
const LABEL_COMPARE = 'Compare the full SKILL.md';
const LABEL_YOUR_DRAFT = 'Your draft';
const LABEL_LATEST_SAVED = 'Saved now';
const LABEL_TECH_DETAILS = 'Technical details';
const LABEL_EXPECTED = 'you opened: ';
const LABEL_LATEST = 'saved now: ';
const LABEL_DOWNLOAD_DRAFT = 'Download draft (.md)';
const LABEL_COPY_DRAFT = 'Copy draft';
const LABEL_COPIED = 'Copied';
const LABEL_DISCARD_RELOAD = 'Discard my edits';
const LABEL_USE_LATEST = 'Keep my edits on the latest version';

export interface ConflictDrawerProps {
  open: boolean;
  draftContent: string;
  latestContent: string;
  expectedDigest: string;
  latestDigest: string;
  // Plain-words lists: edits made here, edits made elsewhere, and the fields both touched.
  mine?: string[];
  theirs?: string[];
  bothFields?: string[];
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
  mine = [],
  theirs = [],
  bothFields = [],
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
            <ChangeList title={LABEL_MINE} items={mine} />
            <ChangeList title={LABEL_THEIRS} items={theirs} />
          </div>

          {bothFields.length > 0 && (
            <div className="fg-caveat fg-caveat--warn" role="note">
              <span>
                {LABEL_BOTH_PREFIX}
                {bothFields.join(', ')}
                {LABEL_BOTH_SUFFIX}
              </span>
            </div>
          )}

          <div style={{ display: 'flex', flexDirection: 'column', gap: '4px' }}>
            <span className="t-label" style={{ color: 'var(--color-text-muted)' }}>
              <span>{LABEL_NEXT}</span>
            </span>
            <ul className="t-body-sm" style={{ margin: 0, paddingLeft: 'var(--space-4)' }}>
              <li>{LABEL_NEXT_KEEP}</li>
              <li>{LABEL_NEXT_DISCARD}</li>
            </ul>
          </div>

          <details className="fg-acc">
            <summary style={{ cursor: 'pointer', fontWeight: 600, color: 'var(--color-text-muted)' }}>
              <span>{LABEL_COMPARE}</span>
            </summary>
            <div
              style={{
                display: 'grid',
                gridTemplateColumns: 'repeat(auto-fit, minmax(240px, 1fr))',
                gap: 'var(--space-3)',
                paddingTop: 'var(--space-2)',
              }}
            >
              <ContentBox title={LABEL_YOUR_DRAFT} text={draftContent} />
              <ContentBox title={LABEL_LATEST_SAVED} text={latestContent} />
            </div>
          </details>

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
        title="Discard your edits?"
        body="Your edits in this browser will be thrown away and the editor will show what is saved now."
        confirmLabel="Discard my edits"
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

function ChangeList({ title, items }: { title: string; items: string[] }) {
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '6px', minWidth: 0 }}>
      <span className="t-label" style={{ color: 'var(--color-text-muted)' }}>
        <span>{title}</span>
      </span>
      {items.length === 0 ? (
        <span className="t-body-sm" style={{ color: 'var(--color-text-muted)' }}>
          {LABEL_NONE}
        </span>
      ) : (
        <ul className="t-body-sm app-wrap" style={{ margin: 0, paddingLeft: 'var(--space-4)' }}>
          {items.map((it, idx) => (
            <li key={idx}>{it}</li>
          ))}
        </ul>
      )}
    </div>
  );
}

function ContentBox({ title, text }: { title: string; text: string }) {
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: '6px', minWidth: 0 }}>
      <span className="t-label" style={{ color: 'var(--color-text-muted)' }}>
        <span>{title}</span>
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
        {text}
      </pre>
    </div>
  );
}
