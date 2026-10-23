import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { afterEach, expect, test, vi } from "vitest";
import { ConnectionContext, type Connection, type ConnectionState } from "../connection";
import type { MapStep, SessionChatResponse, SessionFailure, SessionInfo, SessionResult } from "../generated/server";
import { appRoutes } from "../routes";
import { renderThemed } from "../ui/render";
import type { PostPrompt } from "./prompt";

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
  unread: {},
  actionable: {},
  failure,
});

const chat: SessionChatResponse = {
  conversation_id: "c1",
  messages: [{ id: "m9", role: "agent", kind: "text", text: "hi", at: new Date().toISOString() }],
  round: [],
  background: [],
};

const ok: SessionResult = { session: session("idle") };

const steps = (done: boolean): MapStep[] => [
  { step: "chart", label: "Chart", state: "done" },
  { step: "done", label: "Done", state: done ? "done" : "todo" },
];

function render(s: SessionInfo, opts: { done?: boolean; post?: PostPrompt } = {}) {
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
        detail: { address: "gx:a", kind: "map", summary: "", done: 0, total: 0, agent_time: 0, cost_total: 0, rows: [], queueable: false, queued: false, steps: steps(!!opts.done), impl_address: "gx:a-impl" },
      }),
      chat: async () => ({ kind: "ok", detail: chat }),
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
