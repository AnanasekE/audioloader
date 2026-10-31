import { ApiError, configure, cookieFromSetCookie, get, joinUrl, post, setCookie, withParams } from "./client";

jest.mock("../storage", () => ({
  getItem: jest.fn(async () => null),
  setItem: jest.fn(async () => {}),
  removeItem: jest.fn(async () => {}),
}));

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
beforeEach(() => {
  fetchMock.mockReset();
  (globalThis as any).fetch = fetchMock;
  configure("http://srv:8080/");
});
afterEach(() => setCookie(null));

describe("joinUrl", () => {
  it("joins with exactly one slash", () => {
    expect(joinUrl("http://a/", "/api/v1/me")).toBe("http://a/api/v1/me");
    expect(joinUrl("http://a", "api/v1/me")).toBe("http://a/api/v1/me");
  });
  it("keeps absolute URLs", () => {
    expect(joinUrl("http://a", "https://b/x")).toBe("https://b/x");
  });
});

describe("withParams", () => {
  it("encodes and skips empty values", () => {
    expect(withParams("/x", { q: "a b", limit: 5, offset: undefined, s: "" })).toBe("/x?q=a%20b&limit=5");
    expect(withParams("/x")).toBe("/x");
  });
});

describe("get", () => {
  it("requests the joined URL with credentials", async () => {
    fetchMock.mockResolvedValue(res(200, { secret: false }));
    await expect(get("/api/v1/me")).resolves.toEqual({ secret: false });
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toBe("http://srv:8080/api/v1/me");
    expect(init.credentials).toBe("include");
    expect(init.headers.Cookie).toBeUndefined();
  });

  it("parses the error body", async () => {
    fetchMock.mockResolvedValue(res(401, { error: "wrong passphrase" }));
    await expect(get("/x")).rejects.toMatchObject({ status: 401, message: "wrong passphrase" });
  });

  it("falls back to the status text for non-JSON errors", async () => {
    const r = res(500);
    (r as any).json = async () => {
      throw new Error("no json");
    };
    (r as any).statusText = "Internal Server Error";
    fetchMock.mockResolvedValue(r);
    const err = await get("/x").catch((e: unknown) => e) as any;
    expect(err).toBeInstanceOf(ApiError);
    expect(err.message).toBe("Internal Server Error");
  });

  it("flags a 404", async () => {
    fetchMock.mockResolvedValue(res(404, { error: "x" }));
    const err = await get("/x").catch((e: unknown) => e) as any;
    expect(err.notFound).toBe(true);
  });

  it("turns a network failure into an ApiError", async () => {
    fetchMock.mockRejectedValue(new TypeError("Network request failed"));
    await expect(get("/x")).rejects.toMatchObject({ status: 0 });
  });

  it("sends the stored cookie as a Cookie header", async () => {
    await setCookie("vault=abc");
    fetchMock.mockResolvedValue(res(200, {}));
    await get("/x");
    expect(fetchMock.mock.calls[0][1].headers.Cookie).toBe("vault=abc");
  });
});

describe("post", () => {
  it("sends JSON and returns undefined for 204", async () => {
    fetchMock.mockResolvedValue(res(204));
    await expect(post("/api/v1/vault", { passphrase: "p" })).resolves.toBeUndefined();
    const [, init] = fetchMock.mock.calls[0];
    expect(init.method).toBe("POST");
    expect(init.body).toBe('{"passphrase":"p"}');
    expect(init.headers["Content-Type"]).toBe("application/json");
  });

  it("hands the response to onResponse", async () => {
    fetchMock.mockResolvedValue(res(204, undefined, { "set-cookie": "vault=tok; Path=/" }));
    let seen: string | null = null;
    await post("/api/v1/vault", {}, (r) => (seen = r.headers.get("set-cookie")));
    expect(seen).toBe("vault=tok; Path=/");
  });
});

describe("cookieFromSetCookie", () => {
  it("takes name=value", () => {
    expect(cookieFromSetCookie("vault=tok; Path=/; HttpOnly; SameSite=Lax")).toBe("vault=tok");
  });
  it("ignores clearing and empty cookies", () => {
    expect(cookieFromSetCookie("vault=; Path=/; Max-Age=0")).toBeNull();
    expect(cookieFromSetCookie("vault=gone; Path=/; Max-Age=0")).toBeNull();
    expect(cookieFromSetCookie(null)).toBeNull();
  });
  it("uses the first of several joined cookies", () => {
    expect(cookieFromSetCookie("a=1; Path=/, b=2; Path=/")).toBe("a=1");
  });
});
