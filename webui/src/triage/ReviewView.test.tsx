import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import type { TriageRow, TriageView } from "../generated/server";
import { renderThemed as render } from "../ui/render";
import { ReviewView } from "./ReviewView";

afterEach(cleanup);

const rowOf = (n: number, action: string, over: Partial<TriageRow> = {}): TriageRow => ({
  address: `gx:follow-ups/${n}`,
  title: `row ${n}`,
  action,
  destination: action === "ready" ? "sidebar" : undefined,
  ...over,
});

const viewOf = (over: Partial<TriageView> = {}): TriageView => ({
  project: "gx",
  state: "live",
  flow: "triage",
  steps: [],
  ring_percent: 50,
  revision: 4,
  rows: [rowOf(1, "ready"), rowOf(2, "ready"), rowOf(3, "map", { destination: "queue" }), rowOf(4, "close", { verdict: "fixed" }), rowOf(5, "keep"), rowOf(6, "close", { gone: true })],
  open_questions: 0,
  action_counts: {},
  changed_rows: 0,
  destinations: [
    { kind: "suggested", value: "sidebar", label: "sidebar" },
    { kind: "map-epic", value: "queue", label: "queue" },
    { kind: "new-map-epic", label: "New map epic" },
  ],
  ready_epic: "follow-ups-2026-11-08",
  can_abandon: false,
  history: [],
  ...over,
});

const pick = async (trigger: string, item: string) => {
  fireEvent.click(screen.getByRole("button", { name: trigger }));
  fireEvent.click(await screen.findByRole("menuitem", { name: item }));
};

test("five groups show their counts and notes, and only the ready epic is open", () => {
  render(<ReviewView project="gx" view={viewOf()} refetch={() => {}} save={vi.fn()} />);
  for (const name of ["Ready epic", "Map epics", "Merge into another", "Close", "Keep for later"]) screen.getByText(name);
  screen.getByText("follow-ups-2026-11-08");
  screen.getByText("1 area: queue");
  screen.getByText("fixed 1");
  screen.getByText("row 1");
  expect(screen.queryByText("row 3")).toBeNull();
  expect(screen.queryByText("row 6")).toBeNull();
});

test("a group collapses and expands", () => {
  render(<ReviewView project="gx" view={viewOf()} refetch={() => {}} save={vi.fn()} />);
  fireEvent.click(screen.getByRole("button", { name: /^Ready epic/ }));
  expect(screen.queryByText("row 1")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: /^Map epics/ }));
  screen.getByText("row 3");
});

test("a row action change posts the save with the revision and outlines the row", async () => {
  const save = vi.fn().mockResolvedValue({ revision: 5 });
  const refetch = vi.fn();
  render(<ReviewView project="gx" view={viewOf()} refetch={refetch} save={save} />);
  await pick("Destination for row 1", "queue");
  await waitFor(() => expect(refetch).toHaveBeenCalledTimes(1));
  expect(save).toHaveBeenCalledWith(expect.objectContaining({ project: "gx", revision: 4, address: "gx:follow-ups/1", action: "ready", destination: "queue" }));
  expect(screen.getByText("row 1").closest("[data-changed]")).not.toBeNull();
});

test("Set all changes every row of its group in one run of saves and one refetch", async () => {
  const save = vi.fn().mockResolvedValueOnce({ revision: 5 }).mockResolvedValueOnce({ revision: 6 });
  const refetch = vi.fn();
  render(<ReviewView project="gx" view={viewOf()} refetch={refetch} save={save} />);
  await pick("Set all in Ready epic", "Close");
  await waitFor(() => expect(refetch).toHaveBeenCalledTimes(1));
  expect(save.mock.calls.map(([b]) => [b.address, b.revision, b.action])).toEqual([
    ["gx:follow-ups/1", 4, "close"],
    ["gx:follow-ups/2", 5, "close"],
  ]);
});

test("keep saves as keep_for_later", async () => {
  const save = vi.fn().mockResolvedValue({ revision: 5 });
  render(<ReviewView project="gx" view={viewOf()} refetch={() => {}} save={save} />);
  await pick("Action for row 1", "Keep");
  await waitFor(() => expect(save).toHaveBeenCalled());
  expect(save.mock.calls[0][0]).toMatchObject({ action: undefined, keep_for_later: true });
});

