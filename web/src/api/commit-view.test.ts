import { describe, expect, it } from 'vitest';
import { commitNote, commitSentence } from './commit-view';

describe('commitSentence', () => {
  it('says who gets a committed version and when', () => {
    expect(
      commitSentence({ status: 'committed', hash: '4be19c0ffee', reach: { this_hub_now: true, push_needed: true, push_command: 'git -C ~/skill-hub push' } }),
    ).toBe("Committed 4be19c0. This machine's agents get it now; others after you push (git -C ~/skill-hub push).");
    expect(commitSentence({ status: 'committed', hash: '4be19c0ffee', reach: { this_hub_now: true, push_needed: false } })).toBe(
      "Committed 4be19c0. This machine's agents get it now.",
    );
  });

  it('names the command that finishes a commit that failed', () => {
    const text = commitSentence({ status: 'failed', reason: 'signing needs a passphrase', manual_command: 'git -C hub commit', reach: { this_hub_now: true, push_needed: false } });
    expect(text).toContain('Not committed: signing needs a passphrase.');
    expect(text).toContain('git -C hub commit');
  });

  it('reminds the person to commit when no commit was made', () => {
    expect(commitSentence(undefined)).toContain('not committed');
  });

  it('keeps the short note to who gets the version, or to why there is no commit', () => {
    expect(commitNote({ status: 'committed', hash: 'abcdef0123', reach: { this_hub_now: true, push_needed: false } })).toBe("This machine's agents get it now.");
    expect(commitNote({ status: 'skipped', reason: 'git is not set up', reach: { this_hub_now: true, push_needed: false } })).toContain('Not committed: git is not set up');
  });
});
