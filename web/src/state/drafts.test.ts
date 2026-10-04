import { beforeEach, describe, expect, it } from 'vitest';
import { clearDraft, loadDraft, saveDraft } from './drafts';

describe('drafts', () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it('performs round trip save and load with matching digest', () => {
    const ws = 'workspace-1';
    const kind = 'skill';
    const id = 'my-skill';
    const digest = 'sha256:1111';
    const content = '# My Skill Draft';

    const saved = saveDraft(ws, kind, id, digest, content);
    expect(saved).toBe(true);

    const loaded = loadDraft<string>(ws, kind, id, digest);
    expect(loaded).not.toBeNull();
    expect(loaded?.value).toBe(content);
    expect(loaded?.stale).toBe(false);
  });

  it('detects stale draft when current digest differs from stored baseDigest', () => {
    const ws = 'workspace-1';
    const kind = 'skill';
    const id = 'my-skill';
    const originalDigest = 'sha256:1111';
    const updatedDigest = 'sha256:2222';
    const content = '# My Skill Draft';

    saveDraft(ws, kind, id, originalDigest, content);

    const loaded = loadDraft<string>(ws, kind, id, updatedDigest);
    expect(loaded).not.toBeNull();
    expect(loaded?.value).toBe(content);
    expect(loaded?.stale).toBe(true);
  });

  it('clears draft permanently', () => {
    const ws = 'workspace-1';
    const kind = 'skill';
    const id = 'my-skill';
    const digest = 'sha256:1111';

    saveDraft(ws, kind, id, digest, 'content');
    expect(loadDraft(ws, kind, id, digest)).not.toBeNull();

    clearDraft(ws, kind, id);
    expect(loadDraft(ws, kind, id, digest)).toBeNull();
  });
});
