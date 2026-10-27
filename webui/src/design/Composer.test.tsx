import {
  cleanup,
  fireEvent,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { afterEach, expect, test, vi } from "vitest";
import {
  ConnectionContext,
  type Connection,
  type ConnectionState,
} from "../connection";
import type {
  EpicDetailRow,
  MapStep,
  SessionChatResponse,
  SessionInfo,
  SessionResult,
  SessionRoundCard,
} from "../generated/server";
import { appRoutes } from "../routes";
import { renderThemed } from "../ui/render";
import { WriteError } from "../useWrite";
import type { FetchChat } from "./chat";
import type { PostPrompt } from "./prompt";

afterEach(cleanup);

const session = (
  state = "idle",
  background = false,
  auto: Partial<SessionInfo> = {},
): SessionInfo => ({
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
  auto_advance: true,
  ...auto,
});

const chatOf = (
  over: Partial<SessionChatResponse> = {},
): SessionChatResponse => ({
  conversation_id: "c1",
  messages: [
    {
      id: "m9",
      role: "agent",
      kind: "text",
      text: "hi",
      at: new Date().toISOString(),
    },
  ],
  // The server sends null, not [], when the last turn holds no round.
  round: null,
  background: [],
  ...over,
});

const runEnd: SessionChatResponse["next"] = {
  action: { action: ["/clear", "/gx-design a"] },
};
const runEndNoAction: SessionChatResponse["next"] = {
  action: { reason: "✅ Done: Next: nothing" },
};

const STEP_LABELS: Record<string, string> = {
  chart: "Chart",
  "resolve-tickets": "Resolve tickets",
  spec: "Spec",
  review: "Review",
  "grill-findings": "Grill findings",
  "create-tickets": "Create tickets",
  branch: "Branch",
  done: "Done",
};
const stepsAt = (current: string): MapStep[] => {
  const ids = Object.keys(STEP_LABELS);
  return ids.map((step, i) => ({
    step,
    label: STEP_LABELS[step],
    state:
      i < ids.indexOf(current) ? "done" : step === current ? "current" : "todo",
  }));
};

const ok: SessionResult = { session: session() };

function render(opts: {
  state?: string;
  background?: boolean;
  auto?: Partial<SessionInfo>;
  chat?: SessionChatResponse;
  connected?: boolean;
  post?: PostPrompt;
  fetchChat?: FetchChat;
  rows?: EpicDetailRow[];
  steps?: MapStep[];
}) {
  const state = {
    snapshot: { sessions: [session(opts.state, opts.background, opts.auto)] },
    connected: opts.connected ?? true,
    loadedOnce: true,
    tick: 0,
  } as unknown as ConnectionState;
  const conn = {
    getState: () => state,
    subscribe: () => () => {},
    onEvent: () => () => {},
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
          rows: opts.rows ?? [],
          queueable: false,
          queued: false,
          steps: opts.steps ?? [],
          impl_address: "gx:a-impl",
        },
      }),
      chat:
        opts.fetchChat ??
        (async () => ({ kind: "ok", detail: opts.chat ?? chatOf() })),
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
  expect(post).toHaveBeenCalledWith({
    id: "s-a",
    text: "hello there",
    last_message: "m9",
  });
  await waitFor(() => expect(box.value).toBe(""));
});

test("opening the composer focuses the Message textarea", async () => {
  render({ post: vi.fn<PostPrompt>(async () => ok) });
  const box = await openNote();
  expect(document.activeElement).toBe(box);
});

test("the composer is disabled while the session is Working", async () => {
  render({
    state: "working",
    post: vi.fn<PostPrompt>(async () => ok),
    chat: chatOf({ next: runEnd }),
  });
  const box = await openNote();
  expect(box.disabled).toBe(true);
  expect(screen.queryByRole("button", { name: /Continue/ })).toBeNull();
});

const expand = () => screen.getByLabelText("Open the note in a large editor");

