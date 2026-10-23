import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { afterEach, expect, test, vi } from "vitest";
import { ConnectionContext, type Connection, type ConnectionState } from "../connection";
import type { MapStep, SessionChatResponse, SessionFailure, SessionInfo, SessionResult } from "../generated/server";
import { appRoutes } from "../routes";
import { renderThemed } from "../ui/render";
import type { ResumeSession } from "./open";
import type { PostPrompt } from "./prompt";
import type { FetchScreen } from "./screen";

afterEach(cleanup);

const session = (state: string, failure?: SessionFailure, harness = "claude"): SessionInfo => ({
  id: "s-a",
  project: "gx",
  dir: "/x",
  harness,
  ticket: "gx:a/01",
  design: true,
  status: { state } as SessionInfo["status"],
  status_changed_at: new Date().toISOString(),
  run_started_at: new Date().toISOString(),
  unread: {},
  actionable: {},
  failure,
});

const chat: SessionChatResponse = {
  conversation_id: "c1",
  messages: [{ id: "m9", role: "agent", kind: "text", text: "hi", at: new Date().toISOString() }],
  round: null,
  background: [],
};

const ok: SessionResult = { session: session("idle") };

const steps = (done: boolean): MapStep[] => [
  { step: "chart", label: "Chart", state: "done" },
  { step: "done", label: "Done", state: done ? "done" : "todo" },
];

function render(
  s: SessionInfo,
  opts: { done?: boolean; post?: PostPrompt; resume?: ResumeSession; screen?: FetchScreen; chat?: SessionChatResponse } = {},
) {
  const state = { snapshot: { sessions: [s] }, connected: true, loadedOnce: true, tick: 0 } as unknown as ConnectionState;
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
        detail: { address: "gx:a", kind: "map", summary: "", done: 0, total: 0, agent_time: 0, cost_total: 0, stats: {}, rows: [], queueable: false, queued: false, steps: steps(!!opts.done), impl_address: "gx:a-impl" },
      }),
      chat: async () => ({ kind: "ok", detail: opts.chat ?? chat }),
      prompt: opts.post,
      resume: opts.resume,
      screen: opts.screen,
    }),
    { initialEntries: ["/d/gx:a"] },
  );
  renderThemed(
    <ConnectionContext.Provider value={conn}>
      <RouterProvider router={router} />
    </ConnectionContext.Provider>,
  );
}

const banner = () => screen.findByRole("region", { name: "Session failure" });
const buttonsOf = (el: HTMLElement) => within(el).queryAllByRole("button").map((b) => b.textContent);
const composer = () => screen.queryByRole("form", { name: "Composer" });

test.each([
  ["crashed", "exited", "Crashed", ["Restore", "New session"], true],
  ["ended", "exited", "Ended", ["Restore", "New session"], true],
  ["auth", "blocked", "Signed out", ["Retry"], false],
  ["rate-limit", "blocked", "Rate limited", [], false],
  ["stalled", "working", "Stalled", ["Screen", "New session"], false],
  ["blocked", "blocked", "Blocked", ["Screen"], false],
])("a %s failure shows its label, text and buttons", async (kind, state, label, buttons, replaces) => {
  render(session(state, { kind, label, text: `${kind} text` }));
  const el = await banner();
  expect(within(el).getByText(label)).toBeTruthy();
  expect(within(el).getByText(`${kind} text`)).toBeTruthy();
  expect(buttonsOf(el)).toEqual(buttons);
  await waitFor(() => expect(composer() === null).toBe(replaces));
});

test("a live failure keeps the composer, disabled unless the session is Idle", async () => {
  render(session("blocked", { kind: "auth", label: "Signed out" }));
  await banner();
  fireEvent.click(await screen.findByRole("button", { name: "Note" }));
  expect(((await screen.findByLabelText("Message")) as HTMLTextAreaElement).disabled).toBe(true);
});

test("a rate limit without text shows its label alone", async () => {
  render(session("blocked", { kind: "rate-limit", label: "Rate limited" }));
  const el = await banner();
  expect(el.textContent).toBe("Rate limited");
});

test("Retry sends continue with the last message id", async () => {
  const post = vi.fn<PostPrompt>(async () => ok);
  render(session("blocked", { kind: "auth", label: "Signed out" }), { post });
  fireEvent.click(await screen.findByRole("button", { name: "Retry" }));
  await waitFor(() => expect(post).toHaveBeenCalledWith({ id: "s-a", text: "continue", last_message: "m9" }));
});

test("a Done Design hides the composer and the ended banner and links the implementation epic", async () => {
  render(session("exited", { kind: "ended", label: "Ended" }), { done: true });
  expect(await screen.findByRole("link", { name: /Open implementation epic/ })).toBeTruthy();
  expect(screen.queryByRole("region", { name: "Session failure" })).toBeNull();
  expect(composer()).toBeNull();
});

