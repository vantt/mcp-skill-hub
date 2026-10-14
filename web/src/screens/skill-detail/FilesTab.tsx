import { useCallback, useMemo, useRef, useState } from 'react';
import { useSearchParams } from 'react-router';
import { useQueryClient } from '@tanstack/react-query';
import { ApiError } from '../../api/client';
import { useSkillFileContent, useSkillFiles } from '../../api/queries';
import type { SkillDetail } from '../../api/types';
import { ConfirmDialog } from '../../components/ConfirmDialog';
import { Markdown } from '../../components/Markdown';
import {
  addFile,
  changeCount,
  clearChangeset,
  deleteFile,
  editFile,
  emptyChangeset,
  loadChangeset,
  pendingText,
  renameFile,
  restoreFile,
  saveChangeset,
  treeRows,
  type Changeset,
  type TreeRow,
} from '../../state/changeset';
import { CheckoutButton } from './CheckoutButton';
import { FileTree } from './FileTree';
import { PendingChangesBar } from './PendingChangesBar';
import { ReviewChangesScreen } from './ReviewChangesScreen';

const TEXT = {
  loadingFiles: 'Loading files…',
  staleBanner: 'The hub changed since you started these changes. Review them against the latest files, or discard them.',
  addFile: '+ File',
  addFolder: '+ Folder',
  upload: 'Upload',
  add: 'Add',
  cancel: 'Cancel',
  noFiles: 'This skill has no files yet.',
  restore: 'Restore',
  edit: 'Edit',
  doneEditing: 'Done editing',
  rename: 'Rename',
  delete: 'Delete',
  newPath: 'New path',
  deletedNote: 'This file is deleted when you commit. Restore it to keep it.',
  loadingFile: 'Loading file…',
  tooLarge: 'This file is too large to edit here. It can be replaced by uploading a file with the same name.',
  empty: 'This file is empty.',
};

const PARAM_FILE = 'file';
const PARAM_VIEW = 'view';
const VIEW_REVIEW = 'review';
const MAX_UPLOAD_BYTES = 2 * 1024 * 1024;
const SKILL_MD = 'SKILL.md';

function errorText(err: unknown): string {
  if (err instanceof ApiError) return err.render.WHY || err.render.ERROR || err.message;
  return err instanceof Error ? err.message : String(err);
}

function folderOf(path: string | null): string {
  if (!path || !path.includes('/')) return '';
  return path.slice(0, path.lastIndexOf('/') + 1);
}

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} ${bytes === 1 ? 'byte' : 'bytes'}`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KiB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MiB`;
}

// Reads a file the person picked as base64, which is how binary content travels in a changeset.
async function readAsBase64(file: File): Promise<string> {
  const bytes = new Uint8Array(await file.arrayBuffer());
  let binary = '';
  const chunk = 0x8000;
  for (let i = 0; i < bytes.length; i += chunk) {
    binary += String.fromCharCode(...bytes.subarray(i, i + chunk));
  }
  return window.btoa(binary);
}

interface FilesTabProps {
  skill: SkillDetail;
  workspaceId: string;
}