test("the box is 5 rows, scrolls into view and has the expand button; no round: Message to the agent", async () => {
  const scrollIntoView = vi.fn();
  Element.prototype.scrollIntoView = scrollIntoView;
  const post = vi.fn<PostPrompt>(async () => ok);
  render({ post });
  const box = await openNote();
  expect(box.rows).toBe(5);
  expect(scrollIntoView).toHaveBeenCalled();
  fireEvent.click(expand());
  expect(screen.getByRole("dialog").getAttribute("aria-label")).toBe(
    "Message to the agent",
  );
  fireEvent.change(screen.getByLabelText("Note text in the large editor"), {
    target: { value: "from the modal" },
  });
  fireEvent.click(screen.getByText("Done"));
  expect(box.value).toBe("from the modal");
  fireEvent.click(screen.getByRole("button", { name: "Send" }));
  await waitFor(() => expect(post).toHaveBeenCalledTimes(1));
  expect(post).toHaveBeenCalledWith({
    id: "s-a",
    text: "from the modal",
    last_message: "m9",
  });
});

test("with a round the modal is Note on this round and its text goes in the Note part", async () => {
  Element.prototype.scrollIntoView = vi.fn();
  const post = vi.fn<PostPrompt>(async () => ok);
  render({ post, chat: chatOf({ round: round() }) });
  const box = await openNote();
  expect(box.rows).toBe(5);
  fireEvent.click(expand());
  expect(screen.getByRole("dialog").getAttribute("aria-label")).toBe(
    "Note on this round",
  );
  fireEvent.change(screen.getByLabelText("Note text in the large editor"), {
    target: { value: "round note" },
  });
  fireEvent.click(screen.getByText("Done"));
  expect(box.value).toBe("round note");
  fireEvent.click(screen.getByRole("button", { name: "Send answers" }));
  fireEvent.click(await screen.findByRole("button", { name: "Send anyway" }));
  await waitFor(() => expect(post).toHaveBeenCalledTimes(1));
  expect(post.mock.calls[0][0].text).toMatch(/\n\nNote:\nround note$/);
});

const autoButton = () => screen.findByRole("button", { name: /^Auto:/ });

// The Auto button opens the popover that holds the switches.
const openAuto = async () => {
  fireEvent.click(await autoButton());
  return screen.findByRole("dialog", { name: "Auto" });
};

test("one line: the Auto menu on the left; Note and Send on the right; the note box above the bar", async () => {
  render({ post: vi.fn<PostPrompt>(async () => ok) });
  const auto = await autoButton();
  const note = screen.getByRole("button", { name: "Note" });
  const send = screen.getByRole("button", { name: "Send" });
  const follows = (a: Node, b: Node) =>
    !!(a.compareDocumentPosition(b) & Node.DOCUMENT_POSITION_FOLLOWING);
  expect(follows(auto, note)).toBe(true);
  expect(follows(note, send)).toBe(true);
  const box = await openNote();
  expect(follows(box, auto)).toBe(true);
  const menu = await openAuto();
  const sw = within(menu).getByRole("switch", { name: /Auto-advance/ });
  expect(sw.getAttribute("aria-checked")).toBe("true");
});

test("on a phone Note is icon-only and the Auto button keeps both names", async () => {
  vi.stubGlobal("matchMedia", () => ({
    matches: false,
    addEventListener() {},
    removeEventListener() {},
  }));
  try {
    render({});
    expect((await autoButton()).textContent).toBe("Auto-advanceAuto-queue");
    expect(screen.getByRole("button", { name: "Note" }).textContent).toBe("✎");
  } finally {
    vi.unstubAllGlobals();
  }
});

test("Interrupt replaces Send while working, Continue replaces it after a run-end line", async () => {
  render({ state: "working" });
  await screen.findByRole("button", { name: "Interrupt" });
  expect(screen.queryByRole("button", { name: "Send" })).toBeNull();
  cleanup();
  render({ chat: chatOf({ next: runEnd }), steps: stepsAt("spec") });
  await screen.findByRole("button", { name: /Continue/ });
  expect(screen.queryByRole("button", { name: "Send" })).toBeNull();
});

