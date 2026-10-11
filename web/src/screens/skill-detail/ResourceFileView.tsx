import { useCallback, useEffect, useRef, useState, type CSSProperties } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { confirmSkillMutation, previewSkillResourceUpdate, useSkillResourceContent } from '../../api/queries';
import type { SkillDetail, SkillProposal, SkillResourceContent } from '../../api/types';
import { ApiError, apiFetch } from '../../api/client';
import { proposalPatch, proposalPaths, proposalWarning } from '../../api/proposal-view';
import { ActionBar } from '../../components/ActionBar';
import { ConflictDrawer } from '../../components/ConflictDrawer';
import { Markdown } from '../../components/Markdown';
import { ProposalPreview } from '../../components/ProposalPreview';
import { Toast } from '../../components/Toast';
import { clearDraft, loadDraft, saveDraft } from '../../state/drafts';

const LABEL_BACK = 'Back to resources';
const LABEL_EDIT = 'Edit';
const LABEL_CANCEL_EDIT = 'Cancel edit';
const LABEL_PREVIEW_CHANGES = 'Preview changes';
const LABEL_VALIDATING = 'Validating…';
const LABEL_READ = 'Read';
const LABEL_SOURCE = 'Source';
const LABEL_PREVIEW = 'Preview';
const LABEL_SAVE = 'Save changes';
const MSG_LOADING = 'Loading file…';
const MSG_READ_ONLY = 'Read only.';
const MSG_READ_ONLY_EDITABLE = 'Read only. Choose Edit to change this file.';
const MSG_NO_CHANGES = 'Nothing changed yet. Edit the file, then preview the change before it is saved.';
const MSG_DRAFT_SAVED = 'Draft saved in this browser';
const MSG_PREVIEW_NOTE = 'Preview shows what will change. Nothing is saved until you confirm.';
const MSG_EMPTY = 'This file is empty.';
const MSG_TRUNCATED = 'Showing the first 1 MiB; this file is too large to edit here.';
const MSG_TOO_LARGE = 'This file is larger than 1 MiB, so it cannot be shown or edited here.';
const MSG_MISSING =
  'This file is listed in the catalog but is missing from the skill folder. Restore the file, or run skillhub rebuild if it was removed on purpose.';
const MSG_NOT_REGULAR =
  'This path is not a regular file (for example a symlink or a folder), so its content is not shown. Replace it with a regular file, then run skillhub rebuild.';
const MSG_NOT_EDITABLE = 'This file cannot be edited here.';
const MSG_NOT_SAVED = 'Nothing was saved.';
const MSG_TRUST_WARNING =
  'This skill came from another repository. After you save, agents get no content from it until you approve it again in your terminal (see the Content trust card).';

const DRAFT_KIND = 'resource';
const WIDE_MIN_PX = 1280;

const MONO_BLOCK: CSSProperties = {
  margin: 0,
  padding: 'var(--space-3) var(--space-4)',
  border: '1px solid var(--color-border)',
  borderRadius: 'var(--input-radius)',
  background: 'var(--color-surface-sunken)',
  fontFamily: 'var(--font-mono)',
  fontSize: '13px',
  lineHeight: 1.55,
  whiteSpace: 'pre-wrap',
  overflowWrap: 'anywhere',
  overflow: 'auto',
  maxHeight: '70vh',
};

const PREVIEW_PANE: CSSProperties = {
  padding: 'var(--space-3) var(--space-4)',
  border: '1px solid var(--color-border)',
  borderRadius: 'var(--input-radius)',
  background: 'var(--color-surface-sunken)',
  overflow: 'auto',
};

const TEXTAREA: CSSProperties = {
  fontFamily: 'var(--font-mono)',
  fontSize: '13px',
  lineHeight: '1.55',
  resize: 'vertical',
};

export interface ResourceFileViewProps {
  skill: SkillDetail;
  workspaceId: string;
  path: string; // workspace-relative path from the list
  startInEdit: boolean; // true when opened with Edit
  thirdParty: boolean;
  onClose: () => void;
}

// The unsaved edit kept in browser storage. It records the digest it was based on, so a draft
// restored after the file changed on disk is previewed against that old digest and the server
// reports the conflict instead of overwriting the newer file.
interface ResourceDraft {
  content: string;
  baseDigest: string;
}

