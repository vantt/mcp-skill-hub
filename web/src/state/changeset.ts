import type { SkillChangesInput, SkillFileEntry, SkillTreeChange } from '../api/types';
import { readJSON, remove, scopedKey, writeJSON } from './local-store';

// The edits a person has made to a skill's files and not yet committed. They are made against one
// version of the skill (baseVersion); the hub is only written when the set is committed.
export interface Changeset {
  baseVersion: string;
  changes: SkillTreeChange[];
}

export type RowStatus = '' | 'A' | 'M' | 'D' | 'R';

// One row of the file tree: the file as it will be if the changeset is committed.
export interface TreeRow {
  path: string;
  status: RowStatus;
  from?: string;
  entry?: SkillFileEntry;
}

export function emptyChangeset(baseVersion: string): Changeset {
  return { baseVersion, changes: [] };
}

export function changeCount(set: Changeset): number {
  return set.changes.length;
}

function withChanges(set: Changeset, changes: SkillTreeChange[]): Changeset {
  return { baseVersion: set.baseVersion, changes };
}

// The change that decides what a path is now: an add, a modify or a delete of that path, or the
// rename that moved a file to it.
function changeAt(set: Changeset, path: string): SkillTreeChange | undefined {
  return set.changes.find((c) => c.path === path);
}

function without(set: Changeset, change: SkillTreeChange): SkillTreeChange[] {
  return set.changes.filter((c) => c !== change);
}

function replaceChange(set: Changeset, change: SkillTreeChange, next: SkillTreeChange): SkillTreeChange[] {
  return set.changes.map((c) => (c === change ? next : c));
}

// Puts new text in a file. A file the hub has becomes a modify; a file the set added keeps being an
// add. Text equal to what the hub has removes the modify, so an undone edit leaves no change.
export function editFile(
  set: Changeset,
  file: SkillFileEntry,
  hubText: string,
  text: string,
  encoding: 'utf8' | 'base64' = 'utf8',
): Changeset {
  const unchanged = encoding === 'utf8' && text === hubText;
  const current = changeAt(set, file.path);
  if (!current) {
    if (unchanged) return set;
    return withChanges(set, [
      ...set.changes,
      { action: 'modify', path: file.path, content: text, encoding, expected_digest: file.digest },
    ]);
  }
  if (current.action === 'modify' && unchanged) return withChanges(set, without(set, current));
  return withChanges(set, replaceChange(set, current, { ...current, content: text, encoding }));
}

// Adds a file the hub does not have. Adding over a file the set deleted keeps the hub's file and
// edits it instead.
export function addFile(
  set: Changeset,
  hubFiles: SkillFileEntry[],
  path: string,
  content: string,
  encoding: 'utf8' | 'base64' = 'utf8',
): Changeset {
  const deleted = set.changes.find((c) => c.action === 'delete' && c.path === path);
  if (deleted) {
    const hub = hubFiles.find((f) => f.path === path);
    const next = without(set, deleted);
    return withChanges(set, [
      ...next,
      { action: 'modify', path, content, encoding, expected_digest: hub?.digest ?? deleted.expected_digest },
    ]);
  }
  return withChanges(set, [...set.changes, { action: 'add', path, content, encoding }]);
}

// Deletes a file. A file the set added just disappears; a renamed file is deleted from its old path.
export function deleteFile(set: Changeset, hubFiles: SkillFileEntry[], path: string): Changeset {
  const current = changeAt(set, path);
  if (!current) {
    const hub = hubFiles.find((f) => f.path === path);
    if (!hub) return set;
    return withChanges(set, [...set.changes, { action: 'delete', path, expected_digest: hub.digest }]);
  }
  if (current.action === 'add') return withChanges(set, without(set, current));
  if (current.action === 'rename' && current.from) {
    return withChanges(
      set,
      replaceChange(set, current, { action: 'delete', path: current.from, expected_digest: current.expected_digest }),
    );
  }
  if (current.action === 'modify') {
    return withChanges(set, replaceChange(set, current, { action: 'delete', path, expected_digest: current.expected_digest }));
  }
  return set;
}

