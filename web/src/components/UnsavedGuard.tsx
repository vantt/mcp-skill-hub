import { useEffect } from 'react';
import { useBlocker } from 'react-router';
import { ConfirmDialog } from './ConfirmDialog';

interface UnsavedGuardProps {
  isDirty: boolean;
  message?: string;
}

export function UnsavedGuard({
  isDirty,
  message = 'You have unsaved changes that will be lost.',
}: UnsavedGuardProps) {
  const blocker = useBlocker(isDirty);

  useEffect(() => {
    if (!isDirty) return;
    const handleBeforeUnload = (e: BeforeUnloadEvent) => {
      e.preventDefault();
      e.returnValue = '';
    };
    window.addEventListener('beforeunload', handleBeforeUnload);
    return () => window.removeEventListener('beforeunload', handleBeforeUnload);
  }, [isDirty]);

  return (
    <ConfirmDialog
      open={blocker.state === 'blocked'}
      title="Discard unsaved changes?"
      body={message}
      confirmLabel="Discard changes"
      danger
      onConfirm={() => blocker.proceed?.()}
      onCancel={() => blocker.reset?.()}
    />
  );
}
