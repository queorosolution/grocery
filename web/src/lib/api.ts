import { env } from "./env";
import { errorCodes, type ErrorCode } from "./errors.gen";

// The ONE place the browser calls our API (docs/08 section 5 rule 1).
// Cookies carry the session; no token is ever visible to JavaScript.

export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: ErrorCode | "NETWORK" | "UNKNOWN",
    message: string,
    readonly fields: Readonly<Record<string, string>> = {},
    readonly requestId?: string,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

export interface RequestOptions {
  method?: "GET" | "POST" | "PUT" | "PATCH" | "DELETE";
  body?: unknown;
  query?: Record<string, string | number | boolean | null | undefined>;
  /** Send an Idempotency-Key header (checkout and payment calls). */
  idempotencyKey?: string;
  signal?: AbortSignal;
}

const REFRESH_PATH = "/auth/refresh";

let authLost: (() => void) | null = null;
/** Called after a refresh fails, so the app can open the sign-in modal. */
export function setAuthLostHandler(handler: (() => void) | null): void {
  authLost = handler;
}

function url(path: string, query: RequestOptions["query"]): string {
  const u = new URL(env.apiBase + path);
  for (const [k, v] of Object.entries(query ?? {})) {
    if (v !== null && v !== undefined) u.searchParams.set(k, String(v));
  }
  return u.toString();
}

async function send(path: string, o: RequestOptions): Promise<Response> {
  const headers: Record<string, string> = { "X-Requested-With": "gp-web", Accept: "application/json" };
  if (o.body !== undefined) headers["Content-Type"] = "application/json";
  if (o.idempotencyKey) headers["Idempotency-Key"] = o.idempotencyKey;
  try {
    return await fetch(url(path, o.query), {
      method: o.method ?? "GET",
      headers,
      body: o.body === undefined ? undefined : JSON.stringify(o.body),
      credentials: "include",
      signal: o.signal,
    });
  } catch (err) {
    if (err instanceof DOMException && err.name === "AbortError") throw err;
    throw new ApiError(0, "NETWORK", "Check your connection and try again.");
  }
}

// Many calls can fail at once; they all wait for the same single refresh.
let refreshing: Promise<boolean> | null = null;
function refreshSession(): Promise<boolean> {
  refreshing ??= send(REFRESH_PATH, { method: "POST" })
    .then((r) => r.ok)
    .catch(() => false)
    .finally(() => {
      refreshing = null;
    });
  return refreshing;
}

async function toError(res: Response): Promise<ApiError> {
  let body: { error?: { code?: string; fields?: Record<string, string>; requestId?: string } } = {};
  try {
    body = await res.json();
  } catch {
    // Not JSON (a proxy page, for example). Fall through to UNKNOWN.
  }
  const code = body.error?.code;
  if (code && code in errorCodes) {
    const known = code as ErrorCode;
    return new ApiError(res.status, known, errorCodes[known].message, body.error?.fields, body.error?.requestId);
  }
  return new ApiError(res.status, "UNKNOWN", "Something went wrong. Please try again.");
}

/** Calls the API and returns the parsed JSON. Throws ApiError. One refresh retry on 401. */
export async function api<T>(path: string, options: RequestOptions = {}): Promise<T> {
  let res = await send(path, options);
  if (res.status === 401 && path !== REFRESH_PATH) {
    if (await refreshSession()) res = await send(path, options);
    if (res.status === 401) authLost?.();
  }
  if (!res.ok) throw await toError(res);
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}
