import { ApiError, cookieFromSetCookie, post, setCookie } from "./client";

// unlockVault sends the passphrase. On success the server's cookie is kept in
// secure storage too (the Cookie-header fallback, see README) and reset runs:
// the app is now in secret mode, so nothing of normal mode may stay. A wrong
// passphrase throws an ApiError with status 401 and changes nothing.
export async function unlockVault(passphrase: string, reset: () => Promise<void> | void): Promise<void> {
  await post("/api/v1/vault", { passphrase }, (res) => {
    const c = cookieFromSetCookie(res.headers.get("set-cookie"));
    if (c) void setCookie(c);
  });
  await reset();
}

// lockVault forgets the cookie first, so the app is locked even when the
// server cannot be reached, then asks the server to clear its side.
export async function lockVault(reset: () => Promise<void> | void): Promise<void> {
  await setCookie(null);
  try {
    await post("/api/v1/vault/lock");
  } finally {
    await reset();
  }
}

export function unlockErrorMessage(e: unknown): string {
  if (e instanceof ApiError) {
    if (e.status === 401) return "Wrong passphrase.";
    if (e.status === 404) return "The vault is not set up on this server.";
    return e.message;
  }
  return "Could not unlock.";
}
