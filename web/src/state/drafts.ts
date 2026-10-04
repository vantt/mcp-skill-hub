import { readJSON, remove, scopedKey, writeJSON } from './local-store';

export interface DraftEnvelope<T> {
  baseDigest: string;
  value: T;
  savedAt: number;
}

export interface LoadedDraft<T> {
  value: T;
  stale: boolean;
  savedAt: number;
}

export function saveDraft<T>(
  workspaceId: string,
  kind: string,
  id: string,
  baseDigest: string,
  value: T,
  now = Date.now(),
): boolean {
  const key = scopedKey(workspaceId, `draft.${kind}.${id}`);
  const payload: DraftEnvelope<T> = {
    baseDigest,
    value,
    savedAt: now,
  };
  return writeJSON(key, payload, now);
}

export function loadDraft<T>(
  workspaceId: string,
  kind: string,
  id: string,
  currentDigest: string,
): LoadedDraft<T> | null {
  const key = scopedKey(workspaceId, `draft.${kind}.${id}`);
  const loaded = readJSON<DraftEnvelope<T>>(key);
  if (!loaded || typeof loaded !== 'object' || !('baseDigest' in loaded)) {
    return null;
  }
  return {
    value: loaded.value,
    stale: loaded.baseDigest !== currentDigest,
    savedAt: loaded.savedAt,
  };
}

export function clearDraft(workspaceId: string, kind: string, id: string): void {
  const key = scopedKey(workspaceId, `draft.${kind}.${id}`);
  remove(key);
}
