import MarkdownLib, { type Components } from 'react-markdown';

interface MarkdownProps {
  content: string;
  className?: string;
}

function safeUrlTransform(url: string): string {
  try {
    const trimmed = (url || '').trim();
    if (trimmed.startsWith('#') || trimmed.startsWith('/')) {
      return trimmed;
    }
    const parsed = new URL(trimmed);
    if (parsed.protocol === 'http:' || parsed.protocol === 'https:' || parsed.protocol === 'mailto:') {
      return trimmed;
    }
    return '';
  } catch {
    return '';
  }
}

const components: Components = {
  img: ({ alt }) => {
    return <span>{alt ?? ''}</span>;
  },
  a: ({ href, children, ...props }) => {
    const safeHref = href ? safeUrlTransform(href) : '';
    const domProps = Object.fromEntries(
      Object.entries(props).filter(([key]) => key !== 'node'),
    );
    return (
      <a href={safeHref} rel="noreferrer noopener" {...domProps}>
        {children}
      </a>
    );
  },
};

// A leading `---` block is YAML frontmatter. Markdown reads its last `key: value` line plus the closing
// `---` as a heading, so it is split off and shown as a YAML block instead.
const FRONTMATTER = /^---[ \t]*\r?\n([\s\S]*?)\r?\n---[ \t]*(?:\r?\n|$)/;

export function splitFrontmatter(content: string): { frontmatter: string | null; body: string } {
  const match = FRONTMATTER.exec(content);
  if (!match) return { frontmatter: null, body: content };
  return { frontmatter: match[1] ?? '', body: content.slice(match[0].length) };
}

export function Markdown({ content, className = 'fg-markdown' }: MarkdownProps) {
  const { frontmatter, body } = splitFrontmatter(content);
  return (
    <div className={className}>
      {frontmatter !== null && (
        <pre
          aria-label="Frontmatter"
          style={{
            margin: '0 0 var(--space-4)',
            padding: 'var(--space-3)',
            border: '1px solid var(--color-border)',
            borderRadius: 'var(--input-radius)',
            background: 'var(--color-surface)',
            fontFamily: 'var(--font-mono)',
            fontSize: '12.5px',
            lineHeight: 1.55,
            whiteSpace: 'pre-wrap',
            overflowWrap: 'anywhere',
          }}
        >
          <code>{frontmatter}</code>
        </pre>
      )}
      <MarkdownLib
        skipHtml
        urlTransform={safeUrlTransform}
        components={components}
      >
        {body}
      </MarkdownLib>
    </div>
  );
}
