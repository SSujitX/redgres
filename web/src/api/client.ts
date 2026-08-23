export type ApiErrorBody = {
  error?: { code?: string; message?: string; fields?: Record<string, string> };
  request_id?: string;
};

export type ApiResult<T> = {
  status: number;
  body: T;
  retryAfter: string | null;
};

function headerGet(headers: Headers, name: string): string | null {
  return headers.get(name);
}

export async function apiRequest<T>(
  path: string,
  init: RequestInit & { csrf?: string } = {},
): Promise<ApiResult<T>> {
  const headers = new Headers(init.headers);
  headers.set("Accept", "application/json");
  if (init.body != null) {
    headers.set("Content-Type", "application/json");
  }
  if (init.csrf) {
    headers.set("X-CSRF-Token", init.csrf);
  }
  const { csrf: _csrf, ...rest } = init;
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
  return {
    status: response.status,
    body,
    retryAfter: headerGet(response.headers, "Retry-After"),
  };
}

export function errorMessage(body: ApiErrorBody, fallback: string): string {
  const message = body.error?.message;
  if (typeof message === "string" && message !== "") {
    return message;
  }
  return fallback;
}
