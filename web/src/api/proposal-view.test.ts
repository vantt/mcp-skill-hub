import { describe, expect, it } from 'vitest';
import { proposalFiles, proposalPaths, proposalPatch, proposalWarning } from './proposal-view';

describe('proposal view', () => {
  const api = {
    diff: { added: null, modified: ['skills/core/x/SKILL.md'], deleted: null },
    full_diff: '--- a/x\n+++ b/x\n-a\n+b\n',
    routing_impact: { summary: 's', warnings: ['Check the triggers.'] },
  };

  it('reads the path lists the API sends', () => {
    expect(proposalFiles(api)).toEqual({ added: [], modified: ['skills/core/x/SKILL.md'], deleted: [] });
    expect(proposalPaths(api)).toEqual(['changed: skills/core/x/SKILL.md']);
  });

  it('takes the patch from full_diff and falls back to a string diff', () => {
    expect(proposalPatch(api)).toContain('+b');
    expect(proposalPatch({ diff: '+x' })).toBe('+x');
    expect(proposalPatch({ diff: { added: [], modified: [], deleted: [] } })).toBe('');
  });

  it('joins routing warnings', () => {
    expect(proposalWarning(api)).toBe('Check the triggers.');
    expect(proposalWarning({ routing_impact: { summary: 's', warnings: null } })).toBeUndefined();
  });
});
