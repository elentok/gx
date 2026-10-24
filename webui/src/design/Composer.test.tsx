import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { afterEach, expect, test, vi } from "vitest";
import { ConnectionContext, type Connection, type ConnectionState } from "../connection";
import type { EpicDetailRow, SessionChatResponse, SessionInfo, SessionResult, SessionRoundCard } from "../generated/server";
import { appRoutes } from "../routes";
import { renderThemed } from "../ui/render";
import type { FetchChat } from "./chat";
import type { PostPrompt } from "./prompt";

afterEach(cleanup);

const session = (state = "idle", background = false): SessionInfo => ({
  id: "s-a",
  project: "gx",
  dir: "/x",
  harness: "claude",
  ticket: "gx:a/01",
  design: true,
  status: { state, background } as SessionInfo["status"],
  status_changed_at: new Date().toISOString(),
  run_started_at: new Date().toISOString(),
  unread: {},
  actionable: {},
});

const chatOf = (over: Partial<SessionChatResponse> = {}): SessionChatResponse => ({
  conversation_id: "c1",
  messages: [{ id: "m9", role: "agent", kind: "text", text: "hi", at: new Date().toISOString() }],
  // The server sends null, not [], when the last turn holds no round.
  round: null,
  background: [],
  ...over,
});

const nextOf = (phase: string): SessionChatResponse["next"] => ({ phase, action: {} });

const ok: SessionResult = { session: session() };

function render(opts: {
  state?: string;
  background?: boolean;
  chat?: SessionChatResponse;
  connected?: boolean;
  post?: PostPrompt;
  fetchChat?: FetchChat;
  rows?: EpicDetailRow[];
}) {
  const state = {
    snapshot: { sessions: [session(opts.state, opts.background)] },
    connected: opts.connected ?? true,
    loadedOnce: true,
    tick: 0,
  } as unknown as ConnectionState;
  const conn = { getState: () => state, subscribe: () => () => {}, onEvent: () => () => {} } as unknown as Connection;
  const router = createMemoryRouter(
    appRoutes({
      epicList: async () => ({
        epics: [
          { address: "gx:a", project: "gx", name: "a", kind: "map", state: "designing", done: 0, total: 0, queued: false, queueable: false, blocked_by_epics: [], picker_section: "in-progress" },
        ],
        follow_ups: [],
      }),
      epic: async () => ({
        kind: "ok",
        detail: { address: "gx:a", kind: "map", summary: "", done: 0, total: 0, agent_time: 0, cost_total: 0, stats: {}, rows: opts.rows ?? [], queueable: false, queued: false, steps: [] },
      }),
      chat: opts.fetchChat ?? (async () => ({ kind: "ok", detail: opts.chat ?? chatOf() })),
      prompt: opts.post,
    }),
    { initialEntries: ["/d/gx:a"] },
  );
  renderThemed(
    <ConnectionContext.Provider value={conn}>
      <RouterProvider router={router} />
    </ConnectionContext.Provider>,
  );
}

const openNote = async () => {
  fireEvent.click(await screen.findByRole("button", { name: "Note" }));
  return (await screen.findByLabelText("Message")) as HTMLTextAreaElement;
};

test("a plain message posts its text with the last message id", async () => {
  const post = vi.fn<PostPrompt>(async () => ok);
  render({ post });
  const box = await openNote();
  fireEvent.change(box, { target: { value: "hello there" } });
  fireEvent.click(screen.getByRole("button", { name: "Send" }));
  await waitFor(() => expect(post).toHaveBeenCalledTimes(1));
  expect(post).toHaveBeenCalledWith({ id: "s-a", text: "hello there", last_message: "m9" });
  await waitFor(() => expect(box.value).toBe(""));
});

test("the composer is disabled while the session is Working", async () => {
  render({ state: "working", post: vi.fn<PostPrompt>(async () => ok), chat: chatOf({ next: nextOf("map") }) });
  const box = await openNote();
  expect(box.disabled).toBe(true);
  expect((screen.getByRole("button", { name: /Continue/ }) as HTMLButtonElement).disabled).toBe(true);
});

