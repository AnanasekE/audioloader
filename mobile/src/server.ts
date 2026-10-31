import { getItem, removeItem, setItem } from "./storage";

const KEY = "serverUrl";

// normalizeUrl trims the input, adds http:// when no scheme is given and
// drops trailing slashes. It returns "" for input that is not a URL.
export function normalizeUrl(input: string): string {
  let s = input.trim();
  if (!s) return "";
  if (!/^[a-z][a-z0-9+.-]*:\/\//i.test(s)) s = "http://" + s;
  try {
    const u = new URL(s);
    if (u.protocol !== "http:" && u.protocol !== "https:") return "";
    return (u.origin + u.pathname).replace(/\/+$/, "");
  } catch {
    return "";
  }
}

export const loadServerUrl = () => getItem(KEY);
export const saveServerUrl = (url: string) => setItem(KEY, url);
export const clearServerUrl = () => removeItem(KEY);

// checkServer asks GET /api/v1/me and throws an Error with a readable message
// when the server cannot be reached or does not look like audioloader.
export async function checkServer(url: string): Promise<void> {
  let res: Response;
  try {
    res = await fetch(url + "/api/v1/me", { credentials: "include" });
  } catch {
    throw new Error("Could not reach the server. Check the address and that Tailscale is on.");
  }
  if (!res.ok) throw new Error(`The server answered ${res.status}. Is this an audioloader with the JSON API?`);
  let body: unknown;
  try {
    body = await res.json();
  } catch {
    throw new Error("The server did not answer with JSON. Is this an audioloader?");
  }
  if (typeof body !== "object" || body === null || !("secret" in body)) {
    throw new Error("Unexpected answer from /api/v1/me. Is this an audioloader?");
  }
}
