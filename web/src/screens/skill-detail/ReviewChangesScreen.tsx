import { useEffect, useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { ApiError } from '../../api/client';
import { commitSentence } from '../../api/commit-view';
import { confirmSkillMutation, previewSkillChanges } from '../../api/queries';
import type { SkillChangeFile, SkillChangesPreview, SkillFileEntry, SkillMutationResult } from '../../api/types';
import { ConflictDrawer } from '../../components/ConflictDrawer';
import { DiffView } from '../../components/DiffView';
import { Toast } from '../../components/Toast';
import { toInput, type Changeset } from '../../state/changeset';

const TEXT = {
  back: 'Back to files',
  title: 'Review changes',
  checking: 'Checking your changes…',
  noLint: 'Routing lint: no findings.',
  lint: 'Routing lint',
  fixPrefix: ' Fix: ',
  binaryDiff: 'A binary file: its content is not shown.',
  noDiff: 'Nothing to show.',
  messageLabel: 'Commit message',
  commit: 'Commit to the hub',
  committing: 'Committing…',
  conflictLabel: 'Pending changes',
  hubChanged: 'The hub changed after you started these changes.',
  hubNow: "the hub's files now",
  thirdPartyHead: 'This skill came from ',
  thirdPartyTail: '. After this commit, agents get no content from it until a person approves it again under Waiting for review.',
  anotherRepo: 'another repository',
};

const CHANGE_WORD: Record<SkillChangeFile['change'], string> = {
  A: 'added',
  M: 'modified',
  D: 'deleted',
  R: 'renamed',
};

function errorText(err: unknown): string {
  if (err instanceof ApiError) return err.render.WHY || err.render.ERROR || err.message;
  return err instanceof Error ? err.message : String(err);
}

// What the page says when something failed: the reason and, when the hub gave one, how to fix it.
function failureText(err: unknown): string {
  if (err instanceof ApiError) {
    return [err.render.ERROR, err.render.WHY, err.render.FIX].filter(Boolean).join(' ') || err.message;
  }
  return errorText(err);
}

// The hub moved on since the changes were started. On a preview a moved file is the same thing; on
// a commit an edit_conflict is something else (files with edits Git does not know), shown as it is.
function hubMoved(err: unknown, committing: boolean): boolean {
  if (!(err instanceof ApiError)) return false;
  return err.code === 'stale_proposal' || (!committing && err.code === 'edit_conflict');
}

function lineCounts(file: SkillChangeFile): string {
  if (file.binary) return 'binary';
  const parts: string[] = [];
  if (file.added) parts.push(`+${file.added}`);
  if (file.removed) parts.push(`−${file.removed}`);
  return parts.join(' ') || 'no line changes';
}

// What came back for one set of changes: the preview or the reason it failed.
interface Outcome {
  forSet: Changeset;
  preview?: SkillChangesPreview;
  error?: string;
  conflict?: boolean;
}

interface ReviewChangesScreenProps {
  skillId: string;
  set: Changeset;
  hubFiles: SkillFileEntry[];
  // The changes were started against a version the hub no longer has.
  stale: boolean;
  onBack: () => void;
  onCommitted: () => void;
  onKeepOnLatest: () => Promise<void>;
  onDiscard: () => void;
}

// Shows every pending file with its diff and the routing lint, asks for a commit message, and makes
// one hub commit of the whole set.
export function ReviewChangesScreen({ skillId, set, hubFiles, stale, onBack, onCommitted, onKeepOnLatest, onDiscard }: ReviewChangesScreenProps) {
  const queryClient = useQueryClient();
  const [outcome, setOutcome] = useState<Outcome | null>(null);
  const [message, setMessage] = useState('');
  const [commitError, setCommitError] = useState<string | null>(null);
  const [conflictOpen, setConflictOpen] = useState(stale);
  const [committing, setCommitting] = useState(false);
  const [result, setResult] = useState<SkillMutationResult | null>(null);
  const [toast, setToast] = useState<string | null>(null);

  // The preview is made once for this set of changes, and again when the set changes.
  useEffect(() => {
    if (result) return;
    let cancelled = false;
    previewSkillChanges(skillId, toInput(set))
      .then((preview) => {
        if (cancelled) return;
        setOutcome({ forSet: set, preview });
        setMessage(preview.default_message);
      })
      .catch((err: unknown) => {
        if (cancelled) return;
        setOutcome({ forSet: set, error: failureText(err), conflict: hubMoved(err, false) });
      });
    return () => {
      cancelled = true;
    };
  }, [skillId, set, result]);

  const current = outcome?.forSet === set ? outcome : null;
  const preview = current?.preview ?? null;
  const previewError = current && !current.conflict ? current.error : undefined;
  const error = commitError ?? previewError ?? null;
  const conflict = conflictOpen || Boolean(current?.conflict);
  const pins = preview?.confirmation?.confirmation?.pins ?? preview?.confirmation?.pins ?? null;

  const closeConflict = () => {
    setConflictOpen(false);
    setOutcome((prev) => (prev ? { ...prev, conflict: false, error: prev.conflict ? undefined : prev.error } : prev));
  };

  const commit = async () => {
    if (!preview || !pins) return;
    setCommitting(true);
    setCommitError(null);
    try {
      const done = await confirmSkillMutation(
        pins.proposal_id,
        { proposalDigest: pins.proposal_digest, baseVersion: pins.base_version },
        message.trim() === preview.default_message ? undefined : message.trim(),
      );
      setResult(done);
      setToast(`${skillId} committed`);
      for (const key of [['skill', skillId], ['skill-review', skillId], ['skills'], ['home'], ['review-queue']]) {
        queryClient.invalidateQueries({ queryKey: key });
      }
      onCommitted();
    } catch (err) {
      if (hubMoved(err, true)) setConflictOpen(true);
      else setCommitError(failureText(err));
    } finally {
      setCommitting(false);
    }
  };

  const mine = set.changes.map((c) => (c.action === 'rename' ? `rename ${c.from} → ${c.path}` : `${c.action} ${c.path}`));
  const changedPaths = set.changes.map((c) => (c.action === 'rename' ? (c.from ?? c.path) : c.path));
  const draftText = mine.join('\n');
  const latestText = hubFiles.map((f) => `${f.path}  ${f.digest}`).join('\n');
  const committedTitle = `${skillId} committed`;
  const origin = preview?.trust?.origin || TEXT.anotherRepo;
  const thirdParty = `${TEXT.thirdPartyHead}${origin}${TEXT.thirdPartyTail}`;

  if (result) {
    return (
      <div className="fg-card" style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' }}>
        <div className="fg-card__title">
          <span>{committedTitle}</span>
        </div>
        <p className="t-body" style={{ margin: 0 }} data-testid="commit-result">
          {commitSentence(result.commit)}
        </p>
        <div>
          <button type="button" className="fg-btn fg-btn--primary" onClick={onBack}>
            <span>{TEXT.back}</span>
          </button>
        </div>
        <Toast message={toast} onClose={() => setToast(null)} />
      </div>
    );
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
      <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 'var(--space-3)', flexWrap: 'wrap' }}>
        <h2 className="t-title" style={{ margin: 0, fontSize: '18px' }}>
          {TEXT.title}
        </h2>
        <button type="button" className="fg-btn fg-btn--secondary" onClick={onBack}>
          <span>{TEXT.back}</span>
        </button>
      </div>

      {error && (
        <div className="fg-banner fg-banner--danger" role="alert">
          <span>{error}</span>
        </div>
      )}

      {!preview && !error && <span className="t-body-sm">{TEXT.checking}</span>}

      {preview && (
        <>
          <ul className="app-review-files" aria-label="Changed files">
            {preview.files.map((file) => (
              <li key={file.path}>
                <details>
                  <summary>
                    <span className={`app-filetree__mark app-filetree__mark--${file.change}`} aria-label={CHANGE_WORD[file.change]}>
                      {file.change}
                    </span>{' '}
                    <span className="app-wrap" style={{ fontFamily: 'var(--font-mono)', fontSize: '13px' }}>
                      {file.from ? `${file.from} → ${file.path}` : file.path}
                    </span>{' '}
                    <span className="t-caption">{lineCounts(file)}</span>
                  </summary>
                  {file.diff ? (
                    <DiffView diff={file.diff} />
                  ) : (
                    <span className="t-body-sm">{file.binary ? TEXT.binaryDiff : TEXT.noDiff}</span>
                  )}
                </details>
              </li>
            ))}
          </ul>

          <section aria-label="Routing lint">
            {preview.routing_lint.length === 0 ? (
              <span className="t-body-sm">{TEXT.noLint}</span>
            ) : (
              <>
                <span className="t-label">{TEXT.lint}</span>
                <ul className="t-body-sm" style={{ margin: 0, paddingLeft: 'var(--space-4)' }}>
                  {preview.routing_lint.map((finding) => (
                    <li key={finding.code}>
                      {finding.message}
                      {finding.fix ? `${TEXT.fixPrefix}${finding.fix}` : ''}
                    </li>
                  ))}
                </ul>
              </>
            )}
          </section>

          {preview.trust?.third_party && (
            <div className="fg-caveat fg-caveat--warn" role="note">
              <span>{thirdParty}</span>
            </div>
          )}

          <div className="fg-field">
            <label className="fg-field__label t-label" htmlFor="commit-message">
              <span>{TEXT.messageLabel}</span>
            </label>
            <input id="commit-message" className="fg-input" value={message} onChange={(event) => setMessage(event.target.value)} />
          </div>
          <div style={{ display: 'flex', justifyContent: 'flex-end' }}>
            <button type="button" className="fg-btn fg-btn--primary" disabled={committing || !pins || !message.trim()} onClick={() => void commit()}>
              <span>{committing ? TEXT.committing : TEXT.commit}</span>
            </button>
          </div>
        </>
      )}

      <ConflictDrawer
        open={conflict}
        fileLabel={TEXT.conflictLabel}
        draftContent={draftText}
        latestContent={latestText}
        expectedDigest={set.baseVersion}
        latestDigest={TEXT.hubNow}
        mine={mine}
        theirs={[TEXT.hubChanged]}
        bothFields={changedPaths}
        onUseLatest={() => {
          closeConflict();
          void onKeepOnLatest();
        }}
        onDiscard={() => {
          closeConflict();
          onDiscard();
        }}
        onClose={closeConflict}
      />
    </div>
  );
}