test("a Working session offers Interrupt in place of Send, posting the state token after a confirm", async () => {
  const fetchSpy = vi
    .spyOn(globalThis, "fetch")
    .mockResolvedValue(new Response(JSON.stringify(ok), { status: 200 }));
  render({ state: "working", chat: chatOf() });
  const box = await openNote();
  expect(screen.queryByRole("button", { name: "Send" })).toBeNull();
  expect(box.disabled).toBe(true);
  fireEvent.click(screen.getByRole("button", { name: "Interrupt" }));
  const confirm = await screen.findByRole("alertdialog", {
    name: "Interrupt the agent?",
  });
  expect(
    fetchSpy.mock.calls.some(([url]) => url === "/v1/sessions/interrupt"),
  ).toBe(false);
  fireEvent.click(within(confirm).getByRole("button", { name: "Interrupt" }));
  await waitFor(() =>
    expect(fetchSpy).toHaveBeenCalledWith(
      "/v1/sessions/interrupt",
      expect.objectContaining({ method: "POST" }),
    ),
  );
  const body = JSON.parse(
    fetchSpy.mock.calls.find(([url]) => url === "/v1/sessions/interrupt")![1]!
      .body as string,
  );
  expect(body.id).toBe("s-a");
  expect(body.state).toMatch(/^s-a:working:\d+$/);
  fetchSpy.mockRestore();
});

const postedTo = (
  fetchSpy: { mock: { calls: Parameters<typeof fetch>[] } },
  path: string,
) =>
  JSON.parse(
    fetchSpy.mock.calls.find(([url]) => url === path)![1]!.body as string,
  );

test("the auto-advance switch shows on and off, idle or working, and posts a flip", async () => {
  const fetchSpy = vi
    .spyOn(globalThis, "fetch")
    .mockResolvedValue(new Response(JSON.stringify(ok), { status: 200 }));
  render({ state: "working" });
  expect((await autoButton()).getAttribute("aria-label")).toBe(
    "Auto: Auto-advance on, Auto-queue off",
  );
  const on = within(await openAuto()).getByRole("switch", {
    name: /Auto-advance on/,
  });
  expect(on.getAttribute("aria-checked")).toBe("true");
  fireEvent.click(on);
  await waitFor(() =>
    expect(fetchSpy).toHaveBeenCalledWith(
      "/v1/sessions/auto-advance",
      expect.objectContaining({ method: "POST" }),
    ),
  );
  expect(postedTo(fetchSpy, "/v1/sessions/auto-advance")).toMatchObject({
    id: "s-a",
    off: true,
  });
  fetchSpy.mockRestore();
  cleanup();

  const fetchOff = vi
    .spyOn(globalThis, "fetch")
    .mockResolvedValue(new Response(JSON.stringify(ok), { status: 200 }));
  render({ auto: { auto_advance: false } });
  const off = within(await openAuto()).getByRole("switch", {
    name: /Auto-advance off/,
  });
  expect(off.getAttribute("aria-checked")).toBe("false");
  fireEvent.click(off);
  await waitFor(() =>
    expect(postedTo(fetchOff, "/v1/sessions/auto-advance").off).toBe(false),
  );
  fetchOff.mockRestore();
});

test("a countdown shows the seconds left and Cancel posts to the cancel route", async () => {
  const fetchSpy = vi
    .spyOn(globalThis, "fetch")
    .mockResolvedValue(new Response(JSON.stringify(ok), { status: 200 }));
  render({
    auto: { auto_advance_at: new Date(Date.now() + 9500).toISOString() },
  });
  expect(await screen.findByText(/Continuing in 10s/)).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
  await waitFor(() =>
    expect(fetchSpy).toHaveBeenCalledWith(
      "/v1/sessions/auto-advance/cancel",
      expect.objectContaining({ method: "POST" }),
    ),
  );
  expect(postedTo(fetchSpy, "/v1/sessions/auto-advance/cancel").id).toBe("s-a");
  fetchSpy.mockRestore();
});

test("Auto-queue shows in the menu, off and disabled", async () => {
  render({});
  const queue = within(await openAuto()).getByRole("switch", {
    name: "Auto-queue off",
  });
  expect(queue.getAttribute("aria-checked")).toBe("false");
  expect((queue as HTMLButtonElement).disabled).toBe(true);
});

