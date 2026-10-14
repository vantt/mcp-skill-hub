import { useState } from 'react';
import { Link, useParams } from 'react-router';
import { useQueryClient } from '@tanstack/react-query';
import { ApiError } from '../../api/client';
import { commitSentence } from '../../api/commit-view';
import { confirmSkillMutation, previewSkillApproval, useSkillReviewContent } from '../../api/queries';
import type { ReviewFile, SkillMutationResult, SkillReviewContent } from '../../api/types';
import { DiffView } from '../../components/DiffView';
import { NotFoundPage } from '../../components/NotFoundPage';
import { Skeleton } from '../../components/Skeleton';
import { Toast } from '../../components/Toast';

const CHANGE_WORD: Record<NonNullable<ReviewFile['change']>, string> = { A: 'added', M: 'modified', D: 'deleted' };

const BASELINE_TEXT: Record<SkillReviewContent['baseline'], string> = {
  approved: 'Compared with the content you last approved',
  served: 'Compared with what agents get today',
  none: 'A new draft: every file is new',
};

const TEXT = {
  openedMark: ' (opened)',
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

function counts(file: ReviewFile): string {
  if (file.binary) return 'binary';
  const parts: string[] = [];
  if (file.added) parts.push(`+${file.added}`);
  if (file.removed) parts.push(`−${file.removed}`);
  return parts.join(' ');
}

// Every file of a skill in order, changed files first-class with their diffs, and the license and
// origin on top. Approving is possible once every changed file was opened.
export function FullReviewScreen() {
  const { id } = useParams<{ id: string }>();
  const skillId = id ?? '';
  const queryClient = useQueryClient();
  const { data, isLoading, error } = useSkillReviewContent(skillId);
  const [opened, setOpened] = useState<Set<string>>(new Set());
  const [approving, setApproving] = useState(false);
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
  const needsApproval = data.third_party && !data.approved;
  const heading = `${data.title || skillId}${TEXT.headingTail}`;
  const facts = `${TEXT.license} ${data.license}${TEXT.originSep}${data.origin || TEXT.thisHub}`;
  const baselineLine = `${BASELINE_TEXT[data.baseline]}${data.baseline_commit ? ` (commit ${data.baseline_commit.slice(0, 7)})` : ''}.`;
  const unchangedLabel = `… ${unchanged.length} unchanged ${unchanged.length === 1 ? 'file' : 'files'} (expand)`;
  const leftHint = `(open every changed file first: ${left} left)`;
  const approvedLine = result ? `${TEXT.approved} ${commitSentence(result.commit)}` : '';
  const noteText = data.third_party ? TEXT.alreadyApproved : TEXT.nothingToApprove;

  const markOpened = (path: string) => setOpened((prev) => (prev.has(path) ? prev : new Set(prev).add(path)));

  const approve = async () => {
    setApproving(true);
    setFailure(null);
    try {
      const proposal = await previewSkillApproval(skillId, data.content_digest);
      const pins = proposal.confirmation?.confirmation?.pins ?? proposal.confirmation?.pins;
      if (!pins) throw new Error('The approval could not be prepared.');
      const done = await confirmSkillMutation(pins.proposal_id, { proposalDigest: pins.proposal_digest, baseVersion: pins.base_version });
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

  const renderFile = (file: ReviewFile) => (
    <li key={file.path} className="app-review-file">
      <details onToggle={(event) => event.currentTarget.open && markOpened(file.path)}>
        <summary>
          {file.changed ? (
            <span aria-label={opened.has(file.path) ? 'opened' : 'not opened yet'}>{opened.has(file.path) ? '✓' : '○'}</span>
          ) : (
            <span aria-hidden="true"> </span>
          )}{' '}
          <span className="app-wrap" style={{ fontFamily: 'var(--font-mono)', fontSize: '13px' }}>
            {file.path}
          </span>{' '}
          {file.change && (
            <span className={`app-filetree__mark app-filetree__mark--${file.change}`} aria-label={CHANGE_WORD[file.change]}>
              {file.change}
            </span>
          )}{' '}
          <span className="t-caption">{counts(file)}</span>
          {opened.has(file.path) && file.changed && <span className="t-caption">{TEXT.openedMark}</span>}
        </summary>
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
      </details>
    </li>
  );

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

      {failure && (
        <div className="fg-banner fg-banner--danger" role="alert">
          <span>{failure}</span>
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

      {result ? (
        <div className="fg-card" data-testid="approve-result">
          <p className="t-body" style={{ margin: 0 }}>
            {approvedLine}
          </p>
          <Link to="/review" className="fg-btn fg-btn--secondary" style={{ marginTop: 'var(--space-3)' }}>
            <span>{TEXT.backToQueue}</span>
          </Link>
        </div>
      ) : needsApproval ? (
        <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--space-3)', flexWrap: 'wrap' }}>
          <button type="button" className="fg-btn fg-btn--primary" disabled={left > 0 || approving} onClick={() => void approve()}>
            <span>{approving ? TEXT.approving : TEXT.approve}</span>
          </button>
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