// Renames a file. Renaming a file the set added, or renamed before, only moves it.
export function renameFile(set: Changeset, hubFiles: SkillFileEntry[], from: string, to: string): Changeset {
  if (from === to) return set;
  const current = changeAt(set, from);
  if (!current) {
    const hub = hubFiles.find((f) => f.path === from);
    if (!hub) return set;
    return withChanges(set, [...set.changes, { action: 'rename', from, path: to, expected_digest: hub.digest }]);
  }
  if (current.action === 'add' || current.action === 'rename') {
    return withChanges(set, replaceChange(set, current, { ...current, path: to }));
  }
  if (current.action === 'modify') {
    return withChanges(
      set,
      replaceChange(set, current, {
        action: 'rename', from, path: to, content: current.content, encoding: current.encoding, expected_digest: current.expected_digest,
      }),
    );
  }
  return set;
}

// Puts a deleted file back.
export function restoreFile(set: Changeset, path: string): Changeset {
  const current = set.changes.find((c) => c.action === 'delete' && c.path === path);
  return current ? withChanges(set, without(set, current)) : set;
}

export function toInput(set: Changeset): SkillChangesInput {
  return { base_version: set.baseVersion, changes: set.changes };
}

function byTreeOrder(a: TreeRow, b: TreeRow): number {
  if ((a.path === 'SKILL.md') !== (b.path === 'SKILL.md')) return a.path === 'SKILL.md' ? -1 : 1;
  return a.path < b.path ? -1 : a.path > b.path ? 1 : 0;
}

// The files as they will be after the set is committed. A deleted file stays in the list, marked D,
// so it can be put back; a renamed file shows at its new path, marked R.
export function treeRows(files: SkillFileEntry[], set: Changeset): TreeRow[] {
  const renamedFrom = new Set(set.changes.filter((c) => c.action === 'rename' && c.from).map((c) => c.from));
  const rows: TreeRow[] = [];
  for (const file of files) {
    if (renamedFrom.has(file.path)) continue;
    const change = changeAt(set, file.path);
    const status: RowStatus = change?.action === 'delete' ? 'D' : change?.action === 'modify' ? 'M' : '';
    rows.push({ path: file.path, status, entry: file });
  }
  for (const change of set.changes) {
    if (change.action === 'add') rows.push({ path: change.path, status: 'A' });
    if (change.action === 'rename') {
      rows.push({ path: change.path, status: 'R', from: change.from, entry: files.find((f) => f.path === change.from) });
    }
  }
  return rows.sort(byTreeOrder);
}

// What the person sees for a path now: the pending text when the set has any, otherwise nothing, so
// the caller reads the hub's text.
export function pendingText(set: Changeset, path: string): string | undefined {
  const change = changeAt(set, path);
  if (!change || change.action === 'delete') return undefined;
  if (change.encoding === 'base64') return undefined;
  return change.content;
}

const KIND = 'changes';

function key(workspaceId: string, skillId: string): string {
  return scopedKey(workspaceId, `${KIND}.${skillId}`);
}

// A saved set belongs to one version of the skill. For another version it is returned as stale, so
// the screen can say the hub changed instead of applying it to different files.
export function loadChangeset(workspaceId: string, skillId: string): Changeset | undefined {
  const saved = readJSON<Changeset>(key(workspaceId, skillId));
  if (!saved || typeof saved !== 'object' || !Array.isArray(saved.changes) || typeof saved.baseVersion !== 'string') {
    return undefined;
  }
  return saved;
}

export function saveChangeset(workspaceId: string, skillId: string, set: Changeset): boolean {
  if (set.changes.length === 0) {
    remove(key(workspaceId, skillId));
    return true;
  }
  return writeJSON(key(workspaceId, skillId), set);
}

export function clearChangeset(workspaceId: string, skillId: string): void {
  remove(key(workspaceId, skillId));
}