test("without a countdown there is no countdown text or Cancel", async () => {
  render({});
  await autoButton();
  expect(screen.queryByText(/Continuing in/)).toBeNull();
  expect(screen.queryByRole("button", { name: "Cancel" })).toBeNull();
});

test("Cancel in the Interrupt confirm closes it without posting", async () => {
  const fetchSpy = vi
    .spyOn(globalThis, "fetch")
    .mockResolvedValue(new Response(JSON.stringify(ok), { status: 200 }));
  render({ state: "working" });
  fireEvent.click(await screen.findByRole("button", { name: "Interrupt" }));
  const confirm = await screen.findByRole("alertdialog", {
    name: "Interrupt the agent?",
  });
  fireEvent.click(within(confirm).getByRole("button", { name: "Cancel" }));
  await waitFor(() => expect(screen.queryByRole("alertdialog")).toBeNull());
  expect(
    fetchSpy.mock.calls.some(([url]) => url === "/v1/sessions/interrupt"),
  ).toBe(false);
  fetchSpy.mockRestore();
});

test("an Idle session has no Interrupt", async () => {
  render({ post: vi.fn<PostPrompt>(async () => ok) });
  await openNote();
  expect(screen.queryByRole("button", { name: "Interrupt" })).toBeNull();
});

test("a stale refusal keeps the draft and refetches the chat", async () => {
  const post = vi.fn<PostPrompt>(async () => ({
    refused: true,
    reason: "stale",
    message: "the conversation moved; review it first",
  }));
  const fetchChat = vi.fn<FetchChat>(async () => ({
    kind: "ok",
    detail: chatOf(),
  }));
  render({ post, fetchChat });
  const box = await openNote();
  const before = fetchChat.mock.calls.length;
  fireEvent.change(box, { target: { value: "keep me" } });
  fireEvent.click(screen.getByRole("button", { name: "Send" }));
  expect(
    await screen.findByText("the conversation moved; review it first"),
  ).toBeTruthy();
  expect(box.value).toBe("keep me");
  await waitFor(() =>
    expect(fetchChat.mock.calls.length).toBeGreaterThan(before),
  );
});

test("Send is disabled while the prompt is pending, and a server error shows its text in a toast", async () => {
  let fail: (e: Error) => void = () => {};
  const post = vi.fn<PostPrompt>(
    () => new Promise((_, reject) => (fail = reject)),
  );
  render({ post });
  const box = await openNote();
  fireEvent.change(box, { target: { value: "hello" } });
  fireEvent.click(screen.getByRole("button", { name: "Send" }));
  await waitFor(() =>
    expect(
      (screen.getByRole("button", { name: "Send" }) as HTMLButtonElement)
        .disabled,
    ).toBe(true),
  );
  fail(new WriteError("session is gone"));
  expect(await screen.findByText("session is gone")).toBeTruthy();
  expect(box.value).toBe("hello");
});

test("another refusal shows its message and keeps the draft", async () => {
  const post = vi.fn<PostPrompt>(async () => ({
    refused: true,
    reason: "text-too-long",
    message: "text is over 32768 bytes",
  }));
  render({ post });
  const box = await openNote();
  fireEvent.change(box, { target: { value: "long" } });
  fireEvent.click(screen.getByRole("button", { name: "Send" }));
  expect(await screen.findByText("text is over 32768 bytes")).toBeTruthy();
  expect(box.value).toBe("long");
});

test("Continue after a run-end line sends the continue action", async () => {
  const post = vi.fn<PostPrompt>(async () => ok);
  render({ post, chat: chatOf({ next: runEnd }), steps: stepsAt("spec") });
  fireEvent.click(await screen.findByRole("button", { name: /Continue/ }));
  await waitFor(() =>
    expect(post).toHaveBeenCalledWith({
      id: "s-a",
      text: "",
      last_message: "m9",
      continue: true,
    }),
  );
});

