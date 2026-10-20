import { cleanup, screen } from "@testing-library/react";
import { afterEach, expect, test } from "vitest";
import canned from "../canned-detail.json";
import type { TicketDetail } from "../generated/server";
import { renderThemed } from "../ui/render";
import { sentOf } from "./answer";
import { AnswerResult } from "./AnswerResult";

afterEach(cleanup);

const base = canned as TicketDetail;
const seq = base.latest!.seq;
const answered: TicketDetail = {
  ...base,
  answered: true,
  actions: [],
  latest: { ...base.latest!, outcome: "answered", answer: { source: "web", choice: "yes", note: "go" } },
};

test("delivered says delivered with the working chip", () => {
  renderThemed(<AnswerResult detail={answered} sent={sentOf(seq, base.address, { outcome: "delivered" })} />);
  expect(screen.getByText("Answer sent · delivered")).toBeTruthy();
  expect(screen.getByText("Agent is working")).toBeTruthy();
});

test("queued shows the amber answered state, the answer and the queue position", () => {
  const sent = sentOf(seq, base.address, { outcome: "queued", queue: [{ address: "gx:x/1" }, { address: base.address }] });
  renderThemed(<AnswerResult detail={answered} sent={sent} />);
  expect(screen.getByText("Answered").getAttribute("data-tone")).toBe("waiting");
  expect(screen.getByText("Answer sent · queued, #2 in queue")).toBeTruthy();
  expect(screen.getByText("yes")).toBeTruthy();
  expect(screen.getByText("go")).toBeTruthy();
});

test("adopted says the agent had already finished", () => {
  renderThemed(<AnswerResult detail={answered} sent={sentOf(seq, base.address, { outcome: "adopted" })} />);
  expect(screen.getByText("Answer sent · the agent had already finished")).toBeTruthy();
});

test("no undo or withdraw control exists after a send", () => {
  renderThemed(<AnswerResult detail={answered} sent={sentOf(seq, base.address, { outcome: "queued" })} />);
  expect(screen.queryAllByRole("button")).toHaveLength(0);
  expect(screen.queryByText(/undo|withdraw/i)).toBeNull();
});

test("the line goes away once the agent asks again", () => {
  const next = { ...answered, latest: { ...answered.latest!, seq: seq + 1 } };
  renderThemed(<AnswerResult detail={next} sent={sentOf(seq, base.address, { outcome: "delivered" })} />);
  expect(screen.queryByRole("status")).toBeNull();
});
