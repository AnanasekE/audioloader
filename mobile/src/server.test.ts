import { normalizeUrl } from "./server";

describe("normalizeUrl", () => {
  it("adds a scheme and trims slashes", () => {
    expect(normalizeUrl(" audioloader.ts.net:8080/ ")).toBe("http://audioloader.ts.net:8080");
  });
  it("keeps https and a path prefix", () => {
    expect(normalizeUrl("https://a.b/x//")).toBe("https://a.b/x");
  });
  it("rejects garbage", () => {
    expect(normalizeUrl("")).toBe("");
    expect(normalizeUrl("ftp://a.b")).toBe("");
  });
});