test.each([
  "chart",
  "resolve-tickets",
  "spec",
  "review",
  "grill-findings",
  "create-tickets",
])("Continue shows at the %s step", async (step) => {
  render({
    post: vi.fn<PostPrompt>(async () => ok),
    chat: chatOf({ next: runEnd }),
    steps: stepsAt(step),
  });
  expect(await screen.findByRole("button", { name: /Continue/ })).toBeTruthy();
});

test("while tickets resolve, Continue names the map's current ticket and says Continue once", async () => {
  const rows: EpicDetailRow[] = [
    { address: "gx:a/01", title: "Done one", rendered_status: "done" },
    {
      address: "gx:a/02",
      title: "Server pid lock",
      rendered_status: "open",
      current: true,
    },
  ];
  render({
    post: vi.fn<PostPrompt>(async () => ok),
    chat: chatOf({ next: runEnd }),
    rows,
    steps: stepsAt("resolve-tickets"),
  });
  const button = await screen.findByRole("button", {
    name: "Continue Server pid lock",
  });
  expect(button.textContent?.match(/Continue/g)).toHaveLength(1);
});

test("outside Resolve tickets, Continue is subtitled with the step label", async () => {
  render({
    post: vi.fn<PostPrompt>(async () => ok),
    chat: chatOf({ next: runEnd }),
    steps: stepsAt("grill-findings"),
  });
  expect(
    await screen.findByRole("button", { name: "Continue Grill findings" }),
  ).toBeTruthy();
});

test.each(["branch", "done"])(
  "at the %s step the run-end line has no action, so no Continue, and Progress links to the implementation epic",
  async (step) => {
    render({
      post: vi.fn<PostPrompt>(async () => ok),
      chat: chatOf({ next: runEndNoAction }),
      steps: stepsAt(step),
    });
    await screen.findByRole("button", { name: "Note" });
    expect(screen.queryByRole("button", { name: /Continue/ })).toBeNull();
    expect(
      (await screen.findAllByText(/Open implementation epic/)).length,
    ).toBeGreaterThan(0);
  },
);

test("before Branch, Progress has no implementation epic link", async () => {
  render({
    post: vi.fn<PostPrompt>(async () => ok),
    chat: chatOf({ next: runEnd }),
    steps: stepsAt("create-tickets"),
  });
  await screen.findByRole("button", { name: /Continue/ });
  expect(screen.queryByText(/Open implementation epic/)).toBeNull();
});

test("while disconnected the page says so and the buttons are off", async () => {
  render({
    connected: false,
    post: vi.fn<PostPrompt>(async () => ok),
    chat: chatOf({ next: runEnd }),
  });
  expect(
    (
      (await screen.findByRole("button", {
        name: /Continue/,
      })) as HTMLButtonElement
    ).disabled,
  ).toBe(true);
  expect(screen.getByText("Offline, reconnecting…")).toBeTruthy();
  expect((await openNote()).disabled).toBe(true);
});

const askCard = (
  n: number,
  over: Partial<SessionRoundCard> = {},
): SessionRoundCard => ({
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
  fireEvent.change(s.getByRole("textbox", { name: "Answer to question 3" }), {
    target: { value: "free text" },
  });
  fireEvent.click(s.getByRole("button", { name: "Question 1, answered" }));
  fireEvent.click(s.getByRole("button", { name: "Note on question 1" }));
  fireEvent.change(
    s.getByRole("textbox", { name: "Note text for question 1" }),
    { target: { value: "because" } },
  );
  fireEvent.change(await openNote(), { target: { value: "overall" } });
  fireEvent.click(screen.getByRole("button", { name: "Send answers" }));
  await waitFor(() => expect(post).toHaveBeenCalledTimes(1));
  expect(post).toHaveBeenCalledWith({
    id: "s-a",
    text: "Q1 - T1: Alpha\n  Note: because\nQ2 - T2: No\nQ3 - T3: free text\n\nNote:\noverall",
    last_message: "m9",
  });
});

