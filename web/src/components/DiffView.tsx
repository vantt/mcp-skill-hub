import { useEffect, useState } from 'react';


const LABEL_DIFF = 'Diff';
const LABEL_UNIFIED = 'Unified';
const LABEL_SPLIT = 'Split';
export interface DiffLine {
  type: 'add' | 'del' | 'context' | 'meta';
  text: string;
  lineOld?: number | string;
  lineNew?: number | string;
}

interface DiffViewProps {
  diff: string | DiffLine[];
  stat?: string;
}

export function parseDiff(raw: string): DiffLine[] {
  const lines = raw.split('\n');
  const result: DiffLine[] = [];
  let oldLine = 1;
  let newLine = 1;

  for (const line of lines) {
    if (line.startsWith('@@')) {
      result.push({ type: 'meta', text: line });
    } else if (line.startsWith('+') && !line.startsWith('+++')) {
      result.push({ type: 'add', text: line.slice(1), lineNew: newLine++ });
    } else if (line.startsWith('-') && !line.startsWith('---')) {
      result.push({ type: 'del', text: line.slice(1), lineOld: oldLine++ });
    } else {
      const text = line.startsWith(' ') ? line.slice(1) : line;
      result.push({ type: 'context', text, lineOld: oldLine++, lineNew: newLine++ });
    }
  }
  return result;
}

export function DiffView({ diff, stat }: DiffViewProps) {
  const [split, setSplit] = useState(false);
  const [isWide, setIsWide] = useState(false);

  useEffect(() => {
    const checkWidth = () => {
      setIsWide(window.innerWidth >= 1280);
    };
    checkWidth();
    window.addEventListener('resize', checkWidth);
    return () => window.removeEventListener('resize', checkWidth);
  }, []);

  const parsedLines: DiffLine[] = typeof diff === 'string' ? parseDiff(diff) : diff;

  const leftLines: DiffLine[] = [];
  const rightLines: DiffLine[] = [];

  if (split && isWide) {
    for (const l of parsedLines) {
      if (l.type === 'del') {
        leftLines.push(l);
      } else if (l.type === 'add') {
        rightLines.push(l);
      } else {
        leftLines.push(l);
        rightLines.push(l);
      }
    }
  }

  const prefixChar = (type: DiffLine['type']) => {
    switch (type) {
      case 'add':
        return '+';
      case 'del':
        return '−';
      default:
        return ' ';
    }
  };

  const lineAriaLabel = (type: DiffLine['type']) => {
    switch (type) {
      case 'add':
        return 'Added';
      case 'del':
        return 'Removed';
      default:
        return 'Unchanged';
    }
  };

  const lineBg = (type: DiffLine['type']) => {
    switch (type) {
      case 'add':
        return 'var(--color-success-tint)';
      case 'del':
        return 'var(--color-danger-tint)';
      case 'meta':
        return 'var(--color-surface-raised)';
      default:
        return 'transparent';
    }
  };

  const lineFg = (type: DiffLine['type']) => {
    switch (type) {
      case 'add':
        return 'var(--color-success)';
      case 'del':
        return 'var(--color-danger)';
      default:
        return 'var(--color-text-subtle)';
    }
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-2)' }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <span className="t-label" style={{ color: 'var(--color-text-subtle)' }}>
          <span>{LABEL_DIFF}</span>
          {stat && (
            <span>
              {' '}
              · <span>{stat}</span>
            </span>
          )}
        </span>

        {isWide && (
          <div className="fg-seg" role="radiogroup" aria-label="Diff view mode">
            <button
              type="button"
              className={`fg-seg__btn ${!split ? 'fg-seg__btn--active' : ''}`}
              onClick={() => setSplit(false)}
            >
              <span>{LABEL_UNIFIED}</span>
            </button>
            <button
              type="button"
              className={`fg-seg__btn ${split ? 'fg-seg__btn--active' : ''}`}
              onClick={() => setSplit(true)}
            >
              <span>{LABEL_SPLIT}</span>
            </button>
          </div>
        )}
      </div>

      <div
        style={{
          border: '1px solid var(--color-border)',
          borderRadius: 'var(--radius-sm)',
          overflow: 'auto',
          fontFamily: 'var(--font-mono)',
          fontSize: '12.5px',
          lineHeight: '1.65',
          background: 'var(--color-surface-sunken)',
        }}
      >
        {!split || !isWide ? (
          <div>
            {parsedLines.map((line, idx) => {
              const num = line.lineNew ?? line.lineOld ?? '';
              return (
                <div
                  key={idx}
                  style={{
                    display: 'grid',
                    gridTemplateColumns: '40px 24px 1fr',
                    background: lineBg(line.type),
                  }}
                >
                  <span style={{ textAlign: 'right', paddingRight: '8px', color: 'var(--color-text-subtle)' }}>
                    <span>{String(num)}</span>
                  </span>
                  <span
                    aria-label={lineAriaLabel(line.type)}
                    style={{ textAlign: 'center', color: lineFg(line.type), fontWeight: 'bold' }}
                  >
                    <span>{prefixChar(line.type)}</span>
                  </span>
                  <span style={{ whiteSpace: 'pre-wrap', paddingRight: '12px' }}>
                    <span>{line.text}</span>
                  </span>
                </div>
              );
            })}
          </div>
        ) : (
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr' }}>
            <div style={{ borderRight: '1px solid var(--color-border)' }}>
              {leftLines.map((line, idx) => (
                <div
                  key={idx}
                  style={{
                    display: 'grid',
                    gridTemplateColumns: '24px 1fr',
                    background: lineBg(line.type),
                  }}
                >
                  <span
                    aria-label={lineAriaLabel(line.type)}
                    style={{ textAlign: 'center', color: lineFg(line.type), fontWeight: 'bold' }}
                  >
                    <span>{prefixChar(line.type)}</span>
                  </span>
                  <span style={{ whiteSpace: 'pre-wrap', paddingRight: '8px' }}>
                    <span>{line.text}</span>
                  </span>
                </div>
              ))}
            </div>
            <div>
              {rightLines.map((line, idx) => (
                <div
                  key={idx}
                  style={{
                    display: 'grid',
                    gridTemplateColumns: '24px 1fr',
                    background: lineBg(line.type),
                  }}
                >
                  <span
                    aria-label={lineAriaLabel(line.type)}
                    style={{ textAlign: 'center', color: lineFg(line.type), fontWeight: 'bold' }}
                  >
                    <span>{prefixChar(line.type)}</span>
                  </span>
                  <span style={{ whiteSpace: 'pre-wrap', paddingRight: '8px' }}>
                    <span>{line.text}</span>
                  </span>
                </div>
              ))}
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
