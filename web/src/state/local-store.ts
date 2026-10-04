export const PREFIX = 'skillhub.web.v1';

const THIRTY_DAYS_MS = 30 * 24 * 60 * 60 * 1000;
const MAX_SERIALIZED_LENGTH = 1_000_000;

interface StorageEnvelope<T> {
  v: 1;
  savedAt: number;
  value: T;
}

export function scopedKey(workspaceId: string, name: string): string {
  return `${PREFIX}.${workspaceId}.${name}`;
}

export function globalKey(name: string): string {
  return `${PREFIX}.${name}`;
}

export function writeJSON(key: string, value: unknown, now = Date.now()): boolean {
  try {
    const envelope: StorageEnvelope<unknown> = {
      v: 1,
      savedAt: now,
      value,
    };
    const serialized = JSON.stringify(envelope);
    if (serialized.length > MAX_SERIALIZED_LENGTH) {
      return false;
    }
    window.localStorage.setItem(key, serialized);
    return true;
  } catch {
    return false;
  }
}

export function remove(key: string): void {
  try {
    window.localStorage.removeItem(key);
  } catch {
    // Ignore storage errors.
  }
}

export function readJSON<T>(key: string, now = Date.now()): T | undefined {
  try {
    const raw = window.localStorage.getItem(key);
    if (raw === null) {
      return undefined;
    }
    const parsed = JSON.parse(raw) as Partial<StorageEnvelope<T>>;
    if (!parsed || typeof parsed !== 'object' || parsed.v !== 1 || typeof parsed.savedAt !== 'number') {
      return undefined;
    }
    if (now - parsed.savedAt > THIRTY_DAYS_MS) {
      remove(key);
      return undefined;
    }
    return parsed.value;
  } catch {
    return undefined;
  }
}

export function purgeExpired(now = Date.now()): void {
  try {
    const keysToPurge: string[] = [];
    for (let i = 0; i < window.localStorage.length; i++) {
      const key = window.localStorage.key(i);
      if (key && key.startsWith(PREFIX)) {
        const raw = window.localStorage.getItem(key);
        if (raw) {
          try {
            const parsed = JSON.parse(raw) as Partial<StorageEnvelope<unknown>>;
            if (parsed && typeof parsed.savedAt === 'number' && now - parsed.savedAt > THIRTY_DAYS_MS) {
              keysToPurge.push(key);
            }
          } catch {
            // Leave corrupt entries or remove if desired; purgeExpired removes expired entries under PREFIX.
          }
        }
      }
    }
    for (const key of keysToPurge) {
      window.localStorage.removeItem(key);
    }
  } catch {
    // Ignore storage errors.
  }
}