// The whole skill as a tree of files. Edits, additions, renames, deletions and uploads are pending
// changes kept in this browser; nothing reaches the hub until they are reviewed and committed.
export function FilesTab({ skill, workspaceId }: FilesTabProps) {
  const queryClient = useQueryClient();
  const [searchParams, setSearchParams] = useSearchParams();
  const skillId = skill.skill_id;
  const filesQuery = useSkillFiles(skillId);
  const hubFiles = useMemo(() => filesQuery.data?.files ?? [], [filesQuery.data]);
  const baseVersion = filesQuery.data?.base_version;

  const [set, setSet] = useState<Changeset | null>(null);
  const [staleSet, setStaleSet] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [editing, setEditing] = useState(false);
  const [adding, setAdding] = useState<string | null>(null);
  const [renaming, setRenaming] = useState<string | null>(null);
  const [confirmDiscard, setConfirmDiscard] = useState(false);
  const uploadRef = useRef<HTMLInputElement>(null);

  // Starts from the changes saved in this browser. Changes made against another version of the
  // skill are kept and flagged, so the person decides what to do with them.
  if (baseVersion && !set) {
    const saved = loadChangeset(workspaceId, skillId);
    if (saved && saved.baseVersion !== baseVersion) {
      setSet(saved);
      setStaleSet(true);
    } else {
      setSet(saved ?? emptyChangeset(baseVersion));
    }
  }

  const current = set ?? emptyChangeset(baseVersion ?? '');
  const rows = useMemo(() => treeRows(hubFiles, current), [hubFiles, current]);
  const selectedPath = searchParams.get(PARAM_FILE) ?? SKILL_MD;
  const selectedRow = rows.find((row) => row.path === selectedPath) ?? rows[0] ?? null;
  const count = changeCount(current);
  const reviewing = searchParams.get(PARAM_VIEW) === VIEW_REVIEW;

  // Every change is kept in this browser at once, so a reload or a closed tab loses nothing.
  const update = useCallback(
    (next: Changeset) => {
      setSet(next);
      saveChangeset(workspaceId, skillId, next);
    },
    [skillId, workspaceId],
  );

  const select = (path: string) => {
    setEditing(false);
    setRenaming(null);
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev);
      next.set(PARAM_FILE, path);
      return next;
    }, { replace: true });
  };

  const setView = (view: 'files' | 'review') => {
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev);
      if (view === 'review') next.set(PARAM_VIEW, VIEW_REVIEW);
      else next.delete(PARAM_VIEW);
      return next;
    });
  };

  const discardAll = () => {
    clearChangeset(workspaceId, skillId);
    setSet(emptyChangeset(baseVersion ?? ''));
    setStaleSet(false);
    setEditing(false);
    setConfirmDiscard(false);
    setView('files');
  };

  const refreshFiles = () => {
    queryClient.invalidateQueries({ queryKey: ['skill-files', skillId] });
    queryClient.invalidateQueries({ queryKey: ['skill-file', skillId] });
  };

  // Puts the person's changes on top of the files the hub has now. The versions each change started
  // from are replaced with the current ones, so the review shows exactly what will be overwritten.
  const keepOnLatest = async () => {
    const fresh = await filesQuery.refetch();
    const latest = fresh.data;
    if (!latest || !set) return;
    const rebased: Changeset = {
      baseVersion: latest.base_version,
      changes: set.changes.map((change) => {
        const origin = change.action === 'rename' ? change.from : change.path;
        const hub = latest.files.find((f) => f.path === origin);
        return change.action === 'add' || !hub ? change : { ...change, expected_digest: hub.digest };
      }),
    };
    update(rebased);
    setStaleSet(false);
    refreshFiles();
  };

  if (filesQuery.isError) {
    return (
      <div className="fg-banner fg-banner--danger" role="alert">
        <span>{errorText(filesQuery.error)}</span>
      </div>
    );
  }
  if (!set || !baseVersion) {
    return <span className="t-body-sm">{TEXT.loadingFiles}</span>;
  }

  if (reviewing) {
    return (
      <ReviewChangesScreen
        skillId={skillId}
        set={set}
        hubFiles={hubFiles}
        stale={staleSet}
        onBack={() => setView('files')}
        onCommitted={() => {
          clearChangeset(workspaceId, skillId);
          setSet(emptyChangeset(baseVersion));
          refreshFiles();
        }}
        onKeepOnLatest={keepOnLatest}
        onDiscard={discardAll}
      />
    );
  }

  const addPath = (raw: string) => {
    const path = raw.trim().replace(/^\/+/, '');
    if (!path) return;
    if (rows.some((row) => row.path === path && row.status !== 'D')) {
      setError(`${path} is already in this skill.`);
      return;
    }
    setError(null);
    update(addFile(current, hubFiles, path, ''));
    setAdding(null);
    select(path);
    setEditing(true);
  };

  const upload = async (files: FileList | null) => {
    if (!files) return;
    setError(null);
    let next = current;
    for (const file of Array.from(files)) {
      if (file.size > MAX_UPLOAD_BYTES) {
        setError(`${file.name} is ${formatSize(file.size)}; an upload is limited to ${formatSize(MAX_UPLOAD_BYTES)}.`);
        continue;
      }
      const path = `${folderOf(selectedRow?.path ?? null)}${file.name}`;
      const content = await readAsBase64(file);
      const existing = hubFiles.find((f) => f.path === path);
      const live = treeRows(hubFiles, next).find((row) => row.path === path);
      next =
        existing && live && live.status !== 'D'
          ? editFile(next, existing, '', content, 'base64')
          : addFile(next, hubFiles, path, content, 'base64');
    }
    update(next);
    if (uploadRef.current) uploadRef.current.value = '';
  };

  // New text for the open file. A file the set added or renamed keeps its change and takes the text;
  // a file the hub has becomes a modify, or no change when the text is what the hub has.
  const changeText = (row: TreeRow, hubText: string, text: string) => {
    if (row.status === 'A' || row.status === 'R') {
      update({ ...current, changes: current.changes.map((c) => (c.path === row.path && c.action !== 'delete' ? { ...c, content: text, encoding: 'utf8' } : c)) });
    } else if (row.entry) {
      update(editFile(current, row.entry, hubText, text));
    }
  };

  const rename = (row: TreeRow, to: string) => {
    const path = to.trim().replace(/^\/+/, '');
    if (!path || path === row.path) {
      setRenaming(null);
      return;
    }
    if (rows.some((other) => other.path === path && other.status !== 'D')) {
      setError(`${path} is already in this skill.`);
      return;
    }
    setError(null);
    update(renameFile(current, hubFiles, row.path, path));
    setRenaming(null);
    select(path);
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)' }}>
      {staleSet && (
        <div className="fg-banner fg-banner--warning" role="alert">
          <span className="fg-banner__body">
            <span>{TEXT.staleBanner}</span>
          </span>
        </div>
      )}
      {error && (
        <div className="fg-banner fg-banner--danger" role="alert">
          <span>{error}</span>
        </div>
      )}

      <div className="app-files">
        <aside className="fg-card app-files__tree" aria-label="Files">
          <FileTree rows={rows} selected={selectedRow?.path ?? null} onSelect={select} />
          <div className="app-files__tools">
            <button type="button" className="fg-btn fg-btn--secondary fg-btn--small" onClick={() => setAdding(folderOf(selectedRow?.path ?? null))}>
              <span>{TEXT.addFile}</span>
            </button>
            <button type="button" className="fg-btn fg-btn--secondary fg-btn--small" onClick={() => setAdding('new-folder/')}>
              <span>{TEXT.addFolder}</span>
            </button>
            <button type="button" className="fg-btn fg-btn--secondary fg-btn--small" onClick={() => uploadRef.current?.click()}>
              <span>{TEXT.upload}</span>
            </button>
            <input
              ref={uploadRef}
              type="file"
              multiple
              hidden
              aria-label="Upload files"
              onChange={(event) => void upload(event.target.files)}
            />
          </div>
          {adding !== null && (
            <form
              className="app-files__form"
              onSubmit={(event) => {
                event.preventDefault();
                addPath(new FormData(event.currentTarget).get('path')?.toString() ?? '');
              }}
            >
              <label className="t-caption" htmlFor="new-path">
                {adding.endsWith('/') && adding !== '' && !rows.some((r) => r.path.startsWith(adding))
                  ? 'Folder and first file (a folder exists once it holds a file)'
                  : 'Path of the new file'}
              </label>
              <input id="new-path" name="path" className="fg-input" defaultValue={adding} autoFocus />
              <span style={{ display: 'flex', gap: 'var(--space-2)' }}>
                <button type="submit" className="fg-btn fg-btn--primary fg-btn--small">
                  <span>{TEXT.add}</span>
                </button>
                <button type="button" className="fg-btn fg-btn--ghost fg-btn--small" onClick={() => setAdding(null)}>
                  <span>{TEXT.cancel}</span>
                </button>
              </span>
            </form>
          )}
        </aside>

        <section className="fg-card app-files__pane" aria-label="File">
          {selectedRow ? (
            <FilePane
              key={`${selectedRow.path}:${selectedRow.status}`}
              skillId={skillId}
              row={selectedRow}
              set={current}
              editing={editing}
              renaming={renaming === selectedRow.path}
              onEdit={() => setEditing(true)}
              onStopEditing={() => setEditing(false)}
              onChange={(hubText, text) => changeText(selectedRow, hubText, text)}
              onStartRename={() => setRenaming(selectedRow.path)}
              onCancelRename={() => setRenaming(null)}
              onRename={(to) => rename(selectedRow, to)}
              onDelete={() => update(deleteFile(current, hubFiles, selectedRow.path))}
              onRestore={() => update(restoreFile(current, selectedRow.path))}
            />
          ) : (
            <span className="t-body-sm">{TEXT.noFiles}</span>
          )}
        </section>
      </div>

      <div className="app-files__foot">
        <CheckoutButton skillId={skillId} />
      </div>

      <PendingChangesBar count={count} onDiscard={() => setConfirmDiscard(true)} onReview={() => setView('review')} />
      <ConfirmDialog
        open={confirmDiscard}
        title="Discard these changes?"
        body="The changes in this browser are thrown away. The skill on the hub does not change."
        confirmLabel="Discard changes"
        danger
        onConfirm={discardAll}
        onCancel={() => setConfirmDiscard(false)}
      />
    </div>
  );
}

