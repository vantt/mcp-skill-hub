import { CopyButton } from './CopyButton';

interface CommandBlockProps {
  command: string;
}

// The whole command is always visible (it wraps instead of scrolling) and the button copies all of it.
export function CommandBlock({ command }: CommandBlockProps) {
  return (
    <div
      style={{
        display: 'flex',
        flexWrap: 'wrap',
        alignItems: 'center',
        justifyContent: 'space-between',
        gap: 'var(--space-2) var(--space-3)',
        padding: 'var(--space-2) var(--space-3)',
        background: 'var(--color-surface-sunken)',
        border: '1px solid var(--color-border)',
        borderRadius: 'var(--card-radius)',
        fontFamily: 'var(--font-mono)',
        fontSize: '13px',
      }}
    >
      <code className="app-wrap" style={{ flex: '1 1 160px', minWidth: 0, whiteSpace: 'pre-wrap' }}>
        {command}
      </code>
      <CopyButton text={command} />
    </div>
  );
}
