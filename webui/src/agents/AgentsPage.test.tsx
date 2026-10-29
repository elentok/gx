import { act, cleanup, fireEvent, screen, within } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { ConnectionContext, type Connection, type ConnectionState } from "../connection";
import type { SessionInfo } from "../generated/server";
import type { SessionChatResponse } from "../generated/server";
import { appRoutes, type RouteFetchers } from "../routes";
import { renderThemed } from "../ui/render";
import { WideContext } from "../useWide";

afterEach(cleanup);

const session = (id: string, project: string, over: Partial<SessionInfo> = {}): SessionInfo => ({
  id,
  project,
  dir: "/x",
  harness: "claude",
  status: { state: "idle" },
  status_changed_at: "2026-01-01T00:00:00Z",
  run_started_at: "2026-01-01T00:00:00Z",
  unread: {},
  actionable: {},
  auto_advance: true,
  ...over,
});

const focusPosts: { session: string }[] = [];
beforeEach(() => {
  localStorage.clear();
  focusPosts.length = 0;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_url: string, init: { body: string }) => {
      focusPosts.push(JSON.parse(init.body));
      return new Response("{}");
    }),
  );
});
afterEach(() => vi.unstubAllGlobals());

function render(sessions: SessionInfo[] | undefined, path = "/agents", live = !!sessions, fetchers: RouteFetchers = {}, wide?: boolean) {
  let state = { snapshot: sessions && { sessions }, connected: true, loadedOnce: !!sessions, liveSnapshot: live, streams: 1, tick: 0 } as unknown as ConnectionState;
  const listeners = new Set<() => void>();
  const handlers = new Set<(a?: string) => void>();
  const conn = {
    clientId: "c1",
    getState: () => state,
    subscribe: (fn: () => void) => {
      listeners.add(fn);
      return () => listeners.delete(fn);
    },
    onEvent: (fn: (a?: string) => void) => {
      handlers.add(fn);
      return () => handlers.delete(fn);
    },
    refresh: () => {},
  } as unknown as Connection;
  const router = createMemoryRouter(appRoutes(fetchers), { initialEntries: [path] });
  renderThemed(
    <WideContext.Provider value={wide}>
      <ConnectionContext.Provider value={conn}>
        <RouterProvider router={router} />
      </ConnectionContext.Provider>
    </WideContext.Provider>,
  );
  const update = (patch: Partial<ConnectionState>) => {
    state = { ...state, ...patch } as unknown as ConnectionState;
    act(() => listeners.forEach((fn) => fn()));
  };
  const setSessions = (next: SessionInfo[]) => update({ snapshot: { sessions: next } } as Partial<ConnectionState>);
  const goLive = () => update({ liveSnapshot: true });
  const emit = (address: string) => act(() => handlers.forEach((fn) => fn(address)));
  return { router, setSessions, goLive, emit, update };
}

const rows = () => screen.getAllByRole("link").filter((l) => l.getAttribute("href")?.startsWith("#/agents/"));
const row = (name: string) => rows().find((r) => within(r).queryByText(name))!;

test("groups by project in order of first session, keeping creation order inside a group", () => {
  render([
    session("1", "beta", { name: "b1" }),
    session("2", "alpha", { name: "a1" }),
    session("3", "beta", { name: "b2" }),
    session("4", "gamma", { name: "c1" }),
    session("5", "alpha", { name: "a2" }),
  ]);
  expect(rows().map((r) => r.textContent)).toEqual(["b1", "b2", "a1", "a2", "c1"].map((n) => expect.stringContaining(n)));
  expect(screen.getAllByText(/^(beta|alpha|gamma)$/).map((h) => h.textContent)).toEqual(["beta", "alpha", "gamma"]);
});

