import { act, cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { afterEach, expect, test, vi } from "vitest";
import { ConnectionContext, type Connection, type ConnectionState } from "../connection";
import type { SessionChatMessage, SessionChatResponse, SessionInfo, SessionRoundCard } from "../generated/server";
import { appRoutes } from "../routes";
import { renderThemed } from "../ui/render";
import type { FetchChat } from "./chat";
import type { PostPrompt } from "./prompt";

afterEach(cleanup);

const ago = (h: number) => new Date(Date.now() - h * 3_600_000).toISOString();

const session = (over: Partial<SessionInfo> = {}): SessionInfo => ({
  id: "s-a",
  project: "gx",
  dir: "/x",
  harness: "claude",
  ticket: "gx:a/01",
  design: true,
  status: { state: "idle" },
  status_changed_at: ago(1),
  run_started_at: ago(1),
  unread: {},
  actionable: {},
  auto_advance: true,
  ...over,
});

const msg = (over: Partial<SessionChatMessage>): SessionChatMessage => ({
  id: String(Math.random()),
  role: "agent",
  kind: "text",
  text: "",
  at: ago(1),
  ...over,
});

const chatOf = (messages: SessionChatMessage[], over: Partial<SessionChatResponse> = {}): SessionChatResponse => ({
  conversation_id: "c1",
  messages,
  round: null,
  background: [],
  ...over,
});

const okChat = (c: SessionChatResponse): FetchChat => async () => ({ kind: "ok", detail: c });

function renderChat(sessions: SessionInfo[], fetchChat: FetchChat, prompt?: PostPrompt, mapDetail: Record<string, unknown> = {}) {
  const state = { snapshot: { sessions }, connected: true, loadedOnce: true, tick: 0 } as unknown as ConnectionState;
  const listeners = new Set<(a?: string) => void>();
  const conn = {
    getState: () => state,
    subscribe: () => () => {},
    onEvent: (l: (a?: string) => void) => {
      listeners.add(l);
      return () => listeners.delete(l);
    },
  } as unknown as Connection;
  const router = createMemoryRouter(
    appRoutes({
      epicList: async () => ({
        epics: [
          {
            address: "gx:a",
            project: "gx",
            name: "a",
            kind: "map",
            state: "designing",
            done: 0,
            total: 0,
            queued: false,
            queueable: false,
            blocked_by_epics: [],
          },
        ],
        follow_ups: [],
      }),
      epic: async () => ({
        kind: "ok",
        detail: {
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
          steps: [],
          ...mapDetail,
        },
      }),
      chat: fetchChat,
      prompt,
    }),
    { initialEntries: ["/d/gx:a"] },
  );
  renderThemed(
    <ConnectionContext.Provider value={conn}>
      <RouterProvider router={router} />
    </ConnectionContext.Provider>,
  );
  return { emit: (a?: string) => act(async () => listeners.forEach((l) => l(a))) };
}

test("each entry kind renders", async () => {
  renderChat(
    [session()],
    okChat(
      chatOf(
        [
          msg({ text: "Hello **bold**" }),
          msg({ role: "person", kind: "prompt", text: "do the thing" }),
          msg({ role: "person", kind: "command", text: "/gx-design" }),
          msg({ kind: "tools", tool_count: 12, last_tool: "Read" }),
          msg({ kind: "compacted" }),
        ],
        { truncated: true },
      ),
    ),
  );
  expect((await screen.findByText("bold")).tagName).toBe("STRONG");
  expect(screen.getByText("do the thing")).toBeTruthy();
  expect(screen.getByText("/gx-design")).toBeTruthy();
  expect(screen.queryByText(/tool call/)).toBeNull();
  expect(screen.getByText("Context compacted")).toBeTruthy();
  expect(screen.getByText("Earlier messages hidden")).toBeTruthy();
});

test("tool messages render nothing, busy or idle", async () => {
  const chat = okChat(chatOf([msg({ text: "hi" }), msg({ kind: "tools", tool_count: 3, last_tool: "Grep" })]));
  renderChat([session({ status: { state: "working" } })], chat);
  await screen.findByText("hi");
  expect(screen.queryByText(/tool call|Grep/)).toBeNull();
  cleanup();
  renderChat([session({ status: { state: "idle" } })], chat);
  await screen.findByText("hi");
  expect(screen.queryByText(/tool call|Grep/)).toBeNull();
});

const steps = [
  { step: "chart", label: "Chart", state: "done" },
  { step: "spec", label: "Spec", state: "current" },
  { step: "resolve-tickets", label: "Resolve tickets", state: "todo" },
];

test("a busy turn shows the reported step, falling back to the map's", async () => {
  const chat = okChat(chatOf([msg({ text: "hi" })]));
  renderChat([session({ status: { state: "working" }, reported_step: "chart" })], chat, undefined, { steps });
  expect(await screen.findByText("Working: Chart")).toBeTruthy();
  cleanup();
  renderChat([session({ status: { state: "working" } })], chat, undefined, { steps });
  expect(await screen.findByText("Working: Spec")).toBeTruthy();
});

test("Resolve tickets adds the ticket title", async () => {
  const rows = [{ address: "gx:a/03", title: "Wire it", current: true }];
  renderChat(
    [session({ status: { state: "working" }, reported_step: "resolve-tickets", reported_title: "Reported one" })],
    okChat(chatOf([msg({ text: "hi" })])),
    undefined,
    { steps, rows },
  );
  expect(await screen.findByText("Working: Resolve tickets · Reported one")).toBeTruthy();
});

test("a busy turn without a map step shows a bare Working row", async () => {
  renderChat([session({ status: { state: "working" } })], okChat(chatOf([msg({ text: "hi" })])));
  expect(await screen.findByText("Working")).toBeTruthy();
});

test("an idle session shows no working row", async () => {
  renderChat([session({ reported_step: "spec" })], okChat(chatOf([msg({ text: "hi" })])), undefined, { steps });
  await screen.findByText("hi");
  expect(screen.queryByText(/Working:/)).toBeNull();
});

test("the background strip lists running tasks",async () => {
  renderChat(
    [session()],
    okChat(
      chatOf([], {
        background: [
          { kind: "shell", label: "make web-test", started_at: ago(0) },
          { kind: "agent", label: "survey", started_at: ago(0) },
        ],
      }),
    ),
  );
  const strip = await screen.findByRole("list", { name: "Running in background" });
  expect(within(strip).getByText("shell: make web-test")).toBeTruthy();
  expect(within(strip).getByText("subagent: survey")).toBeTruthy();
});

test("an event for the session refetches and a new conversation replaces the chat", async () => {
  let n = 0;
  const fetchChat = vi.fn<FetchChat>(async () => ({
    kind: "ok",
    detail: ++n === 1 ? chatOf([msg({ text: "old talk" })]) : chatOf([msg({ text: "new talk" })], { conversation_id: "c2" }),
  }));
  const { emit } = renderChat([session()], fetchChat);
  await screen.findByText("old talk");
  await emit("s-other");
  expect(fetchChat).toHaveBeenCalledTimes(1);
  await emit("s-a");
  await screen.findByText("new talk");
  expect(screen.queryByText("old talk")).toBeNull();
});

test("a non-Claude session shows the fixed text and loads no chat", async () => {
  const fetchChat = vi.fn<FetchChat>(okChat(chatOf([])));
  renderChat([session({ harness: "codex" })], fetchChat);
  expect(await screen.findByText("Chat works for Claude sessions only. Open it in gx, Agents tab.")).toBeTruthy();
  expect(fetchChat).not.toHaveBeenCalled();
});

test("a Design with no session has an empty chat", async () => {
  const fetchChat = vi.fn<FetchChat>(okChat(chatOf([])));
  renderChat([], fetchChat);
  await screen.findByRole("region", { name: "Chat" });
  expect(fetchChat).not.toHaveBeenCalled();
});

test("the newest live session drives the chat, else the newest Exited one", async () => {
  const seen: string[] = [];
  const fetchChat: FetchChat = async (id) => {
    seen.push(id);
    return { kind: "ok", detail: chatOf([]) };
  };
  renderChat(
    [
      session({ id: "old-exited", status: { state: "exited" }, status_changed_at: ago(9) }),
      session({ id: "new-exited", status: { state: "exited" }, status_changed_at: ago(1) }),
    ],
    fetchChat,
  );
  await waitFor(() => expect(seen).toEqual(["new-exited"]));
});

const card = (over: Partial<SessionRoundCard>): SessionRoundCard => ({
  label: "1",
  title: "t",
  body: "Which way?",
  kind: "options",
  options: [
    { id: "a", label: "Alpha" },
    { id: "b", label: "Beta" },
  ],
  ...over,
});

const roundChat = () =>
  okChat(
    chatOf([], {
      round: [
        card({ label: "1", body: "First?", recommended: "b" }),
        card({ label: "2", body: "Second?", kind: "yes-no", options: undefined, yes: "Sure", no: "Nope" }),
        card({ label: "3", body: "Third?", kind: "text", options: undefined }),
        card({ label: "4", body: "Fourth?", kind: "text", options: undefined, note: "options could not be read" }),
      ],
    }),
  );

test("a round opens a sheet that folds, and its stepper marks and jumps", async () => {
  renderChat([session()], roundChat());
  const sheet = await screen.findByRole("group", { name: "Questions" });
  expect(within(sheet).getByText("First?")).toBeTruthy();
  expect(within(sheet).getByText("★ recommended")).toBeTruthy();
  expect(within(sheet).getByRole("button", { name: "Question 1" }).getAttribute("aria-current")).toBe("step");

  fireEvent.click(within(sheet).getByRole("button", { name: "Question 3" }));
  expect(within(sheet).getByRole("button", { name: "Question 3" }).getAttribute("aria-current")).toBe("step");
  expect(within(sheet).getByRole("textbox", { name: "Answer to question 3" })).toBeTruthy();

  fireEvent.click(within(sheet).getByRole("button", { name: "Fold questions" }));
  expect(within(sheet).queryByRole("textbox", { name: "Answer to question 3" })).toBeNull();
  fireEvent.click(within(sheet).getByRole("button", { name: "Open questions" }));
  expect(within(sheet).getByRole("textbox", { name: "Answer to question 3" })).toBeTruthy();
});

test("picking an option marks the circle answered and advances; Prev and Next move", async () => {
  renderChat([session()], roundChat());
  const sheet = await screen.findByRole("group", { name: "Questions" });
  fireEvent.click(within(sheet).getByRole("button", { name: "Alpha" }));
  expect(within(sheet).getByRole("button", { name: "Question 1, answered" }).textContent).toBe("✓");
  expect(within(sheet).getByRole("button", { name: "Question 2" }).getAttribute("aria-current")).toBe("step");
  expect(within(sheet).getByRole("button", { name: "Sure" })).toBeTruthy();

  fireEvent.click(within(sheet).getByRole("button", { name: "← Prev" }));
  expect(within(sheet).getByRole("button", { name: "Question 1, answered" }).getAttribute("aria-current")).toBe("step");
  fireEvent.click(within(sheet).getByRole("button", { name: "Next →" }));
  fireEvent.click(within(sheet).getByRole("button", { name: "Next →" }));
  fireEvent.click(within(sheet).getByRole("button", { name: "Next →" }));
  expect(within(sheet).getByText("options could not be read")).toBeTruthy();
  expect((within(sheet).getByRole("button", { name: "Next →" }) as HTMLButtonElement).disabled).toBe(true);
});

test("a question keeps a note behind a note icon", async () => {
  renderChat([session()], roundChat());
  const sheet = await screen.findByRole("group", { name: "Questions" });
  expect(within(sheet).queryByRole("textbox", { name: "Note text for question 1" })).toBeNull();
  fireEvent.click(within(sheet).getByRole("button", { name: "Note on question 1" }));
  fireEvent.change(within(sheet).getByRole("textbox", { name: "Note text for question 1" }), { target: { value: "hm" } });
  expect((within(sheet).getByRole("textbox", { name: "Note text for question 1" }) as HTMLTextAreaElement).value).toBe("hm");
});

test("no round, no sheet", async () => {
  renderChat([session()], okChat(chatOf([msg({ text: "hi" })])));
  await screen.findByText("hi");
  expect(screen.queryByRole("group", { name: "Questions" })).toBeNull();
});

test("a reload keeps the round's answers", async () => {
  renderChat([session()], roundChat());
  fireEvent.click(within(await screen.findByRole("group", { name: "Questions" })).getByRole("button", { name: "Alpha" }));
  cleanup();
  renderChat([session()], roundChat());
  const sheet = await screen.findByRole("group", { name: "Questions" });
  expect(within(sheet).getByRole("button", { name: "Question 1, answered" })).toBeTruthy();
});

test("a reload keeps the notes box, the open question and the scroll", async () => {
  renderChat([session()], roundChat());
  const sheet = await screen.findByRole("group", { name: "Questions" });
  fireEvent.click(within(sheet).getByRole("button", { name: "Question 3" }));
  fireEvent.click(screen.getByRole("button", { name: "Note" }));
  fireEvent.change(screen.getByRole("textbox", { name: "Message" }), { target: { value: "for all" } });
  Object.defineProperty(window, "scrollY", { value: 120, configurable: true });
  fireEvent.scroll(window);
  const scrollTo = vi.spyOn(window, "scrollTo").mockImplementation(() => {});
  cleanup();
  renderChat([session()], roundChat());
  const again = await screen.findByRole("group", { name: "Questions" });
  expect(within(again).getByRole("button", { name: "Question 3" }).getAttribute("aria-current")).toBe("step");
  fireEvent.click(screen.getByRole("button", { name: "Note" }));
  expect((screen.getByRole("textbox", { name: "Message" }) as HTMLTextAreaElement).value).toBe("for all");
  expect(scrollTo).toHaveBeenCalledWith(0, 120);
  scrollTo.mockRestore();
});

test("a new round starts with a clean sheet", async () => {
  renderChat([session()], roundChat());
  fireEvent.click(within(await screen.findByRole("group", { name: "Questions" })).getByRole("button", { name: "Alpha" }));
  cleanup();
  renderChat([session()], okChat(chatOf([], { round: [card({ label: "1", title: "other", body: "Else?" })] })));
  const sheet = await screen.findByRole("group", { name: "Questions" });
  expect(within(sheet).queryByRole("button", { name: "Question 1, answered" })).toBeNull();
});

test("sending the round drops its draft", async () => {
  const post = vi.fn<PostPrompt>(async () => ({}));
  renderChat([session()], roundChat(), post);
  const sheet = await screen.findByRole("group", { name: "Questions" });
  fireEvent.click(within(sheet).getByRole("button", { name: "Alpha" }));
  fireEvent.click(screen.getByRole("button", { name: "Send" }));
  fireEvent.click(await screen.findByRole("button", { name: "Send anyway" }));
  await waitFor(() => expect(post).toHaveBeenCalledTimes(1));
  cleanup();
  renderChat([session()], roundChat(), post);
  const again = await screen.findByRole("group", { name: "Questions" });
  expect(within(again).queryByRole("button", { name: "Question 1, answered" })).toBeNull();
});

const implDetail = { impl_address: "gx:a-impl", steps: [{ step: "branch", label: "Branch", state: "current" }] };

const focusChat = () =>
  okChat(
    chatOf([msg({ text: "earlier talk" })], {
      round: [card({ label: "1", body: "First?" }), card({ label: "3", body: "Third?", kind: "text", options: undefined })],
    }),
  );

test("focus mode hides the messages and the impl link; banner, notice and composer stay", async () => {
  renderChat([session({ failure: { kind: "stalled", label: "Stalled" } as SessionInfo["failure"] })], focusChat(), undefined, implDetail);
  const sheet = await screen.findByRole("group", { name: "Questions" });
  expect(screen.getByText("earlier talk")).toBeTruthy();
  expect(screen.getByRole("link", { name: /Open implementation epic/ })).toBeTruthy();
  fireEvent.click(within(sheet).getByRole("button", { name: "Enter focus mode" }));
  expect(screen.queryByText("earlier talk")).toBeNull();
  expect(screen.queryByRole("link", { name: /Open implementation epic/ })).toBeNull();
  expect(screen.getByRole("group", { name: "Questions" })).toBeTruthy();
  expect(screen.getByLabelText("Session failure")).toBeTruthy();
  expect(screen.getByRole("form", { name: "Composer" })).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Leave focus mode" }));
  expect(await screen.findByText("earlier talk")).toBeTruthy();
  expect(screen.getByRole("link", { name: /Open implementation epic/ })).toBeTruthy();
});

test("focus mode keeps the gx stopped notice", async () => {
  renderChat([session({ actionable: { reason: "boom", gx_raised: true } })], focusChat());
  fireEvent.click(within(await screen.findByRole("group", { name: "Questions" })).getByRole("button", { name: "Enter focus mode" }));
  expect(screen.getByRole("region", { name: "gx stopped" })).toBeTruthy();
});

test("fold leaves focus mode; unfolding with the focus button enters it", async () => {
  renderChat([session()], focusChat());
  const sheet = await screen.findByRole("group", { name: "Questions" });
  fireEvent.click(within(sheet).getByRole("button", { name: "Enter focus mode" }));
  expect(screen.queryByText("earlier talk")).toBeNull();
  fireEvent.click(within(sheet).getByRole("button", { name: "Fold questions" }));
  expect(await screen.findByText("earlier talk")).toBeTruthy();
  expect(within(sheet).queryByRole("button", { name: "Alpha" })).toBeNull();
  fireEvent.click(within(sheet).getByRole("button", { name: "Enter focus mode" }));
  expect(screen.queryByText("earlier talk")).toBeNull();
  expect(within(sheet).getByRole("button", { name: "Alpha" })).toBeTruthy();
});

const enterFocus = async () =>
  fireEvent.click(within(await screen.findByRole("group", { name: "Questions" })).getByRole("button", { name: "Enter focus mode" }));
const inFocus = () => screen.queryByText("earlier talk") === null;

test("Escape leaves focus mode, also from the note field", async () => {
  renderChat([session()], focusChat());
  await enterFocus();
  fireEvent.click(screen.getByRole("button", { name: "Note" }));
  fireEvent.keyDown(screen.getByRole("textbox", { name: "Message" }), { key: "Escape" });
  expect(await screen.findByText("earlier talk")).toBeTruthy();
});

test("Escape with the Send popover open closes only the popover", async () => {
  renderChat([session()], focusChat());
  await enterFocus();
  fireEvent.click(screen.getByRole("button", { name: "Send" }));
  const popover = await screen.findByRole("alertdialog", { name: "Unanswered questions" });
  await act(async () => {});
  fireEvent.keyDown(popover, { key: "Escape" });
  await waitFor(() => expect(screen.queryByRole("alertdialog")).toBeNull());
  expect(inFocus()).toBe(true);
  fireEvent.keyDown(document.body, { key: "Escape" });
  expect(await screen.findByText("earlier talk")).toBeTruthy();
});

test("Escape with the failure banner's screen modal open closes only the modal", async () => {
  renderChat([session({ failure: { kind: "stalled", label: "Stalled" } as SessionInfo["failure"] })], focusChat());
  await enterFocus();
  fireEvent.click(await screen.findByRole("button", { name: "Screen" }));
  await screen.findByRole("dialog", { name: "Screen" });
  fireEvent.keyDown(document.body, { key: "Escape" });
  await waitFor(() => expect(screen.queryByRole("dialog", { name: "Screen" })).toBeNull());
  expect(inFocus()).toBe(true);
  fireEvent.keyDown(document.body, { key: "Escape" });
  expect(await screen.findByText("earlier talk")).toBeTruthy();
});

test("a successful send leaves focus mode; a refused send keeps it and shows the error", async () => {
  const post = vi
    .fn<PostPrompt>()
    .mockResolvedValueOnce({ refused: true, reason: "text-too-long", message: "text is over the limit" })
    .mockResolvedValue({});
  renderChat([session()], focusChat(), post);
  await enterFocus();
  fireEvent.click(screen.getByRole("button", { name: "Send" }));
  fireEvent.click(await screen.findByRole("button", { name: "Send anyway" }));
  expect(await screen.findByText("text is over the limit")).toBeTruthy();
  expect(inFocus()).toBe(true);
  fireEvent.click(screen.getByRole("button", { name: "Send" }));
  fireEvent.click(await screen.findByRole("button", { name: "Send anyway" }));
  expect(await screen.findByText("earlier talk")).toBeTruthy();
});

test("a new round starts docked, and a round that goes away ends focus mode", async () => {
  let current = focusChat();
  const { emit } = renderChat([session()], (...a) => current(...a));
  await enterFocus();
  expect(inFocus()).toBe(true);
  current = okChat(chatOf([msg({ text: "earlier talk" })], { round: [card({ label: "1", title: "other", body: "Else?" })] }));
  await emit("s-a");
  expect(await screen.findByText("earlier talk")).toBeTruthy();
  await enterFocus();
  expect(inFocus()).toBe(true);
  current = okChat(chatOf([msg({ text: "earlier talk" })]));
  await emit("s-a");
  expect(await screen.findByText("earlier talk")).toBeTruthy();
});

test("a fresh mount with a round draft opens docked", async () => {
  renderChat([session()], focusChat());
  fireEvent.click(within(await screen.findByRole("group", { name: "Questions" })).getByRole("button", { name: "Alpha" }));
  cleanup();
  renderChat([session()], focusChat());
  await screen.findByRole("group", { name: "Questions" });
  expect(inFocus()).toBe(false);
});

test("answers, the open question and the note survive switching modes; the text answer has five rows in focus", async () => {
  renderChat([session()], focusChat());
  const sheet = await screen.findByRole("group", { name: "Questions" });
  fireEvent.click(within(sheet).getByRole("button", { name: "Alpha" }));
  fireEvent.click(within(sheet).getByRole("button", { name: "Question 3" }));
  fireEvent.click(screen.getByRole("button", { name: "Note" }));
  fireEvent.change(screen.getByRole("textbox", { name: "Message" }), { target: { value: "for all" } });
  const answer = () => within(sheet).getByRole("textbox", { name: "Answer to question 3" }) as HTMLTextAreaElement;
  expect(answer().rows).toBe(2);
  fireEvent.click(within(sheet).getByRole("button", { name: "Enter focus mode" }));
  expect(answer().rows).toBe(5);
  fireEvent.change(answer(), { target: { value: "typed" } });
  fireEvent.click(within(sheet).getByRole("button", { name: "Leave focus mode" }));
  expect(answer().rows).toBe(2);
  expect(answer().value).toBe("typed");
  expect(within(sheet).getByRole("button", { name: "Question 1, answered" })).toBeTruthy();
  expect(within(sheet).getByRole("button", { name: "Question 3, answered" }).getAttribute("aria-current")).toBe("step");
  expect((screen.getByRole("textbox", { name: "Message" }) as HTMLTextAreaElement).value).toBe("for all");
});

const defaultsText ="Intro\n\n**Decided unless you object**\n\n- Naming: use camelCase\n- Order: alphabetical";

test("defaulted questions render as plain text beside one card, and Send asks nothing about them", async () => {
  const post = vi.fn<PostPrompt>(async () => ({}));
  const fetchChat = okChat(chatOf([msg({ text: defaultsText })], { round: [card({ label: "1", body: "First?" })] }));
  renderChat([session()], fetchChat, post);
  const sheet = await screen.findByRole("group", { name: "Questions" });
  expect(screen.getByText("Decided unless you object")).toBeTruthy();
  expect(screen.getByText("Naming: use camelCase")).toBeTruthy();
  expect(screen.getByText("Order: alphabetical")).toBeTruthy();
  expect(within(sheet).getAllByRole("button", { name: /^Question \d/ })).toHaveLength(1);

  fireEvent.click(within(sheet).getByRole("button", { name: "Alpha" }));
  fireEvent.click(screen.getByRole("button", { name: "Send" }));
  await waitFor(() => expect(post).toHaveBeenCalledTimes(1));
  const sent = JSON.stringify(post.mock.calls[0]);
  expect(sent).toContain("Q1");
  expect(sent).not.toContain("camelCase");
  expect(sent).not.toContain("unanswered");
});

test("an all-defaulted round shows one Accept these defaults card", async () => {
  const accept = card({
    label: "1",
    title: "Accept these defaults?",
    body: "Do you accept the defaults below?",
    kind: "yes-no",
    options: undefined,
  });
  renderChat([session()], okChat(chatOf([msg({ text: defaultsText })], { round: [accept] })));
  const sheet = await screen.findByRole("group", { name: "Questions" });
  expect(within(sheet).getAllByRole("button", { name: /^Question \d/ })).toHaveLength(1);
  expect(within(sheet).getByText("Q1 - Accept these defaults?")).toBeTruthy();
  expect(within(sheet).getByRole("button", { name: "Yes" })).toBeTruthy();
  expect(within(sheet).getByRole("button", { name: "No" })).toBeTruthy();
  expect(screen.getByText("Naming: use camelCase")).toBeTruthy();
});
