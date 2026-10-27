import { expect, test } from "vitest";
import type { EpicDetail, EpicDetailRow } from "./detail";
import { cardLook, type CardLook, cardMeta, designStatParts, loopStatParts, rowContext } from "./rows";

const detailOf = (kind: EpicDetail["kind"], rows: EpicDetailRow[]): EpicDetail => ({
  address: "gx:e",
  kind,
  summary: "",
  done: 0,
  total: rows.length,
  agent_time: 0,
  cost_total: 0,
  stats: {},
  rows,
  queueable: false,
  queued: false,
});

const row = (over: Partial<EpicDetailRow>): EpicDetailRow => ({ address: "gx:e/03", title: "t", rendered_status: "open", ...over });
const NOW = Date.parse("2026-01-01T00:10:00Z");

test.each([
  ["an open row with no blockers", { rendered_status: "open" }, "ready"],
  ["a blocked row", { rendered_status: "blocked", blockers: ["01", "02"] }, "after 01 and 02"],
  ["a paused row keeps its reason and no link word", { rendered_status: "paused", reason: "needs a person" }, "paused · needs a person"],
  ["a failed row", { rendered_status: "error" }, "error"],
  ["a merged-time row", { rendered_status: "claimed", elapsed: 90, cost: 1.5 }, "claimed · 1m · $1.50"],
  ["a fork whose parent is not drawn", { rendered_status: "open", parent: "gx:e/01" }, "ready · fork of 01"],
] as [string, Partial<EpicDetailRow>, string][])("loop meta of %s", (_n, over, text) => {
  const r = row(over);
  expect(cardMeta(r, rowContext(detailOf("loop", [r]), NOW))).toBe(text);
});

test.each([
  ["ready", { type: "prototype" }, false, "prototype · ready"],
  ["blocked", { type: "grilling", rendered_status: "blocked", blockers: ["04"] }, false, "grilling · after 04"],
  ["the rendered word", { type: "spec", rendered_status: "needs-answer" }, false, "spec · needs-answer"],
  ["a claimed row with a live session", { type: "spec", rendered_status: "claimed", claimed_at: "2026-01-01T00:00:00Z" }, true, "spec · claimed · 10m"],
  ["a claimed row with no live session", { type: "spec", rendered_status: "claimed", claimed_at: "2026-01-01T00:00:00Z" }, false, "spec · claimed"],
] as [string, Partial<EpicDetailRow>, boolean, string][])("Design meta of %s", (_n, over, live, text) => {
  const r = row(over);
  expect(cardMeta(r, rowContext(detailOf("map", [r]), NOW, live))).toBe(text);
});

test.each([
  ["an asking row", { needs_answer: true, question: "Which?" }, { kind: "note", text: "Which?", answerPath: "/t/gx:e/03" }],
  ["a needs-repair row", { rendered_status: "needs-repair" }, { kind: "note", fix: true }],
  ["a running row", { rendered_status: "claimed" }, { kind: "link" }],
] as [string, Partial<EpicDetailRow>, CardLook][])("card look of %s", (_n, over, look) => {
  expect(cardLook(row(over))).toEqual(look);
});

const draftImpl = (rows: EpicDetailRow[]): EpicDetail => ({ ...detailOf("loop", rows), address: "gx:e-impl", draft: true, state: "waiting" });
const nOf = (n: number, status = "open") => Array.from({ length: n }, (_, i) => row({ address: `gx:e-impl/${i}`, rendered_status: status }));
const PUBLISHING = { text: "Publishing tickets", bright: true };

test.each([
  ["no rows", [], [PUBLISHING]],
  ["one row", nOf(1), [PUBLISHING, { text: "1 ticket so far" }]],
  ["seven rows", nOf(7), [PUBLISHING, { text: "7 tickets so far" }]],
  ["cancelled rows left out", [...nOf(2), ...nOf(3, "cancelled")], [PUBLISHING, { text: "2 tickets so far" }]],
] as [string, EpicDetailRow[], unknown[]][])("a draft -impl epic with %s", (_n, rows, parts) => {
  expect(loopStatParts({ ...draftImpl(rows), wall_clock: 60, cost_total: 2, agent_time: 30 })).toEqual(parts);
});

test("a finished epic reads Merged, in the done tone", () => {
  expect(loopStatParts({ ...detailOf("loop", []), state: "finished" })[0]).toEqual({ text: "Merged", tone: "done" });
});

test("the Design step part is bright and has no tone", () => {
  const d = { ...detailOf("map", []), steps: [{ step: "spec", label: "spec", state: "current" }] } as unknown as EpicDetail;
  expect(designStatParts(d, false, NOW)[0]).toEqual({ text: "spec", bright: true });
});