test("shows every mark and counts non-Exited sessions as live", () => {
  localStorage.setItem("gx.agents.design", "1");
  render([
    session("a", "p", { name: "plain" }),
    session("b", "p", { name: "crashed", status: { state: "exited" }, exit: "crashed" }),
    session("c", "p", { name: "done", status: { state: "exited" }, exit: "clean" }),
    session("d", "p", { name: "design", design: true, ticket: "gx:m/01" }),
    session("e", "p", { name: "bg", status: { state: "working", background: true } }),
    session("f", "p", { name: "unread", unread: { unread: true } }),
    session("g", "p", { name: "asks", actionable: { reason: "Review the spec" } }),
    session("h", "p", { name: "blocked", status: { state: "blocked" } }),
  ]);
  expect(screen.getByText("6 live")).toBeTruthy();
  expect(within(row("crashed")).getByRole("img", { name: "crashed" })).toBeTruthy();
  expect(within(row("done")).getByRole("img", { name: "exited" })).toBeTruthy();
  expect(within(row("design")).getByRole("img", { name: "Design" })).toBeTruthy();
  expect(within(row("design")).getByText("gx:m/01")).toBeTruthy();
  expect(within(row("bg")).getByText("+bg")).toBeTruthy();
  expect(within(row("plain")).queryByText("+bg")).toBeNull();
  expect(within(row("unread")).getByRole("img", { name: "Unread" })).toBeTruthy();
  expect(getComputedStyle(within(row("unread")).getByText("unread")).fontWeight).toBe("700");
  expect(getComputedStyle(within(row("plain")).getByText("plain")).fontWeight).toBe("400");
  expect(within(row("plain")).queryByRole("img", { name: "Actionable" })).toBeNull();
  expect(within(row("asks")).getByRole("tooltip", { hidden: true }).textContent).toBe("★ Actionable: Review the spec");
  expect(within(row("blocked")).getByRole("img", { name: "Actionable" })).toBeTruthy();
});

test("design sessions are hidden with a count until the toggle is on, and the choice is remembered", () => {
  const sessions = [session("1", "p", { name: "plain" }), session("2", "p", { name: "d1", design: true }), session("3", "p", { name: "d2", design: true })];
  render(sessions);
  expect(rows()).toHaveLength(1);
  expect(screen.getByText("· 2 hidden")).toBeTruthy();
  fireEvent.click(screen.getByRole("checkbox", { name: /Design sessions/ }));
  expect(rows()).toHaveLength(3);
  expect(screen.queryByText(/hidden/)).toBeNull();
  cleanup();
  render(sessions);
  expect(rows()).toHaveLength(3);
  expect((screen.getByRole("checkbox", { name: /Design sessions/ }) as HTMLInputElement).checked).toBe(true);
});

test("a selected design session keeps its row and is not counted as hidden", () => {
  render([session("1", "p", { name: "plain" }), session("2", "p", { name: "d1", design: true }), session("3", "p", { name: "d2", design: true })], "/agents/2");
  expect(rows().map((r) => r.textContent)).toEqual([expect.stringContaining("plain"), expect.stringContaining("d1")]);
  expect(screen.getByText("· 1 hidden")).toBeTruthy();
});

test("a click opens the session and marks its row", () => {
  const { router } = render([session("s-1", "p", { name: "one" }), session("s-2", "p", { name: "two" })]);
  fireEvent.click(row("two"));
  expect(router.state.location.pathname).toBe("/agents/s-2");
  expect(row("two").getAttribute("aria-current")).toBe("page");
  expect(row("one").hasAttribute("aria-current")).toBe(false);
});

test("the main column says what to do, or that there is nothing yet", () => {
  render([session("s-1", "p")]);
  expect(screen.getByText("Pick a session")).toBeTruthy();
  cleanup();
  render([]);
  expect(screen.getByText("No sessions yet")).toBeTruthy();
});

test("reopens the last viewed session when it is listed, else asks to pick one", () => {
  localStorage.setItem("gx.agents.last", "s-2");
  const { router } = render([session("s-1", "p"), session("s-2", "p")]);
  expect(router.state.location.pathname).toBe("/agents/s-2");
  cleanup();
  localStorage.setItem("gx.agents.last", "gone");
  const second = render([session("s-1", "p")]);
  expect(second.router.state.location.pathname).toBe("/agents");
  expect(screen.getByText("Pick a session")).toBeTruthy();
  cleanup();
  localStorage.clear();
  const third = render([session("s-1", "p")]);
  expect(third.router.state.location.pathname).toBe("/agents");
});

test("viewing a session remembers it as the last viewed", () => {
  const { router } = render([session("s-1", "p", { name: "one" }), session("s-2", "p", { name: "two" })]);
  fireEvent.click(row("two"));
  expect(localStorage.getItem("gx.agents.last")).toBe("s-2");
  expect(router.state.location.pathname).toBe("/agents/s-2");
});

