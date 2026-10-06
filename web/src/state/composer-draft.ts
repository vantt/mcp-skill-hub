import { clearDraft, loadDraft, saveDraft } from './drafts';

export interface ComposerDraftData {
  content: string;
  mappings: Array<{ observation_id: string; concept: string }>;
  evidenceDigest: string;
  contentDigest: string;
}

export interface LoadedComposerDraft {
  data: ComposerDraftData;
  needsRebase: boolean;
  staleEvidence: boolean;
  savedAt: number;
}

const DRAFT_KIND = 'composer';

export function saveComposerDraft(
  workspaceId: string,
  insightId: string,
  data: ComposerDraftData,
  now = Date.now(),
): boolean {
  return saveDraft<ComposerDraftData>(
    workspaceId,
    DRAFT_KIND,
    insightId,
    data.contentDigest,
    data,
    now,
  );
}

export function loadComposerDraft(
  workspaceId: string,
  insightId: string,
  currentContentDigest: string,
  currentEvidenceDigest: string,
): LoadedComposerDraft | null {
  const loaded = loadDraft<ComposerDraftData>(
    workspaceId,
    DRAFT_KIND,
    insightId,
    currentContentDigest,
  );
  if (!loaded || !loaded.value) {
    return null;
  }

  const needsRebase = loaded.value.contentDigest !== currentContentDigest;
  const staleEvidence = loaded.value.evidenceDigest !== currentEvidenceDigest;

  return {
    data: loaded.value,
    needsRebase,
    staleEvidence,
    savedAt: loaded.savedAt,
  };
}

export function clearComposerDraft(workspaceId: string, insightId: string): void {
  clearDraft(workspaceId, DRAFT_KIND, insightId);
}
