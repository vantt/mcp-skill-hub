const TOKEN_STORAGE_KEY = 'skillhub.web.token';

const sessionEvents = new EventTarget();

export function captureTokenFromHash(): string | null {
  try {
    const hash = window.location.hash;
    const match = hash.match(/token=([0-9a-fA-F]+)/);
    if (match && match[1]) {
      const token = match[1];
      window.sessionStorage.setItem(TOKEN_STORAGE_KEY, token);
      window.history.replaceState(null, '', window.location.pathname + window.location.search);
      return token;
    }
  } catch {
    // Ignore storage and history errors.
  }
  return null;
}

export function getToken(): string | null {
  try {
    return window.sessionStorage.getItem(TOKEN_STORAGE_KEY);
  } catch {
    return null;
  }
}

export function setToken(token: string): void {
  try {
    window.sessionStorage.setItem(TOKEN_STORAGE_KEY, token);
  } catch {
    // Ignore storage errors.
  }
}

export function clearToken(): void {
  try {
    window.sessionStorage.removeItem(TOKEN_STORAGE_KEY);
  } catch {
    // Ignore storage errors.
  }
}

export function emitSessionExpired(): void {
  sessionEvents.dispatchEvent(new Event('session-expired'));
}

export function onSessionExpired(listener: () => void): () => void {
  sessionEvents.addEventListener('session-expired', listener);
  return () => {
    sessionEvents.removeEventListener('session-expired', listener);
  };
}
