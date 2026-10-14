import { useEffect, useRef, useState } from 'react';
import { CommandBlock } from '../../components/CommandBlock';

const TEXT = {
  open: 'Checkout to a project',
  run: 'Run it in your project, try the skill with your agent, then',
  stop: '.',
  close: 'Close',
};


interface CheckoutButtonProps {
  skillId: string;
}

// Opens a dialog with the command that puts this skill in a project to try with an agent.
export function CheckoutButton({ skillId }: CheckoutButtonProps) {
  const [open, setOpen] = useState(false);
  const dialogRef = useRef<HTMLDivElement>(null);
  const command = `skillhub skill checkout ${skillId}`;
  const commitCommand = `skillhub skill commit ${skillId}`;
  const title = `Checkout ${skillId} to a project`;

  useEffect(() => {
    if (!open) return;
    dialogRef.current?.focus();
    const onKey = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setOpen(false);
    };
    document.addEventListener('keydown', onKey);
    return () => document.removeEventListener('keydown', onKey);
  }, [open]);

  return (
    <>
      <button type="button" className="fg-btn fg-btn--secondary" onClick={() => setOpen(true)}>
        <span>{TEXT.open}</span>
      </button>
      {open && (
        <div className="fg-scrim app-scrim" role="presentation" onClick={() => setOpen(false)}>
          <div
            ref={dialogRef}
            tabIndex={-1}
            role="dialog"
            aria-modal="true"
            aria-labelledby="checkout-title"
            className="fg-modal app-modal"
            onClick={(event) => event.stopPropagation()}
          >
            <h2 id="checkout-title" className="t-title">
              {title}
            </h2>
            <CommandBlock command={command} />
            <p className="t-body-sm" style={{ margin: 0 }}>
              {TEXT.run} <code>{commitCommand}</code>
              {TEXT.stop}
            </p>
            <div style={{ display: 'flex', justifyContent: 'flex-end' }}>
              <button type="button" className="fg-btn fg-btn--secondary" onClick={() => setOpen(false)}>
                <span>{TEXT.close}</span>
              </button>
            </div>
          </div>
        </div>
      )}
    </>
  );
}