test("decides nothing before the first live snapshot", () => {
  localStorage.setItem("gx.agents.last", "s-1");
  const { router, goLive } = render([session("s-1", "p")], "/agents", false);
  expect(router.state.location.pathname).toBe("/agents");
  goLive();
  expect(router.state.location.pathname).toBe("/agents/s-1");
  cleanup();
  const gone = render([session("s-1", "p")], "/agents/nope", false);
  expect(screen.queryByText(/no longer exists/)).toBeNull();
  gone.goLive();
  expect(screen.getByText("Session not found")).toBeTruthy();
});

test("an unknown label shows the not-found panel with the sidebar", () => {
  const { router } = render([session("s-1", "p", { name: "one" })], "/agents/nope");
  expect(screen.getByText("Session not found")).toBeTruthy();
  expect(screen.getByText("nope no longer exists.")).toBeTruthy();
  expect(rows()).toHaveLength(1);
  expect(router.state.location.pathname).toBe("/agents/nope");
});

test("a session removed while viewed shows the panel and the URL stays", () => {
  const { router, setSessions } = render([session("s-1", "p"), session("s-2", "p")], "/agents/s-2");
  expect(screen.queryByText("Session not found")).toBeNull();
  setSessions([session("s-1", "p")]);
  expect(screen.getByText("Session not found")).toBeTruthy();
  expect(router.state.location.pathname).toBe("/agents/s-2");
});

test("the focus report names the selected session, and none for a gone label", async () => {
  render([session("s-1", "p")], "/agents/s-1");
  await vi.waitFor(() => expect(focusPosts.at(-1)?.session).toBe("s-1"));
  cleanup();
  focusPosts.length = 0;
  render([session("s-1", "p")], "/agents/nope");
  await vi.waitFor(() => expect(focusPosts.at(-1)?.session).toBe(""));
});

test("shows the spinner until the snapshot lands, then follows it", () => {
  render(undefined);
  expect(screen.getByRole("status", { name: "Loading agents" })).toBeTruthy();
  cleanup();
  const { setSessions } = render([session("s-1", "p", { name: "one" })]);
  expect(rows()).toHaveLength(1);
  setSessions([session("s-1", "p", { name: "one" }), session("s-2", "p", { name: "two" })]);
  expect(rows()).toHaveLength(2);
});

const chat = (over: Partial<SessionChatResponse> = {}): SessionChatResponse => ({
  conversation_id: "c1",
  messages: [{ id: "m1", role: "agent", kind: "text", text: "hello there", at: "2026-01-01T00:00:00Z" }],
  round: null,
  background: [],
  ...over,
});

const chatOf = (c: SessionChatResponse) => ({ chat: async () => ({ kind: "ok" as const, detail: c }) });

test("the header names the session, status, dir and ticket, with a harness chip only for non-Claude", async () => {
  render([session("s-1", "p", { name: "one", dir: "/work/one", ticket: "gx:m/01" })], "/agents/s-1", true, chatOf(chat()));
  const header = within((await screen.findByRole("heading", { name: /one/ })).closest("header")!);
  expect(header.getByText("Idle")).toBeTruthy();
  expect(header.getByText("/work/one")).toBeTruthy();
  expect(header.getByText("gx:m/01")).toBeTruthy();
  expect(header.queryByText("codex")).toBeNull();
  cleanup();
  render([session("s-2", "p", { name: "two", harness: "codex" })], "/agents/s-2", true, chatOf(chat()));
  expect(await screen.findByText("codex")).toBeTruthy();
});

test("renders messages, the truncation note and the background strip", async () => {
  render(
    [session("s-1", "p")],
    "/agents/s-1",
    true,
    chatOf(chat({ truncated: true, background: [{ kind: "shell", label: "tail -f" }] as SessionChatResponse["background"] })),
  );
  expect(await screen.findByText("hello there")).toBeTruthy();
  expect(screen.getByText("Earlier messages hidden")).toBeTruthy();
  expect(screen.getByRole("list", { name: "Running in background" })).toBeTruthy();
});

test("a busy session shows a plain Working row, and an empty chat says so", async () => {
  render([session("s-1", "p", { status: { state: "working" } })], "/agents/s-1", true, chatOf(chat()));
  expect((await screen.findByRole("status")).textContent).toBe("Working");
  cleanup();
  render([session("s-1", "p")], "/agents/s-1", true, chatOf(chat({ messages: [] })));
  expect(await screen.findByText("No messages yet")).toBeTruthy();
});

