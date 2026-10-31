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

  fireEvent.click(viewButton("Chat"));
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

test("the Chat view's sheet folds; the Questions view's sheet has no fold chevron and no focus button", async () => {
  renderChat([session()], focusChat());
  const sheet = await screen.findByRole("group", { name: "Questions" });
  expect(within(sheet).queryByRole("button", { name: /Fold questions|Open questions/ })).toBeNull();
  expect(screen.queryByRole("button", { name: /focus mode/ })).toBeNull();
  await showChat();
  fireEvent.click(within(sheet).getByRole("button", { name: "Fold questions" }));
  expect(within(sheet).queryByRole("button", { name: "Alpha" })).toBeNull();
  expect(screen.queryByRole("button", { name: /focus mode/ })).toBeNull();
  showQuestions();
  expect(within(sheet).getByRole("button", { name: "Alpha" })).toBeTruthy();
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
  const sheet = await screen.findByRole("group", { name: "Questions" });
  fireEvent.click(within(sheet).getByRole("button", { name: "Alpha" }));
  fireEvent.click(within(sheet).getByRole("button", { name: "Question 3" }));
  fireEvent.click(screen.getByRole("button", { name: "Note" }));
  fireEvent.change(screen.getByRole("textbox", { name: "Message" }), { target: { value: "for all" } });
  const answer = () => within(sheet).getByRole("textbox", { name: "Answer to question 3" }) as HTMLTextAreaElement;
  expect(answer().rows).toBe(5);
  fireEvent.change(answer(), { target: { value: "typed" } });
  await showChat();
  expect(answer().rows).toBe(2);
  showQuestions();
  await showChat();
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
  fireEvent.click(viewButton("Chat"));
  expect(screen.getByText("Decided unless you object")).toBeTruthy();
  expect(screen.getByText("Naming: use camelCase")).toBeTruthy();
  expect(screen.getByText("Order: alphabetical")).toBeTruthy();
  expect(within(sheet).getAllByRole("button", { name: /^Question \d/ })).toHaveLength(1);

  fireEvent.click(within(sheet).getByRole("button", { name: "Alpha" }));
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
