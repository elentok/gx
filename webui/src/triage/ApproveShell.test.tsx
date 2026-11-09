import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { useState } from "react";
import { afterEach, expect, test, vi } from "vitest";
import type { TriageView } from "../generated/server";
import { renderThemed as render } from "../ui/render";
import { ApproveShell, useApprove, useApproveView } from "./ApproveShell";

afterEach(cleanup);

const viewOf = (over: Partial<TriageView> = {}): TriageView => ({
  project: "gx",
  state: "live",
  flow: "triage",
  steps: [],
  ring_percent: 50,
  revision: 4,
  rows: [
    { address: "gx:follow-ups/1", title: "a" },
    { address: "gx:follow-ups/2", title: "b" },
  ],
  open_questions: 2,
  action_counts: { ready: 2 },
  changed_rows: 0,
  destinations: [],
  epic_actions: ["ready", "map"],
  can_abandon: false,
  history: [],
  ...over,
});

// A page stand-in: the shell with its hooks, as TriageBody wires them.
function Harness({ initial, approve, refetch = () => {} }: { initial: TriageView; approve?: (b: { project: string; revision: number }) => Promise<object>; refetch?: () => void }) {
  const [view, setView] = useState(initial);
  const v = useApproveView(view.revision, view.open_questions);
  const approval = useApprove({ project: "gx", view, refetch, approve, carryReview: v.carryReview });
  return (
    <>
      <button onClick={() => setView({ ...view, revision: (view.revision ?? 0) + 1 })}>new revision</button>
      <button onClick={() => setView({ ...view, open_questions: 0 })}>answered</button>
      <ApproveShell view={view} shown={v.view} onShow={v.setView} approval={approval} questions={<p>QBODY</p>} review={<p>RBODY</p>} />
    </>
  );
}

test("the page opens on Questions while a question is open and on Review once none is", () => {
  render(<Harness initial={viewOf()} />);
  screen.getByText("QBODY");
  cleanup();
  render(<Harness initial={viewOf({ open_questions: 0 })} />);
  screen.getByText("RBODY");
});

test("the switch shows Questions N, a check mark at zero, and Review N rows", () => {
  render(<Harness initial={viewOf()} />);
  screen.getByRole("button", { name: "Questions 2" });
  screen.getByRole("button", { name: "Review 2 rows" });
  cleanup();
  render(<Harness initial={viewOf({ open_questions: 0 })} />);
  screen.getByRole("button", { name: "Questions ✓" });
});

test("a toggle holds for that revision only", () => {
  render(<Harness initial={viewOf()} />);
  fireEvent.click(screen.getByRole("button", { name: "Review 2 rows" }));
  screen.getByText("RBODY");
  fireEvent.click(screen.getByText("answered"));
  screen.getByText("RBODY");
  fireEvent.click(screen.getByText("new revision"));
  screen.getByText("RBODY");
  cleanup();
  render(<Harness initial={viewOf()} />);
  fireEvent.click(screen.getByRole("button", { name: "Review 2 rows" }));
  fireEvent.click(screen.getByText("new revision"));
  screen.getByText("QBODY");
});

test("Answer them in the bar switches to the Questions view", () => {
  render(<Harness initial={viewOf()} />);
  fireEvent.click(screen.getByRole("button", { name: "Review 2 rows" }));
  fireEvent.click(screen.getByRole("button", { name: "Answer them" }));
  screen.getByText("QBODY");
});

test("Approve posts the revision the page read and reloads", async () => {
  const approve = vi.fn(async () => ({}));
  const refetch = vi.fn();
  render(<Harness initial={viewOf({ open_questions: 0 })} approve={approve} refetch={refetch} />);
  fireEvent.click(screen.getByRole("button", { name: "Approve and write" }));
  await waitFor(() => expect(approve).toHaveBeenCalledWith({ project: "gx", revision: 4 }));
  await waitFor(() => expect(refetch).toHaveBeenCalledTimes(1));
});

test("already approved reloads", async () => {
  const approve = vi.fn(async () => ({ refused: true, reason: "triage-already-approved", message: "done" }));
  const refetch = vi.fn();
  render(<Harness initial={viewOf({ open_questions: 0 })} approve={approve} refetch={refetch} />);
  fireEvent.click(screen.getByRole("button", { name: "Approve and write" }));
  await waitFor(() => expect(refetch).toHaveBeenCalledTimes(1));
});

test("a stale refusal reloads and lands on Review even with questions open in the new revision", async () => {
  const approve = vi.fn(async () => ({ refused: true, reason: "stale", message: "stale" }));
  const refetch = vi.fn();
  render(<Harness initial={viewOf({ open_questions: 0 })} approve={approve} refetch={refetch} />);
  fireEvent.click(screen.getByRole("button", { name: "Questions ✓" }));
  screen.getByText("QBODY");
  fireEvent.click(screen.getByRole("button", { name: "Approve and write" }));
  await waitFor(() => expect(refetch).toHaveBeenCalledTimes(1));
  fireEvent.click(screen.getByText("new revision"));
  screen.getByText("RBODY");
});
