import { ModeTracker, onModeReset, resetForModeChange } from "./mode";

describe("ModeTracker", () => {
  it("does not report the first answer", () => {
    const t = new ModeTracker();
    expect(t.update({ secret: false })).toBe(false);
    expect(t.update({ secret: false })).toBe(false);
  });
  it("reports each change once", () => {
    const t = new ModeTracker();
    t.update({ secret: false });
    expect(t.update({ secret: true })).toBe(true);
    expect(t.update({ secret: true })).toBe(false);
    expect(t.update({ secret: false })).toBe(true);
  });
});

describe("resetForModeChange", () => {
  it("runs every handler even when one throws", () => {
    const a = jest.fn(() => {
      throw new Error("boom");
    });
    const b = jest.fn();
    const offA = onModeReset(a);
    const offB = onModeReset(b);
    resetForModeChange();
    expect(a).toHaveBeenCalledTimes(1);
    expect(b).toHaveBeenCalledTimes(1);
    offA();
    offB();
    resetForModeChange();
    expect(b).toHaveBeenCalledTimes(1);
  });
});
