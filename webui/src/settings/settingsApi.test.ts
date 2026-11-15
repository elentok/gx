import { afterEach, expect, test, vi } from "vitest"
import type { SettingsEntry } from "../generated/server"
import {
  entryLabel,
  fetchSettings,
  groupBySection,
  numberListHint,
  postSettings,
  removeChange,
  resolveSection,
  searchEntries,
  setChange,
  treeSections,
} from "./settingsApi"

afterEach(() => vi.unstubAllGlobals())

const tree = {
  categories: [
    { name: "Web UI", sections: [{ name: "Access", slug: "access", groups: [] }] },
    { name: "System", sections: [{ name: "Watchdogs", slug: "watchdogs", groups: [] }] },
  ],
  redirects: { "web-ui": "access", "cpu-watchdog": "watchdogs" },
}

test("treeSections lists the sections in page order with their category", () => {
  expect(treeSections(tree.categories)).toEqual([
    { name: "Access", slug: "access", category: "Web UI" },
    { name: "Watchdogs", slug: "watchdogs", category: "System" },
  ])
})

test("resolveSection finds a current slug without redirecting, even when a redirect names it", () => {
  const res = resolveSection({ ...tree, redirects: { access: "watchdogs" } }, "access")
  expect(res.section?.name).toBe("Access")
  expect(res.redirect).toBeUndefined()
})

test("resolveSection redirects a retired slug to the target the response holds", () => {
  expect(resolveSection(tree, "cpu-watchdog").redirect).toBe("watchdogs")
})

test("resolveSection drops a redirect whose target the target lacks, and offers the first section", () => {
  const projectTree = { ...tree, categories: tree.categories.slice(0, 1) }
  const res = resolveSection(projectTree, "cpu-watchdog")
  expect(res.redirect).toBeUndefined()
  expect(res.first?.slug).toBe("access")
})

const entry = (key: string, section: string, set: boolean): SettingsEntry => ({
  key,
  path: key.split("."),
  type: "string",
  section,
  category: "",
  group: "",
  description: "",
  set,
})

test("groups entries by section in the order sent and counts the keys the file holds", () => {
  const groups = groupBySection([
    entry("a", "Budget", true),
    entry("b", "Server", false),
    entry("c", "Budget", false),
    entry("d", "Server", true),
  ])

  expect(groups.map((g) => [g.section, g.entries.map((e) => e.key), g.changed])).toEqual([
    ["Budget", ["a", "c"], 1],
    ["Server", ["b", "d"], 1],
  ])
})

test("identity entries never count as changed", () => {
  const groups = groupBySection([
    { ...entry("name", "Project", true), identity: true },
    { ...entry("repo", "Project", true), identity: true },
    entry("budget", "Project", true),
  ])
  expect(groups.map((g) => g.changed)).toEqual([1])
})

test("search matches name, key path and description case-insensitively; blank matches nothing", () => {
  const es = [
    {
      ...entry("execution-queue.max-agents", "Execution queue", false),
      description: "Live agents.",
    },
    { ...entry("budget.soft", "Budget", false), description: "Soft limit." },
  ]
  const keys = (q: string) => searchEntries(es, q).map((e) => e.key)
  expect(keys("MAX AGENTS")).toEqual(["execution-queue.max-agents"])
  expect(keys("budget.")).toEqual(["budget.soft"])
  expect(keys("limit")).toEqual(["budget.soft"])
  expect(keys("  ")).toEqual([])
  expect(keys("nope")).toEqual([])
})

test("change builders", () => {
  expect(setChange("a.b", 3)).toEqual({ key: "a.b", value: 3 })
  expect(removeChange("a.b")).toEqual({ key: "a.b", remove: true })
})

test("number list hint names the first bad item", () => {
  expect(numberListHint("1, 2.5,3")).toBeUndefined()
  expect(numberListHint("1, x")).toBe('"x" is not a number')
})

test("a registry label wins over the key's last segment", () => {
  expect(
    entryLabel({ ...entry("project.evolve", "Evolve", false), label: "Evolve for this project" }),
  ).toBe("Evolve for this project")
  expect(entryLabel(entry("execution-queue.max-agents", "Execution queue", false))).toBe(
    "Max agents",
  )
})

test("fetchSettings asks for the target and throws on a failed status", async () => {
  const fetchMock = vi.fn(
    async (_url: string) => new Response(JSON.stringify({ entries: [] }), { status: 200 }),
  )
  vi.stubGlobal("fetch", fetchMock)
  expect(await fetchSettings("my proj")).toEqual({ entries: [] })
  expect(fetchMock.mock.calls[0][0]).toBe("/v1/settings?target=my%20proj")

  vi.stubGlobal(
    "fetch",
    vi.fn(async () => new Response("", { status: 404 })),
  )
  await expect(fetchSettings("nope")).rejects.toThrow("HTTP 404")
})

test("postSettings sends the project only for a project target", async () => {
  const fetchMock = vi.fn(
    async (_url: string, _init?: RequestInit) => new Response("{}", { status: 200 }),
  )
  vi.stubGlobal("fetch", fetchMock)

  await postSettings("global", [removeChange("a")])
  await postSettings("gx", [setChange("a", 1)])

  const bodies = fetchMock.mock.calls.map((c) => JSON.parse(String(c[1]?.body)))
  expect(bodies).toEqual([
    { changes: [{ key: "a", remove: true }] },
    { project: "gx", changes: [{ key: "a", value: 1 }] },
  ])
})