test("a round in the chat shows no question sheet", async () => {
  const round = [{ label: "Pick", title: "Which?", options: [{ label: "A" }] }] as unknown as SessionChatResponse["round"];
  render([session("s-1", "p")], "/agents/s-1", true, chatOf(chat({ round })));
  expect(await screen.findByText("hello there")).toBeTruthy();
  expect(screen.queryByText("Which?")).toBeNull();
  expect(screen.queryByRole("group", { name: "View" })).toBeNull();
});

test("a failed re-read keeps the last chat", async () => {
  let calls = 0;
  const { emit } = render([session("s-1", "p")], "/agents/s-1", true, {
    chat: async () => {
      if (++calls > 1) throw new Error("500");
      return { kind: "ok", detail: chat() };
    },
  });
  expect(await screen.findByText("hello there")).toBeTruthy();
  emit("s-1");
  await vi.waitFor(() => expect(calls).toBe(2));
  expect(screen.getByText("hello there")).toBeTruthy();
});

const switcherSessions = () => [
  session("s-1", "alpha", { name: "plain", dir: "/work/plain" }),
  session("s-2", "alpha", { name: "unread one", unread: { unread: true } }),
  session("s-3", "beta", { name: "needs you", ticket: "gx:m/07", actionable: { reason: "Pick one" } }),
  session("s-4", "beta", { name: "the design", design: true, dir: "/work/design" }),
];

const openSwitcher = () => fireEvent.click(screen.getByRole("button", { name: /plain/ }));
const dialog = () => within(screen.getByRole("dialog", { name: "Switch session" }));
const optionNames = () => dialog().getAllByRole("option").map((o) => o.textContent);

test("the title button opens the switcher with Actionable, Unread, Other sections in order", () => {
  render(switcherSessions(), "/agents/s-1");
  openSwitcher();
  expect(dialog().getAllByRole("group").map((g) => g.getAttribute("aria-label"))).toEqual(["Actionable", "Unread", "Other"]);
  const names = optionNames();
  expect(names[0]).toContain("needs you");
  expect(names[0]).toContain("beta · idle · gx:m/07 · Pick one");
  expect(names[1]).toContain("unread one");
  expect(names.slice(2).join()).toContain("plain");
});

test("Cmd-K and Ctrl-K open the switcher", () => {
  render(switcherSessions(), "/agents/s-1");
  fireEvent.keyDown(window, { key: "k", metaKey: true });
  expect(screen.getByRole("dialog", { name: "Switch session" })).toBeTruthy();
  fireEvent.keyDown(window, { key: "k", metaKey: true });
  expect(screen.queryByRole("dialog")).toBeNull();
  fireEvent.keyDown(window, { key: "k", ctrlKey: true });
  expect(screen.getByRole("dialog", { name: "Switch session" })).toBeTruthy();
});

test("Cmd-K opens the switcher before a session is picked", () => {
  render(switcherSessions(), "/agents");
  fireEvent.keyDown(window, { key: "k", metaKey: true });
  expect(dialog().getAllByRole("option")).toHaveLength(5);
});

test("the filter matches the ticket and the dir", () => {
  render(switcherSessions(), "/agents/s-1");
  openSwitcher();
  const input = dialog().getByRole("combobox");
  fireEvent.change(input, { target: { value: "m/07" } });
  expect(optionNames()).toHaveLength(2);
  expect(optionNames()[0]).toContain("needs you");
  fireEvent.change(input, { target: { value: "/work/design" } });
  expect(optionNames()[0]).toContain("the design");
});

test("a design session is listed while the sidebar hides it, and the current one is tagged", () => {
  render(switcherSessions(), "/agents/s-1");
  expect(rows().map((r) => r.textContent).join()).not.toContain("the design");
  openSwitcher();
  const design = dialog().getAllByRole("option").find((o) => o.textContent?.includes("the design"))!;
  expect(within(design).getByRole("img", { name: "Design" })).toBeTruthy();
  expect(dialog().getAllByRole("option").filter((o) => o.getAttribute("aria-current") === "true")).toHaveLength(1);
  expect(dialog().getByText("current")).toBeTruthy();
});

test("switcher rows use the sidebar marks: purple design diamond and unread dot, ? for blocked", () => {
  const blocked = session("s-5", "beta", { name: "stuck" });
  blocked.status = { state: "blocked" } as SessionInfo["status"];
  render([...switcherSessions(), blocked], "/agents/s-1");
  openSwitcher();
  const option = (text: string) => dialog().getAllByRole("option").find((o) => o.textContent?.includes(text))!;
  expect(within(option("the design")).getByRole("img", { name: "Design" }).textContent).toBe("◆");
  expect(within(option("unread one")).getByRole("img", { name: "Unread" }).className).toBe(
    within(rows().find((r) => r.textContent?.includes("unread one"))!).getByRole("img", { name: "Unread" }).className,
  );
  expect(within(option("stuck")).getByRole("img", { name: "blocked" }).textContent).toBe("?");
});

