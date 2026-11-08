// @vitest-environment node
import { expect, test } from "vitest";
import { HUH_MESSAGE } from "../design/round";
import type { TriageRow } from "../generated/server";
import { answersOf, cardOf, changedRows, encodeAnswer, roundOf, rowAnswers } from "./questions";

const rows: TriageRow[] = [
  {
    address: "gx:follow-ups/1",
    title: "a",
    questions: [
      { id: "1", title: "Ship it?", body: "detail", kind: "yes-no", recommended: "no" },
      { id: "2", title: "Where?", kind: "options", options: ["ready", "map"], recommended: "map" },
    ],
    answers: { "2": "map" },
  },
  { address: "gx:follow-ups/2", title: "b", questions: [{ id: "3", title: "Name?", kind: "text" }] },
  { address: "gx:follow-ups/3", title: "gone", gone: true, questions: [{ id: "4", title: "x", kind: "text" }] },
];

test("a yes-no, an options and a text question adapt to round cards, with the plan default recommended", () => {
  expect(cardOf(rows[0].questions![0])).toEqual({ label: "1", title: "Ship it?", body: "detail", kind: "yes-no", recommended: "no" });
  expect(cardOf(rows[0].questions![1])).toEqual({
    label: "2",
    title: "Where?",
    body: "",
    kind: "options",
    options: [
      { id: "ready", label: "ready" },
      { id: "map", label: "map" },
    ],
    recommended: "map",
  });
  expect(cardOf(rows[1].questions![0])).toEqual({ label: "3", title: "Name?", body: "", kind: "text" });
});

test("the round skips gone rows, and the unanswered count matches the open count", () => {
  const round = roundOf(rows);
  expect(round.map((c) => c.label)).toEqual(["1", "2", "3"]);
  const answers = answersOf(rows);
  expect(round.filter((c) => !answers[c.label]).length).toBe(2);
});

test("saved answers decode to a choice, Huh? or text, with the note", () => {
  const r: TriageRow[] = [
    {
      address: "a",
      title: "a",
      questions: [
        { id: "1", title: "t", kind: "yes-no" },
        { id: "2", title: "t", kind: "options", options: ["x"] },
        { id: "3", title: "t", kind: "text" },
      ],
      answers: { "1": `yes\nNote: why`, "2": HUH_MESSAGE, "3": "free text" },
    },
  ];
  expect(answersOf(r)).toEqual({ "1": { choice: "yes", note: "why" }, "2": { huh: true }, "3": { text: "free text" } });
});

test("answers encode back for the row save, and only changed rows are named", () => {
  expect(encodeAnswer({ huh: true })).toBe(HUH_MESSAGE);
  expect(encodeAnswer({ choice: "no", note: " n " })).toBe("no\nNote: n");
  expect(encodeAnswer({ note: "alone" })).toBe("");
  const before = answersOf(rows);
  const after = { ...before, "3": { text: "Zed" } };
  expect(rowAnswers(rows[1], after)).toEqual({ "3": "Zed" });
  expect(changedRows(rows, before, after).map((r) => r.address)).toEqual(["gx:follow-ups/2"]);
});