function baseName(path: string): string {
  return path.split('/').pop() || path;
}

function isMarkdownPath(path: string): boolean {
  return path.toLowerCase().endsWith('.md');
}

// A browser text area reports every newline as LF; compare and edit in that form. The server turns
// LF back into CRLF for files that use CRLF throughout.
function toEditorText(content: string | undefined): string {
  return (content ?? '').replace(/\r\n?/g, '\n');
}

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} ${bytes === 1 ? 'byte' : 'bytes'}`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KiB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MiB`;
}

function errorText(err: unknown): string {
  if (err instanceof ApiError) return err.render.WHY || err.render.ERROR || err.message;
  if (err instanceof Error) return err.message;
  return String(err);
}

function ErrorBanner({ message }: { message: string }) {
  return (
    <div className="fg-banner fg-banner--danger" role="alert">
      <span className="fg-banner__dot" />
      <span className="fg-banner__body">
        <span>{message}</span>
      </span>
    </div>
  );
}

function BackButton({ onClose }: { onClose: () => void }) {
  return (
    <button type="button" className="fg-btn fg-btn--secondary" onClick={onClose}>
      <span>{LABEL_BACK}</span>
    </button>
  );
}

// Shows one companion file of a skill, read-only or in an editor that saves through a previewed
// one-file proposal.
export function ResourceFileView(props: ResourceFileViewProps) {
  const { skill, path, onClose } = props;
  const query = useSkillResourceContent(skill.skill_id, path);

  if (query.data) {
    return <ResourceFileBody key={path} {...props} initialFile={query.data} />;
  }

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
      <ActionBar status={<span>{query.isError ? '' : MSG_LOADING}</span>}>
        <BackButton onClose={onClose} />
      </ActionBar>
      <section className="fg-card">
        <span className="fg-card__title app-wrap" style={{ fontFamily: 'var(--font-mono)', fontSize: '13px' }}>
          <span>{path}</span>
        </span>
      </section>
      {query.isError && <ErrorBanner message={errorText(query.error)} />}
    </div>
  );
}

interface ConflictInfo {
  latest: SkillResourceContent;
}

interface ResourceFileBodyProps extends ResourceFileViewProps {
  initialFile: SkillResourceContent;
}

