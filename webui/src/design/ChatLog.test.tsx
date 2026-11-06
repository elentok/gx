import { act, cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { afterEach, expect, test, vi } from "vitest";
import { ConnectionContext, type Connection, type ConnectionState } from "../connection";
import type { SessionChatMessage, SessionChatResponse, SessionInfo, SessionRoundCard } from "../generated/server";
import { appRoutes } from "../routes";
import { renderThemed } from "../ui/render";
import type { FetchChat } from "./chat";
import type { PostPrompt } from "./prompt";

afterEach(() => {
  cleanup();
  localStorage.clear();
});

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

const outputLines = (n: number) => Array.from({ length: n }, (_, i) => `line ${i + 1}`).join("\n");

test("long command output is cut at 12 lines until Show all", async () => {
  renderChat([session()], okChat(chatOf([msg({ role: "person", kind: "command", text: "/cost" }), msg({ kind: "output", text: outputLines(14) })])));
  const out = await screen.findByTestId("command-output");
  expect(out.textContent).toContain("line 12");
  expect(out.textContent).not.toContain("line 13");
  fireEvent.click(screen.getByRole("button", { name: "Show all (14 lines)" }));
  expect(screen.getByTestId("command-output").textContent).toContain("line 14");
});

test("on a phone the output, its pill and Show all share one right-aligned block capped at 85%", async () => {
  renderChat([session()], okChat(chatOf([msg({ role: "person", kind: "command", text: "/cost" }), msg({ kind: "output", text: outputLines(14) })])));
  const out = await screen.findByTestId("command-output");
  const block = out.parentElement as HTMLElement;
  expect(block.contains(screen.getByRole("button", { name: "Show all (14 lines)" }))).toBe(true);
  const style = getComputedStyle(block);
  expect(style.alignSelf).toBe("flex-end");
  expect(style.maxWidth).toBe("85%");
  expect(getComputedStyle(out).overflow).toBe("hidden");
});

test("command output keeps Markdown characters literal", async () => {
  renderChat([session()], okChat(chatOf([msg({ kind: "output", text: "**not bold**\n# not a heading" })])));
  const out = await screen.findByTestId("command-output");
  expect(out.textContent).toBe("**not bold**\n# not a heading");
  expect(out.querySelector("strong")).toBeNull();
});

test("a command without output shows only the pill", async () => {
  renderChat([session()], okChat(chatOf([msg({ role: "person", kind: "command", text: "/clear" })])));
  await screen.findByText("/clear");
  expect(screen.queryByTestId("command-output")).toBeNull();
  expect(screen.queryByRole("button", { name: /Show all/ })).toBeNull();
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

test("a busy turn names the background tasks it waits on, also in the Questions view", async () => {
  const background = [
    { kind: "shell", label: "make test", started_at: ago(0) },
    { kind: "agent", label: "survey", started_at: ago(0) },
    { kind: "agent", label: "audit", started_at: ago(0) },
  ];
  const working = [session({ status: { state: "working" } })];
  renderChat(working, okChat(chatOf([msg({ text: "hi" })], { background })));
  expect(await screen.findByText("Working · waiting on 1 shell, 2 subagents")).toBeTruthy();
  cleanup();
  renderChat(working, roundChat({ background }));
  await screen.findByRole("group", { name: "Questions" });
  expect(screen.getByText("Working · waiting on 1 shell, 2 subagents")).toBeTruthy();
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

test("a new conversation sticks the scroller to the bottom again, even after the reader scrolled up", async () => {
  const sets: [Element, number][] = [];
  let top = 0;
  vi.spyOn(HTMLElement.prototype, "scrollHeight", "get").mockReturnValue(1000);
  vi.spyOn(HTMLElement.prototype, "scrollTop", "get").mockImplementation(() => top);
  vi.spyOn(HTMLElement.prototype, "scrollTop", "set").mockImplementation(function (this: Element, v: number) {
    top = v;
    sets.push([this, v]);
  });
  try {
    let n = 0;
    const fetchChat = vi.fn<FetchChat>(async () => ({
      kind: "ok",
      detail: ++n === 1 ? chatOf([msg({ text: "old talk" })]) : chatOf([msg({ text: "new talk" })], { conversation_id: "c2" }),
    }));
    const { emit } = renderChat([session()], fetchChat);
    await screen.findByText("old talk");
    const scroller = sets.at(-1)![0];
    expect(sets.at(-1)![1]).toBe(1000);
    top = 0;
    fireEvent.scroll(scroller);
    sets.length = 0;
    await emit("s-a");
    await screen.findByText("new talk");
    expect(sets.at(-1)?.[1]).toBe(1000);
  } finally {
    vi.restoreAllMocks();
  }
});

test("a new conversation drops a decoded archive", async () => {
  const archive = vi.fn(async (_url: string) => new Response(JSON.stringify(chatOf([msg({ text: "from the archive" })])), { status: 200 }));
  vi.stubGlobal("fetch", archive);
  try {
    let n = 0;
    const fetchChat = vi.fn<FetchChat>(async () => ({
      kind: "ok",
      detail: ++n === 1 ? chatOf([], { archived: true }) : chatOf([msg({ text: "fresh start" })], { conversation_id: "c2" }),
    }));
    const { emit } = renderChat([session({ harness: "codex" })], fetchChat);
    fireEvent.click(await screen.findByRole("button", { name: "Decode" }));
    await screen.findByText("from the archive");
    await emit("s-a");
    await screen.findByText("fresh start");
    expect(screen.queryByText("from the archive")).toBeNull();
  } finally {
    vi.unstubAllGlobals();
  }
});

test("a harness with no chat shows the fixed text and loads no chat", async () => {
  const fetchChat = vi.fn<FetchChat>(okChat(chatOf([])));
  renderChat([session({ harness: "other" })], fetchChat);
  expect(await screen.findByText("Chat works for Claude and Codex sessions only. Open it in gx, Agents tab.")).toBeTruthy();
  expect(fetchChat).not.toHaveBeenCalled();
});

test("a Codex session loads its chat and renders every entry kind", async () => {
  const fetchChat = vi.fn<FetchChat>(
    okChat(
      chatOf([
        msg({ text: "Hello **bold**" }),
        msg({ role: "person", kind: "prompt", text: "do the thing" }),
        msg({ role: "person", kind: "command", text: "$gx-design x" }),
        msg({ kind: "tools", tool_count: 3, last_tool: "shell" }),
        msg({ kind: "compacted" }),
      ]),
    ),
  );
  renderChat([session({ harness: "codex" })], fetchChat);
  expect((await screen.findByText("bold")).tagName).toBe("STRONG");
  expect(screen.getByText("do the thing")).toBeTruthy();
  expect(screen.getByText("$gx-design x")).toBeTruthy();
  expect(screen.getByText("Context compacted")).toBeTruthy();
  expect(screen.queryByText(/tool call/)).toBeNull();
  expect(fetchChat).toHaveBeenCalledWith("s-a");
});

test("an archived Codex chat asks first, and Decode shows the decoded messages", async () => {
  const archive = vi.fn(async (_url: string) => new Response(JSON.stringify(chatOf([msg({ text: "from the archive" })])), { status: 200 }));
  vi.stubGlobal("fetch", archive);
  try {
    renderChat([session({ harness: "codex" })], okChat(chatOf([], { archived: true })));
    expect(await screen.findByText("Codex archived this conversation")).toBeTruthy();
    const archiveCalls = () => archive.mock.calls.map((c) => String(c[0])).filter((u) => u.includes("/chat/archive"));
    expect(archiveCalls()).toEqual([]);
    fireEvent.click(screen.getByRole("button", { name: "Decode" }));
    expect(await screen.findByText("from the archive")).toBeTruthy();
    expect(archiveCalls()).toEqual(["/v1/sessions/chat/archive?session=s-a"]);
    expect(screen.queryByText("Codex archived this conversation")).toBeNull();
  } finally {
    vi.unstubAllGlobals();
  }
});

test("an unreadable Codex chat names the version and points to the doctor", async () => {
  renderChat([session({ harness: "codex" })], okChat(chatOf([], { unreadable: true, codex_version: "0.99.1" })));
  const notice = await screen.findByText(/gx cannot read this Codex conversation/);
  const text = notice.textContent?.replace(/\s+/g, " ");
  expect(text).toContain("(Codex 0.99.1)");
  expect(text).toContain("The terminal and the agent are unaffected.");
  expect(text).toContain("gx codex doctor");
});

test("the Design note box hints at $ for a Codex /gx- draft, never for Claude", async () => {
  renderChat([session({ harness: "codex" })], okChat(chatOf([])));
  fireEvent.click(await screen.findByRole("button", { name: "Note" }));
  fireEvent.change(await screen.findByLabelText("Message"), { target: { value: "/gx-design x" } });
  expect(screen.getByText("Codex skills start with $. Type $gx-... instead.")).toBeTruthy();
  cleanup();
  localStorage.clear(); // the draft would open the note box on its own
  renderChat([session()], okChat(chatOf([])));
  fireEvent.click(await screen.findByRole("button", { name: "Note" }));
  fireEvent.change(await screen.findByLabelText("Message"), { target: { value: "/gx-design x" } });
  expect(screen.queryByText(/Codex skills start with/)).toBeNull();
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

const roundChat = (over: Record<string, unknown> = {}) =>
  okChat(
    chatOf([], {
      ...over,
      round: [
        card({ label: "1", body: "First?", recommended: "b" }),
        card({ label: "2", body: "Second?", kind: "yes-no", options: undefined, yes: "Sure", no: "Nope" }),
        card({ label: "3", body: "Third?", kind: "text", options: undefined }),
        card({ label: "4", body: "Fourth?", kind: "text", options: undefined, note: "options could not be read" }),
      ],
    }),
  );

test("a busy turn still shows the Working row while the questions fill the screen", async () => {
  renderChat([session({ status: { state: "working" }, reported_step: "chart" })], roundChat(), undefined, { steps });
  await screen.findByRole("group", { name: "Questions" });
  expect(screen.getByText("Working: Chart")).toBeTruthy();
  expect(screen.getAllByRole("status")).toHaveLength(1);
});

test("a round opens a sheet whose stepper marks and jumps", async () => {
  renderChat([session()], roundChat());
  const sheet = await screen.findByRole("group", { name: "Questions" });
  expect(within(sheet).getByText("First?")).toBeTruthy();
  expect(within(sheet).getByText("★ recommended")).toBeTruthy();
  expect(within(sheet).getByRole("button", { name: "Question 1" }).getAttribute("aria-current")).toBe("step");

  fireEvent.click(within(sheet).getByRole("button", { name: "Question 3" }));
  expect(within(sheet).getByRole("button", { name: "Question 3" }).getAttribute("aria-current")).toBe("step");
  expect(within(sheet).getByRole("textbox", { name: "Answer to question 3" })).toBeTruthy();

  fireEvent.click(viewButton("Chat"));
  expect(screen.queryByRole("group", { name: "Questions" })).toBeNull();
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
  expect((await screen.findByRole("textbox", { name: "Message" }) as HTMLTextAreaElement).value).toBe("for all");
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
  fireEvent.click(screen.getByRole("button", { name: "Send answers" }));
  fireEvent.click(await screen.findByRole("button", { name: "Send anyway" }));
  await waitFor(() => expect(post).toHaveBeenCalledTimes(1));
  cleanup();
  renderChat([session()], roundChat(), post);
  const again = await screen.findByRole("group", { name: "Questions" });
  expect(within(again).queryByRole("button", { name: "Question 1, answered" })).toBeNull();
});

test("during a round the box shows the composer draft; Send answers uses it as the note and clears it", async () => {
  localStorage.setItem("gx.draft.composer.s-a", "shared note");
  const post = vi.fn<PostPrompt>(async () => ({}));
  renderChat([session()], roundChat(), post);
  expect(((await screen.findByRole("textbox", { name: "Message" })) as HTMLTextAreaElement).value).toBe("shared note");
  fireEvent.click(screen.getByRole("button", { name: "Send answers" }));
  fireEvent.click(await screen.findByRole("button", { name: "Send anyway" }));
  await waitFor(() => expect(post).toHaveBeenCalledTimes(1));
  expect(post.mock.calls[0][0].text).toMatch(/\n\nNote:\nshared note$/);
  await waitFor(() => expect(localStorage.getItem("gx.draft.composer.s-a")).toBeNull());
});

const implDetail = { impl_address: "gx:a-impl", steps: [{ step: "branch", label: "Branch", state: "current" }] };

const focusChat = () =>
  okChat(
    chatOf([msg({ text: "earlier talk" })], {
      round: [card({ label: "1", body: "First?" }), card({ label: "3", body: "Third?", kind: "text", options: undefined })],
    }),
  );

const view = () => screen.getByRole("group", { name: "View" });
const viewButton = (name: RegExp | string) => within(view()).getByRole("button", { name });
const inQuestions = () => screen.queryByText("earlier talk") === null;
const showChat = async () => {
  fireEvent.click(viewButton("Chat"));
  await screen.findByText("earlier talk");
};
const showQuestions = () => fireEvent.click(viewButton(/^Questions/));

test("a round opens in Questions: messages and impl link hidden, toggle counts answers; banner, notice and composer stay", async () => {
  renderChat([session({ failure: { kind: "stalled", label: "Stalled" } as SessionInfo["failure"] })], focusChat(), undefined, implDetail);
  const sheet = await screen.findByRole("group", { name: "Questions" });
  expect(inQuestions()).toBe(true);
  expect(screen.queryByRole("link", { name: /Open implementation epic/ })).toBeNull();
  expect(viewButton(/^Questions/).textContent).toBe("Questions (0 of 2)");
  expect(viewButton(/^Questions/).getAttribute("aria-pressed")).toBe("true");
  expect(viewButton("Chat").getAttribute("aria-pressed")).toBe("false");
  expect(screen.getByLabelText("Session failure")).toBeTruthy();
  expect(screen.getByRole("form", { name: "Composer" })).toBeTruthy();
  fireEvent.click(within(sheet).getByRole("button", { name: "Alpha" }));
  expect(viewButton(/^Questions/).textContent).toBe("Questions (1 of 2)");
  await showChat();
  expect(screen.getByRole("link", { name: /Open implementation epic/ })).toBeTruthy();
  expect(viewButton("Chat").getAttribute("aria-pressed")).toBe("true");
});

test("Questions keeps the gx stopped notice", async () => {
  renderChat([session({ actionable: { reason: "boom", gx_raised: true } })], focusChat());
  await screen.findByRole("group", { name: "Questions" });
  expect(screen.getByRole("region", { name: "gx stopped" })).toBeTruthy();
});

test("the Chat view has no sheet; the Questions view's sheet has no fold chevron and no focus button", async () => {
  renderChat([session()], focusChat());
  const sheet = await screen.findByRole("group", { name: "Questions" });
  expect(within(sheet).queryByRole("button", { name: /Fold questions|Open questions/ })).toBeNull();
  expect(screen.queryByRole("button", { name: /focus mode/ })).toBeNull();
  await showChat();
  expect(screen.queryByRole("group", { name: "Questions" })).toBeNull();
  expect(screen.queryByRole("button", { name: /focus mode/ })).toBeNull();
  showQuestions();
  expect(within(await screen.findByRole("group", { name: "Questions" })).getByRole("button", { name: "Alpha" })).toBeTruthy();
  expect(inQuestions()).toBe(true);
});

test("Escape shows the chat, also from the note field", async () => {
  renderChat([session()], focusChat());
  await screen.findByRole("group", { name: "Questions" });
  fireEvent.click(screen.getByRole("button", { name: "Note" }));
  fireEvent.keyDown(screen.getByRole("textbox", { name: "Message" }), { key: "Escape" });
  expect(await screen.findByText("earlier talk")).toBeTruthy();
});

test("Escape with the Send popover open closes only the popover", async () => {
  renderChat([session()], focusChat());
  await screen.findByRole("group", { name: "Questions" });
  fireEvent.click(screen.getByRole("button", { name: "Send answers" }));
  const popover = await screen.findByRole("alertdialog", { name: "Unanswered questions" });
  // The popover takes Escape only once it is ready, so press until it closes.
  await waitFor(() => {
    fireEvent.keyDown(popover, { key: "Escape" });
    expect(screen.queryByRole("alertdialog")).toBeNull();
  });
  expect(inQuestions()).toBe(true);
  fireEvent.keyDown(document.body, { key: "Escape" });
  expect(await screen.findByText("earlier talk")).toBeTruthy();
});

test("Escape with the ⋯ menu open closes only the menu", async () => {
  renderChat([session()], focusChat());
  await screen.findByRole("group", { name: "Questions" });
  fireEvent.click(await screen.findByRole("button", { name: "More actions" }));
  const menu = await screen.findByRole("menu");
  // The menu takes Escape only once it is ready, so press until it closes.
  await waitFor(() => {
    fireEvent.keyDown(menu, { key: "Escape" });
    expect(screen.queryByRole("menu")).toBeNull();
  });
  expect(inQuestions()).toBe(true);
  fireEvent.keyDown(document.body, { key: "Escape" });
  expect(await screen.findByText("earlier talk")).toBeTruthy();
});

test("Escape with the phone ⋯ sheet open closes only the sheet", async () => {
  vi.stubGlobal("matchMedia", () => ({ matches: false, addEventListener() {}, removeEventListener() {} }));
  try {
    renderChat([session()], focusChat());
    await screen.findByRole("group", { name: "Questions" });
    fireEvent.click(await screen.findByRole("button", { name: "More actions" }));
    const menu = await screen.findByRole("menu");
    // The menu takes Escape only once it is ready, so press until it closes.
    await waitFor(() => {
      fireEvent.keyDown(menu, { key: "Escape" });
      expect(screen.queryByRole("menu")).toBeNull();
    });
    expect(inQuestions()).toBe(true);
    fireEvent.keyDown(document.body, { key: "Escape" });
    expect(await screen.findByText("earlier talk")).toBeTruthy();
  } finally {
    vi.unstubAllGlobals();
  }
});

test("Escape with the phone steps modal open closes only the modal", async () => {
  vi.stubGlobal("matchMedia", () => ({ matches: false, addEventListener() {}, removeEventListener() {} }));
  try {
    renderChat([session()], focusChat(), undefined, implDetail);
    await screen.findByRole("group", { name: "Questions" });
    fireEvent.click(await screen.findByRole("button", { name: /^Branch/ }));
    await screen.findByRole("dialog", { name: "Design progress steps" });
    fireEvent.keyDown(document.body, { key: "Escape" });
    await waitFor(() => expect(screen.queryByRole("dialog", { name: "Design progress steps" })).toBeNull());
    expect(inQuestions()).toBe(true);
    fireEvent.keyDown(document.body, { key: "Escape" });
    expect(await screen.findByText("earlier talk")).toBeTruthy();
  } finally {
    vi.unstubAllGlobals();
  }
});

test("Escape with the failure banner's screen modal open closes only the modal", async () => {
  renderChat([session({ failure: { kind: "stalled", label: "Stalled" } as SessionInfo["failure"] })], focusChat());
  await screen.findByRole("group", { name: "Questions" });
  fireEvent.click(await screen.findByRole("button", { name: "Screen" }));
  await screen.findByRole("dialog", { name: "Screen" });
  fireEvent.keyDown(document.body, { key: "Escape" });
  await waitFor(() => expect(screen.queryByRole("dialog", { name: "Screen" })).toBeNull());
  expect(inQuestions()).toBe(true);
  fireEvent.keyDown(document.body, { key: "Escape" });
  expect(await screen.findByText("earlier talk")).toBeTruthy();
});

test("a successful send shows the chat; a refused send keeps Questions and shows the error", async () => {
  const post = vi
    .fn<PostPrompt>()
    .mockResolvedValueOnce({ refused: true, reason: "text-too-long", message: "text is over the limit" })
    .mockResolvedValue({});
  renderChat([session()], focusChat(), post);
  await screen.findByRole("group", { name: "Questions" });
  fireEvent.click(screen.getByRole("button", { name: "Send answers" }));
  fireEvent.click(await screen.findByRole("button", { name: "Send anyway" }));
  expect(await screen.findByText("text is over the limit")).toBeTruthy();
  expect(inQuestions()).toBe(true);
  fireEvent.click(screen.getByRole("button", { name: "Send answers" }));
  fireEvent.click(await screen.findByRole("button", { name: "Send anyway" }));
  expect(await screen.findByText("earlier talk")).toBeTruthy();
});

test("a new round opens Questions again, and a round that goes away shows the chat with no toggle", async () => {
  let current = focusChat();
  const { emit } = renderChat([session()], (...a) => current(...a));
  await screen.findByRole("group", { name: "Questions" });
  await showChat();
  current = okChat(chatOf([msg({ text: "earlier talk" })], { round: [card({ label: "1", title: "other", body: "Else?" })] }));
  await emit("s-a");
  await waitFor(() => expect(inQuestions()).toBe(true));
  current = okChat(chatOf([msg({ text: "earlier talk" })]));
  await emit("s-a");
  expect(await screen.findByText("earlier talk")).toBeTruthy();
  expect(screen.queryByRole("group", { name: "View" })).toBeNull();
});

test("switching from a Claude session in Questions to a harness with no chat leaves Questions: the impl link is back", async () => {
  renderChat(
    [session(), session({ id: "s-b", name: "other", harness: "other", run_started_at: ago(2) })],
    focusChat(),
    undefined,
    implDetail,
  );
  await screen.findByRole("group", { name: "Questions" });
  expect(screen.queryByRole("link", { name: /Open implementation epic/ })).toBeNull();
  fireEvent.click(await screen.findByRole("button", { name: "More actions" }));
  fireEvent.click(await screen.findByRole("menuitem", { name: "other" }));
  expect(await screen.findByRole("link", { name: /Open implementation epic/ })).toBeTruthy();
  expect(screen.queryByRole("group", { name: "View" })).toBeNull();
});

test("no round: no toggle, the messages show", async () => {
  renderChat([session()], okChat(chatOf([msg({ text: "earlier talk" })])));
  expect(await screen.findByText("earlier talk")).toBeTruthy();
  expect(screen.queryByRole("group", { name: "View" })).toBeNull();
});

test("a fresh mount with a round draft opens Questions", async () => {
  renderChat([session()], focusChat());
  fireEvent.click(within(await screen.findByRole("group", { name: "Questions" })).getByRole("button", { name: "Alpha" }));
  cleanup();
  renderChat([session()], focusChat());
  await screen.findByRole("group", { name: "Questions" });
  expect(inQuestions()).toBe(true);
});

test("answers, the open question and the note survive two view switches; the text answer has five rows in Questions", async () => {
  renderChat([session()], focusChat());
  const sheet = async () => within(await screen.findByRole("group", { name: "Questions" }));
  const first = await sheet();
  fireEvent.click(first.getByRole("button", { name: "Alpha" }));
  fireEvent.click(first.getByRole("button", { name: "Question 3" }));
  fireEvent.click(screen.getByRole("button", { name: "Note" }));
  fireEvent.change(screen.getByRole("textbox", { name: "Message" }), { target: { value: "for all" } });
  const answer = async () => (await sheet()).getByRole("textbox", { name: "Answer to question 3" }) as HTMLTextAreaElement;
  expect((await answer()).rows).toBe(5);
  fireEvent.change(await answer(), { target: { value: "typed" } });
  await showChat();
  showQuestions();
  await showChat();
  showQuestions();
  expect((await answer()).rows).toBe(5);
  expect((await answer()).value).toBe("typed");
  const again = await sheet();
  expect(again.getByRole("button", { name: "Question 1, answered" })).toBeTruthy();
  expect(again.getByRole("button", { name: "Question 3, answered" }).getAttribute("aria-current")).toBe("step");
  expect((screen.getByRole("textbox", { name: "Message" }) as HTMLTextAreaElement).value).toBe("for all");
});

const defaultsText ="Intro\n\n**Decided unless you object**\n\n- Naming: use camelCase\n- Order: alphabetical";

test("defaulted questions render as plain text beside one card, and Send asks nothing about them", async () => {
  const post = vi.fn<PostPrompt>(async () => ({}));
  const fetchChat = okChat(chatOf([msg({ text: defaultsText })], { round: [card({ label: "1", body: "First?" })] }));
  renderChat([session()], fetchChat, post);
  const sheet = await screen.findByRole("group", { name: "Questions" });
  expect(within(sheet).getAllByRole("button", { name: /^Question \d/ })).toHaveLength(1);
  fireEvent.click(within(sheet).getByRole("button", { name: "Alpha" }));
  fireEvent.click(viewButton("Chat"));
  expect(screen.getByText("Decided unless you object")).toBeTruthy();
  expect(screen.getByText("Naming: use camelCase")).toBeTruthy();
  expect(screen.getByText("Order: alphabetical")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Send answers" }));
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
  fireEvent.click(viewButton("Chat"));
  expect(screen.getByText("Naming: use camelCase")).toBeTruthy();
});

const withImages = (n: number, text = "") =>
  msg({ role: "person", kind: "prompt", text, images: Array.from({ length: n }, (_, i) => ({ id: `img-${i}` })) });

const thumbs = () => document.querySelector<HTMLElement>('[data-mock-part="chat-thumbs"]')!;

test.each([
  [1, 1, false],
  [2, 2, false],
  [3, 3, false],
  [4, 4, true],
  [10, 4, true],
])("a prompt with %i images draws %i tiles (thumbnails plus a +N tile)", async (n, tiles, more) => {
  renderChat([session()], okChat(chatOf([withImages(n, "look")])));
  await screen.findByText("look");
  expect(within(thumbs()).getAllByRole("button")).toHaveLength(tiles);
  expect(within(thumbs()).getAllByRole("button", { name: /^Image \d+ of/ })).toHaveLength(Math.min(n, 3));
  expect(within(thumbs()).queryByText(`+${n - 3}`) !== null).toBe(more);
  const img = thumbs().querySelector("img")!;
  expect(img.getAttribute("src")).toBe("/v1/sessions/images?session=s-a&id=img-0");
  expect(img.getAttribute("loading")).toBe("lazy");
  expect(img.getAttribute("decoding")).toBe("async");
});

test("a tap on the second of three thumbnails opens the viewer at 2 of 3, Close returns to the chat", async () => {
  renderChat([session()], okChat(chatOf([withImages(3, "look")])));
  await screen.findByText("look");
  fireEvent.click(within(thumbs()).getByRole("button", { name: "Image 2 of 3" }));
  const dialog = screen.getByRole("dialog", { name: "Image" });
  expect(within(dialog).getByText("2 of 3")).toBeTruthy();
  fireEvent.click(within(dialog).getByRole("button", { name: /Close/ }));
  expect(screen.queryByRole("dialog", { name: "Image" })).toBeNull();
});

test("with ten images the +N tile opens the viewer and Next reaches the tenth", async () => {
  renderChat([session()], okChat(chatOf([withImages(10, "look")])));
  await screen.findByText("look");
  fireEvent.click(within(thumbs()).getByRole("button", { name: "7 more" }));
  const dialog = screen.getByRole("dialog", { name: "Image" });
  expect(within(dialog).getByText("4 of 10")).toBeTruthy();
  for (let i = 0; i < 6; i++) fireEvent.click(within(dialog).getByRole("button", { name: /Next/ }));
  expect(within(dialog).getByText("10 of 10")).toBeTruthy();
  fireEvent.keyDown(window, { key: "Escape" });
  expect(screen.queryByRole("dialog", { name: "Image" })).toBeNull();
});

test("an images-only prompt draws the grid and no text", async () => {
  renderChat([session()], okChat(chatOf([withImages(2)])));
  await waitFor(() => expect(thumbs()).toBeTruthy());
  expect(thumbs().parentElement!.querySelector('[data-mock-part="chat-text"]')).toBeNull();
  expect(thumbs().parentElement!.textContent).toBe("");
});

test("a placeholder image draws a plain tile, not an img", async () => {
  const m = msg({ role: "person", kind: "prompt", text: "gone", images: [{ placeholder: true }] });
  renderChat([session()], okChat(chatOf([m])));
  expect(await screen.findByTestId("chat-image-placeholder")).toBeTruthy();
  expect(thumbs().querySelector("img")).toBeNull();
});
