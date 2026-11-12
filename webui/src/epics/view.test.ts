import { expect, test } from "vitest"
import type { EpicListRow } from "../generated/server"
import { sectionsOf } from "./view"

const row = (name: string, over: Partial<EpicListRow> = {}): EpicListRow => ({
  address: `${over.project ?? "gx"}:${name}`,
  project: "gx",
  name,
  kind: "loop",
  state: "ready",
  done: 0,
  total: 3,
  queued: false,
  queueable: true,
  blocked_by_epics: [],
  card_kind: "implementation",
  section: "ready-to-queue",
  addresses: [],
  ...over,
})

const finished = (name: string, project: string) =>
  row(name, { project, state: "finished", section: "finished" })
const names = (sections: ReturnType<typeof sectionsOf>, key: string) =>
  sections.find((s) => s.key === key)!.rows.map((r) => r.name)

test("sections run Needs you, Implementing, Ready to queue, Designing, Ready to design, Finished, On hold", () => {
  expect(sectionsOf([], "").map((s) => s.title)).toEqual([
    "Needs you",
    "Implementing",
    "Ready to queue",
    "Designing",
    "Ready to design",
    "Finished",
    "On hold",
  ])
  expect(sectionsOf([], "").at(-1)!.collapsedByDefault).toBe(true)
})

test("a workspace filter keeps that workspace's cards and drops cards with none", () => {
  const epics = [
    row("a", { workspace: "Default" }),
    row("b", { workspace: "Side" }),
    row("oneoff", { section: "ready-to-queue" }),
    row("held", { workspace: "Side", section: "on-hold" }),
  ]
  expect(names(sectionsOf(epics, "", undefined, undefined, "Side"), "ready-to-queue")).toEqual([
    "b",
  ])
  expect(names(sectionsOf(epics, "", undefined, undefined, "Side"), "on-hold")).toEqual(["held"])
  expect(names(sectionsOf(epics, "", undefined, undefined, undefined), "ready-to-queue")).toEqual([
    "a",
    "b",
    "oneoff",
  ])
})

test("a named workspace shows the Follow-ups card of a project in it and not of another", () => {
  const card = (project: string) => row("Follow-ups", { project, card_kind: "follow-ups" })
  const epics = [card("gx"), card("webapp")]
  const counts = [
    {
      project: "gx",
      open: 1,
      bugs: 1,
      ideas: 0,
      other: 0,
      triage: "none" as const,
      left: 0,
      workspaces: ["Side"],
    },
    {
      project: "webapp",
      open: 1,
      bugs: 1,
      ideas: 0,
      other: 0,
      triage: "none" as const,
      left: 0,
      workspaces: ["Default"],
    },
  ]
  const projects = (w?: string) =>
    sectionsOf(epics, "", undefined, undefined, w, counts).flatMap((s) =>
      s.rows.map((r) => r.project),
    )
  expect(projects("Side")).toEqual(["gx"])
  expect(projects("Default")).toEqual(["webapp"])
  expect(projects(undefined)).toEqual(["gx", "webapp"])
})

test("the text filter matches held cards", () => {
  const epics = [row("held-one", { section: "on-hold" }), row("other", { section: "on-hold" })]
  expect(names(sectionsOf(epics, "one"), "on-hold")).toEqual(["held-one"])
})

test("a project filter keeps only that project's cards, with and without text", () => {
  const epics = [
    row("a1", { project: "a" }),
    row("a2", { project: "a" }),
    row("b1", { project: "b" }),
  ]
  expect(names(sectionsOf(epics, "", undefined, ["a"]), "ready-to-queue")).toEqual(["a1", "a2"])
  expect(names(sectionsOf(epics, "1", undefined, ["a"]), "ready-to-queue")).toEqual(["a1"])
  expect(names(sectionsOf(epics, "", undefined, undefined), "ready-to-queue")).toEqual([
    "a1",
    "a2",
    "b1",
  ])
})

test("Finished is filtered by project first, then capped; text lifts the cap, a project alone does not", () => {
  const epics = [finished("b0", "b"), finished("a0", "a"), finished("a1", "a"), finished("a2", "a")]
  expect(names(sectionsOf(epics, "", 2, ["a"]), "finished")).toEqual(["a0", "a1"])
  expect(names(sectionsOf(epics, "a", 1, ["a"]), "finished")).toEqual(["a0", "a1", "a2"])
})
