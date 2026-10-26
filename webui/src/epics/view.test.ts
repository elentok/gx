import { expect, test } from "vitest";
import type { EpicListRow } from "../generated/server";
import { sectionsOf } from "./view";

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
});

const finished = (name: string, project: string) => row(name, { project, state: "finished", section: "finished" });
const names = (sections: ReturnType<typeof sectionsOf>, key: string) =>
  sections.find((s) => s.key === key)!.rows.map((r) => r.name);

test("sections run Needs you, Ready to queue, In progress, Ready to design, Finished", () => {
  expect(sectionsOf([], "").map((s) => s.title)).toEqual([
    "Needs you",
    "Ready to queue",
    "In progress",
    "Ready to design",
    "Finished",
  ]);
});

test("a project filter keeps only that project's cards, with and without text", () => {
  const epics = [row("a1", { project: "a" }), row("a2", { project: "a" }), row("b1", { project: "b" })];
  expect(names(sectionsOf(epics, "", undefined, "a"), "ready-to-queue")).toEqual(["a1", "a2"]);
  expect(names(sectionsOf(epics, "1", undefined, "a"), "ready-to-queue")).toEqual(["a1"]);
  expect(names(sectionsOf(epics, "", undefined, undefined), "ready-to-queue")).toEqual(["a1", "a2", "b1"]);
});

test("Finished is filtered by project first, then capped; text lifts the cap, a project alone does not", () => {
  const epics = [finished("b0", "b"), finished("a0", "a"), finished("a1", "a"), finished("a2", "a")];
  expect(names(sectionsOf(epics, "", 2, "a"), "finished")).toEqual(["a0", "a1"]);
  expect(names(sectionsOf(epics, "a", 1, "a"), "finished")).toEqual(["a0", "a1", "a2"]);
});
