import { describe, expect, it } from "vitest";
import { exactTime, isPastReset, isStale, limitTone, resetsIn, seenAgo, sinceLabel, wholePercent } from "./format";

const MIN = 60;
const HOUR = 60 * MIN;
const DAY = 24 * HOUR;
const NOW_SEC = 1_800_000_000;
const NOW = NOW_SEC * 1000;

describe("wholePercent and limitTone", () => {
  it.each([
    [79, 79, "running"],
    [79.4, 79, "running"],
    [79.5, 80, "waiting"],
    [80, 80, "waiting"],
    [94, 94, "waiting"],
    [94.5, 95, "deadlocked"],
    [95, 95, "deadlocked"],
  ] as const)("%s shows %s and is %s", (p, whole, tone) => {
    expect(wholePercent(p)).toBe(whole);
    expect(limitTone(wholePercent(p))).toBe(tone);
  });
});

describe("isStale", () => {
  it.each([
    [599, false],
    [600, false],
    [601, true],
  ])("%ss old is stale=%s", (age, stale) => {
    expect(isStale(NOW_SEC - age, NOW)).toBe(stale);
  });
});

describe("isPastReset", () => {
  it.each([
    [NOW_SEC + 1, false],
    [NOW_SEC, true],
    [NOW_SEC - 1, true],
  ])("reset at %s is past=%s", (at, past) => {
    expect(isPastReset(at, NOW)).toBe(past);
  });
});

describe("resetsIn", () => {
  it.each([
    [2 * HOUR + 10 * MIN, "resets in 2h 10m"],
    [2 * DAY + 4 * HOUR, "resets in 2d 4h"],
    [45 * MIN, "resets in 45m"],
    [30, "resets in <1m"],
  ])("%ss ahead is %s", (ahead, text) => {
    expect(resetsIn(NOW_SEC + ahead, NOW)).toBe(text);
  });
});

describe("seenAgo", () => {
  it.each([
    [3 * HOUR, "seen 3h ago"],
    [6 * HOUR, "seen 6h ago"],
  ])("%ss ago is %s", (ago, text) => {
    expect(seenAgo(NOW_SEC - ago, NOW)).toBe(text);
  });
});

describe("exactTime", () => {
  it("matches the Intl call in the browser zone", () => {
    const expected = new Intl.DateTimeFormat("en-US", {
      weekday: "short",
      month: "short",
      day: "numeric",
      hour: "2-digit",
      minute: "2-digit",
      hour12: false,
    }).format(new Date(NOW_SEC * 1000));
    expect(exactTime(NOW_SEC)).toBe(expected);
  });
});

describe("sinceLabel", () => {
  it("names the weekday of the key", () => {
    expect(sinceLabel("2026-10-30")).toBe("since Fri, Oct 30");
  });

  it("does not shift the day in a western zone", () => {
    const old = process.env.TZ;
    process.env.TZ = "America/Los_Angeles";
    try {
      expect(sinceLabel("2026-10-30")).toBe("since Fri, Oct 30");
    } finally {
      if (old === undefined) delete process.env.TZ;
      else process.env.TZ = old;
    }
  });
});
