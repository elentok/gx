import { expect, test } from "vitest";
import type { EpicDetail, EpicDetailRow } from "./detail";
import { cardLook, type CardLook, cardMeta, designStatParts, loopStatParts, nestLive, rowChips, rowContext } from "./rows";

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
  ["waiting, follows the default", { harness_note: "follows default" }, { harness: "", tierLabel: "", model: "", effort: "", note: "follows default" }],
  ["waiting, pinned", { harness: "codex", harness_note: "pinned" }, { harness: "codex", tierLabel: "", model: "", effort: "", note: "pinned" }],
  [
    "running",
    { rendered_status: "claimed", harness: "codex", tier: "strongest", model: "gpt-x", effort: "high" },
    { harness: "codex", tierLabel: "Strongest", model: "gpt-x", effort: "high", note: "" },
  ],
  ["a done row", { rendered_status: "done", harness: "codex", harness_note: "pinned" }, undefined],
  ["a map row with no fields", {}, undefined],
])("rowChips: %s", (_name, over, want) => {
  expect(rowChips(row(over))).toEqual(want);
});

test.each([
  ["an open row with no blockers", { rendered_status: "open" }, "ready"],
  ["a blocked row", { rendered_status: "blocked", blockers: ["01", "02"] }, "after 01 and 02"],
  ["a row blocked by three", { rendered_status: "blocked", blockers: ["01", "02", "03"] }, "after 01, 02 and 03"],
  ["a row blocked by more than three", { rendered_status: "blocked", blockers: ["01", "02", "03", "04", "05"] }, "after 01, 02, 03 + 2 more"],
  ["a paused row keeps its reason and no link word", { rendered_status: "paused", reason: "needs a person" }, "paused · needs a person"],
  ["a failed row", { rendered_status: "error" }, "error"],
  ["a running row", { rendered_status: "claimed", elapsed: 180, cost: 0.4 }, "running · 3m · $0.40"],
  ["a running row without cost", { rendered_status: "claimed", elapsed: 180 }, "running · 3m"],
  ["a running fork whose parent is done", { rendered_status: "claimed", elapsed: 180, cost: 0.4, parent: "gx:e/07" }, "running · 3m · $0.40 · fork of 07"],
  ["a needs-answer row", { rendered_status: "needs-answer", needs_answer: true, elapsed: 90, cost: 1.5 }, "needs answer"],
  ["a needs-repair row", { rendered_status: "needs-repair", reason: "Run failed" }, "needs-repair · Run failed"],
  ["a fork whose parent is not drawn", { rendered_status: "open", parent: "gx:e/01" }, "ready · fork of 01"],
] as [string, Partial<EpicDetailRow>, string][])("loop meta of %s", (_n, over, text) => {
  const r = row(over);
  expect(cardMeta(r, rowContext(detailOf("loop", [r]), NOW))).toBe(text);
});

test.each([
  ["ready", { type: "prototype" }, false, "prototype · ready"],
  ["blocked", { type: "grilling", rendered_status: "blocked", blockers: ["04"] }, false, "grilling · after 04"],
  ["needs answer", { type: "spec", rendered_status: "needs-answer", needs_answer: true }, false, "needs answer"],
  ["a running row with a live session", { type: "spec", rendered_status: "claimed", claimed_at: "2026-01-01T00:00:00Z" }, true, "spec · running · 10m"],
  ["a running row with no live session", { type: "spec", rendered_status: "claimed", claimed_at: "2026-01-01T00:00:00Z" }, false, "spec · running"],
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

test("live rows are in ticket number order, so 9 comes before 10", () => {
  const rows = ["01", "10", "02", "09"].map((id) => row({ address: `gx:e/01${id === "01" ? "" : `i${Number(id)}`}`, parent: id === "01" ? undefined : "gx:e/01" }));
  expect(nestLive(rows).map((r) => r.row.address)).toEqual(["gx:e/01", "gx:e/01i2", "gx:e/01i9", "gx:e/01i10"]);
});
