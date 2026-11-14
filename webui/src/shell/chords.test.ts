import { afterEach, describe, expect, it, vi } from "vitest"
import type { SessionInfo } from "../generated/server"
import {
  agentIdOf,
  canRename,
  chordTable,
  displayKey,
  letterOf,
  matchesKey,
  parseKey,
} from "./chords"

afterEach(() => vi.restoreAllMocks())

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
  ...over,
})

describe("chord table x", () => {
  const x = chordTable({ remove: () => {}, rename: () => {} }).find((c) => c.key === "x")!
  const page = "/agents/s-a"

  it("is available for a chat session, labelled Remove chat", () => {
    const ctx = { path: page, session: session() }
    expect(x.available(ctx)).toBe(true)
    expect(x.label(ctx)).toBe("Remove chat")
  })
  it.each([{ ticket: "gx:e/1" }, { fix_of: "gx:e/1" }, { flow: "triage" }])(
    "is labelled Remove for %j",
    (over) => {
      const ctx = { path: page, session: session(over) }
      expect(x.available(ctx)).toBe(true)
      expect(x.label(ctx)).toBe("Remove")
    },
  )
  it("is unavailable for a Queue agent", () => {
    expect(x.available({ path: page, session: session({ queue: true }) })).toBe(false)
  })
  it("is unavailable off an agent page or without a session", () => {
    expect(x.available({ path: "/designs/a", session: session() })).toBe(false)
    expect(x.available({ path: page })).toBe(false)
  })
  it("calls the handed-in remove opener and is danger", () => {
    const remove = vi.fn()
    const row = chordTable({ remove, rename: () => {} }).find((c) => c.key === "x")!
    row.run()
    expect(remove).toHaveBeenCalledOnce()
    expect(row.danger).toBe(true)
  })
})

describe("chord table c and d", () => {
  it("are always available with their labels", () => {
    const t = chordTable({ remove: () => {}, rename: () => {} })
    expect(t.map((c) => [c.key, c.label({ path: "/" }), c.available({ path: "/" })])).toEqual([
      ["c", "New chat", true],
      ["d", "New design", true],
      ["s", "Focus sidebar", true],
      ["r", "Rename chat", false],
      ["x", "Remove chat", false],
    ])
  })
})

describe("chord table r", () => {
  const rename = vi.fn()
  const r = chordTable({ remove: () => {}, rename }).find((c) => c.key === "r")!
  const page = "/agents/s-a"

  it("is available on a chat's page", () => {
    expect(r.available({ path: page, session: session() })).toBe(true)
    expect(canRename({ path: page, session: session() })).toBe(true)
  })
  it.each([{ queue: true }, { ticket: "gx:e/1" }, { fix_of: "gx:e/1" }, { flow: "triage" }])(
    "is unavailable for %j",
    (over) => {
      expect(r.available({ path: page, session: session(over) })).toBe(false)
    },
  )
  it("is unavailable off an agent page or without a session", () => {
    expect(r.available({ path: "/designs/a", session: session() })).toBe(false)
    expect(r.available({ path: "/agents", session: session() })).toBe(false)
    expect(r.available({ path: page })).toBe(false)
  })
  it("calls the handed-in rename opener", () => {
    r.run()
    expect(rename).toHaveBeenCalledOnce()
  })
})