test("a stale refusal keeps the draft and refetches the chat", async () => {
  const post = vi.fn<PostPrompt>(async () => ({ refused: true, reason: "stale", message: "the conversation moved; review it first" }));
  const fetchChat = vi.fn<FetchChat>(async () => ({ kind: "ok", detail: chatOf() }));
  render({ post, fetchChat });
  const box = await openNote();
  const before = fetchChat.mock.calls.length;
  fireEvent.change(box, { target: { value: "keep me" } });
  fireEvent.click(screen.getByRole("button", { name: "Send" }));
  expect(await screen.findByRole("alert")).toBeTruthy();
  expect(box.value).toBe("keep me");
  await waitFor(() => expect(fetchChat.mock.calls.length).toBeGreaterThan(before));
});

test("another refusal shows its message and keeps the draft", async () => {
  const post = vi.fn<PostPrompt>(async () => ({ refused: true, reason: "text-too-long", message: "text is over 32768 bytes" }));
  render({ post });
  const box = await openNote();
  fireEvent.change(box, { target: { value: "long" } });
  fireEvent.click(screen.getByRole("button", { name: "Send" }));
  expect((await screen.findByRole("alert")).textContent).toBe("text is over 32768 bytes");
  expect(box.value).toBe("long");
});

test.each(["map", "review"])("Continue after a %s run-end line sends the continue action", async (phase) => {
  const post = vi.fn<PostPrompt>(async () => ok);
  render({ post, chat: chatOf({ next: nextOf(phase) }) });
  fireEvent.click(await screen.findByRole("button", { name: /Continue/ }));
  await waitFor(() => expect(post).toHaveBeenCalledWith({ id: "s-a", text: "", last_message: "m9", continue: true }));
});

test("Continue names the map's current ticket and says Continue once", async () => {
  const rows: EpicDetailRow[] = [
    { address: "gx:a/01", title: "Done one", rendered_status: "done" },
    { address: "gx:a/02", title: "Server pid lock", rendered_status: "open", current: true },
  ];
  render({ post: vi.fn<PostPrompt>(async () => ok), chat: chatOf({ next: nextOf("map") }), rows });
  const button = await screen.findByRole("button", { name: "Continue Server pid lock" });
  expect(button.textContent?.match(/Continue/g)).toHaveLength(1);
});

test("Continue to review with no current ticket says Review", async () => {
  render({ post: vi.fn<PostPrompt>(async () => ok), chat: chatOf({ next: nextOf("review") }) });
  expect(await screen.findByRole("button", { name: "Continue Review" })).toBeTruthy();
});

test("an implement run-end line shows no Continue", async () => {
  render({ post: vi.fn<PostPrompt>(async () => ok), chat: chatOf({ next: nextOf("implement") }) });
  await screen.findByRole("button", { name: "Note" });
  expect(screen.queryByRole("button", { name: /Continue/ })).toBeNull();
});

test("while disconnected the page says so and the buttons are off", async () => {
  render({ connected: false, post: vi.fn<PostPrompt>(async () => ok), chat: chatOf({ next: nextOf("map") }) });
  expect(((await screen.findByRole("button", { name: /Continue/ })) as HTMLButtonElement).disabled).toBe(true);
  expect(screen.getByText("Offline, reconnecting…")).toBeTruthy();
  expect((await openNote()).disabled).toBe(true);
});

const askCard = (n: number, over: Partial<SessionRoundCard> = {}): SessionRoundCard => ({
  label: String(n),
  title: `T${n}`,
  body: `Body ${n}?`,
  kind: "options",
  options: [
    { id: "a", label: "Alpha" },
    { id: "b", label: "Beta" },
  ],
  ...over,
});

const round = (): SessionRoundCard[] => [
  askCard(1),
  askCard(2, { kind: "yes-no", options: undefined, yes: "Sure", no: "Nope" }),
  askCard(3, { kind: "text", options: undefined }),
];

