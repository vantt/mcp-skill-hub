import { useEffect } from 'react';

interface ToastProps {
  message: string | null;
  onClose: () => void;
  duration?: number;
}

export function Toast({ message, onClose, duration = 4000 }: ToastProps) {
  useEffect(() => {
    if (!message) return;
    const timer = setTimeout(() => {
      onClose();
    }, duration);
    return () => clearTimeout(timer);
  }, [message, duration, onClose]);

  if (!message) return null;

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(message);
    } catch {
      // Ignore clipboard write error
    }
  };

  return (
    <div
      className="fg-toast-stack"
      role="status"
      aria-live="polite"
      style={{
        position: 'fixed',
        bottom: '24px',
        right: '24px',
        zIndex: 110,
        display: 'flex',
        flexDirection: 'column',
        gap: '8px',
      }}
    >
      <div
        className="fg-toast fg-toast--success"
        style={{
          display: 'flex',
          alignItems: 'center',
          gap: '8px',
          padding: '8px 12px',
          background: 'var(--color-surface-raised)',
          border: '1px solid var(--color-border)',
          borderRadius: 'var(--card-radius)',
          boxShadow: 'var(--shadow-md)',
          fontSize: '13px',
        }}
      >
        <span className="fg-toast__dot" />
        <span>{message}</span>
        <button
          type="button"
          className="fg-btn fg-btn--ghost"
          style={{ padding: '2px 8px', fontSize: '12px' }}
          onClick={handleCopy}
          aria-label="Copy receipt"
        >
          ⧉
        </button>
      </div>
    </div>
  );
}
