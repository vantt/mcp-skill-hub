import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  addRecentRun,
  clearRecentRuns,
  listRecentRuns,
  removeRecentRun,
  updateRecentRun,
} from './recent-runs';

describe('recent-runs state', () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  afterEach(() => {
    vi.restoreAllMocks();
    window.localStorage.clear();
  });

  it('adds and lists recent runs newest first', () => {
    const ws = 'ws-test-1';
    addRecentRun(ws, {
      runId: 'RUN-1',
      sourceId: 'src-1',
      state: 'in_progress',
      openedAt: 1000,
    });
    addRecentRun(ws, {
      runId: 'RUN-2',
      sourceId: 'src-2',
      state: 'prepared',
      openedAt: 2000,
    });

    const runs = listRecentRuns(ws);
    expect(runs).toHaveLength(2);
    expect(runs[0]?.runId).toBe('RUN-2');
    expect(runs[1]?.runId).toBe('RUN-1');
  });

  it('updates state of an existing recent run', () => {
    const ws = 'ws-test-1';
    addRecentRun(ws, {
      runId: 'RUN-1',
      sourceId: 'src-1',
      state: 'in_progress',
      openedAt: 1000,
    });

    updateRecentRun(ws, 'RUN-1', 'finalized');
    const runs = listRecentRuns(ws);
    expect(runs).toHaveLength(1);
    expect(runs[0]?.state).toBe('finalized');
  });

  it('removes a recent run by runId', () => {
    const ws = 'ws-test-1';
    addRecentRun(ws, {
      runId: 'RUN-1',
      sourceId: 'src-1',
      state: 'in_progress',
      openedAt: 1000,
    });
    addRecentRun(ws, {
      runId: 'RUN-2',
      sourceId: 'src-2',
      state: 'prepared',
      openedAt: 2000,
    });

    removeRecentRun(ws, 'RUN-1');
    const runs = listRecentRuns(ws);
    expect(runs).toHaveLength(1);
    expect(runs[0]?.runId).toBe('RUN-2');
  });

  it('caps list at 50 entries', () => {
    const ws = 'ws-test-1';
    for (let i = 1; i <= 55; i++) {
      addRecentRun(ws, {
        runId: `RUN-${i}`,
        sourceId: 'src-1',
        state: 'finalized',
        openedAt: i * 10,
      });
    }

    const runs = listRecentRuns(ws);
    expect(runs).toHaveLength(50);
    expect(runs[0]?.runId).toBe('RUN-55');
    expect(runs[49]?.runId).toBe('RUN-6');
  });

  it('isolates recent runs per workspace', () => {
    addRecentRun('ws-A', {
      runId: 'RUN-A',
      sourceId: 'src-A',
      state: 'finalized',
      openedAt: 1000,
    });
    addRecentRun('ws-B', {
      runId: 'RUN-B',
      sourceId: 'src-B',
      state: 'in_progress',
      openedAt: 2000,
    });

    const listA = listRecentRuns('ws-A');
    const listB = listRecentRuns('ws-B');

    expect(listA).toHaveLength(1);
    expect(listA[0]?.runId).toBe('RUN-A');
    expect(listB).toHaveLength(1);
    expect(listB[0]?.runId).toBe('RUN-B');
  });

  it('handles storage unavailable gracefully', () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('localStorage is blocked');
    });
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('localStorage is blocked');
    });

    // None of these should throw
    expect(() => {
      addRecentRun('ws-err', {
        runId: 'RUN-ERR',
        sourceId: 'src-1',
        state: 'in_progress',
        openedAt: 1000,
      });
    }).not.toThrow();

    expect(() => {
      updateRecentRun('ws-err', 'RUN-ERR', 'finalized');
    }).not.toThrow();

    expect(() => {
      removeRecentRun('ws-err', 'RUN-ERR');
    }).not.toThrow();

    expect(() => {
      clearRecentRuns('ws-err');
    }).not.toThrow();

    expect(listRecentRuns('ws-err')).toEqual([]);
  });
});
