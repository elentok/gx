import { afterEach, describe, expect, it, vi } from "vitest"
import { chatEntity, type Entity } from "../actions/entity"
import { chordLetters, menuActions } from "../actions/registry"
import type { SessionInfo } from "../generated/server"
import { registerHistoryOpener } from "../history/historyOpener"
import {
  agentIdOf,
  canRename,
  chatDraftIdOf,
  chordTable,
  displayKey,
  draftIdOf,
  entityChords,
  letterOf,
  matchesKey,
  parseKey,
} from "./chords"
import { getBigPrompt, setBigPrompt } from "../ui/expandable-text/bigPromptStore"

afterEach(() => vi.restoreAllMocks())

const startNewDesign = vi.hoisted(() => vi.fn())
vi.mock("../design/newDesign", () => ({ startNewDesign }))

describe("agentIdOf", () => {
  it("reads the id of an agent page, with or without a trailing slash", () => {
    expect(agentIdOf("/agents/s1")).toBe("s1")
    expect(agentIdOf("/agents/s1/")).toBe("s1")
  })

  it("is undefined elsewhere", () => {
    expect(agentIdOf("/")).toBeUndefined()
    expect(agentIdOf("/agents")).toBeUndefined()
    expect(agentIdOf("/agents/s1/files")).toBeUndefined()
  })
})

const mac = () => vi.spyOn(navigator, "platform", "get").mockReturnValue("MacIntel")
const linux = () => vi.spyOn(navigator, "platform", "get").mockReturnValue("Linux x86_64")

const ev = (over: Partial<KeyboardEvent> = {}) => ({
  code: "KeyE",
  metaKey: false,
  ctrlKey: false,
  altKey: false,
  shiftKey: false,
  ...over,
})

describe("matchesKey", () => {
  it("matches cmd+e on a Mac event", () => {
    expect(matchesKey(ev({ metaKey: true }), "cmd+e")).toBe(true)
  })
  it("matches alt+e although key is Dead", () => {
    expect(matchesKey(ev({ altKey: true, key: "Dead" }), "alt+e")).toBe(true)
  })
  it("rejects Shift+Cmd+E for cmd+e", () => {
    expect(matchesKey(ev({ metaKey: true, shiftKey: true }), "cmd+e")).toBe(false)
  })
  it("matches ctrl+e only with Ctrl", () => {
    expect(matchesKey(ev({ ctrlKey: true }), "ctrl+e")).toBe(true)
    expect(matchesKey(ev({ metaKey: true }), "ctrl+e")).toBe(false)
    expect(matchesKey(ev({ ctrlKey: true, metaKey: true }), "ctrl+e")).toBe(false)
  })
  it("never matches an empty or unparsable setting", () => {
    expect(matchesKey(ev(), "")).toBe(false)
    expect(matchesKey(ev(), undefined)).toBe(false)
    expect(matchesKey(ev({ metaKey: true }), "cmd+")).toBe(false)
    expect(matchesKey(ev({ metaKey: true }), "hyper+e")).toBe(false)
    expect(matchesKey(ev({ metaKey: true }), "cmd+ee")).toBe(false)
  })
  it("matches digit keys by Digit code", () => {
    expect(matchesKey(ev({ code: "Digit1", altKey: true }), "alt+1")).toBe(true)
    expect(matchesKey(ev({ code: "KeyE", altKey: true }), "alt+1")).toBe(false)
  })
})

// Mirrors the keys in TestLeaderKeyEntries (config/registry/entries_core_test.go). Keep both in step.
// Only keys where parseKey and checkLeaderKey agree are listed. They differ in two ways:
// parseKey trims and lower-cases its input, so it accepts "cmd+E", and it does not need a cmd, ctrl
// or alt modifier, so it accepts "e" and "shift+e". The server refuses both when the setting is
// saved, so a stored value is always valid.
describe("parseKey", () => {
  it.each(["cmd+e", "alt+e", "ctrl+shift+j", "alt+7"])("accepts %s", (key) => {
    expect(parseKey(key)).not.toBeNull()
  })
  it.each(["cmd+", "cmd+ee", "meta+e", "alt+space", "ctrl+f1", ""])("rejects %j", (key) => {
    expect(parseKey(key)).toBeNull()
  })
})

