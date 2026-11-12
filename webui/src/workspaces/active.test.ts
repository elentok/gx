import { afterEach, describe, expect, test, vi } from "vitest"
import type { EpicListRow, SessionPlacement, Workspace } from "../generated/server"
import {
  ACTIVE_KEY,
  readStoredWorkspace,
  resolveActive,
  routePending,
  writeStoredWorkspace,
} from "./active"

afterEach(() => vi.restoreAllMocks())

const workspaces: Workspace[] = [
  { name: "Main", default: true, members: 1 },
  { name: "Side", members: 1 },
  { name: "Ops", members: 0 },
]

const row = (name: string, over: Partial<EpicListRow> = {}): EpicListRow =>
  ({
    address: `demo:${name}`,
    addresses: [`demo:${name}`],
    section: "implementing",
    workspace: "Side",
    ...over,
  }) as EpicListRow

const rows = [
  row("blog", { addresses: ["demo:blog", "demo:blog-impl"] }),
  row("ops", { workspace: "Ops" }),
  row("held", { section: "on-hold", workspace: "Ops" }),
  row("gone", { workspace: "Deleted" }),
  row("one", { address: "demo:one/01", addresses: ["demo:one/01"], workspace: "" }),
]

const placements: SessionPlacement[] = [
  { session: "chat1", kind: "chat", workspace: "Ops" },
  { session: "ep1", kind: "epic", epic: "demo:ops" },
  { session: "ep2", kind: "epic", epic: "demo:held" },
  { session: "oth", kind: "other" },
]

const resolve = (route: string, stored: string | null = "Main") =>
  resolveActive({ route, rows, placements, workspaces, stored })

describe("resolveActive", () => {
  test.each([
    ["design page", "/d/demo:blog", "Side"],
    ["epic page by its -impl address", "/e/demo:blog-impl", "Side"],
    ["ticket page stands for its epic", "/t/demo:ops/03", "Ops"],
    ["agent in a chat", "/agents/chat1", "Ops"],
    ["agent under an epic", "/agents/ep1", "Ops"],
  ])("open item wins: %s", (_, route, want) => expect(resolve(route)).toBe(want))

  test.each([
    ["held epic", "/e/demo:held"],
    ["ticket of a held epic", "/t/demo:held/01"],
    ["agent under a held epic", "/agents/ep2"],
    ["One-off ticket", "/t/demo:one/01"],
    ["Other agent", "/agents/oth"],
    ["unknown item", "/e/demo:nope"],
    ["Epics", "/"],
    ["Stats", "/stats"],
    ["Settings", "/settings"],
  ])("keeps the stored workspace: %s", (_, route) => expect(resolve(route, "Side")).toBe("Side"))

  test("an item in a deleted workspace falls back to Default", () =>
    expect(resolve("/e/demo:gone", null)).toBe("Main"))
  test("a stored workspace that is gone falls back to Default", () =>
    expect(resolve("/stats", "Deleted")).toBe("Main"))
  test("nothing stored resolves to Default", () => expect(resolve("/stats", null)).toBe("Main"))
})

describe("routePending", () => {
  const pending = (route: string) => routePending({ route, placements })
  test("an agent the snapshot does not know is pending", () =>
    expect(pending("/agents/new")).toBe(true))
  test.each(["/agents/oth", "/agents/chat1", "/e/demo:blog", "/agents", "/stats"])(
    "a known or non-agent route is not: %s",
    (route) => expect(pending(route)).toBe(false),
  )
})

describe("stored workspace", () => {
  test("round trips", () => {
    expect(readStoredWorkspace()).toBeNull()
    writeStoredWorkspace("Side")
    expect(localStorage.getItem(ACTIVE_KEY)).toBe("Side")
    expect(readStoredWorkspace()).toBe("Side")
  })

  test("a throwing localStorage reads null and writes silently", () => {
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("blocked")
    })
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("blocked")
    })
    expect(readStoredWorkspace()).toBeNull()
    expect(() => writeStoredWorkspace("Side")).not.toThrow()
  })
})
