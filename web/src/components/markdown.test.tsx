import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { Markdown } from './Markdown';

describe('Markdown', () => {
  it('safely renders untrusted markdown and strips dangerous elements and URLs', () => {
    const dangerousInput = [
      '<script>alert(1)</script>',
      '<img src=x onerror=alert(1)>',
      '[x](javascript:alert(1))',
      '![t](https://tracker.example/p.gif)',
      '<iframe src=https://e.x>',
      '[ok](https://example.com)',
    ].join('\n\n');

    const { container } = render(<Markdown content={dangerousInput} />);

    // Assert: no script, iframe or img element in the output
    expect(container.querySelector('script')).toBeNull();
    expect(container.querySelector('iframe')).toBeNull();
    expect(container.querySelector('img')).toBeNull();

    // Assert: no href starting with javascript:
    const allLinks = Array.from(container.querySelectorAll('a'));
    for (const link of allLinks) {
      const href = link.getAttribute('href') ?? '';
      expect(href.toLowerCase().startsWith('javascript:')).toBe(false);
    }

    // Assert: the https://example.com link is present
    const okLink = screen.getByRole('link', { name: 'ok' });
    expect(okLink).toHaveAttribute('href', 'https://example.com');
    expect(okLink).toHaveAttribute('rel', 'noreferrer noopener');

    // Assert: image alt text is rendered as span
    expect(screen.getByText('t')).toBeInTheDocument();
  });
});
