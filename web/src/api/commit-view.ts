import type { CommitOutcome } from './types';

// Who gets a committed version and when, in a person's words. For a change to
// skills the server says whether the agents get them now, for example not while
// a skill is a draft.
export function reachSentence(commit: CommitOutcome): string {
  const push = commit.reach?.push_needed ? (commit.reach.push_command ?? 'git push') : '';
  const agents = commit.reach?.agents;
  if (agents) {
    return push ? `${agents} Other clones get it after you push (${push}).` : agents;
  }
  return push
    ? `This machine's agents get it now; others after you push (${push}).`
    : "This machine's agents get it now.";
}

// What a short message after a change says about the commit: who gets it, or why there is no commit.
export function commitNote(commit: CommitOutcome | undefined): string {
  return commit?.status === 'committed' ? reachSentence(commit) : commitSentence(commit);
}

// The sentence that says what a commit did and who gets the new version, in a person's words.
export function commitSentence(commit: CommitOutcome | undefined): string {
  if (!commit) return 'The change is saved but was not committed; commit the changed files when you are ready.';
  if (commit.status === 'committed') {
    return `Committed ${(commit.hash ?? '').slice(0, 7)}. ${reachSentence(commit)}`;
  }
  const finish = commit.manual_command ? ` Finish it with: ${commit.manual_command}` : '';
  if (commit.status === 'failed') return `Not committed: ${commit.reason ?? 'the commit failed'}.${finish}`;
  if (commit.status === 'deferred') return 'The commit comes with the rest of this change.';
  return `Not committed: ${commit.reason ?? 'no commit was asked for'}.${finish}`;
}