const sheet = () => screen.findByRole("group", { name: "Questions" });

test("Send answers the whole round in one prompt", async () => {
  const post = vi.fn<PostPrompt>(async () => ok);
  render({ post, chat: chatOf({ round: round() }) });
  const s = within(await sheet());
  fireEvent.click(s.getByRole("button", { name: "Alpha" }));
  fireEvent.click(s.getByRole("button", { name: "Nope" }));
  fireEvent.change(s.getByRole("textbox", { name: "Answer to question 3" }), { target: { value: "free text" } });
  fireEvent.click(s.getByRole("button", { name: "Question 1, answered" }));
  fireEvent.click(s.getByRole("button", { name: "Note on question 1" }));
  fireEvent.change(s.getByRole("textbox", { name: "Note text for question 1" }), { target: { value: "because" } });
  fireEvent.change(await openNote(), { target: { value: "overall" } });
  fireEvent.click(screen.getByRole("button", { name: "Send" }));
  await waitFor(() => expect(post).toHaveBeenCalledTimes(1));
  expect(post).toHaveBeenCalledWith({
    id: "s-a",
    text: "Q1 - T1: Alpha\n  Note: because\nQ2 - T2: No\nQ3 - T3: free text\n\nNote:\noverall",
    last_message: "m9",
  });
});

test("unanswered questions ask before sending; Answer them opens the first one", async () => {
  const post = vi.fn<PostPrompt>(async () => ok);
  render({ post, chat: chatOf({ round: round() }) });
  const s = within(await sheet());
  fireEvent.click(s.getByRole("button", { name: "Beta" }));
  fireEvent.click(s.getByRole("button", { name: "Question 3" }));
  fireEvent.click(screen.getByRole("button", { name: "Send" }));
  expect(screen.getByText(/2 questions are unanswered \(Q2, Q3\)\. Send anyway\?/)).toBeTruthy();
  expect(post).not.toHaveBeenCalled();

  fireEvent.click(screen.getByRole("button", { name: "Answer them" }));
  expect(screen.queryByRole("alertdialog")).toBeNull();
  expect(s.getByRole("button", { name: "Question 2" }).getAttribute("aria-current")).toBe("step");

  fireEvent.click(screen.getByRole("button", { name: "Send" }));
  fireEvent.click(screen.getByRole("button", { name: "Send anyway" }));
  await waitFor(() => expect(post).toHaveBeenCalledTimes(1));
  expect(post.mock.calls[0][0].text).toBe("Q1 - T1: Beta\nQ2 - T2: (unanswered)\nQ3 - T3: (unanswered)");
});

test("a stale refusal keeps every answer and note of the round", async () => {
  const post = vi.fn<PostPrompt>(async () => ({ refused: true, reason: "stale", message: "the conversation moved" }));
  const fetchChat = vi.fn<FetchChat>(async () => ({ kind: "ok", detail: chatOf({ round: round() }) }));
  render({ post, fetchChat });
  const s = within(await sheet());
  fireEvent.click(s.getByRole("button", { name: "Alpha" }));
  const box = await openNote();
  fireEvent.change(box, { target: { value: "keep" } });
  const before = fetchChat.mock.calls.length;
  fireEvent.click(screen.getByRole("button", { name: "Send" }));
  fireEvent.click(screen.getByRole("button", { name: "Send anyway" }));
  expect(await screen.findByRole("alert")).toBeTruthy();
  await waitFor(() => expect(fetchChat.mock.calls.length).toBeGreaterThan(before));
  expect(screen.getByRole("button", { name: "Question 1, answered" })).toBeTruthy();
  expect(box.value).toBe("keep");
});

