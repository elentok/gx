// @vitest-environment node
import { expect, test } from "vitest";
import {
  CardActionAnswer,
  CardKindDesign,
  CardKindFollowUps,
  CardKindImplementation,
  CardKindOneOff,
  SectionImplementing,
  SectionNeedsYou,
  type EpicListRow,
  type FollowUpCount,
} from "../generated/server";
import { nextQuestion } from "./nextQuestion";

const row = (name: string, over: Partial<EpicListRow> = {}): EpicListRow =>
  ({
    address: `p:${name}`,
    project: "p",
    name,
    workspace: "Default",
    card_kind: CardKindImplementation,
    section: SectionNeedsYou,
    waiting_since: "2026-01-01T00:00:00Z",
    ...over,
  }) as EpicListRow;

const at = (day: number) => `2026-01-0${day}T00:00:00Z`;
const followUps = (project: string, workspaces: string[]): FollowUpCount => ({ project, workspaces }) as FollowUpCount;
const names = (rows: EpicListRow[]) => rows.map((r) => r.name);

// Server order is oldest first.
const a = row("a", { waiting_since: at(1) });
const b = row("b", { waiting_since: at(2) });
const c = row("c", { waiting_since: at(3) });
const list = { epics: [a, b, c], follow_ups: [] as FollowUpCount[] };

test("orders newest first and counts", () => {
  const r = nextQuestion(list, "Default", "/epics");
  expect(names(r.cards)).toEqual(["c", "b", "a"]);
  expect(r.count).toBe(3);
});

test("a non-card page gives the newest", () => {
  expect(nextQuestion(list, "Default", "/epics").next?.name).toBe("c");
});

test("the card after the current epic, wrapping at the end", () => {
  expect(nextQuestion(list, "Default", "/e/p:c").next?.name).toBe("b");
  expect(nextQuestion(list, "Default", "/e/p:a").next?.name).toBe("c");
});

test("a question card's Answer link is the target and its epic page counts as the card", () => {
  const q = row("q", {
    waiting_since: at(5),
    actions: [{ kind: CardActionAnswer, link: "#/t/p:q/1" }] as EpicListRow["actions"],
  });
  const l = { epics: [a, q], follow_ups: [] };
  expect(nextQuestion(l, "Default", "/epics").next?.name).toBe("q");
  expect(nextQuestion(l, "Default", "/t/p:q/1").next?.name).toBe("a");
  expect(nextQuestion(l, "Default", "/e/p:q").next?.name).toBe("a");
});

test.each([
  ["permission prompt", { state_pill: "Permission" }],
  ["paused ticket", { state_pill: "Paused" }],
  ["blocked merge", { state_pill: "Blocked merge" }],
  ["epic awaiting merge", { state_pill: "Awaiting merge" }],
  ["deadlock", { state_pill: "Deadlock" }],
  ["Design question", { card_kind: CardKindDesign }],
  ["Follow-ups review", { card_kind: CardKindFollowUps }],
  ["one-off card", { card_kind: CardKindOneOff }],
] as [string, Partial<EpicListRow>][])("%s is a candidate", (_, over) => {
  const l = { epics: [row("x", over)], follow_ups: [followUps("p", ["Default"])] };
  expect(nextQuestion(l, "Default", "/epics").next?.name).toBe("x");
});

test("a Follow-ups card on its triage page steps to the next", () => {
  const f = row("f", { card_kind: CardKindFollowUps, waiting_since: undefined });
  const l = { epics: [f, a], follow_ups: [followUps("p", ["Default"])] };
  expect(names(nextQuestion(l, "Default", "/epics").cards)).toEqual(["a", "f"]);
  expect(nextQuestion(l, "Default", "/triage/p").next?.name).toBe("a");
});

test("a Design card on its Design page and a one-off card on its ticket page step on", () => {
  const d = row("d", { card_kind: CardKindDesign, waiting_since: at(3) });
  const o = row("o", { card_kind: CardKindOneOff, waiting_since: at(2) });
  const l = { epics: [a, o, d], follow_ups: [] };
  expect(nextQuestion(l, "Default", "/d/p:d").next?.name).toBe("o");
  expect(nextQuestion(l, "Default", "/t/p:o").next?.name).toBe("a");
});

test("cards of another workspace are skipped", () => {
  const far = row("far", { workspace: "Work", waiting_since: at(9) });
  expect(names(nextQuestion({ epics: [far, a], follow_ups: [] }, "Default", "/epics").cards)).toEqual(["a"]);
});

test("a Follow-ups card of a project outside the workspace is skipped", () => {
  const f = row("f", { card_kind: CardKindFollowUps, project: "q", waiting_since: undefined });
  const l = { epics: [f], follow_ups: [followUps("q", ["Work"])] };
  expect(nextQuestion(l, "Default", "/epics").count).toBe(0);
});

test("a one-off card always counts", () => {
  const o = row("o", { card_kind: CardKindOneOff, workspace: undefined });
  expect(nextQuestion({ epics: [o], follow_ups: [] }, "Default", "/epics").count).toBe(1);
});

test("other sections are not candidates", () => {
  const busy = row("busy", { section: SectionImplementing });
  expect(nextQuestion({ epics: [busy], follow_ups: [] }, "Default", "/epics").count).toBe(0);
});

test("Follow-ups cards sort last, server order reversed among them", () => {
  const f1 = row("f1", { card_kind: CardKindFollowUps, project: "p", waiting_since: undefined });
  const f2 = row("f2", { card_kind: CardKindFollowUps, project: "q", waiting_since: undefined });
  const l = { epics: [f1, f2, a], follow_ups: [followUps("p", ["Default"]), followUps("q", ["Default"])] };
  expect(names(nextQuestion(l, "Default", "/epics").cards)).toEqual(["a", "f2", "f1"]);
});

test("no candidates gives none", () => {
  expect(nextQuestion({ epics: [], follow_ups: [] }, "Default", "/epics")).toEqual({ next: undefined, cards: [], count: 0 });
});
