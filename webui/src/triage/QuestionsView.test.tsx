import { act, cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { useState } from "react";
import { afterEach, expect, test, vi } from "vitest";
import type { TriageView } from "../generated/server";
import { renderThemed as render } from "../ui/render";
import { QuestionsView } from "./QuestionsView";
import { HUH_MESSAGE } from "./questions";

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
  epic_actions: ["ready", "map"],
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

test("typing in one row's q1 saves that row only, though every row has a q1", async () => {
  const save = vi.fn().mockResolvedValue({ revision: 5 });
  const rows = ["01", "02", "03"].map((n) => ({
    address: `gx:follow-ups/${n}`,
    title: `row ${n}`,
    questions: [{ id: "q1", title: `Q for ${n}?`, kind: "text" }],
  }));
  render(<QuestionsView project="gx" view={viewOf({ rows, open_questions: 3 })} refetch={vi.fn()} save={save} />);
  screen.getByText("0 of 3 answered");
  fireEvent.change(screen.getByRole("textbox", { name: "Answer to question 1.1" }), { target: { value: "agree" } });
  await waitFor(() => expect(save).toHaveBeenCalled());
  expect(save.mock.calls.map((c) => [c[0].address, c[0].answers])).toEqual([["gx:follow-ups/01", { q1: "agree" }]]);
  screen.getByText("1 of 3 answered");
});

test("after the record changes elsewhere, the next edit builds on the new answers, not the replaced draft", async () => {
  const save = vi.fn().mockResolvedValue({ revision: 9 });
  const rows = ["01", "02"].map((n) => ({
    address: `gx:follow-ups/${n}`,
    title: `row ${n}`,
    questions: [{ id: "q1", title: `Q ${n}?`, kind: "text" }],
  }));
  const view = (revision: number, answers: Record<string, string>) => viewOf({ revision, rows: rows.map((r) => ({ ...r, answers: r.address.endsWith("01") ? answers : undefined })), open_questions: 2 });
  let elsewhere: () => void = () => {};
  function Harness() {
    const [v, setV] = useState(view(4, {}));
    elsewhere = () => setV(view(7, {}));
    return <QuestionsView project="gx" view={v} refetch={vi.fn()} save={save} />;
  }
  render(<Harness />);
  fireEvent.change(screen.getByRole("textbox", { name: "Answer to question 1.1" }), { target: { value: "stale draft" } });
  await waitFor(() => expect(save).toHaveBeenCalledTimes(1));
  save.mockClear();
  // Someone else clears row 01 (revision 7, not ours); the draft must not come back with the next edit.
  act(() => elsewhere());
  fireEvent.click(screen.getByRole("button", { name: "Next →" }));
  fireEvent.change(screen.getByRole("textbox", { name: "Answer to question 2.1" }), { target: { value: "two" } });
  await waitFor(() => expect(save).toHaveBeenCalled());
  expect(save.mock.calls.map((c) => [c[0].address, c[0].answers])).toEqual([["gx:follow-ups/02", { q1: "two" }]]);
});

test("typing a note keeps every character while saves and reloads cross", async () => {
  // A stand-in server: each save bumps the revision; a refetch answers with the state it had when asked, a beat later.
  const server = { revision: 4, answers: { "1": "yes" } as Record<string, string> };
  const rowWith = (answers: Record<string, string>) => ({
    address: "gx:follow-ups/1",
    title: "a",
    questions: [{ id: "1", title: "Ship it?", kind: "yes-no", recommended: "yes" }],
    answers,
  });
  const snapshot = () => viewOf({ revision: server.revision, rows: [rowWith({ ...server.answers })] });
  const save = vi.fn(async (body: { answers: Record<string, string> }) => {
    await new Promise((r) => setTimeout(r, 15));
    server.answers = { ...body.answers };
    return { revision: ++server.revision };
  });
  let setView: (v: TriageView) => void = () => {};
  function Harness() {
    const [view, set] = useState(snapshot());
    setView = set;
    const refetch = () => {
      const seen = snapshot();
      setTimeout(() => setView(seen), 40);
    };
    return <QuestionsView project="gx" view={view} refetch={refetch} save={save as never} />;
  }
  render(<Harness />);
  fireEvent.click(screen.getByRole("button", { name: "Note on question 1.1" }));
  // Typing appends to what is on screen, as a person does.
  for (const ch of "remember the budget") {
    const box = screen.getByRole("textbox", { name: "Note text for question 1.1" }) as HTMLTextAreaElement;
    fireEvent.change(box, { target: { value: box.value + ch } });
    await new Promise((r) => setTimeout(r, 12));
  }
  await new Promise((r) => setTimeout(r, 1500));
  expect((screen.getByRole("textbox", { name: "Note text for question 1.1" }) as HTMLTextAreaElement).value).toBe("remember the budget");
  expect(server.answers["1"]).toBe("yes\nNote: remember the budget");
});

test("each question links to its ticket, in a new tab", () => {
  render(<QuestionsView project="gx" view={viewOf()} refetch={vi.fn()} save={vi.fn()} />);
  const link = screen.getByRole("link", { name: "Open ticket: 1 · a" });
  expect(link.getAttribute("href")).toBe("#/t/gx:follow-ups/1");
  expect(link.getAttribute("target")).toBe("_blank");
});

test("Send back is off until every question is answered", () => {
  render(<QuestionsView project="gx" view={viewOf()} refetch={vi.fn()} save={vi.fn()} sendBack={vi.fn()} />);
  expect((screen.getByRole("button", { name: "Send back to the agent" }) as HTMLButtonElement).disabled).toBe(true);
  screen.getByText("Answer every question to send them back.");
});

test("Send back saves what is on screen first, then sends the revision that save produced", async () => {
  const calls: string[] = [];
  const save = vi.fn(async () => {
    calls.push("save");
    return { revision: 5 };
  });
  const sendBack = vi.fn(async (body: { revision: number }) => {
    calls.push(`send-back@${body.revision}`);
    return { revision: 6 };
  });
  const refetch = vi.fn();
  const view = viewOf({ open_questions: 0 });
  view.rows[0].answers = { "1": "no" };
  render(<QuestionsView project="gx" view={view} refetch={refetch} save={save} sendBack={sendBack} />);
  // A change not saved yet: pressing Yes saves at once, and Send back waits for that save.
  fireEvent.click(screen.getByRole("button", { name: "Yes" }));
  fireEvent.click(screen.getByRole("button", { name: "Send back to the agent" }));
  await waitFor(() => expect(sendBack).toHaveBeenCalled());
  expect(calls).toEqual(["save", "send-back@5"]);
  expect(sendBack).toHaveBeenCalledWith({ project: "gx", revision: 5 });
});

test("a refused send back says why and reloads", async () => {
  const sendBack = vi.fn().mockResolvedValue({ refused: true, reason: "triage-phase", message: "send back is refused: 1 questions are still open" });
  const refetch = vi.fn();
  const view = viewOf({ open_questions: 0 });
  view.rows[0].answers = { "1": "yes" };
  render(<QuestionsView project="gx" view={view} refetch={refetch} save={vi.fn()} sendBack={sendBack} />);
  fireEvent.click(screen.getByRole("button", { name: "Send back to the agent" }));
  await waitFor(() => expect(refetch).toHaveBeenCalled());
  expect(sendBack).toHaveBeenCalledWith({ project: "gx", revision: 4 });
});
