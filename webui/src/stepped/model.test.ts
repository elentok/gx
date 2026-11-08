// @vitest-environment node
import { describe, expect, it } from "vitest";
import type { EpicDetail } from "../epic/detail";
import type { EpicDetailRow, MapStep } from "../generated/server";
import { session } from "../agents/agentsTestKit";
import { flowSession } from "../design/view";
import { designModel, triageModel } from "./model";

const DESIGN_STEPS = ["chart", "resolve-tickets", "spec", "review", "grill-findings", "create-tickets", "branch", "done"];
const TRIAGE_STEPS = ["verify", "plan", "approve", "write", "done"];

const stepsAt = (names: string[], current: number): MapStep[] =>
  names.map((step, i) => ({ step, label: step, state: i < current ? "done" : i === current ? "current" : "todo" }));

const row = (n: string, title: string, over: Partial<EpicDetailRow> = {}): EpicDetailRow => ({
  address: `gx:a/${n}`,
  title,
  rendered_status: "open",
  ...over,
});

const detailOf = (over: Partial<EpicDetail> = {}): EpicDetail => ({
  address: "gx:a",
  kind: "map",
  summary: "",
  done: 0,
  total: 0,
  agent_time: 0,
  cost_total: 0,
  stats: {},
  rows: [],
  queueable: false,
  queued: false,
  steps: stepsAt(DESIGN_STEPS, 2),
  ring_progress: 40,
  ...over,
});

describe("designModel", () => {
  it("gives the eight steps with states, the ring percent and the current step", () => {
    const m = designModel(detailOf());
    expect(m.flow).toBe("design");
    expect(m.key).toBe("gx:a");
    expect(m.steps.map((s) => [s.step, s.state])).toEqual(
      DESIGN_STEPS.map((s, i) => [s, i < 2 ? "done" : i === 2 ? "current" : "todo"]),
    );
    expect(m.ringPercent).toBe(40);
    expect(m.current?.step).toBe("spec");
    expect(m.nextTitle).toBe("spec");
    expect(m.done).toBe(false);
    expect(m.tickets).toEqual([]);
  });

  it("names the current map ticket as nextTitle while tickets resolve", () => {
    const rows = [row("01", "Model"), row("03", "Chat content", { current: true })];
    const m = designModel(detailOf({ steps: stepsAt(DESIGN_STEPS, 1), rows }));
    expect(m.nextTitle).toBe("Chat content");
    expect(m.tickets).toEqual(rows);
  });

  it("is done once the done step is done, and tolerates a detail without steps", () => {
    expect(designModel(detailOf({ steps: stepsAt(DESIGN_STEPS, 8).map((s) => ({ ...s, state: "done" })) })).done).toBe(true);
    expect(designModel(detailOf({ steps: undefined })).steps).toEqual([]);
  });
});

describe("triageModel", () => {
  it("gives five steps keyed by project", () => {
    const m = triageModel({ key: "gx", steps: stepsAt(TRIAGE_STEPS, 1), ringPercent: 20 });
    expect(m.flow).toBe("triage");
    expect(m.key).toBe("gx");
    expect(m.steps).toHaveLength(5);
    expect(m.current?.step).toBe("plan");
    expect(m.ringPercent).toBe(20);
    expect(m.done).toBe(false);
  });
});

describe("flowSession", () => {
  const triage = (id: string, over = {}) => session(id, "gx", { flow: "triage", flow_key: "gx", ...over });

  it("returns the newest live session by flow and key", () => {
    const sessions = [
      triage("old", { run_started_at: "2026-01-01T00:00:00Z" }),
      triage("new", { run_started_at: "2026-01-02T00:00:00Z" }),
      triage("dead", { run_started_at: "2026-01-03T00:00:00Z", status: { state: "exited" } }),
      triage("other", { flow_key: "other" }),
    ];
    expect(flowSession("triage", "gx", sessions)?.id).toBe("new");
  });

  it("falls back to the newest Exited session", () => {
    const sessions = [
      triage("a", { status: { state: "exited" }, status_changed_at: "2026-01-01T00:00:00Z" }),
      triage("b", { status: { state: "exited" }, status_changed_at: "2026-01-02T00:00:00Z" }),
    ];
    expect(flowSession("triage", "gx", sessions)?.id).toBe("b");
  });

  it("never matches a Design session for a Triage key", () => {
    const design = session("d", "gx", { design: true, flow: "design", flow_key: "gx:a", ticket: "gx:a/01" });
    expect(flowSession("triage", "gx", [design])).toBeUndefined();
    expect(flowSession("triage", "gx:a", [design])).toBeUndefined();
  });
});
