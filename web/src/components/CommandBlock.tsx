import { CopyButton } from './CopyButton';

interface CommandBlockProps {
  command: string;
}

export function CommandBlock({ command }: CommandBlockProps) {
  return (
    <div
      style={{
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        gap: 'var(--space-3)',
        padding: 'var(--space-2) var(--space-3)',
        background: 'var(--color-surface-sunken)',
        border: '1px solid var(--color-border)',
        borderRadius: 'var(--card-radius)',
        fontFamily: 'var(--font-mono)',
        fontSize: '13px',
      }}
    >
      <code style={{ overflowX: 'auto', whiteSpace: 'nowrap' }}>{command}</code>
      <CopyButton text={command} />
    </div>
  );
}