// Q1 answered, Q2 and Q3 not; Send is clicked and the popover is open.
const sendWithGaps = async (post: PostPrompt) => {
  render({ post, chat: chatOf({ round: round() }) });
  const s = within(await sheet());
  fireEvent.click(s.getByRole("button", { name: "Beta" }));
  fireEvent.click(s.getByRole("button", { name: "Question 3" }));
  fireEvent.click(screen.getByRole("button", { name: "Send answers" }));
  const popover = await screen.findByRole("alertdialog");
  // The popover settles a tick after it opens: focus lands, then it hears Escape and outside presses.
  await waitFor(() =>
    expect(document.activeElement).toBe(
      within(popover).getByRole("button", { name: "Send anyway" }),
    ),
  );
  return { s, popover };
};

test("Send with gaps opens a popover listing the unanswered questions; Send anyway has focus and sends", async () => {
  const post = vi.fn<PostPrompt>(async () => ok);
  const { popover } = await sendWithGaps(post);
  const p = within(popover);
  expect(popover.getAttribute("aria-label")).toBe("Unanswered questions");
  expect(p.getByText("2 questions are unanswered")).toBeTruthy();
  expect(p.getByText("1 of 3 answered")).toBeTruthy();
  expect(p.getAllByRole("listitem").map((li) => li.textContent)).toEqual([
    "Q2T2›",
    "Q3T3›",
  ]);
  expect(p.queryByRole("button", { name: "Answer them" })).toBeNull();
  await waitFor(() =>
    expect(document.activeElement).toBe(
      p.getByRole("button", { name: "Send anyway" }),
    ),
  );
  expect(post).not.toHaveBeenCalled();

  fireEvent.click(p.getByRole("button", { name: "Send anyway" }));
  await waitFor(() => expect(post).toHaveBeenCalledTimes(1));
  expect(post.mock.calls[0][0].text).toBe(
    "Q1 - T1: Beta\nQ2 - T2: (unanswered)\nQ3 - T3: (unanswered)",
  );
  await waitFor(() => expect(screen.queryByRole("alertdialog")).toBeNull());
});

test("one gap says 'question is unanswered'", async () => {
  const { s } = await sendWithGaps(vi.fn<PostPrompt>(async () => ok));
  fireEvent.click(s.getByRole("button", { name: "Question 2" }));
  fireEvent.click(s.getByRole("button", { name: "Sure" }));
  fireEvent.click(screen.getByRole("button", { name: "Send answers" }));
  fireEvent.click(screen.getByRole("button", { name: "Send answers" }));
  expect(
    within(await screen.findByRole("alertdialog")).getByText(
      "1 question is unanswered",
    ),
  ).toBeTruthy();
});

test("Escape closes the popover and focuses Send", async () => {
  const post = vi.fn<PostPrompt>(async () => ok);
  const { popover } = await sendWithGaps(post);
  fireEvent.keyDown(popover, { key: "Escape" });
  await waitFor(() => expect(screen.queryByRole("alertdialog")).toBeNull());
  await waitFor(() =>
    expect(document.activeElement).toBe(
      screen.getByRole("button", { name: "Send answers" }),
    ),
  );
  expect(post).not.toHaveBeenCalled();
});

test("an outside click closes the popover and focuses Send", async () => {
  const post = vi.fn<PostPrompt>(async () => ok);
  await sendWithGaps(post);
  // The popover listens for outside presses a tick after it opens, so press until it hears one.
  await waitFor(() => {
    fireEvent.pointerDown(document.body);
    fireEvent.pointerUp(document.body);
    expect(screen.queryByRole("alertdialog")).toBeNull();
  });
  await waitFor(() =>
    expect(document.activeElement).toBe(
      screen.getByRole("button", { name: "Send answers" }),
    ),
  );
  expect(post).not.toHaveBeenCalled();
});

test("a second Send click closes the popover and sends nothing", async () => {
  const post = vi.fn<PostPrompt>(async () => ok);
  await sendWithGaps(post);
  fireEvent.click(screen.getByRole("button", { name: "Send answers" }));
  await waitFor(() => expect(screen.queryByRole("alertdialog")).toBeNull());
  expect(post).not.toHaveBeenCalled();
});