function ResourceFileBody({ skill, workspaceId, path, startInEdit, thirdParty, onClose, initialFile }: ResourceFileBodyProps) {
  const queryClient = useQueryClient();
  const skillId = skill.skill_id;
  const draftId = `${skillId}:${path}`;
  const fileName = baseName(path);
  const markdown = isMarkdownPath(path);
  const kind = skill.resources?.find((r) => r.path === path)?.kind ?? '';

  // The saved version this view is based on; it moves on after a save or a conflict resolution.
  const [file, setFile] = useState<SkillResourceContent>(initialFile);
  const [initialDraft] = useState(() =>
    initialFile.editable ? loadDraft<ResourceDraft>(workspaceId, DRAFT_KIND, draftId, initialFile.digest) : null,
  );
  const [text, setText] = useState(() =>
    initialDraft && typeof initialDraft.value?.content === 'string'
      ? initialDraft.value.content
      : toEditorText(initialFile.content),
  );
  const [expectedDigest, setExpectedDigest] = useState(() =>
    initialDraft && typeof initialDraft.value?.baseDigest === 'string' && initialDraft.value.baseDigest
      ? initialDraft.value.baseDigest
      : initialFile.digest,
  );
  const [mode, setMode] = useState<'read' | 'edit'>(startInEdit && initialFile.editable ? 'edit' : 'read');
  const [readView, setReadView] = useState<'read' | 'source'>('read');
  const [editView, setEditView] = useState<'edit' | 'preview'>('edit');
  const [isWide, setIsWide] = useState(false);
  const [lastSaved, setLastSaved] = useState<number | null>(initialDraft ? initialDraft.savedAt : null);

  const [loading, setLoading] = useState(false);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const [proposal, setProposal] = useState<SkillProposal | null>(null);
  const [conflict, setConflict] = useState<ConflictInfo | null>(null);
  const [toastMessage, setToastMessage] = useState<string | null>(null);

  const dirty = text !== toEditorText(file.content);

  useEffect(() => {
    const handleResize = () => setIsWide(window.innerWidth >= WIDE_MIN_PX);
    handleResize();
    window.addEventListener('resize', handleResize);
    return () => window.removeEventListener('resize', handleResize);
  }, []);

  // Keeps the latest unsaved state so it can be written once more when the view closes.
  const pending = useRef<{ dirty: boolean; draft: ResourceDraft }>({ dirty, draft: { content: text, baseDigest: expectedDigest } });
  useEffect(() => {
    pending.current = { dirty, draft: { content: text, baseDigest: expectedDigest } };
  });

  // Autosave the draft; a text equal to the saved file needs no draft.
  useEffect(() => {
    if (!file.editable) return;
    const timer = setTimeout(() => {
      if (dirty) {
        saveDraft<ResourceDraft>(workspaceId, DRAFT_KIND, draftId, expectedDigest, { content: text, baseDigest: expectedDigest });
        setLastSaved(Date.now());
      } else {
        clearDraft(workspaceId, DRAFT_KIND, draftId);
        setLastSaved(null);
      }
    }, 500);
    return () => clearTimeout(timer);
  }, [text, dirty, expectedDigest, file.editable, workspaceId, draftId]);

  // Leaving the view before the autosave timer fires must not lose the last keystrokes.
  useEffect(
    () => () => {
      const { dirty: unsaved, draft } = pending.current;
      if (unsaved) saveDraft<ResourceDraft>(workspaceId, DRAFT_KIND, draftId, draft.baseDigest, draft);
    },
    [workspaceId, draftId],
  );

  const fetchLatest = useCallback(
    () =>
      queryClient.fetchQuery({
        queryKey: ['skill-resource', skillId, path],
        queryFn: () =>
          apiFetch<SkillResourceContent>(
            `/skills/${encodeURIComponent(skillId)}/resources/content?path=${encodeURIComponent(path)}`,
          ),
        staleTime: 0,
      }),
    [queryClient, skillId, path],
  );

  const requestPreview = async (digest: string, content: string) => {
    const prop = await previewSkillResourceUpdate(skillId, { path, content, expected_digest: digest });
    setProposal(prop);
  };

  const handlePreviewChanges = async () => {
    setLoading(true);
    setErrorMessage(null);
    try {
      await requestPreview(expectedDigest, text);
    } catch (err) {
      if (err instanceof ApiError && err.code === 'edit_conflict') {
        try {
          setConflict({ latest: await fetchLatest() });
        } catch (fetchErr) {
          setErrorMessage(errorText(fetchErr));
        }
      } else {
        setErrorMessage(errorText(err));
      }
    } finally {
      setLoading(false);
    }
  };

  const handleConfirm = async (pins: { proposalId: string; proposalDigest: string; baseVersion: string }) => {
    try {
      const result = await confirmSkillMutation(pins.proposalId, pins);
      setProposal(null);
      if (result.status === 'error') {
        // Nothing was applied; the draft stays so the person can preview again.
        setErrorMessage(result.summary || MSG_NOT_SAVED);
        return;
      }
      clearDraft(workspaceId, DRAFT_KIND, draftId);
      setToastMessage(`Saved ${fileName}.`);
      for (const queryKey of [
        ['skill', skillId],
        ['skill-review', skillId],
        ['skill-resources', skillId],
        ['skill-resource', skillId, path],
        ['skills'],
        ['home'],
      ]) {
        queryClient.invalidateQueries({ queryKey });
      }
      setMode('read');
      try {
        const latest = await fetchLatest();
        setFile(latest);
        setText(toEditorText(latest.content));
        setExpectedDigest(latest.digest);
      } catch {
        // The save is confirmed; without a fresh read the view shows the text as typed, and a later
        // edit is checked against the old digest, which the server reports as a conflict.
        setFile((prev) => ({ ...prev, content: text }));
      }
    } catch (err) {
      setProposal(null);
      setErrorMessage(errorText(err));
    }
  };

  const handleDiscardConflict = () => {
    const latest = conflict?.latest ?? file;
    clearDraft(workspaceId, DRAFT_KIND, draftId);
    setFile(latest);
    setText(toEditorText(latest.content));
    setExpectedDigest(latest.digest);
    setConflict(null);
    setErrorMessage(null);
    if (!latest.editable) setMode('read');
  };

  // Puts the person's text on top of the version saved now and previews that.
  const handleUseLatestAsBase = async () => {
    setLoading(true);
    setErrorMessage(null);
    try {
      const latest = conflict?.latest ?? (await fetchLatest());
      setFile(latest);
      setExpectedDigest(latest.digest);
      saveDraft<ResourceDraft>(workspaceId, DRAFT_KIND, draftId, latest.digest, { content: text, baseDigest: latest.digest });
      setConflict(null);
      await requestPreview(latest.digest, text);
    } catch (err) {
      setErrorMessage(errorText(err));
    } finally {
      setLoading(false);
    }
  };

  const pins = proposal?.confirmation?.confirmation?.pins ?? proposal?.confirmation?.pins ?? null;
  const previewWarning = [thirdParty ? MSG_TRUST_WARNING : '', proposal ? proposalWarning(proposal) ?? '' : '']
    .filter(Boolean)
    .join(' ');

  const statusText =
    mode === 'edit'
      ? !dirty
        ? MSG_NO_CHANGES
        : lastSaved
          ? `${MSG_DRAFT_SAVED}. ${MSG_PREVIEW_NOTE}`
          : MSG_PREVIEW_NOTE
      : file.editable
        ? MSG_READ_ONLY_EDITABLE
        : MSG_READ_ONLY;

  const meta = [formatSize(file.size_bytes), kind].filter(Boolean).join(' · ');

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-4)' }}>
      <ActionBar status={<span>{statusText}</span>}>
        {mode === 'edit' ? (
          <>
            <button
              type="button"
              className="fg-btn fg-btn--primary"
              onClick={handlePreviewChanges}
              disabled={loading || !dirty}
            >
              <span>{loading ? LABEL_VALIDATING : LABEL_PREVIEW_CHANGES}</span>
            </button>
            <button type="button" className="fg-btn fg-btn--secondary" onClick={() => setMode('read')}>
              <span>{LABEL_CANCEL_EDIT}</span>
            </button>
          </>
        ) : (
          file.editable && (
            <button type="button" className="fg-btn fg-btn--primary" onClick={() => setMode('edit')}>
              <span>{LABEL_EDIT}</span>
            </button>
          )
        )}
        <BackButton onClose={onClose} />
      </ActionBar>

      {errorMessage && <ErrorBanner message={errorMessage} />}

      <section className="fg-card" style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)', minWidth: 0 }}>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', flexWrap: 'wrap', gap: 'var(--space-2)' }}>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-1)', minWidth: 0, flex: '1 1 260px' }}>
            <span className="fg-card__title app-wrap" style={{ fontFamily: 'var(--font-mono)', fontSize: '13px' }}>
              <span>{path}</span>
            </span>
            <span className="t-caption" style={{ color: 'var(--color-text-subtle)' }}>
              <span>{meta}</span>
            </span>
          </div>

          {markdown && mode === 'read' && file.content !== undefined && (
            <div className="fg-seg">
              <SegButton active={readView === 'read'} label={LABEL_READ} onClick={() => setReadView('read')} />
              <SegButton active={readView === 'source'} label={LABEL_SOURCE} onClick={() => setReadView('source')} />
            </div>
          )}
          {markdown && mode === 'edit' && !isWide && (
            <div className="fg-seg">
              <SegButton active={editView === 'edit'} label={LABEL_EDIT} onClick={() => setEditView('edit')} />
              <SegButton active={editView === 'preview'} label={LABEL_PREVIEW} onClick={() => setEditView('preview')} />
            </div>
          )}
        </div>

        {mode === 'read' ? (
          <ReadOnlyContent file={file} markdown={markdown} readView={readView} startedInEdit={startInEdit} />
        ) : (
          <EditArea
            fileName={fileName}
            text={text}
            onChange={setText}
            markdown={markdown}
            isWide={isWide}
            editView={editView}
          />
        )}
      </section>

      {proposal && (
        <ProposalPreview
          open
          title={`Save changes to ${fileName}?`}
          target={skillId}
          toState={skill.lifecycle_state}
          paths={proposalPaths(proposal)}
          changes={[`Edited ${path}`]}
          warning={previewWarning || undefined}
          diff={proposalPatch(proposal)}
          proposalId={pins?.proposal_id || ''}
          proposalDigest={pins?.proposal_digest || ''}
          baseVersion={pins?.base_version || ''}
          confirmLabel={LABEL_SAVE}
          onConfirm={handleConfirm}
          onCancel={() => setProposal(null)}
        />
      )}

      <ConflictDrawer
        open={conflict !== null}
        fileLabel={fileName}
        draftContent={text}
        latestContent={conflict?.latest.content ?? ''}
        expectedDigest={expectedDigest}
        latestDigest={conflict?.latest.digest ?? ''}
        mine={[`Edited ${fileName} here`]}
        theirs={[`${fileName} changed on disk`]}
        bothFields={[fileName]}
        onUseLatest={handleUseLatestAsBase}
        onDiscard={handleDiscardConflict}
        onClose={() => setConflict(null)}
      />

      <Toast message={toastMessage} onClose={() => setToastMessage(null)} />
    </div>
  );
}

