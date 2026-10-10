import { describe, expect, it } from 'vitest';
import { changedFields, describeChanges, mergeOntoLatest, type EditFields } from './edit-fields';

const base: EditFields = {
  name: 'Demo',
  description: 'Old words',
  content: 'one\ntwo',
  operations: 'review',
  triggers: 'a, b',
  notFor: '',
  minScope: 'single_step',
};

describe('edit fields', () => {
  it('lists only the fields that differ, ignoring list spacing', () => {
    expect(changedFields(base, { ...base, triggers: 'a,b' })).toEqual([]);
    expect(changedFields(base, { ...base, description: 'New', minScope: 'project' })).toEqual([
      'description',
      'minScope',
    ]);
  });

  it('says in words what changes', () => {
    const next = { ...base, description: 'New words', triggers: 'a, c', content: 'one\nthree\nfour' };
    expect(describeChanges(base, next)).toEqual([
      'Description: "Old words" becomes "New words".',
      'Instructions (SKILL.md): 2 lines added, 1 line removed.',
      'Triggers: added "c"; removed "b".',
    ]);
  });

  it('keeps an edit made elsewhere when the person did not touch that field', () => {
    const mine = { ...base, content: 'one\nmine' };
    const latest = { ...base, description: 'Changed elsewhere' };
    const merged = mergeOntoLatest(base, mine, latest);
    expect(merged.description).toBe('Changed elsewhere');
    expect(merged.content).toBe('one\nmine');
  });

  it('lets the person win a field changed in both places', () => {
    const mine = { ...base, description: 'Mine' };
    const latest = { ...base, description: 'Theirs' };
    expect(mergeOntoLatest(base, mine, latest).description).toBe('Mine');
  });
});
