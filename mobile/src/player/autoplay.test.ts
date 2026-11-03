import type { Track } from "@/api/types";
import { asAutoplay, autoplayPath, historyFrom, pushHistory } from "./autoplay";

const t = (id: string, streamUrl = `/s/${id}`): Track => ({ id, title: id, kind: "music", duration: 10, streamUrl, imageUrl: "", artists: [], showId: "", position: 0, finished: false });

describe("history", () => {
  it("keeps the last 50, newest last, no repeats", () => {
    expect(pushHistory(["a", "b"], "c")).toEqual(["a", "b", "c"]);
    expect(pushHistory(["a", "b", "c"], "a")).toEqual(["b", "c", "a"]);
    let h: string[] = [];
    for (let i = 0; i < 60; i++) h = pushHistory(h, `t${i}`);
    expect(h).toHaveLength(50);
    expect(h[0]).toBe("t10");
    expect(h[49]).toBe("t59");
  });
  it("historyFrom reads a list of strings, else empty", () => {
    expect(historyFrom(["a", "b"])).toEqual(["a", "b"]);
    expect(historyFrom(["a", 3])).toEqual([]);
    expect(historyFrom("a")).toEqual([]);
    expect(historyFrom(undefined)).toEqual([]);
    expect(historyFrom(Array.from({ length: 80 }, (_, i) => `t${i}`))).toHaveLength(50);
  });
});

describe("autoplayPath", () => {
  it("seeds with the last 5 newest first and excludes the history", () => {
    const path = autoplayPath(["a", "b", "c", "d", "e", "f", "g"]);
    expect(path).toBe("/api/v1/autoplay?seed=g&seed=f&seed=e&seed=d&seed=c&exclude=a&exclude=b&exclude=c&exclude=d&exclude=e&exclude=f&exclude=g&n=10");
  });
  it("works with no history", () => {
    expect(autoplayPath([], 5)).toBe("/api/v1/autoplay?n=5");
  });
});

describe("asAutoplay", () => {
  it("marks suggestions, skipping placeholders and what is queued", () => {
    const got = asAutoplay([t("a"), t("b", ""), t("c")], [t("c")]);
    expect(got.map((x) => x.id)).toEqual(["a"]);
    expect(got[0].auto).toBe(true);
  });
});
