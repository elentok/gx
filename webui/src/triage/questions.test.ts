// @vitest-environment node
import { expect, test } from "vitest"
import type { TriageRow } from "../generated/server"
import {
  HUH_MESSAGE,
  agreeWithRecommendations,
  answersOf,
  cardOf,
  changedRows,
  decidedOf,
  encodeAnswer,
  roundOf,
  rowAnswers,
} from "./questions"

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
  {
    address: "gx:follow-ups/2",
    title: "b",
    questions: [{ id: "3", title: "Name?", kind: "text" }],
  },
  {
    address: "gx:follow-ups/3",
    title: "gone",
    gone: true,
    questions: [{ id: "4", title: "x", kind: "text" }],
  },
]

test("a yes-no, an options and a text question adapt to round cards, with the plan default recommended", () => {
  expect(cardOf(rows[0].questions![0], "1.1")).toEqual({
    label: "1.1",
    title: "Ship it?",
    body: "detail",
    kind: "yes-no",
    recommended: "no",
  })
  expect(cardOf(rows[0].questions![1], "1.2")).toEqual({
    label: "1.2",
    title: "Where?",
    body: "",
    kind: "options",
    options: [
      { id: "ready", label: "ready" },
      { id: "map", label: "map" },
    ],
    recommended: "map",
  })
  expect(cardOf(rows[1].questions![0], "2.3")).toEqual({
    label: "2.3",
    title: "Name?",
    body: "",
    kind: "text",
  })
})

test("the round skips gone rows, and the unanswered count matches the open count", () => {
  const round = roundOf(rows)
  expect(round.map((c) => c.label)).toEqual(["1.1", "1.2", "2.3"])
  expect(round[0].context).toBe("1 · a")
  expect(round[0].contextHref).toBe("#/t/gx:follow-ups/1")
  const answers = answersOf(rows)
  expect(round.filter((c) => !answers[c.label]).length).toBe(2)
})

test("saved answers decode to a choice, Huh? or text, with the note", () => {
  const r: TriageRow[] = [
    {
      address: "gx:follow-ups/1",
      title: "a",
      questions: [
        { id: "1", title: "t", kind: "yes-no" },
        { id: "2", title: "t", kind: "options", options: ["x"] },
        { id: "3", title: "t", kind: "text" },
      ],
      answers: { "1": `yes\nNote: why`, "2": HUH_MESSAGE, "3": "free text" },
    },
  ]
  expect(answersOf(r)).toEqual({
    "1.1": { choice: "yes", note: "why" },
    "1.2": { huh: true },
    "1.3": { text: "free text" },
  })
})

test("answers encode back for the row save, and only changed rows are named", () => {
  expect(encodeAnswer({ huh: true })).toBe(HUH_MESSAGE)
  expect(encodeAnswer({ choice: "no", note: " n " })).toBe("no\nNote: n")
  expect(encodeAnswer({ note: "alone" })).toBe("")
  const before = answersOf(rows)
  const after = { ...before, "2.3": { text: "Zed" } }
  expect(rowAnswers(rows, rows[1], after)).toEqual({ "3": "Zed" })
  expect(changedRows(rows, before, after).map((r) => r.address)).toEqual(["gx:follow-ups/2"])
})

test("rows that each have a q1 keep separate answers", () => {
  const twins: TriageRow[] = [
    {
      address: "gx:follow-ups/01",
      title: "a",
      questions: [{ id: "q1", title: "A?", kind: "text" }],
    },
    {
      address: "gx:follow-ups/02",
      title: "b",
      questions: [{ id: "q1", title: "B?", kind: "text" }],
    },
  ]
  expect(roundOf(twins).map((c) => c.label)).toEqual(["1.1", "2.1"])
  const before = answersOf(twins)
  const after = { ...before, "1.1": { text: "agree" } }
  expect(rowAnswers(twins, twins[0], after)).toEqual({ q1: "agree" })
  expect(rowAnswers(twins, twins[1], after)).toEqual({})
  expect(changedRows(twins, before, after).map((r) => r.address)).toEqual(["gx:follow-ups/01"])
  const saved = [{ ...twins[0], answers: { q1: "agree" } }, twins[1]]
  expect(Object.keys(answersOf(saved))).toEqual(["1.1"])
})

test("a row with no number takes its position, and a clash gets a prime", () => {
  const odd: TriageRow[] = [
    { address: "gx:epic-a", title: "a", questions: [{ id: "q1", title: "A?", kind: "text" }] },
    { address: "gx:x/2", title: "b", questions: [{ id: "q1", title: "B?", kind: "text" }] },
    { address: "gx:y/2", title: "c", questions: [{ id: "q1", title: "C?", kind: "text" }] },
  ]
  expect(roundOf(odd).map((c) => c.label)).toEqual(["1.1", "2.1", "2'.1"])
})

test("a quick question with options is a choice card with its default recommended; yes and no is a yes-no card", () => {
  const pick = cardOf(
    {
      id: "q1",
      title: "Where?",
      kind: "quick",
      options: ["parser", "caller"],
      recommended: "parser",
    },
    "1.1",
  )
  expect(pick).toEqual({
    label: "1.1",
    title: "Where?",
    body: "",
    kind: "options",
    options: [
      { id: "parser", label: "parser" },
      { id: "caller", label: "caller" },
    ],
    recommended: "parser",
  })
  expect(
    cardOf(
      { id: "q2", title: "Keep it?", kind: "quick", options: ["yes", "no"], recommended: "no" },
      "1.2",
    ).kind,
  ).toBe("yes-no")
  expect(cardOf({ id: "d1", title: "Which store?", kind: "design" }, "1.3").kind).toBe("text")
  const row: TriageRow[] = [
    {
      address: "gx:follow-ups/01",
      title: "a",
      questions: [{ id: "q1", title: "t", kind: "quick", options: ["x", "y"] }],
      answers: { q1: "y" },
    },
  ]
  expect(answersOf(row)).toEqual({ "1.1": { choice: "y" } })
})

test("agreeWithRecommendations fills unanswered cards with a recommendation and keeps given answers", () => {
  const round = roundOf(rows)
  const given = { "1.2": { choice: "ready" } }
  const out = agreeWithRecommendations(round, given)
  expect(out["1.1"]).toEqual({ choice: "no" })
  expect(out["1.2"]).toEqual({ choice: "ready" })
})

test("auto questions leave the round for the decided list until reopened", () => {
  const auto: TriageRow[] = [
    {
      address: "gx:follow-ups/3",
      title: "c",
      questions: [
        { id: "q1", title: "Ask?", kind: "yes-no", recommended: "yes" },
        { id: "q2", title: "Decided?", kind: "yes-no", recommended: "no", auto: true },
      ],
    },
  ]
  expect(roundOf(auto).map((c) => c.label)).toEqual(["3.1"])
  expect(decidedOf(auto).map((c) => c.label)).toEqual(["3.2"])
  expect(roundOf(auto, new Set(["3.2"])).map((c) => c.label)).toEqual(["3.1", "3.2"])
  expect(decidedOf(auto, new Set(["3.2"]))).toEqual([])
})
