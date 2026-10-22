import { describe, expect, it } from "vitest";
import canned from "./canned-snapshot.json";
import type { QueueMode, Snapshot } from "./snapshot";
import { REPAIR_HINT, snapshotView } from "./view";

const now = new Date("2026-10-19T12:00:00Z");
const snap = canned as Snapshot;
const view = (mode?: QueueMode) => snapshotView({ ...snap, mode }, now);
const byName = (name: string, mode?: QueueMode) => view(mode).epics.find((e) => e.name === name)!;

describe("snapshotView", () => {
  it("orders epics by state and keeps server order within a state", () => {
    expect(view().epics.map((e) => e.name)).toEqual(["park", "dead", "run", "run2", "wait", "await"]);
  });

  it("maps state to tone and badge", () => {
    expect(byName("park")).toMatchObject({ tone: "parked", badge: "Parked" });
    expect(byName("dead")).toMatchObject({ tone: "deadlocked", badge: "Deadlocked", reason: "blocked cycle" });
    expect(byName("run")).toMatchObject({ tone: "running", badge: "Running" });
    expect(byName("wait")).toMatchObject({ tone: "waiting", badge: "Waiting for a slot" });
    expect(byName("await")).toMatchObject({ tone: "muted", badge: "Awaiting merge" });
  });

  it("labels Waiting by queue mode and leaves Running unchanged", () => {
    expect(byName("wait", "paused").badge).toBe("Waiting: queue paused");
    expect(byName("wait", "draining").badge).toBe("Waiting: queue draining");
    expect(byName("wait", "running").badge).toBe("Waiting for a slot");
    expect(byName("run", "paused").badge).toBe("Running");
  });

  it("lists active tickets before parked ones, and only Parked for Waiting", () => {
    expect(byName("run").tickets.map((t) => [t.address, t.id, t.tone])).toEqual([
      ["gx:run/03", "03", "muted"],
      ["gx:run/04", "04", "parked"],
    ]);
    expect(byName("wait").tickets.map((t) => [t.address, t.tone])).toEqual([
      ["gx:wait/03", "parked"],
      ["gx:wait/04", "waiting"],
    ]);
  });

  it("marks only a running epic and its working tickets as live", () => {
    expect(byName("run").live).toBe(true);
    expect(byName("wait").live).toBe(false);
    expect(byName("run").tickets.map((t) => [t.address, t.live ?? false])).toEqual([
      ["gx:run/03", true],
      ["gx:run/04", false],
    ]);
  });

  it("gives an answerable row its summary and a ticket route", () => {
    expect(byName("wait").tickets[0]).toMatchObject({
      detail: "Which option do you want?",
      href: "#/t/gx:wait/03",
    });
  });

  it("gives a repair row its reason and a hint, with no link", () => {
    const row = byName("run").tickets[1];
    expect(row).toMatchObject({ detail: "worktree has uncommitted changes", hint: REPAIR_HINT });
    expect(row.href).toBeUndefined();
  });

  it("shows an answered row as amber with its queue position", () => {
    expect(byName("wait").tickets[1]).toMatchObject({ tone: "waiting", state: "Answered · #2 in queue" });
    expect(byName("wait").tickets[1].href).toBeUndefined();
  });

  it("formats recently finished rows", () => {
    expect(view().recentlyFinished).toEqual([
      { address: "gx:old", name: "old", project: "gx", done: 4, total: 4, oneOff: false, finished: "finished 2h ago" },
      {
        address: "gx:one-offs/07",
        name: "fix-flaky-test",
        project: "gx",
        done: 0,
        total: 0,
        oneOff: true,
        finished: "finished 1h ago",
      },
    ]);
  });
});
