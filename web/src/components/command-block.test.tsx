import { fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { CommandBlock } from './CommandBlock';

describe('CommandBlock', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('shows the whole command and copies all of it', () => {
    const command =
      'skillhub skill approve-content some-very-long-skill-identifier --digest sha256:4b8b9721c6bf793c383376a9e6a1fda5c20c8d34552505623990c0b50b7182d0';
    const writeText = vi.fn().mockResolvedValue(undefined);
    vi.stubGlobal('navigator', { clipboard: { writeText } });

    render(<CommandBlock command={command} />);

    const code = screen.getByText(command);
    expect(code.style.whiteSpace).toBe('pre-wrap');
    expect(code.className).toContain('app-wrap');
    fireEvent.click(screen.getByRole('button', { name: 'Copy command' }));
    expect(writeText).toHaveBeenCalledWith(command);
  });
});
