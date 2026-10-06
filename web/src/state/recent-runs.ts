import { readJSON, remove as removeKey, scopedKey, writeJSON } from './local-store';

export interface RecentRunItem {
  runId: string;
  sourceId: string;
  idempotencyKey?: string;
  state: string;
  openedAt: number;
}

const STORAGE_NAME = 'recent-runs';
const MAX_RECENT_RUNS = 50;

export function addRecentRun(workspaceId: string, item: RecentRunItem, now = Date.now()): void {
  const key = scopedKey(workspaceId, STORAGE_NAME);
  const current = listRecentRuns(workspaceId);
  const filtered = current.filter((r) => r.runId !== item.runId);
  const updated = [item, ...filtered].slice(0, MAX_RECENT_RUNS);
  writeJSON(key, updated, now);
}

export function updateRecentRun(workspaceId: string, runId: string, state: string, now = Date.now()): void {
  const key = scopedKey(workspaceId, STORAGE_NAME);
  const current = listRecentRuns(workspaceId);
  const updated = current.map((r) => (r.runId === runId ? { ...r, state } : r));
  writeJSON(key, updated, now);
}

export function removeRecentRun(workspaceId: string, runId: string, now = Date.now()): void {
  const key = scopedKey(workspaceId, STORAGE_NAME);
  const current = listRecentRuns(workspaceId);
  const updated = current.filter((r) => r.runId !== runId);
  writeJSON(key, updated, now);
}

export function listRecentRuns(workspaceId: string): RecentRunItem[] {
  const key = scopedKey(workspaceId, STORAGE_NAME);
  const raw = readJSON<RecentRunItem[]>(key);
  if (!Array.isArray(raw)) {
    return [];
  }
  return raw.slice(0, MAX_RECENT_RUNS);
}

export function clearRecentRuns(workspaceId: string): void {
  const key = scopedKey(workspaceId, STORAGE_NAME);
  removeKey(key);
}
