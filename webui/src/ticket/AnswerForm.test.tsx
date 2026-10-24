import { ChakraProvider } from "@chakra-ui/react";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import canned from "../canned-detail.json";
import type { DetailRound, TicketDetail } from "../generated/server";
import { renderThemed } from "../ui/render";
import { system } from "../ui/theme";
import type { AnswerBody, PostAnswer } from "./answer";
import { AnswerForm } from "./AnswerForm";

afterEach(cleanup);

const base = canned as TicketDetail;
const withLatest = (latest: Partial<DetailRound>): TicketDetail => ({
  ...base,
  latest: { ...base.latest!, ...latest },
});
const ok: PostAnswer = async () => ({ outcome: "answered" });

test("yes-no renders Yes and No with nothing selected, and posts the choice, state and note", async () => {
  const post = vi.fn<(b: AnswerBody) => ReturnType<PostAnswer>>(ok);
  renderThemed(<AnswerForm detail={withLatest({ kind: "yes-no" })} post={post} />);
  const yes = screen.getByRole("button", { name: "Yes" });
  expect(yes.getAttribute("aria-pressed")).toBe("false");
  expect(screen.getByRole("button", { name: "No" }).getAttribute("aria-pressed")).toBe("false");
  fireEvent.click(yes);
  fireEvent.change(screen.getByLabelText("Note (optional)"), { target: { value: "go" } });
  fireEvent.click(screen.getByRole("button", { name: "Send answer" }));
  await waitFor(() => expect(post).toHaveBeenCalled());
  expect(post).toHaveBeenCalledWith({ address: "gx:demo/12", state: "tok-1", choice: "yes", note: "go" });
});

test("options show consequences and the Recommended badge on the suggested one only; Send sends nothing until picked", () => {
  const post = vi.fn(ok);
  const detail = withLatest({
    kind: "options",
    recommended: "b",
    options: [
      { id: "a", label: "Redis", consequence: "Survives restarts" },
      { id: "b", label: "Memory", consequence: "Loses data" },
    ],
  });
  renderThemed(<AnswerForm detail={detail} post={post} />);
  expect(screen.getAllByText("Recommended")).toHaveLength(1);
  expect(screen.getByRole("button", { name: /Memory/ }).textContent).toContain("Recommended");
  expect(screen.getByText("Survives restarts")).toBeTruthy();
  for (const b of screen.getAllByRole("button", { name: /Redis|Memory/ })) {
    expect(b.getAttribute("aria-pressed")).toBe("false");
  }
  const send = screen.getByRole("button", { name: "Send answer" }) as HTMLButtonElement;
  expect(send.disabled).toBe(true);
  fireEvent.click(send);
  expect(post).not.toHaveBeenCalled();
});

test("text shows no note box: the typed text is the answer", () => {
  renderThemed(<AnswerForm detail={withLatest({ kind: "text" })} post={vi.fn(ok)} />);
  expect(screen.getByLabelText("Your answer")).toBeTruthy();
  expect(screen.queryByLabelText("Note (optional)")).toBeNull();
});

test("text posts the typed text", async () => {
  const post = vi.fn<(b: AnswerBody) => ReturnType<PostAnswer>>(ok);
  renderThemed(<AnswerForm detail={withLatest({ kind: "text" })} post={post} />);
  fireEvent.change(screen.getByLabelText("Your answer"), { target: { value: "Redis" } });
  fireEvent.click(screen.getByRole("button", { name: "Send answer" }));
  await waitFor(() => expect(post).toHaveBeenCalled());
  expect(post).toHaveBeenCalledWith({ address: "gx:demo/12", state: "tok-1", text: "Redis" });
});

test("controls are disabled while the request runs", async () => {
  let release: () => void = () => {};
  const post: PostAnswer = () => new Promise((r) => (release = () => r({ outcome: "answered" })));
  renderThemed(<AnswerForm detail={withLatest({ kind: "yes-no" })} post={post} />);
  fireEvent.click(screen.getByRole("button", { name: "Yes" }));
  fireEvent.click(screen.getByRole("button", { name: "Send answer" }));
  await waitFor(() => expect((screen.getByRole("button", { name: "No" }) as HTMLButtonElement).disabled).toBe(true));
  expect((screen.getByRole("button", { name: "Send answer" }) as HTMLButtonElement).disabled).toBe(true);
  expect((screen.getByLabelText("Note (optional)") as HTMLTextAreaElement).disabled).toBe(true);
  release();
  await waitFor(() => expect(screen.queryByRole("button", { name: "Send answer" })).toBeNull());
});

const refuse = (reason: string, message: string): PostAnswer => async () => ({ refused: true, reason, message });

async function sendYes(post: PostAnswer, onStale?: () => void) {
  renderThemed(<AnswerForm detail={withLatest({ kind: "yes-no" })} post={post} onStale={onStale} />);
  fireEvent.click(screen.getByRole("button", { name: "Yes" }));
  fireEvent.change(screen.getByLabelText("Note (optional)"), { target: { value: "keep me" } });
  fireEvent.click(screen.getByRole("button", { name: "Send answer" }));
}

test("a refusal shows its message inline and keeps the note", async () => {
  await sendYes(refuse("not-answerable", "Already answered"));
  expect(await screen.findByText("Already answered")).toBeTruthy();
  expect((screen.getByLabelText("Note (optional)") as HTMLTextAreaElement).value).toBe("keep me");
});

test("agent-not-ready tells the person to approve the prompt first", async () => {
  await sendYes(refuse("agent-not-ready", "The agent is not ready."));
  expect((await screen.findByRole("alert")).textContent).toContain("Approve the agent's prompt first");
});

test("stale refetches and keeps the note", async () => {
  const onStale = vi.fn();
  await sendYes(refuse("stale", "the question changed; answer the current one"), onStale);
  await waitFor(() => expect(onStale).toHaveBeenCalled());
  expect((screen.getByLabelText("Note (optional)") as HTMLTextAreaElement).value).toBe("keep me");
});

test("a new question swaps in with the note kept and the pick cleared", async () => {
  const post = refuse("stale", "changed");
  const { rerender } = renderThemed(<AnswerForm detail={withLatest({ kind: "yes-no" })} post={post} />);
  fireEvent.click(screen.getByRole("button", { name: "Yes" }));
  fireEvent.change(screen.getByLabelText("Note (optional)"), { target: { value: "keep me" } });
  rerender(
    <ChakraProvider value={system}>
      <AnswerForm detail={{ ...withLatest({ kind: "yes-no", summary: "New question" }), state: "tok-2" }} post={post} />
    </ChakraProvider>,
  );
  expect(screen.getByRole("button", { name: "Yes" }).getAttribute("aria-pressed")).toBe("false");
  expect((screen.getByLabelText("Note (optional)") as HTMLTextAreaElement).value).toBe("keep me");
});

test("no answer action renders nothing", () => {
  renderThemed(<AnswerForm detail={{ ...withLatest({ kind: "text" }), actions: [] }} post={ok} />);
  expect(screen.queryByRole("textbox")).toBeNull();
});
