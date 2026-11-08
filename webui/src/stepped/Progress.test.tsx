import { cleanup, screen, within } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import type { MapStep } from "../generated/server";
import { renderThemed } from "../ui/render";
import { designModel, triageModel } from "./model";
import { Progress, StepsSheet, summaryLine } from "./Progress";

afterEach(cleanup);

const stepsAt = (names: string[], current: number): MapStep[] =>
  names.map((step, i) => ({ step, label: step.toUpperCase(), state: i < current ? "done" : i === current ? "current" : "todo" }));

const DESIGN = ["chart", "resolve-tickets", "spec", "review", "grill-findings", "create-tickets", "branch", "done"];
const TRIAGE = ["verify", "plan", "approve", "write", "done"];

const triage = () => triageModel({ key: "gx", steps: stepsAt(TRIAGE, 2) });

it("renders a Triage's five steps and marks the current one", () => {
  renderThemed(<Progress model={triage()} />);
  expect(screen.getAllByRole("listitem")).toHaveLength(5);
  const current = screen.getAllByRole("listitem").filter((li) => li.dataset.state === "current");
  expect(current.map((li) => li.dataset.step)).toEqual(["approve"]);
});

it("renders a Design's eight steps", () => {
  const model = designModel({
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
    steps: stepsAt(DESIGN, 0),
  });
  renderThemed(<Progress model={model} />);
  expect(screen.getAllByRole("listitem")).toHaveLength(8);
});

it("shows the flow's slot only under Resolve tickets, and only when given", () => {
  const model = designModel({
    address: "gx:a",
    kind: "map",
    summary: "",
    done: 0,
    total: 0,
    agent_time: 0,
    cost_total: 0,
    stats: {},
    rows: [{ address: "gx:a/03", title: "Chat content", rendered_status: "open", current: true }],
    queueable: false,
    queued: false,
    steps: stepsAt(DESIGN, 1),
  });
  const { unmount } = renderThemed(<Progress model={model} />);
  expect(screen.queryByTestId("slot")).toBeNull();
  unmount();
  renderThemed(<Progress model={model} rows={<span data-testid="slot" />} />);
  const li = screen.getByTestId("slot").closest("li")!;
  expect(li.dataset.step).toBe("resolve-tickets");
  expect(summaryLine(model)).toBe("RESOLVE-TICKETS · 03 Chat content (1 of 1)");
});

it("labels the steps sheet by flow", () => {
  renderThemed(<StepsSheet model={triage()} ring={null} onClose={() => {}} />);
  const dialog = screen.getByRole("dialog");
  expect(within(dialog).getByText(/Progress · 0%/)).toBeTruthy();
  expect(dialog.getAttribute("aria-label")).toBe("Triage progress steps");
});
