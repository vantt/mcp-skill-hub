import { fireEvent, render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ProposalPreview } from './ProposalPreview';

describe('ProposalPreview', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  const defaultProps = {
    open: true,
    title: 'Create draft skill',
    target: 'new-skill',
    fromState: 'none',
    toState: 'draft',
    paths: ['skills/core/new-skill/SKILL.md'],
    impact: 'Created as draft.',
    diff: '--- old\n+++ new\n@@ -1,2 +1,3 @@\n context line\n-removed line\n+added line',
    stat: '+1 −1',
    proposalId: 'PROP-test-1234',
    proposalDigest: 'sha256:abcd1234abcd1234',
    baseVersion: 'base-v1',
    onConfirm: vi.fn(),
    onCancel: vi.fn(),
    onNewPreview: vi.fn(),
  };

  it('renders pins and diff prefixes', () => {
    render(<ProposalPreview {...defaultProps} />);

    // Assert pins rendered
    expect(screen.getByTestId('pin-proposal-id')).toHaveTextContent('PROP-test-1234');
    expect(screen.getByTestId('pin-proposal-digest')).toHaveTextContent('sha256:abcd1234abcd1234');
    expect(screen.getByTestId('pin-base-version')).toHaveTextContent('base-v1');

    // Assert diff lines and prefixes
    expect(screen.getByText('+')).toBeInTheDocument();
    expect(screen.getByText('−')).toBeInTheDocument();
    expect(screen.getByText('added line')).toBeInTheDocument();
    expect(screen.getByText('removed line')).toBeInTheDocument();
  });

  it('calls the confirm handler once even on double click', () => {
    const onConfirm = vi.fn();
    render(<ProposalPreview {...defaultProps} onConfirm={onConfirm} />);

    const confirmBtn = screen.getByRole('button', { name: 'Confirm' });
    fireEvent.click(confirmBtn);
    fireEvent.click(confirmBtn);

    expect(onConfirm).toHaveBeenCalledTimes(1);
    expect(onConfirm).toHaveBeenCalledWith({
      proposalId: 'PROP-test-1234',
      proposalDigest: 'sha256:abcd1234abcd1234',
      baseVersion: 'base-v1',
    });
  });

  it('disables Confirm button in stale state', () => {
    const onConfirm = vi.fn();
    render(<ProposalPreview {...defaultProps} isStale={true} onConfirm={onConfirm} />);

    const confirmBtn = screen.getByRole('button', { name: 'Confirm' });
    expect(confirmBtn).toBeDisabled();

    fireEvent.click(confirmBtn);
    expect(onConfirm).not.toHaveBeenCalled();

    expect(screen.getByText('This proposal is no longer current. The base changed after it was created.')).toBeInTheDocument();
  });

  it('network-error retry sends the same pins', () => {
    const onConfirm = vi.fn();
    render(
      <ProposalPreview
        {...defaultProps}
        networkError="Network connection lost."
        onConfirm={onConfirm}
      />,
    );

    expect(screen.getByText('Network connection lost.')).toBeInTheDocument();
    const retryBtn = screen.getByRole('button', { name: 'Retry' });
    fireEvent.click(retryBtn);

    expect(onConfirm).toHaveBeenCalledWith({
      proposalId: 'PROP-test-1234',
      proposalDigest: 'sha256:abcd1234abcd1234',
      baseVersion: 'base-v1',
    });
  });

  it('closes on Escape and returns focus', () => {
    const opener = document.createElement('button');
    document.body.appendChild(opener);
    opener.focus();

    const onCancel = vi.fn();
    render(<ProposalPreview {...defaultProps} onCancel={onCancel} />);

    fireEvent.keyDown(document, { key: 'Escape' });
    expect(onCancel).toHaveBeenCalled();

    opener.remove();
  });
});
