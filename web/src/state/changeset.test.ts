import { beforeEach, describe, expect, it } from 'vitest';
import type { SkillFileEntry } from '../api/types';
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
  toInput,
  treeRows,
} from './changeset';

function entry(path: string): SkillFileEntry {
  return { path, size_bytes: 3, digest: `sha256:${path}`, text: true, editable: true, executable: false };
}

const files = [entry('scripts/extract.sh'), entry('references/forms.md'), entry('SKILL.md'), entry('references/tables.md')];

describe('changeset', () => {
  beforeEach(() => window.localStorage.clear());

  it('lists SKILL.md first and nested paths in order, with a status per changed file', () => {
    let set = emptyChangeset('v1');
    set = editFile(set, files[1]!, 'old', 'new');
    set = addFile(set, files, 'references/ocr.md', '# OCR');
    set = deleteFile(set, files, 'scripts/extract.sh');
    const rows = treeRows(files, set).map((row) => `${row.path}:${row.status}`);
    expect(rows).toEqual([
      'SKILL.md:',
      'references/forms.md:M',
      'references/ocr.md:A',
      'references/tables.md:',
      'scripts/extract.sh:D',
    ]);
    expect(changeCount(set)).toBe(3);
  });

  it('turns edits back into no change, and sends the file digest it started from', () => {
    let set = editFile(emptyChangeset('v1'), files[1]!, 'old', 'new');
    expect(toInput(set).changes[0]).toMatchObject({ action: 'modify', path: 'references/forms.md', expected_digest: 'sha256:references/forms.md' });
    set = editFile(set, files[1]!, 'old', 'old');
    expect(changeCount(set)).toBe(0);
    expect(editFile(emptyChangeset('v1'), files[1]!, 'old', 'old').changes).toEqual([]);
  });

  it('drops an added file when it is deleted and moves it when it is renamed', () => {
    let set = addFile(emptyChangeset('v1'), files, 'notes.md', 'hi');
    set = renameFile(set, files, 'notes.md', 'docs/notes.md');
    expect(set.changes).toEqual([{ action: 'add', path: 'docs/notes.md', content: 'hi', encoding: 'utf8' }]);
    expect(pendingText(set, 'docs/notes.md')).toBe('hi');
    set = deleteFile(set, files, 'docs/notes.md');
    expect(changeCount(set)).toBe(0);
  });

  it('renames a hub file once, keeps its edit, and deletes the old path when it is deleted', () => {
    let set = editFile(emptyChangeset('v1'), files[1]!, 'old', 'new');
    set = renameFile(set, files, 'references/forms.md', 'references/forms-guide.md');
    expect(set.changes).toEqual([
      { action: 'rename', from: 'references/forms.md', path: 'references/forms-guide.md', content: 'new', encoding: 'utf8', expected_digest: 'sha256:references/forms.md' },
    ]);
    set = renameFile(set, files, 'references/forms-guide.md', 'guide.md');
    expect(set.changes).toHaveLength(1);
    expect(set.changes[0]).toMatchObject({ action: 'rename', from: 'references/forms.md', path: 'guide.md' });
    const rows = treeRows(files, set).map((row) => `${row.path}:${row.status}`);
    expect(rows).toContain('guide.md:R');
    expect(rows).not.toContain('references/forms.md:');
    set = deleteFile(set, files, 'guide.md');
    expect(set.changes).toEqual([{ action: 'delete', path: 'references/forms.md', expected_digest: 'sha256:references/forms.md' }]);
  });

  it('puts a deleted file back, and adding over a deleted path edits the hub file', () => {
    let set = deleteFile(emptyChangeset('v1'), files, 'references/tables.md');
    expect(restoreFile(set, 'references/tables.md').changes).toEqual([]);
    set = addFile(set, files, 'references/tables.md', 'again');
    expect(set.changes).toEqual([
      { action: 'modify', path: 'references/tables.md', content: 'again', encoding: 'utf8', expected_digest: 'sha256:references/tables.md' },
    ]);
  });

  it('keeps an uploaded file out of the text the editor shows', () => {
    const set = addFile(emptyChangeset('v1'), files, 'assets/logo.png', 'AAEC', 'base64');
    expect(pendingText(set, 'assets/logo.png')).toBeUndefined();
    expect(toInput(set).changes[0]).toMatchObject({ action: 'add', encoding: 'base64' });
  });

  it('is saved per skill and cleared by Discard', () => {
    const set = addFile(emptyChangeset('v1'), files, 'a.md', 'x');
    expect(saveChangeset('ws', 'pdf-tools', set)).toBe(true);
    expect(loadChangeset('ws', 'pdf-tools')).toEqual(set);
    expect(loadChangeset('ws', 'other')).toBeUndefined();
    clearChangeset('ws', 'pdf-tools');
    expect(loadChangeset('ws', 'pdf-tools')).toBeUndefined();
    saveChangeset('ws', 'pdf-tools', set);
    saveChangeset('ws', 'pdf-tools', emptyChangeset('v1'));
    expect(loadChangeset('ws', 'pdf-tools')).toBeUndefined();
  });
});