test("a ready row's destinations are the suggested epics, the open map epics and a typed new one", async () => {
  const save = vi.fn().mockResolvedValue({ revision: 5 });
  render(<ReviewView project="gx" view={viewOf()} refetch={() => {}} save={save} />);
  fireEvent.click(screen.getByRole("button", { name: "Destination for row 1" }));
  await screen.findAllByRole("menuitem");
  expect(screen.getAllByRole("menuitem").map((e) => e.textContent)).toEqual(["sidebar", "queue", "New map epic…"]);
  fireEvent.click(screen.getByRole("menuitem", { name: "New map epic…" }));
  fireEvent.change(screen.getByLabelText("New map epic name"), { target: { value: "fresh" } });
  fireEvent.submit(screen.getByLabelText("New map epic name"));
  await waitFor(() => expect(save).toHaveBeenCalled());
  expect(save.mock.calls[0][0]).toMatchObject({ address: "gx:follow-ups/1", action: "ready", destination: "fresh" });
});

test("Evidence shows the verdict details read-only and closes without a save", () => {
  const save = vi.fn();
  const rows = [
    rowOf(4, "close", {
      verdict: "fixed",
      evidence: "commit abc123 fixed it",
      files: ["server/a.go"],
      questions: [{ id: "q1", title: "Is it still needed?", kind: "yes-no" }],
      destination: "sidebar",
    }),
  ];
  render(<ReviewView project="gx" view={viewOf({ rows })} refetch={() => {}} save={save} />);
  fireEvent.click(screen.getByRole("button", { name: /^Close/ }));
  fireEvent.click(screen.getByRole("button", { name: "Evidence" }));
  const dialog = screen.getByRole("dialog", { name: "Evidence for row 4" });
  for (const text of ["fixed", "commit abc123 fixed it", "server/a.go", "Is it still needed?", "sidebar"]) expect(dialog.textContent).toContain(text);
  fireEvent.click(screen.getByRole("button", { name: "Close" }));
  expect(screen.queryByRole("dialog")).toBeNull();
  expect(save).not.toHaveBeenCalled();
});

test("Merge into lists only the ready and map rows", async () => {
  const rows = [rowOf(1, "ready"), rowOf(2, "map", { destination: "queue" }), rowOf(3, "close"), rowOf(4, "keep"), rowOf(5, "merge", { destination: "gx:follow-ups/1" }), rowOf(6, "ready", { gone: true })];
  render(<ReviewView project="gx" view={viewOf({ rows })} refetch={() => {}} save={vi.fn()} />);
  fireEvent.click(screen.getByRole("button", { name: /^Merge into another/ }));
  fireEvent.click(screen.getByRole("button", { name: "Destination for row 5" }));
  await screen.findAllByRole("menuitem");
  expect(screen.getAllByRole("menuitem").map((e) => e.textContent)).toEqual(["row 1", "row 2"]);
});

test("a row merged into a gone target is outlined", () => {
  const rows = [rowOf(5, "merge", { destination: "gx:follow-ups/1", merge_orphan: true })];
  render(<ReviewView project="gx" view={viewOf({ rows })} refetch={() => {}} save={vi.fn()} />);
  fireEvent.click(screen.getByRole("button", { name: /^Merge into another/ }));
  const row = screen.getByText("row 5").closest("[data-orphan]");
  expect(row).not.toBeNull();
  expect(getComputedStyle(row as Element).boxShadow).toContain("tone-deadlocked-fg");
});

test("a stale refusal refetches", async () => {
  const save = vi.fn().mockResolvedValue({ refused: true, reason: "stale" });
  const refetch = vi.fn();
  render(<ReviewView project="gx" view={viewOf()} refetch={refetch} save={save} />);
  await pick("Action for row 1", "Close");
  await waitFor(() => expect(refetch).toHaveBeenCalledTimes(1));
  expect(screen.getByText("row 1").closest("[data-changed]")).toBeNull();
});
