import { expect, test } from "vitest"
import { diffHref } from "../hrefs"
import { diffQuery, parseDiffUrl, type DiffUrl } from "./diffUrl"

const roundTrip = (url: DiffUrl) => parseDiffUrl(new URLSearchParams(diffQuery(url)))

test("every part round-trips", () => {
  const url: DiffUrl = {
    project: "gx",
    first: "main",
    second: "feature/x y",
    epic: "gx:epic-a",
    commits: ["abc", "def"],
    ticket: "gx:epic-a/03",
    file: "src/a b.ts",
  }
  expect(roundTrip(url)).toEqual(url)
})

test("absent parts are dropped", () => {
  const url: DiffUrl = { project: "gx", first: "a", second: "b" }
  expect(diffQuery(url)).toBe("project=gx&base=a&tip=b")
  expect(roundTrip(url)).toEqual(url)
})

test("an empty address parses to empty refs", () => {
  expect(parseDiffUrl(new URLSearchParams(""))).toEqual({ project: "", first: "", second: "" })
})

test.each([
  ["chat:abc123", { kind: "chat", id: "abc123" }],
  ["design:calmer diff", { kind: "design", id: "calmer diff" }],
  ["ticket:gx:better-diff-impl/03", { kind: "ticket", id: "gx:better-diff-impl/03" }],
])("worktree=%s round-trips and splits at the first colon only", (value, worktree) => {
  const url: DiffUrl = { project: "gx", first: "", second: "", worktree: worktree as never }
  expect(roundTrip(url)).toEqual(url)
  expect(new URLSearchParams(diffQuery(url)).get("worktree")).toBe(value)
})

test("a worktree URL drops base and tip", () => {
  const parsed = parseDiffUrl(new URLSearchParams("project=gx&worktree=chat:a&base=x&tip=y"))
  expect(parsed).toEqual({
    project: "gx",
    first: "",
    second: "",
    worktree: { kind: "chat", id: "a" },
  })
  const url: DiffUrl = { ...parsed, first: "x", second: "y" }
  expect(diffQuery(url)).toBe("project=gx&worktree=chat%3Aa")
})

test("a worktree without a known kind or id is ignored", () => {
  const p = (w: string) => parseDiffUrl(new URLSearchParams({ project: "gx", worktree: w }))
  expect(p("nope:x").worktree).toBeUndefined()
  expect(p("chat:").worktree).toBeUndefined()
  expect(p("chat").worktree).toBeUndefined()
})

test("diffHref is a hash link to the diff route", () => {
  expect(diffHref({ project: "gx", first: "a", second: "b" })).toBe(
    "#/diff?project=gx&base=a&tip=b",
  )
})