test("Enter on a row navigates to it, and Escape closes", () => {
  const { router } = render(switcherSessions(), "/agents/s-1");
  openSwitcher();
  fireEvent.keyDown(dialog().getByRole("combobox"), { key: "Enter" });
  expect(router.state.location.pathname).toBe("/agents/s-3");
  expect(screen.queryByRole("dialog")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: /needs you/ }));
  fireEvent.keyDown(window, { key: "Escape" });
  expect(screen.queryByRole("dialog")).toBeNull();
});

// Answers the prompt POST with `answer`; everything else (focus reports) gets an empty object.
const prompts: { id: string; text: string }[] = [];
const stubPrompt = (answer: () => Response) =>
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string, init: { body: string }) => {
      if (url !== "/v1/sessions/prompt") return new Response("{}");
      prompts.push(JSON.parse(init.body));
      return answer();
    }),
  );
const accepted = () => new Response(JSON.stringify({ refused: false }));
const refused = (reason: string, message?: string) => () => new Response(JSON.stringify({ refused: true, reason, message }));

const composerBox = () => screen.findByRole("textbox", { name: "Message" }) as Promise<HTMLTextAreaElement>;
const sendButton = () => screen.getByRole("button", { name: "Send" }) as HTMLButtonElement;
const type = async (text: string) => fireEvent.change(await composerBox(), { target: { value: text } });

test("Send is off while Working, with the Working reason, and the box stays editable", async () => {
  render([session("s-1", "p", { status: { state: "working" } })], "/agents/s-1", true, chatOf(chat()));
  await type("hi");
  expect(sendButton().disabled).toBe(true);
  expect((await composerBox()).value).toBe("hi");
  expect(screen.getByText("Working. Send turns on when the session is idle.")).toBeTruthy();
});

test("the reason line names each state in order", async () => {
  const { update } = render([session("s-1", "p")], "/agents/s-1", true, chatOf(chat()));
  await composerBox();
  update({ connected: false });
  expect(screen.getByText("Offline. Send turns on when gx reconnects.")).toBeTruthy();
  expect(sendButton().disabled).toBe(true);
  cleanup();
  render([session("s-2", "p", { harness: "codex" })], "/agents/s-2", true, chatOf(chat()));
  expect(await screen.findByText("Sending works for Claude sessions only")).toBeTruthy();
  cleanup();
  render([session("s-3", "p", { status: { state: "exited" } })], "/agents/s-3", true, chatOf(chat()));
  expect(await screen.findByText("Exited. Restore it in gx, Agents tab.")).toBeTruthy();
  cleanup();
  render([session("s-4", "p", { status: { state: "rate-limited" } })], "/agents/s-4", true, chatOf(chat()));
  expect(await screen.findByText("Rate limited. Send turns on when the session is idle.")).toBeTruthy();
});

test("Idle with text enables Send; accept clears the draft and the box", async () => {
  prompts.length = 0;
  stubPrompt(accepted);
  render([session("s-1", "p")], "/agents/s-1", true, chatOf(chat()));
  expect(sendButton().disabled).toBe(true);
  await type("hello");
  expect(screen.getByText("Draft saved in this browser")).toBeTruthy();
  expect(localStorage.getItem("gx.draft.composer.s-1")).toBe("hello");
  fireEvent.click(sendButton());
  await vi.waitFor(() => expect(localStorage.getItem("gx.draft.composer.s-1")).toBeNull());
  expect(prompts).toEqual([expect.objectContaining({ id: "s-1", text: "hello" })]);
  expect((await composerBox()).value).toBe("");
  expect(screen.queryByText("Draft saved in this browser")).toBeNull();
});

test("a refusal keeps the draft and shows its reason in red, inline", async () => {
  stubPrompt(refused("prefix-not-allowed", "Messages starting with / or ! are not sent."));
  render([session("s-1", "p")], "/agents/s-1", true, chatOf(chat()));
  await type("/clear");
  fireEvent.click(sendButton());
  const line = await screen.findAllByText("Messages starting with / or ! are not sent.");
  expect(getComputedStyle(line[0]).color).not.toBe(getComputedStyle(screen.getByText("hello there")).color);
  expect((await composerBox()).value).toBe("/clear");
  expect(localStorage.getItem("gx.draft.composer.s-1")).toBe("/clear");
});

