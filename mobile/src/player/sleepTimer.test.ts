import * as S from "./sleepTimer";

const MIN = 60_000;
const T0 = 1_000_000;

describe("sleep timer", () => {
  it("is off by default", () => {
    expect(S.isActive(S.off)).toBe(false);
    expect(S.remaining(S.off, T0)).toBeNull();
    expect(S.shouldStop(S.off, T0)).toBe(false);
    expect(S.badge(S.off, T0)).toBe("");
  });

  it("counts down in wall-clock time", () => {
    const t = S.start(30, T0);
    expect(S.remaining(t, T0)).toBe(30 * MIN);
    expect(S.remaining(t, T0 + 29 * MIN)).toBe(MIN);
    expect(S.minutesLeft(t, T0)).toBe(30);
    expect(S.minutesLeft(t, T0 + 29 * MIN + 1)).toBe(1);
    expect(S.badge(S.start(23, T0), T0)).toBe("23m");
    expect(S.shouldStop(t, T0 + 30 * MIN - 1)).toBe(false);
    expect(S.shouldStop(t, T0 + 30 * MIN)).toBe(true);
    expect(S.remaining(t, T0 + 31 * MIN)).toBe(0);
  });

  it("fades over the last ten seconds", () => {
    const t = S.start(1, T0);
    expect(S.shouldFade(t, T0 + 49_000)).toBe(false);
    expect(S.fadeFactor(t, T0 + 49_000)).toBe(1);
    expect(S.shouldFade(t, T0 + 50_000)).toBe(true);
    expect(S.fadeFactor(t, T0 + 55_000)).toBe(0.5);
    expect(S.shouldFade(t, T0 + 60_000)).toBe(false);
    expect(S.fadeFactor(t, T0 + 60_000)).toBe(0);
  });

  it("extends a running countdown, or starts one", () => {
    expect(S.remaining(S.extend(S.start(10, T0), 15, T0 + MIN), T0 + MIN)).toBe(24 * MIN);
    expect(S.remaining(S.extend(S.off, 15, T0), T0)).toBe(15 * MIN);
    expect(S.remaining(S.extend(S.endOfTrack(), 15, T0), T0)).toBe(15 * MIN);
  });

  it("end of track has no countdown and never stops by time", () => {
    const t = S.endOfTrack();
    expect(S.isActive(t)).toBe(true);
    expect(S.remaining(t, T0)).toBeNull();
    expect(S.shouldStop(t, T0 + 99 * MIN)).toBe(false);
    expect(S.shouldFade(t, T0)).toBe(false);
    expect(S.fadeFactor(t, T0)).toBe(1);
    expect(S.badge(t, T0)).toBe("End");
  });
});
