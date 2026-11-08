import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { useState } from "react";
import { afterEach, expect, test, vi } from "vitest";
import { HUH_MESSAGE } from "../design/round";
import type { TriageView } from "../generated/server";
import { renderThemed as render } from "../ui/render";
import { QuestionsView } from "./QuestionsView";

afterEach(cleanup);

const viewOf = (over: Partial<TriageView> = {}): TriageView => ({
  project: "gx",
  state: "live",
  flow: "triage",
  steps: [],
  ring_percent: 50,
  revision: 4,
  rows: [
    {
      address: "gx:follow-ups/1",
      title: "a",
      questions: [{ id: "1", title: "Ship it?", kind: "yes-no", recommended: "yes" }],
    },
  ],
  open_questions: 1,
  action_counts: { ready: 1 },
  changed_rows: 0,
  destinations: [],
  can_abandon: false,
  history: [],
  ...over,
});

test("picking a choice posts the row save with the revision, and the view refetches", async () => {
  const save = vi.fn().mockResolvedValue({ revision: 5 });
  const refetch = vi.fn();
  render(<QuestionsView project="gx" view={viewOf()} refetch={refetch} save={save} />);
  screen.getByText("0 of 1 answered");
  screen.getByText("★ recommended");
  fireEvent.click(screen.getByRole("button", { name: "Yes" }));
  await waitFor(() => expect(refetch).toHaveBeenCalled());
  expect(save).toHaveBeenCalledWith(expect.objectContaining({ project: "gx", revision: 4, address: "gx:follow-ups/1", answers: { "1": "yes" } }));
  screen.getByText("1 of 1 answered");
});

test("a stale refusal refetches and the reloaded answers replace the draft", async () => {
  const save = vi.fn().mockResolvedValue({ refused: true, reason: "stale" });
  const refetch = vi.fn();
  const fresh = viewOf({ revision: 9 });
  fresh.rows[0].answers = { "1": "no" };
  // The page stand-in: the refetch lands the elsewhere-changed view.
  function Harness() {
    const [view, setView] = useState(viewOf());
    return <QuestionsView project="gx" view={view} refetch={() => { refetch(); setView(fresh); }} save={save} />;
  }
  render(<Harness />);
  fireEvent.click(screen.getByRole("button", { name: "Yes" }));
  await waitFor(() => expect(refetch).toHaveBeenCalledTimes(1));
  await waitFor(() => expect(screen.getByRole("button", { name: "No" }).getAttribute("aria-pressed")).toBe("true"));
});

test("Huh? is saved as the answer", async () => {
  const save = vi.fn().mockResolvedValue({ revision: 5 });
  render(<QuestionsView project="gx" view={viewOf()} refetch={() => {}} save={save} />);
  fireEvent.click(screen.getByText("Huh?"));
  await waitFor(() => expect(save).toHaveBeenCalled());
  expect(save.mock.calls[0][0].answers).toEqual({ "1": HUH_MESSAGE });
});

test("the page note says where answers go", () => {
  render(<QuestionsView project="gx" view={viewOf()} refetch={() => {}} save={vi.fn()} />);
  screen.getByText("Answers go into each ticket body when the Write step runs.");
});