describe("letterOf", () => {
  it("reads the letter from code", () => {
    expect(letterOf({ code: "KeyC" })).toBe("c")
    expect(letterOf({ code: "Digit1" })).toBeNull()
  })
})

describe("displayKey", () => {
  it("uses Mac symbols on a Mac", () => {
    mac()
    expect(displayKey("cmd+e")).toBe("⌘E")
    expect(displayKey("alt+j")).toBe("⌥J")
  })
  it("uses words elsewhere", () => {
    linux()
    expect(displayKey("ctrl+e")).toBe("Ctrl+E")
    expect(displayKey("alt+j")).toBe("Alt+J")
  })
  it("is empty for an unparsable setting", () => {
    expect(displayKey("")).toBe("")
  })
})

const session = (over: Partial<SessionInfo> = {}): SessionInfo => ({
  id: "s-a",
  name: "worker",
  project: "gx",
  dir: "/x",
  harness: "claude",
  status: { state: "idle" },
  status_changed_at: "2026-01-01T00:00:00Z",
  run_started_at: "2026-01-01T00:00:00Z",
  unread: {},
  actionable: {},
  auto_advance: true,
  auto_queue: false,
  offers_history: true,
  ...over,
})

const noop = () => {}
/** What `useActions` hands the table for an entity: the registry's menu actions, ready to run. */
const actionsOf = (e: Entity, run: () => void = noop) =>
  menuActions(e).map((d) => ({
    id: d.id,
    label: d.label,
    chord: d.chord,
    danger: !!d.danger,
    disabled: false,
    run,
  }))
const tableOf = (e?: Entity, run?: () => void) =>
  chordTable({ actions: e ? actionsOf(e, run) : [], history: noop })
const entityRows = (e: Entity) =>
  tableOf(e)
    .filter((c) => c.group === "entity")
    .map((c) => c.key)
    .sort()

const page = (e: Entity): Entity => ({ ...e, surface: "page" })
const base = { key: "gx:ep", name: "ep", surface: "page" } as const
const chat = (over: Partial<SessionInfo> = {}) => page(chatEntity(session(over)))

describe("entity chords by entity", () => {
  it.each<[string, Entity, string[]]>([
    ["a plain chat", chat(), ["m", "r", "w", "x", "y"]],
    ["a Queue agent", chat({ queue: true }), ["y"]],
    ["a Fix agent", chat({ fix_of: "gx:e/1" }), ["x", "y"]],
    ["a managed agent", { ...chat({ ticket: "gx:e/1" }), agent: "managed" }, ["x", "y"]],
    ["a live epic", { ...base, kind: "epic", workspace: "Default" }, ["a", "w", "y"]],
    ["a Design", { ...base, kind: "design", workspace: "Default" }, ["a", "w", "y"]],
    ["an archived epic", { ...base, kind: "epic", archived: true }, ["a", "y"]],
    ["a ticket", { ...base, kind: "ticket" }, ["y"]],
    ["a design draft", { ...base, kind: "design-draft" }, ["r", "w", "x"]],
    ["a chat draft", { ...base, kind: "chat-draft" }, ["r", "w", "x"]],
  ])("%s offers exactly its registry chords", (_name, entity, keys) => {
    expect(entityRows(entity)).toEqual(keys)
  })

  it("offers no entity chord with no open entity", () => {
    expect(tableOf().filter((c) => c.group === "entity")).toEqual([])
  })

  it("takes letters and labels from the registry", () => {
    const rows = Object.fromEntries(
      tableOf(chat()).map((c) => [c.key, c.label({ path: "/agents/s-a" })]),
    )
    expect(rows).toMatchObject({
      r: "Rename",
      m: "Switch model",
      w: "Move to workspace",
      y: "Copy ID",
      x: "Remove",
    })
  })

  it("is Archive on a live epic and Unarchive on an archived one, on the same letter", () => {
    const label = (e: Entity) =>
      tableOf(e)
        .find((c) => c.key === "a")
        ?.label({ path: "/" })
    expect(label({ ...base, kind: "epic", workspace: "Default" })).toBe("Archive")
    expect(label({ ...base, kind: "epic", archived: true })).toBe("Unarchive")
  })

  it("marks Remove, and only Remove, as danger", () => {
    const danger = tableOf(chat())
      .filter((c) => c.danger)
      .map((c) => c.key)
    expect(danger).toEqual(["x"])
  })

  it("runs the registry action and leaves out one that cannot run", () => {
    const run = vi.fn()
    const row = tableOf(chat(), run).find((c) => c.key === "x")!
    row.run()
    expect(run).toHaveBeenCalledOnce()
    const gone = entityChords([
      { id: "remove", label: "Remove", chord: "x", danger: true, disabled: true, run },
    ])
    expect(gone).toEqual([])
  })

  it("never shares a letter between two chords, entity or app", () => {
    const entities: Entity[] = [
      chat(),
      { ...base, kind: "epic", workspace: "Default" },
      { ...base, kind: "epic", archived: true },
      { ...base, kind: "design-draft" },
    ]
    for (const e of entities) {
      const keys = tableOf(e).map((c) => c.key)
      expect(new Set(keys).size, keys.join("")).toBe(keys.length)
    }
    const app = tableOf().map((c) => c.key)
    for (const letter of chordLetters()) expect(app).not.toContain(letter)
  })
})

