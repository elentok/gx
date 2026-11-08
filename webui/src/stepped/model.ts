import type { EpicDetail } from "../epic/detail";
import type { EpicDetailRow, MapStep } from "../generated/server";

// The generated file is imported for types only: its tygo constants are not runtime values.
export const RESOLVE_TICKETS = "resolve-tickets";
const DONE = "done";

export type Flow = "design" | "triage";

/** What the step line, Progress tree, steps sheet and Continue read: one shape for every stepped flow. */
export interface SteppedModel {
  flow: Flow;
  /** A Design's map address, a Triage's project. */
  key: string;
  steps: MapStep[];
  /** 0 to 100. */
  ringPercent: number;
  done: boolean;
  current?: MapStep;
  /** Continue's subtitle: the next ticket while tickets resolve, else the current step. */
  nextTitle?: string;
  /** The map's tickets while Resolve tickets is the current step (Design only); else empty. */
  tickets: EpicDetailRow[];
}

/** A Triage payload's steps, as a model. Its own payload arrives in a later ticket. */
export function triageModel(input: { key: string; steps: MapStep[]; ringPercent?: number }): SteppedModel {
  const current = input.steps.find((s) => s.state === "current");
  return {
    flow: "triage",
    key: input.key,
    steps: input.steps,
    ringPercent: input.ringPercent ?? 0,
    done: input.steps.some((s) => s.step === DONE && s.state === "done"),
    current,
    nextTitle: current?.label,
    tickets: [],
  };
}

/** A Design's model from its map's epic detail. A detail from an older server has no steps. */
export function designModel(detail: EpicDetail): SteppedModel {
  const steps = detail.steps ?? [];
  const current = steps.find((s) => s.state === "current");
  const resolving = current?.step === RESOLVE_TICKETS;
  return {
    flow: "design",
    key: detail.address,
    steps,
    ringPercent: detail.ring_progress ?? 0,
    done: steps.some((s) => s.step === DONE && s.state === "done"),
    current,
    nextTitle: (resolving && detail.rows.find((r) => r.current)?.title) || current?.label,
    tickets: resolving ? detail.rows : [],
  };
}
