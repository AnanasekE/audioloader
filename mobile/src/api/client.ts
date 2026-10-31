import { getItem, removeItem, setItem } from "../storage";

// HTTP client for the JSON API. Auth is the vault cookie (plan 21).
//
// React Native's fetch keeps cookies natively, so every request uses
// credentials: "include". As a fallback that works either way, the cookie
// from the Set-Cookie header of POST /api/v1/vault is also kept in secure
// storage and sent as a Cookie header (see mobile/README.md).

const COOKIE_KEY = "vaultCookie";

export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }
  get notFound() {
    return this.status === 404;
  }
}

let baseUrl = "";
let cookie: string | null = null;

// configure sets the server URL (no trailing slash needed).
export function configure(url: string) {
  baseUrl = url.replace(/\/+$/, "");
}

export function getBaseUrl() {
  return baseUrl;
}

// joinUrl joins the base URL and a path that starts with "/" (or not).
export function joinUrl(base: string, path: string): string {
  if (/^https?:\/\//i.test(path)) return path;
  return base.replace(/\/+$/, "") + "/" + path.replace(/^\/+/, "");
}

export function absoluteUrl(path: string): string {
  return joinUrl(baseUrl, path);
}

// cookieFromSetCookie returns the "name=value" part of a Set-Cookie header,
// or null when it is empty or clears the cookie.
export function cookieFromSetCookie(header: string | null | undefined): string | null {
  if (!header) return null;
  // several cookies may be joined with ", "; the vault cookie is the first
  const first = header.split(/,(?=\s*[^;,=\s]+=)/)[0];
  const pair = first.split(";")[0].trim();
  const eq = pair.indexOf("=");
  if (eq <= 0) return null;
  if (pair.slice(eq + 1) === "") return null;
  if (/;\s*max-age=0/i.test(first) || /;\s*max-age=-/i.test(first)) return null;
  return pair;
}

export async function loadCookie() {
  try {
    cookie = (await getItem(COOKIE_KEY)) || null;
  } catch {
    cookie = null;
  }
}

export async function setCookie(value: string | null) {
  cookie = value;
  try {
    if (value) await setItem(COOKIE_KEY, value);
    else await removeItem(COOKIE_KEY);
  } catch {}
}

export function getCookie() {
  return cookie;
}

// authHeaders are for requests the player and image views make themselves
// (stream and cover URLs), which do not go through get/post.
export function authHeaders(): Record<string, string> {
  return cookie ? { Cookie: cookie } : {};
}

async function errorFrom(res: Response): Promise<ApiError> {
  let msg = res.statusText || `HTTP ${res.status}`;
  try {
    const body = await res.json();
    if (body && typeof body.error === "string") msg = body.error;
  } catch {}
  return new ApiError(res.status, msg);
}

async function request(method: string, path: string, body?: unknown): Promise<Response> {
  const headers: Record<string, string> = { Accept: "application/json", ...authHeaders() };
  const init: RequestInit = { method, headers, credentials: "include" };
  if (body !== undefined) {
    headers["Content-Type"] = "application/json";
    init.body = JSON.stringify(body);
  }
  let res: Response;
  try {
    res = await fetch(absoluteUrl(path), init);
  } catch {
    throw new ApiError(0, "Could not reach the server");
  }
  if (!res.ok) throw await errorFrom(res);
  return res;
}

export type Params = Record<string, string | number | undefined>;

export function withParams(path: string, params?: Params): string {
  if (!params) return path;
  const q = Object.entries(params)
    .filter(([, v]) => v !== undefined && v !== "")
    .map(([k, v]) => `${encodeURIComponent(k)}=${encodeURIComponent(String(v))}`)
    .join("&");
  return q ? `${path}${path.includes("?") ? "&" : "?"}${q}` : path;
}

export async function get<T>(path: string, params?: Params): Promise<T> {
  const res = await request("GET", withParams(path, params));
  return (await res.json()) as T;
}

// post sends a JSON body. It returns the parsed body, or undefined for 204.
// The Response is passed to onResponse first, for the Set-Cookie header.
export async function post<T = void>(path: string, body?: unknown, onResponse?: (res: Response) => void): Promise<T> {
  const res = await request("POST", path, body);
  onResponse?.(res);
  if (res.status === 204) return undefined as T;
  const text = await res.text();
  return (text ? JSON.parse(text) : undefined) as T;
}
