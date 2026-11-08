// @vitest-environment node
import { expect, test } from "vitest";
import type { FollowUpCount } from "../generated/server";
import { badgeLine, badgeTotal, badgeWaiting } from "./followUpsBadge";

const count = (over: Partial<FollowUpCount> = {}): FollowUpCount => ({
  project: "gx",
  open: 3,
  bugs: 2,
  ideas: 1,
  other: 0,
  triage: "none",
  left: 0,
  ...over,
});

test("the badge sums the open follow-ups over every project", () => {
  expect(badgeTotal([count({ open: 12 }), count({ project: "webapp", open: 4 })])).toBe(16);
  expect(badgeTotal([])).toBe(0);
});

test("a line without a Triage shows the split and the count, and targets the Epics card", () => {
  const line = badgeLine(count({ open: 15, bugs: 12, ideas: 3 }));
  expect(line).toMatchObject({ text: "12 bugs · 3 ideas", count: 15, live: false, path: "/epics", state: { focus: "follow-ups:gx" } });
  expect(badgeLine(count()).text).toBe("2 bugs · 1 idea");
  expect(badgeLine(count({ open: 5, bugs: 2, ideas: 1, other: 2 })).text).toBe("2 bugs · 1 idea · 2 other");
});

test("a running or waiting Triage reads Triage · N left and opens the Triage page", () => {
  for (const triage of ["running", "waiting"] as const) {
    const line = badgeLine(count({ triage, left: 12 }));
    expect(line).toMatchObject({ text: "Triage · 12 left", live: true, path: "/triage/gx" });
    expect(line.count).toBeUndefined();
  }
});

test("a stopped Triage is not live: the line is the plain count", () => {
  expect(badgeLine(count({ triage: "stopped", left: 4 }))).toMatchObject({ live: false, path: "/epics" });
});

test("the badge is amber for a waiting entry only", () => {
  expect(badgeWaiting([count({ triage: "waiting" }), count()])).toBe(true);
  expect(badgeWaiting([count({ triage: "running" }), count({ triage: "stopped" }), count()])).toBe(false);
});