test("stale refetches the chat and keeps the draft", async () => {
  stubPrompt(refused("stale"));
  let calls = 0;
  render([session("s-1", "p")], "/agents/s-1", true, {
    chat: async () => {
      calls++;
      return { kind: "ok", detail: chat() };
    },
  });
  await type("keep me");
  await vi.waitFor(() => expect(calls).toBe(1));
  fireEvent.click(sendButton());
  await vi.waitFor(() => expect(calls).toBe(2));
  expect((await composerBox()).value).toBe("keep me");
});

test("a non-Claude session loads no chat, shows the notice, Show screen opens the screen modal and the composer says why it is off", async () => {
  const chatCalls: unknown[] = [];
  const screenCalls: string[] = [];
  render([session("s-2", "p", { harness: "codex" })], "/agents/s-2", true, {
    chat: async () => {
      chatCalls.push(1);
      return { kind: "ok", detail: chat() };
    },
    screen: async (target) => {
      screenCalls.push(target);
      return { kind: "ok", detail: { rows: ["$ codex"] } } as never;
    },
  });
  expect(await screen.findByText("Chat works for Claude sessions only.")).toBeTruthy();
  expect(chatCalls).toEqual([]);
  expect(screen.getByText("Sending works for Claude sessions only")).toBeTruthy();
  expect(sendButton().disabled).toBe(true);
  fireEvent.click(screen.getByRole("button", { name: "Show screen" }));
  expect((await screen.findByLabelText("Screen rows")).textContent).toBe("$ codex");
  expect(screenCalls).toEqual(["s-2"]);
});

test.each([
  { kind: "crashed", over: { status: { state: "exited" }, exit: "crashed" }, label: "Agent crashed", buttons: [] },
  { kind: "blocked", over: { status: { state: "blocked" } }, label: "Waiting at a prompt in the terminal", buttons: ["Screen"] },
  { kind: "rate-limit", over: { status: { state: "rate-limited" } }, label: "Rate limited", buttons: [] },
] as { kind: string; over: Partial<SessionInfo>; label: string; buttons: string[] }[])(
  "a non-Claude session with a $kind failure shows the banner with only its buttons",
  async (c) => {
    render([session("s-2", "p", { ...c.over, harness: "codex", failure: { kind: c.kind, label: c.label } })], "/agents/s-2", true);
    const banner = within(await screen.findByRole("region", { name: "Session failure" }));
    expect(banner.getByText(c.label)).toBeTruthy();
    expect(banner.queryAllByRole("button").map((b) => b.textContent)).toEqual(c.buttons);
    expect(screen.queryByRole("button", { name: "Restore" })).toBeNull();
    expect(screen.queryByRole("button", { name: "New session" })).toBeNull();
    expect(screen.getByText("Chat works for Claude sessions only.")).toBeTruthy();
  },
);

test.each(["auth", "stalled"])("a non-Claude session with a %s failure, which needs a transcript, or none, shows no banner", async (kind) => {
  render([session("s-2", "p", { harness: "codex", failure: { kind, label: "x" } })], "/agents/s-2", true);
  await screen.findByText("Chat works for Claude sessions only.");
  expect(screen.queryByRole("region", { name: "Session failure" })).toBeNull();
  cleanup();
  render([session("s-2", "p", { harness: "codex" })], "/agents/s-2", true);
  await screen.findByText("Chat works for Claude sessions only.");
  expect(screen.queryByRole("region", { name: "Session failure" })).toBeNull();
});

test("a design session shows its plain chat with no sheet, and links to its map's design page", async () => {
  const round = [{ id: "q1", header: "Pick", question: "Which one?", options: [] }] as unknown as SessionChatResponse["round"];
  render([session("s-1", "p", { design: true, ticket: "gx:map/01" })], "/agents/s-1", true, chatOf(chat({ round })));
  expect(await screen.findByText("hello there")).toBeTruthy();
  expect(screen.queryByText("Which one?")).toBeNull();
  expect(screen.getByRole("link", { name: "Open design page →" }).getAttribute("href")).toBe("#/d/gx:map");
  cleanup();
  render([session("s-1", "p", { ticket: "gx:map/01" })], "/agents/s-1", true, chatOf(chat()));
  await screen.findByText("hello there");
  expect(screen.queryByRole("link", { name: "Open design page →" })).toBeNull();
});

