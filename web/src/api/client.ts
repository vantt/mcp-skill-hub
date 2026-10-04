import { emitSessionExpired, getToken } from '../state/session';

export interface ErrorRender {
  ERROR: string;
  WHY: string;
  FIX: string;
}

export interface ApiErrorEnvelope {
  code: string;
  render: ErrorRender;
  retryable?: boolean;
  correlation_id?: string;
}

export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly render: ErrorRender;
  readonly retryable?: boolean;
  readonly correlationId?: string;

  constructor(status: number, data: ApiErrorEnvelope) {
    const errorMsg = data.render?.ERROR
      ? (data.render.WHY ? `${data.render.ERROR}: ${data.render.WHY}` : data.render.ERROR)
      : data.code;
    super(errorMsg);
    this.name = 'ApiError';
    this.status = status;
    this.code = data.code;
    this.render = data.render ?? {
      ERROR: 'An unexpected error occurred.',
      WHY: '',
      FIX: '',
    };
    this.retryable = data.retryable;
    this.correlationId = data.correlation_id;
  }
}

export async function apiFetch<T>(path: string, init?: RequestInit): Promise<T> {
  let url = path;
  if (!url.startsWith('/api/')) {
    url = `/api/v1${url.startsWith('/') ? '' : '/'}${url}`;
  }

  const headers = new Headers(init?.headers);
  const token = getToken();
  if (token && !headers.has('Authorization')) {
    headers.set('Authorization', `Bearer ${token}`);
  }
  if (init?.body && !headers.has('Content-Type') && typeof init.body === 'string') {
    headers.set('Content-Type', 'application/json');
  }

  const response = await fetch(url, {
    ...init,
    headers,
  });

  if (response.status === 401) {
    emitSessionExpired();
  }

  if (!response.ok) {
    let errorData: ApiErrorEnvelope;
    try {
      const json = await response.json();
      errorData = (json && typeof json === 'object' && 'error' in json && json.error)
        ? (json.error as ApiErrorEnvelope)
        : (json as ApiErrorEnvelope);
    } catch {
      errorData = {
        code: 'internal_error',
        render: {
          ERROR: response.statusText || 'Request failed.',
          WHY: `HTTP ${response.status}`,
          FIX: 'Retry the request.',
        },
      };
    }
    throw new ApiError(response.status, errorData);
  }

  if (response.status === 204) {
    return undefined as T;
  }

  return (await response.json()) as T;
}
