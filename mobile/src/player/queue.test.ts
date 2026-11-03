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
  it("Play next after Play next keeps the order added", () => {
    const q1 = Q.playNext(abcd(), t("x"), 0);
    const q2 = Q.playNext(q1, t("y"), 1);
    expect(ids(q2)).toBe("abxycd");
    expect(ids(Q.playNext(q2, [t("p"), t("q")], 2))).toBe("abxypqcd");
  });
  it("a batch goes in as one block, and nextAt never passes the end", () => {
    expect(ids(Q.playNext(abcd(), [t("x"), t("y")]))).toBe("abxycd");
    expect(ids(Q.playNext(abcd(), t("x"), 9))).toBe("abcdx");
    expect(Q.playNextSlot(abcd(), 1)).toBe(3);
  });
  it("addToEnd takes a batch and an empty batch changes nothing", () => {
    expect(ids(Q.addToEnd(abcd(), [t("x"), t("y")]))).toBe("abcdxy");
    const q = abcd();
    expect(Q.addToEnd(q, [])).toBe(q);
    expect(Q.playNext(q, [])).toEqual(q);
    expect(Q.addToEnd(Q.emptyQueue, [])).toBe(Q.emptyQueue);
  });
  it("addToEnd appends", () => {
    expect(ids(Q.addToEnd(abcd(), t("x")))).toBe("abcdx");
  });
  it("both start a queue from empty, a batch too", () => {
    expect(Q.playNext(Q.emptyQueue, [t("x"), t("y")])).toEqual({ items: [t("x"), t("y")], index: 0 });
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

describe("repeat", () => {
  it("cycles off, all, one", () => {
    expect(Q.nextRepeat("off")).toBe("all");
    expect(Q.nextRepeat("all")).toBe("one");
    expect(Q.nextRepeat("one")).toBe("off");
  });
  it("repeatFrom keeps a valid mode, else off", () => {
    expect(Q.repeatFrom("all")).toBe("all");
    expect(Q.repeatFrom("one")).toBe("one");
    expect(Q.repeatFrom("loop")).toBe("off");
    expect(Q.repeatFrom(undefined)).toBe("off");
  });
  it("only all wraps at the end", () => {
    const last = Q.setQueue(["a", "b", "c"].map(t), 2);
    expect(Q.nextIndex(last, "off")).toBe(-1);
    expect(Q.nextIndex(last, "one")).toBe(-1);
    expect(Q.nextIndex(last, "all")).toBe(0);
    expect(Q.hasNext(last)).toBe(false);
    expect(Q.hasNext(last, "all")).toBe(true);
    expect(Q.current(Q.next(last, "all"))?.id).toBe("a");
    expect(Q.next(last)).toBe(last);
  });
  it("moves on normally in the middle, and an empty queue has no next", () => {
    expect(Q.nextIndex(abcd(), "all")).toBe(2);
    expect(Q.nextIndex(Q.emptyQueue, "all")).toBe(-1);
  });
  it("loopMode maps to expo-audio, and the sleep timer beats repeat one", () => {
    expect(Q.loopMode("off", false)).toBe("none");
    expect(Q.loopMode("all", false)).toBe("all");
    expect(Q.loopMode("one", false)).toBe("single");
    expect(Q.loopMode("one", true)).toBe("none");
    expect(Q.loopMode("all", true)).toBe("all");
  });
});

describe("autoplay tracks stay last", () => {
  const withAuto = () => Q.appendAuto(abcd(), [{ ...t("x"), auto: true }, { ...t("y"), auto: true }]);
  it("autoStart finds the first autoplay track after the current one", () => {
    expect(Q.autoStart(withAuto())).toBe(4);
    expect(Q.autoStart(abcd())).toBe(4);
    expect(Q.autoStart({ ...withAuto(), index: 4 })).toBe(5);
  });
  it("addToEnd and playNext go before them", () => {
    expect(ids(Q.addToEnd(withAuto(), t("p")))).toBe("abcdpxy");
    expect(ids(Q.playNext(withAuto(), t("p"), 9))).toBe("abcdpxy");
    expect(ids(Q.playNext(withAuto(), t("p")))).toBe("abpcdxy");
  });
  it("with only autoplay tracks left, additions still go first", () => {
    const q = Q.appendAuto(Q.setQueue([t("a")], 0), [{ ...t("x"), auto: true }]);
    expect(ids(Q.addToEnd(q, t("p")))).toBe("apx");
  });
  it("appendAuto adds at the very end and ignores an empty list", () => {
    expect(ids(withAuto())).toBe("abcdxy");
    const q = abcd();
    expect(Q.appendAuto(q, [])).toBe(q);
  });
});