test("the draft is back after a remount, and one written through the store shows in the box", async () => {
  render([session("s-1", "p")], "/agents/s-1", true, chatOf(chat()));
  await type("draft");
  cleanup();
  render([session("s-1", "p")], "/agents/s-1", true, chatOf(chat()));
  expect((await composerBox()).value).toBe("draft");
  cleanup();
  localStorage.setItem("gx.draft.composer.s-2", "from the design page");
  render([session("s-2", "p")], "/agents/s-2", true, chatOf(chat()));
  expect((await composerBox()).value).toBe("from the design page");
});

const failureCases: {
  kind: string;
  over: Partial<SessionInfo>;
  label: string;
  text?: string;
  buttons: string[];
  reason?: string;
  sendOff: boolean;
}[] = [
  { kind: "crashed", over: { status: { state: "exited" }, exit: "crashed" }, label: "Agent crashed", buttons: [], reason: "Exited. Restore it in gx, Agents tab.", sendOff: true },
  { kind: "ended", over: { status: { state: "exited" }, exit: "clean" }, label: "Session ended", buttons: [], reason: "Exited. Restore it in gx, Agents tab.", sendOff: true },
  { kind: "auth", over: {}, label: "Not signed in", text: "Run /login", buttons: ["Retry"], sendOff: false },
  { kind: "stalled", over: { status: { state: "working" } }, label: "No progress for 15 min", buttons: ["Screen"], reason: "Working. Send turns on when the session is idle.", sendOff: true },
  { kind: "blocked", over: { status: { state: "blocked" } }, label: "Waiting at a prompt in the terminal", buttons: ["Screen"], reason: "Blocked. Send turns on when the session is idle.", sendOff: true },
  { kind: "rate-limit", over: { status: { state: "rate-limited" } }, label: "Rate limited", text: "resets at 5pm", buttons: [], reason: "Rate limited. Send turns on when the session is idle.", sendOff: true },
];

test.each(failureCases)("$kind failure: label, text, only its buttons, and the composer state", async (c) => {
  const failure = { kind: c.kind, label: c.label, text: c.text };
  render([session("s-1", "p", { ...c.over, failure })], "/agents/s-1", true, chatOf(chat()));
  const banner = within(await screen.findByRole("region", { name: "Session failure" }));
  expect(banner.getByText(c.label)).toBeTruthy();
  if (c.text) expect(banner.getByText(c.text)).toBeTruthy();
  expect(banner.queryAllByRole("button").map((b) => b.textContent)).toEqual(c.buttons);
  expect(screen.queryByRole("button", { name: "Restore" })).toBeNull();
  expect(screen.queryByRole("button", { name: "New session" })).toBeNull();
  expect(screen.getByText("hello there")).toBeTruthy();
  await type("hi");
  if (c.reason) expect(screen.getByText(c.reason)).toBeTruthy();
  expect(sendButton().disabled).toBe(c.sendOff);
});

test("Retry on an auth failure posts continue with the last message id", async () => {
  prompts.length = 0;
  stubPrompt(accepted);
  render([session("s-1", "p", { failure: { kind: "auth", label: "Not signed in" } })], "/agents/s-1", true, chatOf(chat()));
  fireEvent.click(await screen.findByRole("button", { name: "Retry" }));
  await vi.waitFor(() => expect(prompts).toEqual([expect.objectContaining({ id: "s-1", text: "continue", last_message: "m1" })]));
});

test("+ New session opens the form prefilled with the selected session's project, and a create opens the new session", async () => {
  const trees = [
    { project: "p", root: "/dev/p", worktrees: [] },
    { project: "q", root: "/dev/q", worktrees: [] },
  ];
  const posts: unknown[] = [];
  const { router } = render([session("s-1", "q")], "/agents/s-1", true, {
    worktrees: async () => trees,
    createSession: async (body) => (posts.push(body), { session: session("q-9", "q") }),
    chat: async () => ({ kind: "not-found" }),
  });
  fireEvent.click(screen.getByRole("button", { name: "+ New session" }));
  const project = (await screen.findByRole("combobox", { name: "Project" })) as HTMLSelectElement;
  expect(project.value).toBe("q");
  fireEvent.click(screen.getByRole("button", { name: "Create session" }));
  await vi.waitFor(() => expect(router.state.location.pathname).toBe("/agents/q-9"));
  expect(posts).toEqual([expect.objectContaining({ project: "q", dir: "/dev/q" })]);
  expect(screen.queryByRole("dialog", { name: "New session" })).toBeNull();
});

