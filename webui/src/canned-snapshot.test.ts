import { describe, expect, it } from "vitest";
import canned from "./canned-snapshot.json";
import type { Snapshot } from "./snapshot";

const snap = canned as Snapshot;

// tickets.RenderedStatus.Word values.
const STATUS_WORDS = [
  "open",
  "claimed",
  "blocked",
  "needs-answer",
  "needs-repair",
  "done",
  "cancelled",
  "draft",
  "waiting-for-children",
  "error",
];

// server.RecoveryPending and RecoveryEscalated.
const RECOVERY_WORDS = ["pending", "escalated"];

describe("canned snapshot", () => {
  it("addresses epics as project:epic", () => {
    const finishedEpics = snap.recently_finished.filter((f) => !f.one_off);
    for (const e of [...snap.epics, ...finishedEpics]) {
      expect(e.address).toBe(`${e.project}:${e.name}`);
    }
  });

  it("addresses finished one-offs as project:epic/id", () => {
    for (const f of snap.recently_finished.filter((f) => f.one_off)) {
      expect(f.address).toMatch(new RegExp(`^${f.project}:[^/]+/[^/]+$`));
    }
  });

  it("addresses epic tickets as project:epic/id", () => {
    for (const e of snap.epics) {
      for (const t of [...e.active, ...e.parked]) {
        expect(t.address).toMatch(new RegExp(`^${e.address}/[^/]+$`));
      }
    }
  });

  it("uses the real status and recovery words", () => {
    const epicTickets = snap.epics.flatMap((e) => [...e.active, ...e.parked]);
    for (const t of [...snap.tickets, ...epicTickets]) {
      expect(STATUS_WORDS).toContain(t.status);
    }
    for (const t of snap.tickets) expect(STATUS_WORDS).toContain(t.rendered_status);
    for (const t of epicTickets.filter((t) => t.recovery)) {
      expect(RECOVERY_WORDS).toContain(t.recovery);
    }
  });

  it("parks tickets in a parked status", () => {
    for (const e of snap.epics) {
      for (const t of e.parked) expect(["needs-answer", "needs-repair"]).toContain(t.status);
    }
  });
});
