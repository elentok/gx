import { cleanup, fireEvent, screen } from "@testing-library/react";
import { afterEach, expect, test } from "vitest";
import canned from "../canned-detail.json";
import type { TicketDetail } from "../generated/server";
import { renderThemed } from "../ui/render";
import { sentOf } from "./answer";
import { AnswerResult } from "./AnswerResult";

afterEach(cleanup);

const base = canned as TicketDetail;
const seq = base.latest!.seq;
const answeredWith = (answer: object, extra: object = {}): TicketDetail => ({
  ...base,
  answered: true,
  actions: [],
  latest: { ...base.latest!, handoff: undefined, outcome: "answered", answer: { source: "web", ...answer }, ...extra },
  chat: [...base.chat, { from: "web", text: (answer as { choice?: string }).choice ?? "yes" }],
});
const answered = answeredWith({ choice: "yes" });
const sentAs = (outcome: string, queue?: { address: string }[]) => sentOf(seq, base.address, { outcome, queue });
const status = () => screen.queryByRole("status", { name: "Answer result" });

test.each([
  ["delivered", undefined, "Delivered ·", "Agent is working", "running"],
  ["queued", [{ address: "gx:x/1" }, { address: base.address }], "Queued ·", "#2 in queue", "waiting"],
  ["adopted", undefined, "Sent ·", "The agent had already finished", "done"],
])("%s outcome gets its own words and tone", (outcome, queue, head, tail, tone) => {
  renderThemed(<AnswerResult detail={answered} sent={sentAs(outcome, queue)} />);
  expect(status()!.textContent).toBe(`${head}${tail}`);
  expect(screen.getByTestId("status-dot").getAttribute("data-tone")).toBe(tone);
  expect(screen.queryByText("SENT")).toBeNull();
  expect(screen.queryByText("Answered")).toBeNull();
});

test("the answered card has no AGENT or WEB bubble", () => {
  renderThemed(<AnswerResult detail={answered} sent={sentAs("delivered")} />);
  expect(screen.queryByText("AGENT")).toBeNull();
  expect(screen.queryByText("WEB")).toBeNull();
  expect(screen.queryByText("SENT")).toBeNull();
  expect(screen.queryByText("Answered")).toBeNull();
});

test("the card reads label, detail, question, answer in order, with the answer once", () => {
  renderThemed(<AnswerResult detail={answered} sent={sentAs("delivered")} />);
  const text = document.body.textContent!;
  const order = ["THE AGENT ASKED", "Should the cache live", "Which backend?", "You", "yes", "Delivered"].map((s) => text.indexOf(s));
  expect(order.every((i) => i >= 0)).toBe(true);
  expect(order).toEqual([...order].sort((a, b) => a - b));
  expect(screen.getAllByText("yes")).toHaveLength(1);
});

test("a gx question is labelled GX ASKED", () => {
  renderThemed(<AnswerResult detail={answeredWith({ choice: "yes" }, { by: "gx" })} sent={null} />);
  expect(screen.getByText("GX ASKED")).toBeTruthy();
});

test("the note shows under the answer; a note-only answer reads no option picked", () => {
  const noteOnly: TicketDetail = { ...answeredWith({ note: "neither" }), chat: base.chat };
  renderThemed(<AnswerResult detail={noteOnly} sent={sentAs("delivered")} />);
  expect(screen.getByText("no option picked")).toBeTruthy();
  expect(screen.getByText("neither")).toBeTruthy();
});

test("the answer is the last web message when the chat has several exchanges", () => {
  const detail: TicketDetail = {
    ...answered,
    chat: [
      { from: "agent", text: "q1" },
      { from: "web", text: "Huh?" },
      { from: "agent", text: "explained" },
      { from: "web", text: "Redis" },
    ],
  };
  renderThemed(<AnswerResult detail={detail} sent={null} />);
  expect(screen.getByText("Redis")).toBeTruthy();
  expect(screen.queryByText("Huh?")).toBeNull();
});

test("an adopted send shows the card with what was sent, though the round records no answer", () => {
  const unanswered: TicketDetail = { ...base, chat: [], latest: { ...base.latest!, answer: undefined } };
  renderThemed(<AnswerResult detail={unanswered} sent={sentOf(seq + 1, base.address, { outcome: "adopted" }, "go on")} />);
  expect(screen.getByText("go on")).toBeTruthy();
  expect(status()!.textContent).toBe("Sent ·The agent had already finished");
  cleanup();
  renderThemed(<AnswerResult detail={unanswered} sent={sentAs("delivered")} />);
  expect(screen.queryByRole("group", { name: "Answered question" })).toBeNull();
});

test("no status line without a send outcome, and none for another round", () => {
  renderThemed(<AnswerResult detail={answered} sent={null} />);
  expect(status()).toBeNull();
  cleanup();
  renderThemed(<AnswerResult detail={answered} sent={{ seq: seq + 1, outcome: "delivered" }} />);
  expect(status()).toBeNull();
});

test("prompt shows unless it repeats the question; closed_reason stays muted", () => {
  const detail = answeredWith({ choice: "yes" }, { prompt: "Still waiting on you", closed_reason: "superseded" });
  renderThemed(<AnswerResult detail={detail} sent={sentAs("delivered")} />);
  expect(screen.getByText("Still waiting on you")).toBeTruthy();
  expect(screen.getByText("Closed: superseded")).toBeTruthy();
  cleanup();
  renderThemed(<AnswerResult detail={answeredWith({ choice: "yes" }, { prompt: "Which backend?" })} sent={null} />);
  expect(screen.getAllByText("Which backend?")).toHaveLength(1);
});

test("a short detail without notes has no toggle; notes add one that reveals them", () => {
  renderThemed(<AnswerResult detail={answered} sent={null} />);
  expect(screen.queryByRole("button")).toBeNull();
  cleanup();
  const withNotes = answeredWith({ choice: "yes" }, { handoff: "I compared both." });
  renderThemed(<AnswerResult detail={withNotes} sent={null} />);
  expect(screen.queryByText("I compared both.")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: /Show more/ }));
  expect(screen.getByText("I compared both.")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: /Show less/ }));
  expect(screen.queryByText("I compared both.")).toBeNull();
});

test("a cut-off detail gets the toggle", () => {
  const spy = (name: "scrollHeight" | "clientHeight", value: number) =>
    Object.defineProperty(HTMLElement.prototype, name, { configurable: true, get: () => value });
  spy("scrollHeight", 100);
  spy("clientHeight", 40);
  try {
    renderThemed(<AnswerResult detail={answered} sent={null} />);
    expect(screen.getByRole("button", { name: /Show more/ })).toBeTruthy();
  } finally {
    delete (HTMLElement.prototype as unknown as Record<string, unknown>).scrollHeight;
    delete (HTMLElement.prototype as unknown as Record<string, unknown>).clientHeight;
  }
});

test("no undo or withdraw control exists after a send", () => {
  renderThemed(<AnswerResult detail={answered} sent={sentAs("queued")} />);
  expect(screen.queryAllByRole("button")).toHaveLength(0);
  expect(screen.queryByText(/undo|withdraw/i)).toBeNull();
});
