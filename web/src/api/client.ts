export type ApiErrorBody = {
  error?: { code?: string; message?: string; fields?: Record<string, string> };
  request_id?: string;
};

export type ApiResult<T> = {
  status: number;
  body: T;
  retryAfter: string | null;
};

export type ApiRequestInit = RequestInit & {
  csrf?: string;
};

type InternalApiRequestInit = ApiRequestInit & {
  csrfRetried?: boolean;
};

const csrfHashInvalidMessage = "CSRF token is invalid";
const sessionPath = "/api/v1/session";

const csrfListeners = new Set<(csrf: string) => void>();

export function subscribeCsrf(listener: (csrf: string) => void): () => void {
  csrfListeners.add(listener);
  return () => {
    csrfListeners.delete(listener);
  };
}

function publishCsrf(csrf: string): void {
  for (const listener of csrfListeners) {
    listener(csrf);
  }
}

function headerGet(headers: Headers, name: string): string | null {
  return headers.get(name);
}

function isCsrfHashInvalid(status: number, body: unknown): boolean {
  if (status !== 403 || body == null || typeof body !== "object") {
    return false;
  }
  const error = (body as ApiErrorBody).error;
  return error?.code === "csrf_invalid" && error.message === csrfHashInvalidMessage;
}

async function refreshSessionCsrf(): Promise<string | null> {
  const response = await fetch(sessionPath, {
    method: "GET",
    credentials: "same-origin",
    headers: { Accept: "application/json" },
  });
  if (response.status !== 200) {
    return null;
  }
  let payload: { csrf_token?: unknown } = {};
  try {
    payload = (await response.json()) as { csrf_token?: unknown };
  } catch {
    return null;
  }
  if (typeof payload.csrf_token !== "string" || payload.csrf_token === "") {
    return null;
  }
  publishCsrf(payload.csrf_token);
  return payload.csrf_token;
}

export async function apiRequest<T>(
  path: string,
  init: InternalApiRequestInit = {},
): Promise<ApiResult<T>> {
  const headers = new Headers(init.headers);
  headers.set("Accept", "application/json");
  if (init.body != null) {
    headers.set("Content-Type", "application/json");
  }
  if (init.csrf) {
    headers.set("X-CSRF-Token", init.csrf);
  }
  const { csrf: _csrf, csrfRetried, ...rest } = init;
  const response = await fetch(path, {
    ...rest,
    credentials: "same-origin",
    headers,
  });
  let body = {} as T;
  try {
    body = (await response.json()) as T;
  } catch {
    body = {} as T;
  }
  const result: ApiResult<T> = {
    status: response.status,
    body,
    retryAfter: headerGet(response.headers, "Retry-After"),
  };
  if (!csrfRetried && path !== sessionPath && isCsrfHashInvalid(result.status, result.body)) {
    const nextCsrf = await refreshSessionCsrf();
    if (nextCsrf) {
      return apiRequest<T>(path, { ...init, csrf: nextCsrf, csrfRetried: true });
    }
  }
  return result;
}

export function errorMessage(body: ApiErrorBody, fallback: string): string {
  const message = body.error?.message;
  if (typeof message === "string" && message !== "") {
    return message;
  }
  return fallback;
}
