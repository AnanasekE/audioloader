import { configure, getCookie, setCookie } from "./client";
import { lockVault, unlockErrorMessage, unlockVault } from "./vault";

jest.mock("../storage", () => ({
  getItem: jest.fn(async () => null),
  setItem: jest.fn(async () => {}),
  removeItem: jest.fn(async () => {}),
}));

import { onModeReset, resetForModeChange } from "../mode";

function res(status: number, body?: unknown, headers: Record<string, string> = {}) {
  return {
    ok: status >= 200 && status < 300,
    status,
    statusText: "",
    headers: { get: (k: string) => headers[k.toLowerCase()] ?? null },
    json: async () => body,
    text: async () => (body === undefined ? "" : JSON.stringify(body)),
  } as unknown as Response;
}

const fetchMock = jest.fn();
beforeEach(async () => {
  fetchMock.mockReset();
  (globalThis as any).fetch = fetchMock;
  configure("http://srv");
  await setCookie(null);
});

describe("unlockVault", () => {
  it("keeps the cookie and resets", async () => {
    fetchMock.mockResolvedValue(res(204, undefined, { "set-cookie": "vault=tok; Path=/; HttpOnly" }));
    const reset = jest.fn();
    await unlockVault("pw", reset);
    expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({ passphrase: "pw" });
    expect(getCookie()).toBe("vault=tok");
    expect(reset).toHaveBeenCalledTimes(1);
  });

  it("a wrong passphrase changes nothing", async () => {
    fetchMock.mockResolvedValue(res(401, { error: "wrong passphrase" }));
    const reset = jest.fn();
    const err = await unlockVault("bad", reset).catch((e: unknown) => e);
    expect(unlockErrorMessage(err)).toBe("Wrong passphrase.");
    expect(getCookie()).toBeNull();
    expect(reset).not.toHaveBeenCalled();
  });
});

describe("lockVault", () => {
  it("clears the cookie and resets", async () => {
    await setCookie("vault=tok");
    fetchMock.mockResolvedValue(res(204));
    const reset = jest.fn();
    await lockVault(reset);
    expect(getCookie()).toBeNull();
    expect(fetchMock.mock.calls[0][0]).toBe("http://srv/api/v1/vault/lock");
    expect(reset).toHaveBeenCalledTimes(1);
  });

  it("locks locally even when the server is unreachable", async () => {
    await setCookie("vault=tok");
    fetchMock.mockRejectedValue(new TypeError("offline"));
    const reset = jest.fn();
    await expect(lockVault(reset)).rejects.toBeTruthy();
    expect(getCookie()).toBeNull();
    expect(reset).toHaveBeenCalledTimes(1);
  });
});

describe("reset on mode change", () => {
  it("runs the registered player and cache handlers", () => {
    const stopPlayer = jest.fn();
    const clearQueue = jest.fn();
    const off = [onModeReset(stopPlayer), onModeReset(clearQueue)];
    resetForModeChange();
    expect(stopPlayer).toHaveBeenCalledTimes(1);
    expect(clearQueue).toHaveBeenCalledTimes(1);
    off.forEach((f) => f());
  });
});