test("a Done Design still shows a crashed banner", async () => {
  render(session("exited", { kind: "crashed", label: "Crashed" }), { done: true });
  expect(buttonsOf(await banner())).toEqual(["Restore", "New session"]);
  expect(composer()).toBeNull();
});

test("a non-Claude session shows the kinds that need no transcript", async () => {
  render(session("exited", { kind: "crashed", label: "Crashed" }, "codex"));
  expect(buttonsOf(await banner())).toEqual(["Restore", "New session"]);
  cleanup();
  render(session("blocked", { kind: "auth", label: "Signed out" }, "codex"));
  await screen.findByText(/Chat works for Claude sessions only/);
  expect(screen.queryByRole("region", { name: "Session failure" })).toBeNull();
});

const crashed = (recrashed?: boolean) => session("exited", { kind: "crashed", label: "Crashed", recrashed });
const refusal = (reason: string): SessionResult => ({ refused: true, reason, message: `refused ${reason}` });

test("Restore sends the state token and keeps a half-answered round's draft", async () => {
  const resume = vi.fn<ResumeSession>(async () => ok);
  const round: SessionChatResponse = {
    ...chat,
    round: [{ label: "1", title: "Name", body: "What name?", kind: "text" }],
  };
  render(crashed(), { resume, chat: round });
  const answer = (await screen.findByLabelText("Answer to question 1")) as HTMLTextAreaElement;
  fireEvent.change(answer, { target: { value: "half an answer" } });
  fireEvent.click(await screen.findByRole("button", { name: "Restore" }));
  await waitFor(() => expect(resume).toHaveBeenCalledTimes(1));
  const body = resume.mock.calls[0][0];
  expect(body.id).toBe("s-a");
  expect(body.state).toMatch(/^s-a:exited:\d+$/);
  expect(((await screen.findByLabelText("Answer to question 1")) as HTMLTextAreaElement).value).toBe("half an answer");
});

test("a quick re-crash hides Restore and makes New session primary", async () => {
  render(crashed(true));
  const el = await banner();
  expect(buttonsOf(el)).toEqual(["New session"]);
  expect(within(el).getByRole("button", { name: "New session" }).getAttribute("data-tone")).toBe("running");
});

test.each(["dir-gone", "no-resume"])("a %s refusal hides Restore and makes New session primary", async (reason) => {
  const resume = vi.fn<ResumeSession>(async () => refusal(reason));
  render(crashed(), { resume });
  fireEvent.click(await screen.findByRole("button", { name: "Restore" }));
  await waitFor(() => expect(buttonsOf(screen.getByRole("region", { name: "Session failure" }))).toEqual(["New session"]));
});

test("another refusal keeps Restore and shows its message", async () => {
  const resume = vi.fn<ResumeSession>(async () => refusal("session-live"));
  render(crashed(), { resume });
  fireEvent.click(await screen.findByRole("button", { name: "Restore" }));
  expect((await screen.findByRole("alert")).textContent).toBe("refused session-live");
  expect(buttonsOf(await banner())).toEqual(["Restore", "New session"]);
});

test("Screen shows the rows and Refresh fetches them again", async () => {
  const fetchRows = vi
    .fn<FetchScreen>()
    .mockResolvedValueOnce({ kind: "ok", detail: { rows: ["first row", "second"] } })
    .mockResolvedValueOnce({ kind: "ok", detail: { rows: ["changed row"] } });
  render(session("blocked", { kind: "blocked", label: "Blocked" }), { screen: fetchRows });
  fireEvent.click(await screen.findByRole("button", { name: "Screen" }));
  const dialog = await screen.findByRole("dialog", { name: "Screen" });
  await within(dialog).findByText(/first row/);
  expect(fetchRows).toHaveBeenCalledWith("s-a");
  fireEvent.click(within(dialog).getByRole("button", { name: "Refresh" }));
  await within(dialog).findByText(/changed row/);
  expect(fetchRows).toHaveBeenCalledTimes(2);
});

test("a gx-raised Actionable shows why gx stopped, above the composer", async () => {
  render({ ...session("idle"), actionable: { reason: "Continue failed: step 0: prompt never went working", gx_raised: true } });
  const el = await screen.findByRole("region", { name: "gx stopped" });
  expect(within(el).getByText("Continue failed: step 0: prompt never went working")).toBeTruthy();
  expect(composer()).toBeTruthy();
});

test("an agent-raised Actionable shows no gx stopped banner", async () => {
  render({ ...session("idle"), actionable: { reason: "Grilling: waiting for your answers" } });
  await screen.findByRole("form", { name: "Composer" });
  expect(screen.queryByRole("region", { name: "gx stopped" })).toBeNull();
});