test("a popover row opens the sheet at that question and focuses its first control, even folded", async () => {
  const post = vi.fn<PostPrompt>(async () => ok);
  const { s, popover: gaps } = await sendWithGaps(post);
  // Folding needs the Chat view; close the popover first and open it again after.
  fireEvent.keyDown(gaps, { key: "Escape" });
  await waitFor(() => expect(screen.queryByRole("alertdialog")).toBeNull());
  fireEvent.click(screen.getByRole("button", { name: "Chat" }));
  fireEvent.click(s.getByRole("button", { name: "Fold questions" }));
  fireEvent.click(screen.getByRole("button", { name: "Send answers" }));
  const popover = await screen.findByRole("alertdialog");
  await waitFor(() =>
    expect(document.activeElement).toBe(
      within(popover).getByRole("button", { name: "Send anyway" }),
    ),
  );
  fireEvent.click(within(popover).getByRole("button", { name: /Q3/ }));
  await waitFor(() => expect(screen.queryByRole("alertdialog")).toBeNull());
  await waitFor(() =>
    expect(document.activeElement).toBe(
      s.getByRole("textbox", { name: "Answer to question 3" }),
    ),
  );
  expect(
    s.getByRole("button", { name: "Question 3" }).getAttribute("aria-current"),
  ).toBe("step");

  fireEvent.click(screen.getByRole("button", { name: "Send answers" }));
  fireEvent.click(
    within(await screen.findByRole("alertdialog")).getByRole("button", {
      name: /Q2/,
    }),
  );
  await waitFor(() =>
    expect(document.activeElement).toBe(
      s.getByRole("button", { name: "Sure" }),
    ),
  );
  expect(post).not.toHaveBeenCalled();
});

test("Send with every question answered posts at once", async () => {
  const post = vi.fn<PostPrompt>(async () => ok);
  render({ post, chat: chatOf({ round: [askCard(1)] }) });
  fireEvent.click(within(await sheet()).getByRole("button", { name: "Alpha" }));
  fireEvent.click(screen.getByRole("button", { name: "Send answers" }));
  await waitFor(() => expect(post).toHaveBeenCalledTimes(1));
  expect(screen.queryByRole("alertdialog")).toBeNull();
});

test("a stale refusal keeps every answer and note of the round", async () => {
  const post = vi.fn<PostPrompt>(async () => ({
    refused: true,
    reason: "stale",
    message: "the conversation moved",
  }));
  const fetchChat = vi.fn<FetchChat>(async () => ({
    kind: "ok",
    detail: chatOf({ round: round() }),
  }));
  render({ post, fetchChat });
  const s = within(await sheet());
  fireEvent.click(s.getByRole("button", { name: "Alpha" }));
  const box = await openNote();
  fireEvent.change(box, { target: { value: "keep" } });
  const before = fetchChat.mock.calls.length;
  fireEvent.click(screen.getByRole("button", { name: "Send answers" }));
  fireEvent.click(await screen.findByRole("button", { name: "Send anyway" }));
  expect(await screen.findByText("the conversation moved")).toBeTruthy();
  await waitFor(() =>
    expect(fetchChat.mock.calls.length).toBeGreaterThan(before),
  );
  expect(
    screen.getByRole("button", { name: "Question 1, answered" }),
  ).toBeTruthy();
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
  render({
    state: "working",
    background: true,
    post,
    chat: chatOf({ round: [askCard(1)] }),
  });
  const s = within(await sheet());
  fireEvent.click(s.getByRole("button", { name: "Alpha" }));
  fireEvent.click(screen.getByRole("button", { name: "Send answers" }));
  await waitFor(() => expect(post).toHaveBeenCalledTimes(1));
});

test("a question card shows its number and title above the body", async () => {
  render({ chat: chatOf({ round: [askCard(1, { title: "Pick `one`" })] }) });
  const s = within(await sheet());
  expect(
    s.getByText(
      (_, el) => el?.textContent === "Q1 - Pick one" && el.tagName === "SPAN",
    ),
  ).toBeTruthy();
  expect(s.getByText("Body 1?")).toBeTruthy();
});

test("a round's composer has no Focus button", async () => {
  render({ chat: chatOf({ round: [askCard(1)] }) });
  await sheet();
  expect(screen.queryByRole("button", { name: "Focus" })).toBeNull();
});
