import { useState } from 'react';
import { Link, useParams } from 'react-router';
import { useQueryClient } from '@tanstack/react-query';
import { ApiError } from '../../api/client';
import { commitSentence } from '../../api/commit-view';
import {
  confirmSkillMutation,
  discardSubmission,
  previewSkillApproval,
  previewSubmissionApproval,
  useSkillReviewContent,
  useSubmissionReview,
} from '../../api/queries';
import type {
  ReviewFile,
  SkillMutationResult,
  SkillProposal,
  SkillReviewContent,
  SubmissionApprovalPreview,
  SubmissionReview,
} from '../../api/types';
import { DiffView } from '../../components/DiffView';
import { NotFoundPage } from '../../components/NotFoundPage';
import { Skeleton } from '../../components/Skeleton';
import { Toast } from '../../components/Toast';

const CHANGE_WORD: Record<NonNullable<ReviewFile['change']>, string> = { A: 'added', M: 'modified', D: 'deleted' };

const BASELINE_TEXT: Record<SkillReviewContent['baseline'], string> = {
  approved: 'Compared with the content you last approved',
  served: 'Compared with what agents get today',
  hub: 'Compared with the skill on this hub today',
  none: 'A new draft: every file is new',
};

const TEXT = {
  openedMark: '(opened)',
  selectAll: 'Select all and open',
  collapseAll: 'Collapse all',
  truncated: 'Only the first 1 MiB is shown.',
  back: '← Waiting for review',
  headingTail: ' · review content',
  license: 'License:',
  originSep: ' · Origin: ',
  thisHub: 'this hub',
  noChange: 'No file changed against what is compared here.',
  approved: 'Approved.',
  backToQueue: 'Back to Waiting for review',
  openSkill: 'Open the skill',
  approve: 'Approve',
  approving: 'Approving…',
  discard: 'Discard',
  confirmDiscard: 'Discard this submission? Nothing is applied and it leaves the queue.',
  yesDiscard: 'Yes, discard',
  keep: 'Keep it',
  discarded: 'Discarded.',
  newDraftNote: 'It is a draft now: open the skill to activate it when it is ready.',
  leftOut: 'Left out by the submitting machine: ',
  conflictsHead: 'The hub and the submission both changed these files since it was checked out:',
  conflictsFix: 'Decide each in a terminal on the hub machine: skillhub skill review ',
  acceptFlag: ' --accept <path>=hub',
  alreadyApproved: 'Agents already get this content: it is approved.',
  nothingToApprove:
    'This skill is not from another repository, so there is nothing to approve. Activate it from its page when it is ready.',
};

function binaryNote(file: ReviewFile): string {
  return `A binary file (${file.size_bytes} bytes): its content is not shown. Check it outside this page.`;
}

// The reason and, when the hub gave one, how to fix it.
function errorText(err: unknown): string {
  if (err instanceof ApiError) {
    return [err.render.ERROR, err.render.WHY, err.render.FIX].filter(Boolean).join(' ') || err.message;
  }
  return err instanceof Error ? err.message : String(err);
}

function counts(file: ReviewFile): React.ReactNode {
  if (file.binary) return 'binary';
  const added = file.added ? `+${file.added}` : '';
  const removed = file.removed ? `−${file.removed}` : '';
  return (
    <>
      {added && <span className="app-review-file__added">{added}</span>}
      {removed && <span className="app-review-file__removed">{removed}</span>}
    </>
  );
}

