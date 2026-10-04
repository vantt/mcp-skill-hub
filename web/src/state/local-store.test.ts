import { beforeEach, describe, expect, it, vi } from 'vitest';
import {
  PREFIX,
  globalKey,
  purgeExpired,
  readJSON,
  remove,
  scopedKey,
  writeJSON,
} from './local-store';

describe('local-store', () => {
  beforeEach(() => {
    window.localStorage.clear();
    vi.restoreAllMocks();
  });

  it('performs round trip read and write', () => {
    const key = globalKey('theme');
    const data = { name: 'precision', accent: 'amber' };
    expect(writeJSON(key, data)).toBe(true);
    expect(readJSON(key)).toEqual(data);
  });

  it('scoping keeps two workspace IDs apart', () => {
    const keyA = scopedKey('ws-a', 'recent');
    const keyB = scopedKey('ws-b', 'recent');
    writeJSON(keyA, { count: 1 });
    writeJSON(keyB, { count: 2 });

    expect(readJSON(keyA)).toEqual({ count: 1 });
    expect(readJSON(keyB)).toEqual({ count: 2 });
  });

  it('expires items at 30 days plus 1 ms and cleans them up', () => {
    const key = globalKey('expiring');
    const now = 1_000_000_000;
    const thirtyDaysMs = 30 * 24 * 60 * 60 * 1000;

    writeJSON(key, 'value', now);
    // Exact 30 days -> still valid
    expect(readJSON(key, now + thirtyDaysMs)).toBe('value');

    // 30 days + 1 ms -> expired and removed
    expect(readJSON(key, now + thirtyDaysMs + 1)).toBeUndefined();
    expect(window.localStorage.getItem(key)).toBeNull();
  });

  it('returns undefined on corrupt JSON or invalid envelope', () => {
    const key = globalKey('corrupt');
    window.localStorage.setItem(key, '{not-valid-json');
    expect(readJSON(key)).toBeUndefined();

    const badEnvelopeKey = globalKey('bad-env');
    window.localStorage.setItem(badEnvelopeKey, JSON.stringify({ v: 2, value: 'new-version' }));
    expect(readJSON(badEnvelopeKey)).toBeUndefined();
  });

  it('returns false on oversize write (> 1,000,000 characters)', () => {
    const key = globalKey('oversize');
    const largeString = 'x'.repeat(1_000_001);
    expect(writeJSON(key, largeString)).toBe(false);
    expect(window.localStorage.getItem(key)).toBeNull();
  });

  it('handles localStorage throwing without throwing', () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('SecurityError: denied');
    });
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('QuotaExceededError');
    });
    vi.spyOn(Storage.prototype, 'removeItem').mockImplementation(() => {
      throw new Error('SecurityError: denied');
    });

    expect(writeJSON('test', 123)).toBe(false);
    expect(readJSON('test')).toBeUndefined();
    expect(() => remove('test')).not.toThrow();
    expect(() => purgeExpired()).not.toThrow();
  });

  it('purgeExpired removes expired entries under PREFIX and leaves non-PREFIX keys untouched', () => {
    const now = 10_000_000_000;
    const thirtyDaysMs = 30 * 24 * 60 * 60 * 1000;

    const expiredKey = `${PREFIX}.ws-1.old`;
    const validKey = `${PREFIX}.ws-1.fresh`;
    const foreignKey = 'foreign.app.key';

    writeJSON(expiredKey, 'old', now - thirtyDaysMs - 100);
    writeJSON(validKey, 'fresh', now - 1000);
    window.localStorage.setItem(foreignKey, 'untouched-value');

    purgeExpired(now);

    expect(window.localStorage.getItem(expiredKey)).toBeNull();
    expect(readJSON(validKey, now)).toBe('fresh');
    expect(window.localStorage.getItem(foreignKey)).toBe('untouched-value');
  });
});
