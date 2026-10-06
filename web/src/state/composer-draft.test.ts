import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import {
  clearComposerDraft,
  loadComposerDraft,
  saveComposerDraft,
  type ComposerDraftData,
} from './composer-draft';

describe('composer-draft state', () => {
  const ws = 'test-workspace';
  const insightId = 'ins-test-1';

  beforeEach(() => {
    window.localStorage.clear();
  });

  afterEach(() => {
    window.localStorage.clear();
  });

  it('saves and loads draft when digests match', () => {
    const draftData: ComposerDraftData = {
      content: '# New skill content\n',
      mappings: [{ observation_id: 'obs-1', concept: 'retry' }],
      contentDigest: 'sha256:content-digest-1',
      evidenceDigest: 'sha256:evidence-digest-1',
    };

    saveComposerDraft(ws, insightId, draftData);

    const loaded = loadComposerDraft(
      ws,
      insightId,
      'sha256:content-digest-1',
      'sha256:evidence-digest-1',
    );
    expect(loaded).not.toBeNull();
    expect(loaded?.data).toEqual(draftData);
    expect(loaded?.needsRebase).toBe(false);
    expect(loaded?.staleEvidence).toBe(false);
  });

  it('reports needsRebase when contentDigest differs', () => {
    const draftData: ComposerDraftData = {
      content: '# New skill content\n',
      mappings: [{ observation_id: 'obs-1', concept: 'retry' }],
      contentDigest: 'sha256:old-content-digest',
      evidenceDigest: 'sha256:evidence-digest-1',
    };

    saveComposerDraft(ws, insightId, draftData);

    // Skill content changed outside, so current digest is new
    const loaded = loadComposerDraft(
      ws,
      insightId,
      'sha256:new-content-digest',
      'sha256:evidence-digest-1',
    );
    expect(loaded).not.toBeNull();
    expect(loaded?.needsRebase).toBe(true);
    expect(loaded?.staleEvidence).toBe(false);
  });

  it('reports staleEvidence when evidenceDigest differs', () => {
    const draftData: ComposerDraftData = {
      content: '# New skill content\n',
      mappings: [{ observation_id: 'obs-1', concept: 'retry' }],
      contentDigest: 'sha256:content-digest-1',
      evidenceDigest: 'sha256:old-evidence-digest',
    };

    saveComposerDraft(ws, insightId, draftData);

    // Upstream changed, updating evidence digest
    const loaded = loadComposerDraft(
      ws,
      insightId,
      'sha256:content-digest-1',
      'sha256:new-evidence-digest',
    );
    expect(loaded).not.toBeNull();
    expect(loaded?.needsRebase).toBe(false);
    expect(loaded?.staleEvidence).toBe(true);
  });

  it('reports both needsRebase and staleEvidence when both differ', () => {
    const draftData: ComposerDraftData = {
      content: '# New skill content\n',
      mappings: [{ observation_id: 'obs-1', concept: 'retry' }],
      contentDigest: 'sha256:old-content-digest',
      evidenceDigest: 'sha256:old-evidence-digest',
    };

    saveComposerDraft(ws, insightId, draftData);

    const loaded = loadComposerDraft(
      ws,
      insightId,
      'sha256:new-content-digest',
      'sha256:new-evidence-digest',
    );
    expect(loaded).not.toBeNull();
    expect(loaded?.needsRebase).toBe(true);
    expect(loaded?.staleEvidence).toBe(true);
  });

  it('clears draft from store', () => {
    const draftData: ComposerDraftData = {
      content: '# New skill content\n',
      mappings: [],
      contentDigest: 'sha256:c1',
      evidenceDigest: 'sha256:e1',
    };

    saveComposerDraft(ws, insightId, draftData);
    clearComposerDraft(ws, insightId);

    const loaded = loadComposerDraft(ws, insightId, 'sha256:c1', 'sha256:e1');
    expect(loaded).toBeNull();
  });
});