// Every file of a skill in order, changed files first-class with their diffs, and the license and
// origin on top. Approving is possible once every changed file was opened. The same screen reviews a
// skill another machine submitted: approving then applies the files and approves them in one commit.
export function FullReviewScreen() {
  const { id, sid } = useParams<{ id: string; sid: string }>();
  const submissionId = sid ?? '';
  const queryClient = useQueryClient();
  const skillQuery = useSkillReviewContent(submissionId ? '' : (id ?? ''));
  const submissionQuery = useSubmissionReview(submissionId);
  const { data, isLoading, error } = submissionId ? submissionQuery : skillQuery;
  const submission: SubmissionReview | undefined = submissionId ? submissionQuery.data : undefined;
  const skillId = data?.skill_id ?? id ?? '';
  const [opened, setOpened] = useState<Set<string>>(new Set());
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const [approving, setApproving] = useState(false);
  const [confirmingDiscard, setConfirmingDiscard] = useState(false);
  const [discarded, setDiscarded] = useState(false);
  const [failure, setFailure] = useState<string | null>(null);
  const [result, setResult] = useState<SkillMutationResult | null>(null);
  const [toast, setToast] = useState<string | null>(null);

  if (error instanceof ApiError && error.status === 404) return <NotFoundPage />;
  if (isLoading || !data) {
    return (
      <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' }}>
        <Skeleton width="50%" height="32px" />
        <Skeleton height="240px" />
      </div>
    );
  }

  const changed = data.files.filter((file) => file.changed);
  const unchanged = data.files.filter((file) => !file.changed);
  const left = changed.filter((file) => !opened.has(file.path)).length;
  const conflicts = submission?.conflicts ?? [];
  const conflictText = `${TEXT.conflictsHead} ${conflicts.join(', ')}. ${TEXT.conflictsFix}${submissionId}${TEXT.acceptFlag}`;
  const needsApproval = data.third_party && !data.approved;
  const heading = `${data.title || skillId}${TEXT.headingTail}`;
  const facts = `${TEXT.license} ${data.license}${TEXT.originSep}${data.origin || TEXT.thisHub}`;
  const baselineLine = `${BASELINE_TEXT[data.baseline]}${data.baseline_commit ? ` (commit ${data.baseline_commit.slice(0, 7)})` : ''}.`;
  const unchangedLabel = `… ${unchanged.length} unchanged ${unchanged.length === 1 ? 'file' : 'files'} (expand)`;
  const leftHint = `(open every changed file first: ${left} left)`;
  const approvedLine = result ? `${TEXT.approved} ${commitSentence(result.commit)}` : '';
  const noteText = data.third_party ? TEXT.alreadyApproved : TEXT.nothingToApprove;

  const markOpened = (path: string) => setOpened((prev) => (prev.has(path) ? prev : new Set(prev).add(path)));
  const toggleExpanded = (path: string, open: boolean) => {
    setExpanded((prev) => {
      if (prev.has(path) === open) return prev;
      const next = new Set(prev);
      if (open) next.add(path);
      else next.delete(path);
      return next;
    });
    if (open) markOpened(path);
  };
  const openAllChanged = () => {
    const paths = changed.map((file) => file.path);
    setExpanded((prev) => new Set([...prev, ...paths]));
    setOpened((prev) => new Set([...prev, ...paths]));
  };
  const collapseAll = () => setExpanded(new Set());
  const openedCount = changed.length - left;
  const allOpen = left === 0;
  const progressText = `${openedCount} of ${changed.length} changed ${changed.length === 1 ? 'file' : 'files'} opened`;

  const discard = async () => {
    setFailure(null);
    try {
      await discardSubmission(submissionId);
      setDiscarded(true);
      queryClient.invalidateQueries({ queryKey: ['review-queue'] });
      queryClient.invalidateQueries({ queryKey: ['home'] });
    } catch (err) {
      setFailure(errorText(err));
    } finally {
      setConfirmingDiscard(false);
    }
  };

  const approve = async () => {
    setApproving(true);
    setFailure(null);
    try {
      let message: string | undefined;
      let proposal: SkillProposal | SubmissionApprovalPreview;
      if (submissionId) {
        const preview = await previewSubmissionApproval(submissionId, data.content_digest);
        message = preview.default_message;
        proposal = preview;
      } else {
        proposal = await previewSkillApproval(skillId, data.content_digest);
      }
      const pins = proposal.confirmation?.confirmation?.pins ?? proposal.confirmation?.pins;
      if (!pins) throw new Error('The approval could not be prepared.');
      const confirmPins = { proposalDigest: pins.proposal_digest, baseVersion: pins.base_version };
      const done = submissionId
        ? await confirmSkillMutation(pins.proposal_id, confirmPins, message, submissionId)
        : await confirmSkillMutation(pins.proposal_id, confirmPins);
      setResult(done);
      setToast(`${data.title || skillId} approved`);
      for (const key of [['skill', skillId], ['skill-review', skillId], ['skills'], ['home'], ['review-queue']]) {
        queryClient.invalidateQueries({ queryKey: key });
      }
    } catch (err) {
      setFailure(errorText(err));
    } finally {
      setApproving(false);
    }
  };

  const renderFile = (file: ReviewFile) => {
    const seen = opened.has(file.path);
    return (
      <li key={file.path} className={`app-review-file${file.changed && !seen ? ' app-review-file--pending' : ''}`}>
        <details open={expanded.has(file.path)} onToggle={(event) => toggleExpanded(file.path, event.currentTarget.open)}>
          <summary className="app-review-file__summary">
            {file.changed ? (
              <span
                className={`app-review-file__state${seen ? ' app-review-file__state--seen' : ''}`}
                aria-label={seen ? 'opened' : 'not opened yet'}
              >
                {seen ? '✓' : '○'}
              </span>
            ) : (
              <span className="app-review-file__state" aria-hidden="true" />
            )}
            <span className="app-review-file__path app-wrap">{file.path}</span>
            {file.change && (
              <span className={`app-filetree__mark app-filetree__mark--${file.change}`} aria-label={CHANGE_WORD[file.change]}>
                {file.change}
              </span>
            )}
            <span className="app-review-file__counts t-caption">{counts(file)}</span>
            {seen && file.changed && <span className="t-caption app-review-file__opened">{TEXT.openedMark.trim()}</span>}
          </summary>
          <div className="app-review-file__body">
            {file.binary ? (
              <span className="t-body-sm">{binaryNote(file)}</span>
            ) : file.diff ? (
              <DiffView diff={file.diff} />
            ) : file.content ? (
              <pre className="app-files__source">{file.content}</pre>
            ) : (
              <span className="t-body-sm">{file.change === 'D' ? 'This file is gone.' : 'This file is empty.'}</span>
            )}
            {file.truncated && <span className="t-caption">{TEXT.truncated}</span>}
          </div>
        </details>
      </li>
    );
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
      <div>
        <Link to="/review" className="t-body-sm">
          {TEXT.back}
        </Link>
      </div>
      <header style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-1)' }}>
        <h1 className="t-title" style={{ margin: 0, fontSize: '24px', fontWeight: 600 }}>
          {heading}
        </h1>
        <span className="t-body-sm" style={{ color: 'var(--color-text-muted)' }}>
          {facts}
        </span>
        <span className="t-caption" style={{ color: 'var(--color-text-subtle)' }}>
          {baselineLine}
        </span>
      </header>

      {conflicts.length > 0 && (
        <div className="fg-banner fg-banner--danger" role="alert">
          <span>{conflictText}</span>
        </div>
      )}
      {failure && (
        <div className="fg-banner fg-banner--danger" role="alert">
          <span>{failure}</span>
        </div>
      )}

      {submission && submission.excluded.length > 0 && (
        <span className="t-caption" style={{ color: 'var(--color-text-muted)' }}>
          {TEXT.leftOut}
          {submission.excluded.map((file) => `${file.path} (${file.reason})`).join('; ')}
        </span>
      )}
      {changed.length > 0 && (
        <div className="app-review-toolbar" role="group" aria-label="Changed files progress">
          <div className="app-review-toolbar__progress">
            <span className="t-body-sm" data-testid="open-progress">
              {progressText}
            </span>
            <progress max={changed.length} value={openedCount} aria-label="Changed files opened" />
          </div>
          <div className="app-review-toolbar__actions">
            <button type="button" className="fg-btn fg-btn--secondary fg-btn--small" onClick={openAllChanged} disabled={allOpen && changed.every((f) => expanded.has(f.path))}>
              <span>{TEXT.selectAll}</span>
            </button>
            <button type="button" className="fg-btn fg-btn--ghost fg-btn--small" onClick={collapseAll} disabled={expanded.size === 0}>
              <span>{TEXT.collapseAll}</span>
            </button>
          </div>
        </div>
      )}
      <ul className="app-review-files" aria-label="Changed files">
        {changed.map(renderFile)}
      </ul>
      {changed.length === 0 && <span className="t-body-sm">{TEXT.noChange}</span>}
      {unchanged.length > 0 && (
        <details>
          <summary>{unchangedLabel}</summary>
          <ul className="app-review-files" aria-label="Unchanged files">
            {unchanged.map(renderFile)}
          </ul>
        </details>
      )}

      {discarded ? (
        <div className="fg-card" data-testid="discard-result">
          <p className="t-body" style={{ margin: 0 }}>
            {TEXT.discarded}
          </p>
          <Link to="/review" className="fg-btn fg-btn--secondary" style={{ marginTop: 'var(--space-3)' }}>
            <span>{TEXT.backToQueue}</span>
          </Link>
        </div>
      ) : result ? (
        <div className="fg-card" data-testid="approve-result">
          <p className="t-body" style={{ margin: 0 }}>
            {approvedLine}
          </p>
          {submission?.case === 'new' && (
            <p className="t-body-sm" style={{ margin: 0 }}>
              {TEXT.newDraftNote}{' '}
              <Link to={`/skills/${encodeURIComponent(skillId)}`}>{TEXT.openSkill}</Link>
            </p>
          )}
          <Link to="/review" className="fg-btn fg-btn--secondary" style={{ marginTop: 'var(--space-3)' }}>
            <span>{TEXT.backToQueue}</span>
          </Link>
        </div>
      ) : needsApproval ? (
        <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-3)', flexWrap: 'wrap' }}>
          <button
            type="button"
            className="fg-btn fg-btn--primary"
            disabled={left > 0 || approving || conflicts.length > 0}
            onClick={() => void approve()}
          >
            <span>{approving ? TEXT.approving : TEXT.approve}</span>
          </button>
          {submissionId &&
            (confirmingDiscard ? (
              <span style={{ display: 'inline-flex', alignItems: 'center', gap: 'var(--space-2)', flexWrap: 'wrap' }}>
                <span className="t-body-sm">{TEXT.confirmDiscard}</span>
                <button type="button" className="fg-btn fg-btn--secondary" onClick={() => void discard()}>
                  <span>{TEXT.yesDiscard}</span>
                </button>
                <button type="button" className="fg-btn fg-btn--secondary" onClick={() => setConfirmingDiscard(false)}>
                  <span>{TEXT.keep}</span>
                </button>
              </span>
            ) : (
              <button type="button" className="fg-btn fg-btn--secondary" onClick={() => setConfirmingDiscard(true)}>
                <span>{TEXT.discard}</span>
              </button>
            ))}
          {left > 0 && (
            <span className="t-caption" data-testid="approve-hint">
              {leftHint}
            </span>
          )}
        </div>
      ) : (
        <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)', alignItems: 'flex-start' }}>
          <div className="fg-caveat fg-caveat--info" role="note">
            <span>{noteText}</span>
          </div>
          <Link className="fg-btn fg-btn--secondary" to={`/skills/${encodeURIComponent(skillId)}`}>
            <span>{TEXT.openSkill}</span>
          </Link>
        </div>
      )}
      <Toast message={toast} onClose={() => setToast(null)} />
    </div>
  );
}
