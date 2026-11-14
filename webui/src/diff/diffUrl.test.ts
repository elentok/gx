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

test("diffHref is a hash link to the diff route", () => {
  expect(diffHref({ project: "gx", first: "a", second: "b" })).toBe(
    "#/diff?project=gx&base=a&tip=b",
  )
})