const entryTrees = [
  { project: "p", root: "/dev/p", worktrees: [] },
  { project: "q", root: "/dev/q", worktrees: [] },
];
const entryOptions = { worktrees: async () => entryTrees, chat: async () => ({ kind: "not-found" as const }) };
const formProject = async () => ((await screen.findByRole("combobox", { name: "Project" })) as HTMLSelectElement).value;

test("a group's + opens the form with that project, not the selected session's", async () => {
  render([session("s-1", "p"), session("s-2", "q")], "/agents/s-1", true, entryOptions);
  fireEvent.click(screen.getByRole("button", { name: "New session in q" }));
  expect(await formProject()).toBe("q");
});

test.each([
  ["Pick a session", [session("s-1", "p")]],
  ["No sessions yet", []],
])("the %s panel's New session button opens the form", async (text, sessions) => {
  render(sessions, "/agents", true, entryOptions);
  expect(screen.getByText(text)).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "New session" }));
  expect(await screen.findByRole("dialog", { name: "New session" })).toBeTruthy();
});

test("the switcher ends with + New session, reached by keys, and opens the form with the selected project", async () => {
  render([session("s-1", "p"), session("s-2", "q")], "/agents/s-2", true, entryOptions);
  fireEvent.keyDown(window, { key: "k", metaKey: true });
  const names = optionNames();
  expect(names[names.length - 1]).toContain("+ New session");
  const input = dialog().getByRole("combobox");
  fireEvent.keyDown(input, { key: "ArrowUp" });
  fireEvent.keyDown(input, { key: "Enter" });
  expect(screen.queryByRole("dialog", { name: "Switch session" })).toBeNull();
  expect(await formProject()).toBe("q");
});

test("the switcher's New session row stays while the filter matches nothing", () => {
  render([session("s-1", "p")], "/agents/s-1", true, entryOptions);
  fireEvent.keyDown(window, { key: "k", metaKey: true });
  fireEvent.change(dialog().getByRole("combobox"), { target: { value: "zzz" } });
  expect(optionNames()).toEqual(["+ New session"]);
});

test("phone: #/agents is the list and never redirects to the last session", () => {
  localStorage.setItem("gx.agents.last", "s-1");
  const r = render([session("s-1", "p")], "/agents", true, {}, false);
  expect(r.router.state.location.pathname).toBe("/agents");
  expect(rows()).toHaveLength(1);
});

test("phone: a session shows ‹, the title button and project · status, and ‹ returns to the list", () => {
  render([session("s-1", "gx", { name: "plain" })], "/agents/s-1", true, {}, false);
  expect(screen.getByRole("button", { name: /plain/ })).toBeTruthy();
  expect(screen.getByText("gx · Idle")).toBeTruthy();
  expect(rows()).toHaveLength(0);
  expect(screen.getByRole("link", { name: "All sessions" }).getAttribute("href")).toBe("#/agents");
});

test("phone: the title button opens a full-screen switcher with Close, and Close dismisses it", () => {
  render([session("s-1", "gx", { name: "plain" })], "/agents/s-1", true, {}, false);
  fireEvent.click(screen.getByRole("button", { name: /plain/ }));
  const d = dialog();
  expect(getComputedStyle(d.getByRole("combobox").closest("[role=dialog]")!).position).toBe("fixed");
  fireEvent.click(d.getByRole("button", { name: "Close" }));
  expect(screen.queryByRole("dialog", { name: "Switch session" })).toBeNull();
});

test("phone: the switcher's New session row opens the form as a sheet", async () => {
  render([session("s-1", "gx", { name: "plain" })], "/agents/s-1", true, entryOptions, false);
  fireEvent.click(screen.getByRole("button", { name: /plain/ }));
  fireEvent.click(dialog().getByText("+ New session"));
  const form = await screen.findByRole("dialog", { name: "New session" });
  expect(within(form).getByTestId("grab-handle")).toBeTruthy();
});

test("phone: the gone panel links back to the sessions", () => {
  render([session("s-1", "p")], "/agents/nope", true, {}, false);
  expect(screen.getByRole("link", { name: "Back to sessions" }).getAttribute("href")).toBe("#/agents");
});
