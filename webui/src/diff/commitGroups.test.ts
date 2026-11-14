import { expect, test } from "vitest"
import type { DiffCommit } from "../generated/server"
import { groupCommits } from "./commitGroups"

const c = (sha: string, subject: string, over: Partial<DiffCommit> = {}): DiffCommit => ({
  sha,
  subject,
  time: "2026-01-01T00:00:00Z",
  files: [],
  ...over,
})
const titles = new Map([["gx:e-impl/03", "Add the thing"]])
const shas = (g: { commits: DiffCommit[] }) => g.commits.map((x) => x.sha)

test("ticket groups carry the ticket title, or Ticket <id> when none is known", () => {
  const groups = groupCommits(
    [c("1", "a", { ticket: "gx:e-impl/03" }), c("2", "b", { ticket: "gx:e-impl/07" })],
    "gx:e-impl",
    titles,
  )
  expect(groups.map((g) => g.label)).toEqual(["Add the thing", "Ticket 07"])
})

test("ticket groups follow the order of their first commit", () => {
  const groups = groupCommits(
    [
      c("1", "a", { ticket: "gx:e-impl/07" }),
      c("2", "b", { ticket: "gx:e-impl/03" }),
      c("3", "c", { ticket: "gx:e-impl/07" }),
    ],
    "gx:e-impl",
    titles,
  )
  expect(groups.map((g) => g.key)).toEqual(["gx:e-impl/07", "gx:e-impl/03"])
  expect(shas(groups[0])).toEqual(["1", "3"])
})

test("Design is a trailer-less <map>: subject; the rest is Other commits, after the tickets", () => {
  const groups = groupCommits(
    [
      c("1", "e: plan"),
      c("2", "plain"),
      c("3", "Merge branch x", { merge: true }),
      c("4", "t", { ticket: "gx:e-impl/03" }),
    ],
    "gx:e-impl",
    titles,
  )
  expect(groups.map((g) => g.label)).toEqual(["Add the thing", "Design", "Other commits"])
  expect(shas(groups[1])).toEqual(["1"])
  expect(shas(groups[2])).toEqual(["2", "3"])
  expect(groups[2].commits[1].merge).toBe(true)
})

test("there is no Design group without an epic", () => {
  const groups = groupCommits([c("1", "e: plan")], undefined, titles)
  expect(groups.map((g) => g.label)).toEqual(["Other commits"])
})

test("an empty commit still appears in its group", () => {
  const groups = groupCommits(
    [c("1", "t", { ticket: "gx:e-impl/03", files: [] })],
    "gx:e-impl",
    titles,
  )
  expect(shas(groups[0])).toEqual(["1"])
})
