import { describe, expect, it } from "vitest"
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
  type SessionInfo,
  type SessionPlacement,
} from "../generated/server"
import { session } from "../agents/agentsTestKit"
import { buildSidebar } from "./sidebarModel"

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
  }) as EpicListRow

const base = { workspace: "Default" }
const place = (
  session: string,
  kind: string,
  extra: Partial<SessionPlacement> = {},
): SessionPlacement => ({ session, kind, ...extra })

describe("buildSidebar", () => {
  it("a Design row reads Working while its own session works", () => {
    const rows = [row("p:d", { card_kind: CardKindDesign, section: SectionDesigning })]
    const working = session("d1", "p", { status: { state: "working" } } as never)
    const placements = [place("d1", "epic", { epic: "p:d" })]
    const s = buildSidebar({ ...base, rows, sessions: [working], placements })
    expect(s.rows[0].status).toEqual({ words: "Working", tone: "running" })
    const idle = session("d1", "p", { status: { state: "idle" } } as never)
    const s2 = buildSidebar({ ...base, rows, sessions: [idle], placements })
    expect(s2.rows[0].status.tone).not.toBe("running")
  })

  it("makes one row per Design and epic, with a joined card as the Design row", () => {
    const rows = [
      row("p:a", {
        card_kind: CardKindDesign,
        section: SectionDesigning,
        addresses: ["p:a", "p:a-impl"],
      }),
      row("p:b", {}),
      row("p:c", { card_kind: CardKindOneOff, workspace: "" }),
      row("p:d", { card_kind: CardKindFollowUps, workspace: "" }),
    ]
    const s = buildSidebar({ ...base, rows, sessions: [], placements: [] })
    expect(s.rows.map((r) => [r.key, r.kind])).toEqual([
      ["p:a", "design"],
      ["p:b", "implementation"],
    ])
  })

  it("nests agents under their epic", () => {
    const rows = [row("p:a", { addresses: ["p:a"] }), row("p:b", {})]
    const sessions = [session("s1", "p", { name: "q", queue: true }), session("s2", "p")]
    const placements = [place("s1", "epic", { epic: "p:a" }), place("s2", "epic", { epic: "p:b" })]
    const s = buildSidebar({ ...base, rows, sessions, placements })
    expect(s.rows[0].agents.map((a) => a.id)).toEqual(["s1"])
    expect(s.rows[1].agents.map((a) => a.id)).toEqual(["s2"])
  })

  it("flags a Fix agent only; a Queue agent and a chat are not", () => {
    const rows = [row("p:a", { addresses: ["p:a"] })]
    const sessions = [
      session("s1", "p", { name: "fixer", fix_of: "p:a/03" }),
      session("s2", "p", { name: "q", queue: true }),
      session("s3", "p"),
    ]
    const placements = [
      place("s1", "epic", { epic: "p:a" }),
      place("s2", "epic", { epic: "p:a" }),
      place("s3", "other"),
    ]
    const s = buildSidebar({ ...base, rows, sessions, placements })
    expect(Object.fromEntries(s.rows[0].agents.map((a) => [a.id, a.fix]))).toEqual({
      s1: true,
      s2: false,
    })
    expect(s.other[0].fix).toBe(false)
  })

  it("carries each agent's session, nested and Other, a Fix agent too", () => {
    const rows = [row("p:a", { addresses: ["p:a"] })]
    const sessions = [
      session("s1", "p", { name: "fixer", fix_of: "p:a/03" }),
      session("s2", "p", { name: "q", queue: true }),
      session("s3", "p"),
    ]
    const placements = [
      place("s1", "epic", { epic: "p:a" }),
      place("s2", "epic", { epic: "p:a" }),
      place("s3", "other"),
    ]
    const s = buildSidebar({ ...base, rows, sessions, placements })
    expect(s.rows[0].agents.map((a) => a.session)).toEqual([sessions[0], sessions[1]])
    expect(s.other[0].session).toBe(sessions[2])
  })

  it("maps Needs you, Working and dimmed server words", () => {
    const rows = [
      row("p:a", { section: SectionNeedsYou }),
      row("p:b", { state: "running" }),
      row("p:c", { section: SectionFinished, status_words: "Finished" }),
    ]
    const s = buildSidebar({ ...base, rows, sessions: [], placements: [] })
    expect(s.rows.map((r) => r.status)).toEqual([
      { words: "Needs you", tone: "waiting" },
      { words: "Working", tone: "running" },
      { words: "Finished", tone: "muted" },
    ])
  })

  it("lists a held row apart, flagged held, without a number", () => {
    const rows = [row("p:a", {}), row("p:h", { section: SectionOnHold, status_words: "Finished" })]
    const s = buildSidebar({ ...base, rows, sessions: [], placements: [] })
    expect(s.rows.map((r) => r.key)).toEqual(["p:a"])
    expect(s.held).toMatchObject([{ key: "p:h", held: true, status: { words: "Finished" } }])
    expect(s.held[0].number).toBeUndefined()
  })

  it("shows only the active workspace's rows and chats", () => {
    const rows = [row("p:a", {}), row("p:x", { workspace: "Other ws" })]
    const sessions = [session("c1", "p", { name: "mine" }), session("c2", "p", { name: "theirs" })]
    const placements = [
      place("c1", "chat", { workspace: "Default" }),
      place("c2", "chat", { workspace: "Other ws" }),
    ]
    const s = buildSidebar({ ...base, rows, sessions, placements })
    expect(s.rows.map((r) => r.key)).toEqual(["c1", "p:a"])
  })

  it("names chats, gives them no ring and maps their status", () => {
    const sessions = [
      session("c1", "p", { name: "talk", status: { state: "working" } }),
      session("c2", "p", { actionable: { reason: "Pick" } }),
    ]
    const placements = [
      place("c1", "chat", { workspace: "Default" }),
      place("c2", "chat", { workspace: "Default" }),
    ]
    const s = buildSidebar({ ...base, rows: [], sessions, placements })
    expect(s.rows).toMatchObject([
      { title: "c2", kind: "chat", status: { words: "Needs you", tone: "waiting" } },
      { title: "talk", kind: "chat", status: { words: "Working", tone: "running" } },
    ])
    expect(s.rows[0].ring).toBeUndefined()
  })

  it("keeps a Triage session out of the chat rows: it sits under Other", () => {
    const sessions = [session("t1", "p", { flow: "triage", flow_key: "p" })]
    const s = buildSidebar({ ...base, rows: [], sessions, placements: [place("t1", "other")] })
    expect(s.rows).toEqual([])
    expect(s.other.map((a) => a.id)).toEqual(["t1"])
    expect(s.other[0].href).toBe("#/triage/p")
  })

  it("numbers only own rows, 1 to 9", () => {
    const rows = Array.from({ length: 11 }, (_, i) => row(`p:e${i}`, {}))
    rows.push(row("p:h", { section: SectionOnHold }))
    const sessions = [session("s1", "p", { queue: true }), session("o1", "p")]
    const placements = [place("s1", "epic", { epic: "p:e0" }), place("o1", "other")]
    const s = buildSidebar({ ...base, rows, sessions, placements })
    expect(s.rows.map((r) => r.number)).toEqual([1, 2, 3, 4, 5, 6, 7, 8, 9, undefined, undefined])
    expect(s.rows[0].agents[0].id).toBe("s1")
    expect("number" in s.rows[0].agents[0]).toBe(false)
    expect(s.held[0].number).toBeUndefined()
    expect(s.other.map((a) => a.id)).toEqual(["o1"])
  })

  describe("sort", () => {
    const keys = (rs: { key: string }[]) => rs.map((r) => r.key)

    it("needs-you puts Needs you first, then names case-insensitively with ties in server order", () => {
      const rows = [
        row("p:b", { name: "beta" }),
        row("p:A", { name: "Alpha" }),
        row("p:n", { name: "zed", section: SectionNeedsYou }),
        row("p:a2", { name: "alpha" }),
      ]
      const sessions = [session("c1", "p", { name: "Aardvark" })]
      const placements = [place("c1", "chat", { workspace: "Default" })]
      const s = buildSidebar({ ...base, rows, sessions, placements })
      expect(keys(s.rows)).toEqual(["p:n", "c1", "p:A", "p:a2", "p:b"])
      expect(s.rows.map((r) => r.number)).toEqual([1, 2, 3, 4, 5])
    })

    it("unknown sort reads as needs-you", () => {
      const rows = [row("p:a", { name: "a" }), row("p:n", { name: "z", section: SectionNeedsYou })]
      expect(
        keys(buildSidebar({ ...base, rows, sessions: [], placements: [], sort: "bogus" }).rows),
      ).toEqual(["p:n", "p:a"])
    })

    it("name ignores Needs you", () => {
      const rows = [row("p:n", { name: "z", section: SectionNeedsYou }), row("p:a", { name: "a" })]
      expect(
        keys(buildSidebar({ ...base, rows, sessions: [], placements: [], sort: "name" }).rows),
      ).toEqual(["p:a", "p:n"])
    })

    it("activity is newest first, no time last by name, and counts a Design's session time", () => {
      const rows = [
        row("p:old", { last_activity_at: "2026-01-01T00:00:00Z" }),
        row("p:none", { name: "none" }),
        row("p:design", { card_kind: CardKindDesign, section: SectionDesigning }),
        row("p:also", { name: "also" }),
      ]
      const sessions = [
        session("d1", "p", { status_changed_at: "2026-03-01T00:00:00Z" } as never),
        session("c1", "p", { status_changed_at: "2026-02-01T00:00:00Z" } as never),
      ]
      const placements = [
        place("d1", "epic", { epic: "p:design" }),
        place("c1", "chat", { workspace: "Default" }),
      ]
      const s = buildSidebar({ ...base, rows, sessions, placements, sort: "activity" })
      expect(keys(s.rows)).toEqual(["p:design", "c1", "p:old", "p:also", "p:none"])
    })

    it("activity orders a held row by its session when newer than its card", () => {
      const rows = [
        row("p:h1", { section: SectionOnHold, last_activity_at: "2026-01-01T00:00:00Z" }),
        row("p:h2", { section: SectionOnHold, last_activity_at: "2026-02-01T00:00:00Z" }),
      ]
      const sessions = [session("s1", "p", { status_changed_at: "2026-03-01T00:00:00Z" } as never)]
      const placements = [place("s1", "epic", { epic: "p:h1" })]
      const s = buildSidebar({ ...base, rows, sessions, placements, sort: "activity" })
      expect(keys(s.held)).toEqual(["p:h1", "p:h2"])
    })

    it("sorts nested agents and Other", () => {
      const rows = [row("p:a", {})]
      const sessions = [
        session("s1", "p", { name: "b" }),
        session("s2", "p", { name: "a", actionable: { reason: "Pick" } }),
        session("o1", "p", { name: "y" }),
        session("o2", "p", { name: "x" }),
      ]
      const placements = [
        place("s1", "epic", { epic: "p:a" }),
        place("s2", "epic", { epic: "p:a" }),
        place("o1", "other"),
        place("o2", "other"),
      ]
      const s = buildSidebar({ ...base, rows, sessions, placements, sort: "name" })
      expect(s.rows[0].agents.map((a) => a.id)).toEqual(["s2", "s1"])
      expect(s.other.map((a) => a.id)).toEqual(["o2", "o1"])
    })

    it("By project groups rows per project, chats last under Chats", () => {
      const rows = [
        row("z:a", { project: "zed", name: "a" }),
        row("b:b", { project: "bee", name: "b" }),
        row("b:a", { project: "bee", name: "a" }),
      ]
      const sessions = [session("c1", "p", { name: "chat" })]
      const placements = [place("c1", "chat", { workspace: "Default" })]
      const s = buildSidebar({ ...base, rows, sessions, placements, sort: "project" })
      expect(s.rows.map((r) => [r.group, r.key])).toEqual([
        ["bee", "b:a"],
        ["bee", "b:b"],
        ["Chats", "c1"],
        ["zed", "z:a"],
      ])
    })

    it("By status groups rows per status: Needs you, Working, then the rest", () => {
      const rows = [
        row("p:a", { status_words: "Idle" }),
        row("p:b", { section: SectionNeedsYou }),
        row("p:c", { status_words: "Blocked" }),
      ]
      const s = buildSidebar({ ...base, rows, sessions: [], placements: [], sort: "status" })
      expect(s.rows.map((r) => [r.group, r.key])).toEqual([
        ["Needs you", "p:b"],
        ["Blocked", "p:c"],
        ["Idle", "p:a"],
      ])
    })

    it("other Sorts leave rows ungrouped", () => {
      const s = buildSidebar({
        ...base,
        rows: [row("p:a", {})],
        sessions: [],
        placements: [],
        sort: "name",
      })
      expect(s.rows[0].group).toBeUndefined()
    })

    describe("nested agents, Other and On hold under every Sort", () => {
      const at = (iso: string) => ({ status_changed_at: iso }) as Partial<SessionInfo>
      const ids = (xs: { id: string }[]) => xs.map((x) => x.id)
      const agentFixture = () => {
        const sessions = [
          session("a-none", "p", { name: "m", ...at("") }),
          session("b-old", "p", { name: "z", ...at("2026-01-01T00:00:00Z") }),
          session("c-new", "p", { name: "n", ...at("2026-02-01T00:00:00Z") }),
          session("d-need", "p", { name: "q", actionable: { reason: "Pick" }, ...at("") }),
        ]
        const ep = (id: string) => place(id, "epic", { epic: "p:a" })
        const oth = (id: string) => place(id, "other")
        return {
          rows: [row("p:a", {})],
          sessions,
          nested: sessions.map((s) => ep(s.id)),
          other: sessions.map((s) => oth(s.id)),
        }
      }
      const expected = {
        activity: ["c-new", "b-old", "a-none", "d-need"],
        name: ["a-none", "c-new", "d-need", "b-old"],
        "needs-you": ["d-need", "a-none", "c-new", "b-old"],
      }

      for (const sort of ["activity", "name", "needs-you"] as const) {
        it(`${sort}: nested agents and Other share one order, no-time rows included`, () => {
          const f = agentFixture()
          const s = buildSidebar({
            ...base,
            rows: f.rows,
            sessions: f.sessions,
            placements: f.nested,
            sort,
          })
          expect(ids(s.rows[0].agents)).toEqual(expected[sort])
          const o = buildSidebar({
            ...base,
            rows: [],
            sessions: f.sessions,
            placements: f.other,
            sort,
          })
          expect(ids(o.other)).toEqual(expected[sort])
        })

        it(`${sort}: On hold rows sort by the same rule, with and without a time`, () => {
          const rows = [
            row("p:h-none", { name: "mid", section: SectionOnHold }),
            row("p:h-old", {
              name: "zed",
              section: SectionOnHold,
              last_activity_at: "2026-01-01T00:00:00Z",
            }),
            row("p:h-new", {
              name: "alpha",
              section: SectionOnHold,
              last_activity_at: "2026-02-01T00:00:00Z",
            }),
          ]
          const s = buildSidebar({ ...base, rows, sessions: [], placements: [], sort })
          expect(keys(s.held)).toEqual(
            sort === "activity"
              ? ["p:h-new", "p:h-old", "p:h-none"]
              : ["p:h-new", "p:h-none", "p:h-old"],
          )
          expect(s.held.every((r) => r.number === undefined && r.agents.length === 0)).toBe(true)
        })
      }
    })

    it("a Design row opens the Design page, an epic its title link", () => {
      const rows = [row("p:a", { card_kind: CardKindDesign, section: SectionDesigning })]
      expect(buildSidebar({ ...base, rows, sessions: [], placements: [] }).rows[0].href).toBe(
        "#/d/p:a",
      )
    })
  })

  it("lists Other sessions the same contents in every workspace", () => {
    const sessions = [session("o1", "p")]
    const placements = [place("o1", "other")]
    const a = buildSidebar({ ...base, rows: [], sessions, placements })
    const b = buildSidebar({ ...base, workspace: "Elsewhere", rows: [], sessions, placements })
    expect(a.other).toEqual(b.other)
    expect(a.other).toHaveLength(1)
  })
})
