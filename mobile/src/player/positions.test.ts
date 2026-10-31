import { bodyToSave, dueForSave, isEpisode, startPosition } from "./positions";

describe("bodyToSave", () => {
  it("saves the position mid-episode", () => {
    expect(bodyToSave(600, 3600)).toEqual({ seconds: 600, finished: false });
  });
  it("counts the last 30 s as finished and resets to 0", () => {
    expect(bodyToSave(3580, 3600)).toEqual({ seconds: 0, finished: true });
    expect(bodyToSave(3570, 3600)).toEqual({ seconds: 3570, finished: false });
  });
  it("finishes when the track ended", () => {
    expect(bodyToSave(10, 3600, true)).toEqual({ seconds: 0, finished: true });
  });
  it("does not finish when the duration is not known yet", () => {
    expect(bodyToSave(5, 0)).toEqual({ seconds: 5, finished: false });
  });
});

describe("startPosition", () => {
  it("resumes at the saved position", () => {
    expect(startPosition({ position: 120, finished: false })).toBe(120);
  });
  it("starts over when finished or never started", () => {
    expect(startPosition({ position: 120, finished: true })).toBe(0);
    expect(startPosition({ position: 0, finished: false })).toBe(0);
    expect(startPosition({ position: 0, finished: false })).toBe(0);
  });
  it("prefers a position saved this session", () => {
    expect(startPosition({ position: 120, finished: true }, 300)).toBe(300);
    expect(startPosition({ position: 120, finished: false }, 0)).toBe(0);
  });
});

describe("isEpisode", () => {
  it("is true only for kind episode", () => {
    expect(isEpisode({ kind: "episode" })).toBe(true);
    expect(isEpisode({ kind: "music" })).toBe(false);
  });
});

describe("dueForSave", () => {
  it("is true after 15 s", () => {
    expect(dueForSave(0, 14_999)).toBe(false);
    expect(dueForSave(0, 15_000)).toBe(true);
  });
});
