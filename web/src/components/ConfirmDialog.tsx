import { useEffect, useRef, useState } from 'react';


const LABEL_RATIONALE = 'Rationale';
const LABEL_CANCEL = 'Cancel';
interface ConfirmDialogProps {
  open: boolean;
  title: string;
  body: string;
  confirmLabel: string;
  danger?: boolean;
  needsRationale?: boolean;
  onConfirm: (rationale?: string) => void;
  onCancel: () => void;
}

export function ConfirmDialog({
  open,
  title,
  body,
  confirmLabel,
  danger = false,
  needsRationale = false,
  onConfirm,
  onCancel,
}: ConfirmDialogProps) {
  const [rationale, setRationale] = useState('');
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

  const isConfirmDisabled = needsRationale && !rationale.trim();

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
        zIndex: 100,
        padding: 'var(--space-4)',
      }}
      onClick={onCancel}
    >
      <div
        ref={dialogRef}
        className="fg-modal"
        role="alertdialog"
        aria-modal="true"
        aria-labelledby="dc-t"
        tabIndex={-1}
        onClick={(e) => e.stopPropagation()}
        style={{
          width: '480px',
          maxWidth: '100%',
          display: 'flex',
          flexDirection: 'column',
          gap: 'var(--space-4)',
          background: 'var(--color-surface)',
          padding: 'var(--space-4)',
          borderRadius: 'var(--card-radius)',
          boxShadow: 'var(--shadow-lg)',
          outline: 'none',
        }}
      >
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <div id="dc-t" className="fg-modal__title" style={{ fontSize: '18px', fontWeight: 600 }}>
            <span>{title}</span>
          </div>
          <button type="button" className="fg-icon-btn" aria-label="Close" onClick={onCancel}>
            ✕
          </button>
        </div>

        <div style={{ color: 'var(--color-text-muted)' }}>
          <p style={{ margin: 0 }}>
            <span>{body}</span>
          </p>
          {needsRationale && (
            <div style={{ marginTop: 'var(--space-3)', display: 'flex', flexDirection: 'column', gap: 'var(--space-1)' }}>
              <label htmlFor="dc-rationale" className="t-label">
                <span>{LABEL_RATIONALE}</span>
              </label>
              <textarea
                id="dc-rationale"
                className="fg-input fg-input--area"
                style={{ width: '100%', minHeight: '72px', boxSizing: 'border-box' }}
                value={rationale}
                onChange={(e) => setRationale(e.target.value)}
                placeholder="Reason for this decision..."
              />
            </div>
          )}
        </div>

        <div style={{ display: 'flex', justifyContent: 'flex-end', gap: 'var(--space-2)' }}>
          <button
            type="button"
            className="fg-btn fg-btn--ghost"
            onClick={() => {
              setRationale('');
              onCancel();
            }}
          >
            <span>{LABEL_CANCEL}</span>
          </button>
          <button
            type="button"
            className={`fg-btn ${danger ? 'fg-btn--danger' : 'fg-btn--primary'}`}
            disabled={isConfirmDisabled}
            onClick={() => {
              setRationale('');
              onConfirm(rationale);
            }}
          >
            <span>{confirmLabel}</span>
          </button>
        </div>
      </div>
    </div>
  );
}
