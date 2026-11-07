import { describe, expect, it } from "vitest";
import {
  CardKindDesign,
  CardKindFollowUps,
  CardKindImplementation,
  CardKindOneOff,
  SectionDesigning,
  SectionFinished,
  SectionImplementing,
  SectionNeedsYou,
  SectionOnHold,
  type EpicListRow,
  type SessionPlacement,
} from "../generated/server";
import { session } from "../agents/agentsTestKit";
import { buildSidebar } from "./sidebarModel";

const row = (address: string, over: Partial<EpicListRow>): EpicListRow =>
  ({
    address,
    project: "p",
    name: address,
    kind: "loop",
    state: "idle",
    done: 1,
    total: 4,
    queued: false,
    queueable: false,
    blocked_by_epics: [],
    workspace: "Default",
    card_kind: CardKindImplementation,
    section: SectionImplementing,
    status_words: "Idle",
    ...over,
  }) as EpicListRow;

const base = { workspace: "Default" };
const place = (session: string, kind: string, extra: Partial<SessionPlacement> = {}): SessionPlacement => ({ session, kind, ...extra });

describe("buildSidebar", () => {
  it("makes one row per Design and epic, with a joined card as the Design row", () => {
    const rows = [
      row("p:a", { card_kind: CardKindDesign, section: SectionDesigning, addresses: ["p:a", "p:a-impl"] }),
      row("p:b", {}),
      row("p:c", { card_kind: CardKindOneOff, workspace: "" }),
      row("p:d", { card_kind: CardKindFollowUps, workspace: "" }),
    ];
    const s = buildSidebar({ ...base, rows, sessions: [], placements: [] });
    expect(s.rows.map((r) => [r.key, r.kind])).toEqual([
      ["p:a", "design"],
      ["p:b", "implementation"],
    ]);
  });

  it("nests agents under their epic", () => {
    const rows = [row("p:a", { addresses: ["p:a"] }), row("p:b", {})];
    const sessions = [session("s1", "p", { name: "q", queue: true }), session("s2", "p")];
    const placements = [place("s1", "epic", { epic: "p:a" }), place("s2", "epic", { epic: "p:b" })];
    const s = buildSidebar({ ...base, rows, sessions, placements });
    expect(s.rows[0].agents.map((a) => a.id)).toEqual(["s1"]);
    expect(s.rows[1].agents.map((a) => a.id)).toEqual(["s2"]);
  });

  it("maps Needs you, Working and dimmed server words", () => {
    const rows = [
      row("p:a", { section: SectionNeedsYou }),
      row("p:b", { state: "running" }),
      row("p:c", { section: SectionFinished, status_words: "Finished" }),
    ];
    const s = buildSidebar({ ...base, rows, sessions: [], placements: [] });
    expect(s.rows.map((r) => r.status)).toEqual([
      { words: "Needs you", tone: "waiting" },
      { words: "Working", tone: "running" },
      { words: "Finished", tone: "muted" },
    ]);
  });

  it("lists a held row apart, flagged held, without a number", () => {
    const rows = [row("p:a", {}), row("p:h", { section: SectionOnHold, status_words: "Finished" })];
    const s = buildSidebar({ ...base, rows, sessions: [], placements: [] });
    expect(s.rows.map((r) => r.key)).toEqual(["p:a"]);
    expect(s.held).toMatchObject([{ key: "p:h", held: true, status: { words: "Finished" } }]);
    expect(s.held[0].number).toBeUndefined();
  });

  it("shows only the active workspace's rows and chats", () => {
    const rows = [row("p:a", {}), row("p:x", { workspace: "Other ws" })];
    const sessions = [session("c1", "p", { name: "mine" }), session("c2", "p", { name: "theirs" })];
    const placements = [place("c1", "chat", { workspace: "Default" }), place("c2", "chat", { workspace: "Other ws" })];
    const s = buildSidebar({ ...base, rows, sessions, placements });
    expect(s.rows.map((r) => r.key)).toEqual(["c1", "p:a"]);
  });

  it("names chats, gives them no ring and maps their status", () => {
    const sessions = [
      session("c1", "p", { name: "talk", status: { state: "working" } }),
      session("c2", "p", { actionable: { reason: "Pick" } }),
    ];
    const placements = [place("c1", "chat", { workspace: "Default" }), place("c2", "chat", { workspace: "Default" })];
    const s = buildSidebar({ ...base, rows: [], sessions, placements });
    expect(s.rows).toMatchObject([
      { title: "c2", kind: "chat", status: { words: "Needs you", tone: "waiting" } },
      { title: "talk", kind: "chat", status: { words: "Working", tone: "running" } },
    ]);
    expect(s.rows[0].ring).toBeUndefined();
  });

  it("numbers only own rows, 1 to 9", () => {
    const rows = Array.from({ length: 11 }, (_, i) => row(`p:e${i}`, {}));
    rows.push(row("p:h", { section: SectionOnHold }));
    const sessions = [session("s1", "p", { queue: true }), session("o1", "p")];
    const placements = [place("s1", "epic", { epic: "p:e0" }), place("o1", "other")];
    const s = buildSidebar({ ...base, rows, sessions, placements });
    expect(s.rows.map((r) => r.number)).toEqual([1, 2, 3, 4, 5, 6, 7, 8, 9, undefined, undefined]);
    expect(s.rows[0].agents[0].id).toBe("s1");
    expect("number" in s.rows[0].agents[0]).toBe(false);
    expect(s.held[0].number).toBeUndefined();
    expect(s.other.map((a) => a.id)).toEqual(["o1"]);
  });

  describe("sort", () => {
    const keys = (rs: { key: string }[]) => rs.map((r) => r.key);

    it("needs-you puts Needs you first, then names case-insensitively with ties in server order", () => {
      const rows = [row("p:b", { name: "beta" }), row("p:A", { name: "Alpha" }), row("p:n", { name: "zed", section: SectionNeedsYou }), row("p:a2", { name: "alpha" })];
      const sessions = [session("c1", "p", { name: "Aardvark" })];
      const placements = [place("c1", "chat", { workspace: "Default" })];
      const s = buildSidebar({ ...base, rows, sessions, placements });
      expect(keys(s.rows)).toEqual(["p:n", "c1", "p:A", "p:a2", "p:b"]);
      expect(s.rows.map((r) => r.number)).toEqual([1, 2, 3, 4, 5]);
    });

    it("unknown sort reads as needs-you", () => {
      const rows = [row("p:a", { name: "a" }), row("p:n", { name: "z", section: SectionNeedsYou })];
      expect(keys(buildSidebar({ ...base, rows, sessions: [], placements: [], sort: "bogus" }).rows)).toEqual(["p:n", "p:a"]);
    });

    it("name ignores Needs you", () => {
      const rows = [row("p:n", { name: "z", section: SectionNeedsYou }), row("p:a", { name: "a" })];
      expect(keys(buildSidebar({ ...base, rows, sessions: [], placements: [], sort: "name" }).rows)).toEqual(["p:a", "p:n"]);
    });

    it("activity is newest first, no time last by name, and counts a Design's session time", () => {
      const rows = [
        row("p:old", { last_activity_at: "2026-01-01T00:00:00Z" }),
        row("p:none", { name: "none" }),
        row("p:design", { card_kind: CardKindDesign, section: SectionDesigning }),
        row("p:also", { name: "also" }),
      ];
      const sessions = [session("d1", "p", { status_changed_at: "2026-03-01T00:00:00Z" } as never), session("c1", "p", { status_changed_at: "2026-02-01T00:00:00Z" } as never)];
      const placements = [place("d1", "epic", { epic: "p:design" }), place("c1", "chat", { workspace: "Default" })];
      const s = buildSidebar({ ...base, rows, sessions, placements, sort: "activity" });
      expect(keys(s.rows)).toEqual(["p:design", "c1", "p:old", "p:also", "p:none"]);
    });

    it("activity orders a held row by its session when newer than its card", () => {
      const rows = [
        row("p:h1", { section: SectionOnHold, last_activity_at: "2026-01-01T00:00:00Z" }),
        row("p:h2", { section: SectionOnHold, last_activity_at: "2026-02-01T00:00:00Z" }),
      ];
      const sessions = [session("s1", "p", { status_changed_at: "2026-03-01T00:00:00Z" } as never)];
      const placements = [place("s1", "epic", { epic: "p:h1" })];
      const s = buildSidebar({ ...base, rows, sessions, placements, sort: "activity" });
      expect(keys(s.held)).toEqual(["p:h1", "p:h2"]);
    });

    it("sorts nested agents and Other", () => {
      const rows = [row("p:a", {})];
      const sessions = [
        session("s1", "p", { name: "b" }),
        session("s2", "p", { name: "a", actionable: { reason: "Pick" } }),
        session("o1", "p", { name: "y" }),
        session("o2", "p", { name: "x" }),
      ];
      const placements = [place("s1", "epic", { epic: "p:a" }), place("s2", "epic", { epic: "p:a" }), place("o1", "other"), place("o2", "other")];
      const s = buildSidebar({ ...base, rows, sessions, placements, sort: "name" });
      expect(s.rows[0].agents.map((a) => a.id)).toEqual(["s2", "s1"]);
      expect(s.other.map((a) => a.id)).toEqual(["o2", "o1"]);
    });

    it("a Design row opens the Design page, an epic its title link", () => {
      const rows = [row("p:a", { card_kind: CardKindDesign, section: SectionDesigning })];
      expect(buildSidebar({ ...base, rows, sessions: [], placements: [] }).rows[0].href).toBe("#/d/p:a");
    });
  });

  it("lists Other sessions the same contents in every workspace", () => {
    const sessions = [session("o1", "p")];
    const placements = [place("o1", "other")];
    const a = buildSidebar({ ...base, rows: [], sessions, placements });
    const b = buildSidebar({ ...base, workspace: "Elsewhere", rows: [], sessions, placements });
    expect(a.other).toEqual(b.other);
    expect(a.other).toHaveLength(1);
  });
});
