import { useState } from 'react';
import { useT } from '../i18n';

interface CopyButtonProps {
  text: string;
  label?: string;
  className?: string;
}

export function CopyButton({ text, label, className = 'fg-btn fg-btn--secondary' }: CopyButtonProps) {
  const t = useT();
  const [copied, setCopied] = useState(false);

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      // Ignore clipboard write failure
    }
  };

  const displayText = copied ? t('action.copied') : (label ?? t('action.copy_command'));

  return (
    <button
      type="button"
      className={className}
      onClick={handleCopy}
      aria-label={displayText}
    >
      <span>{displayText}</span>
    </button>
  );
}