interface FilePaneProps {
  skillId: string;
  row: TreeRow;
  set: Changeset;
  editing: boolean;
  renaming: boolean;
  onEdit: () => void;
  onStopEditing: () => void;
  onChange: (hubText: string, text: string) => void;
  onStartRename: () => void;
  onCancelRename: () => void;
  onRename: (to: string) => void;
  onDelete: () => void;
  onRestore: () => void;
}

// One file: its text (read or edit), or a note when it is a binary or too large to edit here.
function FilePane({ skillId, row, set, editing, renaming, onEdit, onStopEditing, onChange, onStartRename, onCancelRename, onRename, onDelete, onRestore }: FilePaneProps) {
  const entry = row.entry;
  const origin = row.from ?? row.path;
  const content = useSkillFileContent(skillId, origin, entry?.digest ?? '');
  const pending = pendingText(set, row.path);
  const hubText = content.data?.content ?? '';
  const text = pending ?? hubText;
  const isNew = row.status === 'A';
  const pendingBinary = set.changes.some((c) => c.path === row.path && c.encoding === 'base64');
  const textFile = isNew ? !pendingBinary : (content.data?.text ?? entry?.text ?? true);
  const editable = row.status !== 'D' && !pendingBinary && (isNew || (entry?.editable ?? false));
  const loading = !isNew && !pending && content.isPending;
  const binaryNote = `This is a binary file${entry ? ` (${formatSize(entry.size_bytes)})` : ''}. It can be replaced by uploading a file with the same name, renamed or deleted, not edited here.`;

  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--space-3)', minWidth: 0 }}>
      <div className="app-files__head">
        <span className="app-wrap" style={{ fontFamily: 'var(--font-mono)', fontSize: '13px', flex: 1, minWidth: 0 }}>
          {row.path}
          {row.from ? ` (renamed from ${row.from})` : ''}
        </span>
        {row.status === 'D' ? (
          <button type="button" className="fg-btn fg-btn--secondary fg-btn--small" onClick={onRestore}>
            <span>{TEXT.restore}</span>
          </button>
        ) : (
          <span style={{ display: 'flex', flexWrap: 'wrap', gap: 'var(--space-2)' }}>
            {editable && !editing && (
              <button type="button" className="fg-btn fg-btn--primary fg-btn--small" onClick={onEdit}>
                <span>{TEXT.edit}</span>
              </button>
            )}
            {editing && (
              <button type="button" className="fg-btn fg-btn--secondary fg-btn--small" onClick={onStopEditing}>
                <span>{TEXT.doneEditing}</span>
              </button>
            )}
            {row.path !== SKILL_MD && (
              <>
                <button type="button" className="fg-btn fg-btn--secondary fg-btn--small" onClick={onStartRename}>
                  <span>{TEXT.rename}</span>
                </button>
                <button type="button" className="fg-btn fg-btn--secondary fg-btn--small" onClick={onDelete}>
                  <span>{TEXT.delete}</span>
                </button>
              </>
            )}
          </span>
        )}
      </div>

      {renaming && (
        <form
          className="app-files__form"
          onSubmit={(event) => {
            event.preventDefault();
            onRename(new FormData(event.currentTarget).get('to')?.toString() ?? '');
          }}
        >
          <label className="t-caption" htmlFor="rename-to">
            {TEXT.newPath}
          </label>
          <input id="rename-to" name="to" className="fg-input" defaultValue={row.path} autoFocus />
          <span style={{ display: 'flex', gap: 'var(--space-2)' }}>
            <button type="submit" className="fg-btn fg-btn--primary fg-btn--small">
              <span>{TEXT.rename}</span>
            </button>
            <button type="button" className="fg-btn fg-btn--ghost fg-btn--small" onClick={onCancelRename}>
              <span>{TEXT.cancel}</span>
            </button>
          </span>
        </form>
      )}

      {row.status === 'D' && <span className="t-body-sm">{TEXT.deletedNote}</span>}
      {row.status !== 'D' && loading && <span className="t-body-sm">{TEXT.loadingFile}</span>}
      {row.status !== 'D' && !loading && !textFile && (
        <span className="t-body-sm">
          {binaryNote}
        </span>
      )}
      {row.status !== 'D' && !loading && textFile && !editable && (
        <span className="t-body-sm">{TEXT.tooLarge}</span>
      )}
      {row.status !== 'D' && !loading && textFile && editing && editable && (
        <textarea
          className="fg-input fg-input--area app-files__editor"
          aria-label={`Edit ${row.path}`}
          value={text}
          onChange={(event) => {
            const next = event.target.value.replace(/\r\n?/g, '\n');
            onChange(hubText, next);
          }}
        />
      )}
      {row.status !== 'D' && !loading && textFile && !editing && (
        row.path.toLowerCase().endsWith('.md') ? (
          text ? <Markdown content={text} /> : <span className="t-body-sm">{TEXT.empty}</span>
        ) : (
          <pre className="app-files__source">{text || TEXT.empty}</pre>
        )
      )}
    </div>
  );
}
