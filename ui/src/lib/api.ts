/** Error returned by the Restora API (see docs/CONVENTIONS.md for the format). */
export class ApiError extends Error {
  readonly code: string;
  readonly hint?: string;
  readonly status?: number;

  constructor(code: string, message: string, options: { hint?: string; status?: number } = {}) {
    super(message);
    this.name = "ApiError";
    this.code = code;
    this.hint = options.hint;
    this.status = options.status;
  }
}

interface ErrorBody {
  error?: { code?: string; message?: string; hint?: string };
}

type Method = "GET" | "POST" | "PUT" | "DELETE";

async function request<T>(method: Method, path: string, body?: unknown, signal?: AbortSignal): Promise<T> {
  const headers: Record<string, string> = { Accept: "application/json" };
  if (body !== undefined) {
    headers["Content-Type"] = "application/json";
  }
  let response: Response;
  try {
    response = await fetch(path, {
      method,
      headers,
      signal,
      body: body === undefined ? undefined : JSON.stringify(body),
    });
  } catch (cause) {
    if (cause instanceof DOMException && cause.name === "AbortError") {
      throw cause;
    }
    throw new ApiError("network_error", cause instanceof Error ? cause.message : "Network request failed");
  }

  if (!response.ok) {
    const failure = (await response.json().catch(() => ({}))) as ErrorBody;
    throw new ApiError(failure.error?.code ?? "http_error", failure.error?.message ?? `HTTP ${response.status}`, {
      hint: failure.error?.hint,
      status: response.status,
    });
  }
  if (response.status === 204) {
    return undefined as T;
  }
  return (await response.json()) as T;
}

export function apiGet<T>(path: string, signal?: AbortSignal): Promise<T> {
  return request<T>("GET", path, undefined, signal);
}

/** Sends a POST, with a JSON body when one is given. Without a body it is an action such as "back up now". */
export function apiPost<T>(path: string, body?: unknown, signal?: AbortSignal): Promise<T> {
  return request<T>("POST", path, body, signal);
}

export function apiPut<T>(path: string, body: unknown): Promise<T> {
  return request<T>("PUT", path, body);
}

export function apiDelete(path: string): Promise<void> {
  return request<void>("DELETE", path);
}