function SegButton({ active, label, onClick }: { active: boolean; label: string; onClick: () => void }) {
  return (
    <button
      type="button"
      className={`fg-seg__btn ${active ? 'fg-seg__btn--active' : ''}`}
      aria-pressed={active}
      onClick={onClick}
    >
      <span>{label}</span>
    </button>
  );
}

// Explains why a file is not shown or not editable; null when it is an editable text file.
function unavailableNotice(file: SkillResourceContent): { tone: 'info' | 'warn'; text: string } | null {
  if (file.truncated) return { tone: 'info', text: MSG_TRUNCATED };
  switch (file.reason) {
    case 'binary':
      return {
        tone: 'info',
        text: `Binary file: its content is not shown. Size ${formatSize(file.size_bytes)}, digest ${file.digest}.`,
      };
    case 'too_large':
      return { tone: 'info', text: MSG_TOO_LARGE };
    case 'missing':
      return { tone: 'warn', text: MSG_MISSING };
    case 'not_regular':
      return { tone: 'warn', text: MSG_NOT_REGULAR };
    default:
      return file.editable ? null : { tone: 'info', text: MSG_NOT_EDITABLE };
  }
}

function ReadOnlyContent({
  file,
  markdown,
  readView,
  startedInEdit,
}: {
  file: SkillResourceContent;
  markdown: boolean;
  readView: 'read' | 'source';
  startedInEdit: boolean;
}) {
  const notice = unavailableNotice(file);
  // Opened with Edit on a file that cannot be edited: say so before the reason.
  const noticeText =
    notice && startedInEdit && !file.editable && notice.text !== MSG_NOT_EDITABLE
      ? `${MSG_NOT_EDITABLE} ${notice.text}`
      : notice?.text;
  // Raw bytes of a binary file are never shown, whatever the response carries.
  const content = file.reason === 'binary' ? undefined : file.content;
  return (
    <>
      {notice && (
        <div className={`fg-caveat fg-caveat--${notice.tone}`} role="note">
          <span>{noticeText}</span>
        </div>
      )}
      {content !== undefined &&
        (content === '' ? (
          <span className="t-body-sm" style={{ color: 'var(--color-text-muted)' }}>
            <span>{MSG_EMPTY}</span>
          </span>
        ) : markdown && readView === 'read' ? (
          <div style={PREVIEW_PANE}>
            <Markdown content={content} />
          </div>
        ) : (
          <pre style={MONO_BLOCK} data-testid="resource-source">
            <code>{content}</code>
          </pre>
        ))}
    </>
  );
}

function EditArea({
  fileName,
  text,
  onChange,
  markdown,
  isWide,
  editView,
}: {
  fileName: string;
  text: string;
  onChange: (value: string) => void;
  markdown: boolean;
  isWide: boolean;
  editView: 'edit' | 'preview';
}) {
  const textarea = (minHeight: string) => (
    <textarea
      className="fg-input fg-input--area"
      style={{ ...TEXTAREA, minHeight }}
      value={text}
      onChange={(e) => onChange(e.target.value)}
      aria-label={`${fileName} content`}
      spellCheck={false}
    />
  );
  const preview = (minHeight: string) => (
    <div style={{ ...PREVIEW_PANE, minHeight }}>
      <Markdown content={text} />
    </div>
  );

  if (!markdown) return textarea('440px');
  if (isWide) {
    return (
      <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 'var(--space-3)' }}>
        {textarea('440px')}
        {preview('440px')}
      </div>
    );
  }
  return editView === 'edit' ? textarea('380px') : preview('380px');
}