describe("draft page paths", () => {
  it("reads the draft id from the path", () => {
    expect(draftIdOf("/drafts/d1")).toBe("d1")
    expect(draftIdOf("/drafts/d1/x")).toBeUndefined()
    expect(draftIdOf("/agents/d1")).toBeUndefined()
  })
  it("tells a chat draft's path from a design draft's", () => {
    expect(chatDraftIdOf("/chat-drafts/c1")).toBe("c1")
    expect(chatDraftIdOf("/drafts/c1")).toBeUndefined()
    expect(draftIdOf("/chat-drafts/c1")).toBeUndefined()
  })
})

describe("app chords", () => {
  it("are always available, in their own group, with their labels", () => {
    const t = tableOf()
    expect(t.map((c) => [c.key, c.label({ path: "/" }), c.available({ path: "/" })])).toEqual([
      ["c", "New chat draft", true],
      ["C", "New chat", true],
      ["d", "New design", true],
      ["s", "Focus sidebar", true],
      ["b", "Big prompt box", true],
      ["h", "History", false],
    ])
    expect(t.every((c) => c.group !== "entity")).toBe(true)
  })
})

describe("chord table h", () => {
  const h = tableOf().filter((c) => c.key === "h")[0]

  it("is the last row, labelled History", () => {
    const t = tableOf()
    expect(t[t.length - 1].key).toBe("h")
    expect(h.label({ path: "/" })).toBe("History")
  })
  it("is available exactly while an opener is registered, whatever the path", () => {
    expect(h.available({ path: "/drafts/d1" })).toBe(false)
    const off = registerHistoryOpener(() => {})
    expect(h.available({ path: "/drafts/d1" })).toBe(true)
    expect(h.available({ path: "/epics" })).toBe(true)
    off()
    expect(h.available({ path: "/drafts/d1" })).toBe(false)
  })
})

describe("chord table b", () => {
  afterEach(() => setBigPrompt(false))
  const b = () => tableOf().find((c) => c.key === "b")!

  it("is offered on every page", () => {
    for (const path of ["/agents/s1", "/drafts/d1", "/designs/x", "/epics", "/agents"]) {
      expect(b().available({ path })).toBe(true)
    }
  })

  it("flips the store and relabels", () => {
    expect(b().label({ path: "/" })).toBe("Big prompt box")
    b().run()
    expect(getBigPrompt()).toBe(true)
    expect(b().label({ path: "/" })).toBe("Normal prompt box")
    b().run()
    expect(getBigPrompt()).toBe(false)
  })
})

describe("chord table d", () => {
  it("runs the New design action once", () => {
    startNewDesign.mockClear()
    tableOf()
      .find((c) => c.key === "d")
      ?.run()
    expect(startNewDesign).toHaveBeenCalledTimes(1)
  })
})

describe("canRename", () => {
  const path = "/agents/s-a"
  it("holds on a plain chat's page only", () => {
    expect(canRename({ path, session: session() })).toBe(true)
    expect(canRename({ path, session: session({ queue: true }) })).toBe(false)
    expect(canRename({ path: "/epics", session: session() })).toBe(false)
    expect(canRename({ path })).toBe(false)
  })
})
