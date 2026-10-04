import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError, apiFetch } from './client';
import { captureTokenFromHash, clearToken, onSessionExpired, setToken } from '../state/session';
import { loadGolden } from '../test/golden';

describe('api client', () => {
  beforeEach(() => {
    window.sessionStorage.clear();
    clearToken();
    vi.restoreAllMocks();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('sends the Authorization header with stored token', async () => {
    setToken('test-secret-token');

    let capturedHeaders: Headers | undefined;
    vi.stubGlobal(
      'fetch',
      vi.fn().mockImplementation((_url: string, init?: RequestInit) => {
        capturedHeaders = new Headers(init?.headers);
        return Promise.resolve(new Response(JSON.stringify({ ok: true }), { status: 200 }));
      }),
    );

    const result = await apiFetch<{ ok: boolean }>('/session');
    expect(result).toEqual({ ok: true });
    expect(capturedHeaders?.get('Authorization')).toBe('Bearer test-secret-token');
  });

  it('throws ApiError with status 404 and code invalid_request from skill-unknown.json', async () => {
    const errorFixture = loadGolden('skill-unknown');

    vi.stubGlobal(
      'fetch',
      vi.fn().mockImplementation(() => {
        return Promise.resolve(
          new Response(JSON.stringify(errorFixture), {
            status: 404,
            statusText: 'Not Found',
            headers: { 'Content-Type': 'application/json' },
          }),
        );
      }),
    );

    await expect(apiFetch('/skills/unknown')).rejects.toThrowError(ApiError);

    try {
      await apiFetch('/skills/unknown');
    } catch (err) {
      const apiErr = err as ApiError;
      expect(apiErr.status).toBe(404);
      expect(apiErr.code).toBe('invalid_request');
      expect(apiErr.render.ERROR).toContain('conflicts with the current object state');
      expect(apiErr.render.FIX).toContain('Refresh the object state');
    }
  });

  it('emits session-expired on 401 response', async () => {
    let expiredFired = false;
    const unsubscribe = onSessionExpired(() => {
      expiredFired = true;
    });

    vi.stubGlobal(
      'fetch',
      vi.fn().mockImplementation(() => {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              error: {
                code: 'invalid_request',
                render: { ERROR: 'unauthorized', WHY: '', FIX: '' },
              },
            }),
            {
              status: 401,
              statusText: 'Unauthorized',
              headers: { 'Content-Type': 'application/json' },
            },
          ),
        );
      }),
    );

    await expect(apiFetch('/home')).rejects.toThrowError(ApiError);
    expect(expiredFired).toBe(true);

    unsubscribe();
  });

  it('captureTokenFromHash stores the token and strips the hash', () => {
    window.location.hash = '#token=0123456789abcdef0123456789abcdef';
    const replaceStateSpy = vi.spyOn(window.history, 'replaceState');

    const captured = captureTokenFromHash();
    expect(captured).toBe('0123456789abcdef0123456789abcdef');
    expect(window.sessionStorage.getItem('skillhub.web.token')).toBe(
      '0123456789abcdef0123456789abcdef',
    );
    expect(replaceStateSpy).toHaveBeenCalledWith(
      null,
      '',
      window.location.pathname + window.location.search,
    );
  });
});