test("question bodies and option labels render their markdown", async () => {
  const card = askCard(1, {
    body: "Keep **the old route**?\n\n- one\n- two",
    options: [
      { id: "a", label: "Use `designs/open`" },
      { id: "b", label: "Beta" },
    ],
  });
  render({ chat: chatOf({ round: [card] }) });
  const s = await sheet();
  expect(within(s).getByText("the old route").tagName).toBe("STRONG");
  expect(within(s).getAllByRole("listitem")).toHaveLength(2);
  const option = within(s).getByRole("button", { name: "Use designs/open" });
  expect(within(option).getByText("designs/open").tagName).toBe("CODE");
  expect(option.querySelector("p, div")).toBeNull();
});

test("a finished turn with a background task still takes answers", async () => {
  const post = vi.fn<PostPrompt>(async () => ok);
  render({ state: "working", background: true, post, chat: chatOf({ round: [askCard(1)] }) });
  const s = within(await sheet());
  fireEvent.click(s.getByRole("button", { name: "Alpha" }));
  fireEvent.click(screen.getByRole("button", { name: "Send" }));
  await waitFor(() => expect(post).toHaveBeenCalledTimes(1));
});

test("a question card shows its number and title above the body", async () => {
  render({ chat: chatOf({ round: [askCard(1, { title: "Pick `one`" })] }) });
  const s = within(await sheet());
  expect(s.getByText((_, el) => el?.textContent === "Q1 - Pick one" && el.tagName === "SPAN")).toBeTruthy();
  expect(s.getByText("Body 1?")).toBeTruthy();
});

const focusDialog = async () => {
  fireEvent.click(await screen.findByRole("button", { name: "⤢ Focus" }));
  return screen.findByRole("dialog", { name: "Questions, full screen" });
};

test("Focus opens the round full screen; Send there answers it and collapses back", async () => {
  const post = vi.fn<PostPrompt>(async () => ok);
  render({ post, chat: chatOf({ round: [askCard(1), askCard(2)] }) });
  const d = within(await focusDialog());
  expect(screen.queryByRole("group", { name: "Questions" })).toBeNull();
  fireEvent.click(d.getByRole("button", { name: "Alpha" }));
  fireEvent.click(d.getByRole("button", { name: "Beta" }));
  expect(d.getByText("2 of 2 answered")).toBeTruthy();
  fireEvent.change(d.getByRole("textbox", { name: "Message" }), { target: { value: "overall" } });
  fireEvent.click(d.getByRole("button", { name: "Send" }));
  await waitFor(() => expect(post).toHaveBeenCalledTimes(1));
  expect(post.mock.calls[0][0].text).toBe("Q1 - T1: Alpha\nQ2 - T2: Beta\n\nNote:\noverall");
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  expect(await sheet()).toBeTruthy();
});

test("full screen keeps the note draft and the answers, and Escape collapses back", async () => {
  render({ post: vi.fn<PostPrompt>(async () => ok), chat: chatOf({ round: round() }) });
  fireEvent.change(await openNote(), { target: { value: "draft" } });
  const d = within(await focusDialog());
  expect((d.getByRole("textbox", { name: "Message" }) as HTMLTextAreaElement).value).toBe("draft");
  fireEvent.click(d.getByRole("button", { name: "Alpha" }));
  fireEvent.keyDown(window, { key: "Escape" });
  expect(screen.queryByRole("dialog")).toBeNull();
  expect(within(await sheet()).getByRole("button", { name: "Question 1, answered" })).toBeTruthy();
  expect((screen.getByRole("textbox", { name: "Message" }) as HTMLTextAreaElement).value).toBe("draft");
});

test("Answer them in full screen moves to the first unanswered question", async () => {
  render({ post: vi.fn<PostPrompt>(async () => ok), chat: chatOf({ round: round() }) });
  const d = within(await focusDialog());
  fireEvent.click(d.getByRole("button", { name: "Alpha" }));
  fireEvent.click(d.getByRole("button", { name: "Question 1, answered" }));
  fireEvent.click(d.getByRole("button", { name: "Send" }));
  fireEvent.click(d.getByRole("button", { name: "Answer them" }));
  expect(d.getByRole("button", { name: "Question 2" }).getAttribute("aria-current")).toBe("step");
});
