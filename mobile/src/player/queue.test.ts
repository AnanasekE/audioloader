import type { Track } from "@/api/types";
import * as Q from "./queue";

const t = (id: string): Track => ({ id, title: id, kind: "music", duration: 10, streamUrl: `/s/${id}`, imageUrl: "", artists: [], showId: "", position: 0, finished: false });
const ids = (q: Q.QueueState) => q.items.map((x) => x.id).join("");
const abcd = () => Q.setQueue(["a", "b", "c", "d"].map(t), 1);

describe("setQueue", () => {
  it("starts at the tapped track with the rest after it", () => {
    const q = abcd();
    expect(ids(q)).toBe("abcd");
    expect(Q.current(q)?.id).toBe("b");
  });
  it("keeps tracks once and the index on the same track", () => {
    const q = Q.setQueue(["a", "b", "a", "c"].map(t), 3);
    expect(ids(q)).toBe("abc");
    expect(Q.current(q)?.id).toBe("c");
  });
  it("is empty for no tracks or a bad index", () => {
    expect(Q.setQueue([], 0)).toEqual(Q.emptyQueue);
    expect(Q.setQueue([t("a")], 5)).toEqual(Q.emptyQueue);
  });
});

describe("adding", () => {
  it("playNext goes right after the current track", () => {
    const q = Q.playNext(abcd(), t("x"));
    expect(ids(q)).toBe("abxcd");
    expect(q.index).toBe(1);
  });
  it("addToEnd appends", () => {
    expect(ids(Q.addToEnd(abcd(), t("x")))).toBe("abcdx");
  });
  it("both start a queue from empty", () => {
    expect(Q.playNext(Q.emptyQueue, t("x"))).toEqual({ items: [t("x")], index: 0 });
    expect(Q.addToEnd(Q.emptyQueue, t("x"))).toEqual({ items: [t("x")], index: 0 });
  });
});

describe("removeAt", () => {
  it("before the current track shifts the index", () => {
    const q = Q.removeAt(abcd(), 0);
    expect(ids(q)).toBe("bcd");
    expect(Q.current(q)?.id).toBe("b");
  });
  it("after the current track keeps it", () => {
    const q = Q.removeAt(abcd(), 3);
    expect(Q.current(q)?.id).toBe("b");
  });
  it("the current track moves on to the next, or the last", () => {
    expect(Q.current(Q.removeAt(abcd(), 1))?.id).toBe("c");
    expect(Q.current(Q.removeAt(Q.jumpTo(abcd(), 3), 3))?.id).toBe("c");
  });
  it("the last track empties the queue", () => {
    expect(Q.removeAt(Q.setQueue([t("a")], 0), 0)).toEqual(Q.emptyQueue);
  });
  it("ignores a bad position", () => {
    const q = abcd();
    expect(Q.removeAt(q, 9)).toBe(q);
  });
});

describe("move", () => {
  it("reorders upcoming tracks and keeps the current one", () => {
    const q = Q.move(abcd(), 3, 2);
    expect(ids(q)).toBe("abdc");
    expect(Q.current(q)?.id).toBe("b");
  });
  it("moving across the current track adjusts the index", () => {
    expect(Q.current(Q.move(abcd(), 0, 2))?.id).toBe("b");
    expect(Q.current(Q.move(abcd(), 3, 0))?.id).toBe("b");
    expect(Q.move(abcd(), 3, 0).index).toBe(2);
  });
  it("moving the current track carries the index", () => {
    const q = Q.move(abcd(), 1, 3);
    expect(ids(q)).toBe("acdb");
    expect(Q.current(q)?.id).toBe("b");
  });
});

describe("next and previous", () => {
  it("walk the queue and stop at the ends", () => {
    let q = abcd();
    q = Q.next(q);
    expect(Q.current(q)?.id).toBe("c");
    q = Q.next(Q.next(q));
    expect(Q.current(q)?.id).toBe("d");
    expect(Q.hasNext(q)).toBe(false);
    q = Q.previous(Q.previous(Q.previous(Q.previous(q))));
    expect(Q.current(q)?.id).toBe("a");
    expect(Q.hasPrevious(q)).toBe(false);
  });
});
