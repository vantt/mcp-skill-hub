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

export function Markdown({ content, className = 'fg-markdown' }: MarkdownProps) {
  return (
    <div className={className}>
      <MarkdownLib
        skipHtml
        urlTransform={safeUrlTransform}
        components={components}
      >
        {content}
      </MarkdownLib>
    </div>
  );
}
