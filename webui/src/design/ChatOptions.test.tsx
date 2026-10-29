import { cleanup, screen } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { afterEach, expect, test } from "vitest";
import { ConnectionContext, type Connection, type ConnectionState } from "../connection";
import type { SessionChatResponse, SessionFailure, SessionInfo } from "../generated/server";
import { renderThemed } from "../ui/render";
import { ALL_OPTIONS, DesignChat, type ChatOptions } from "./ChatLog";

afterEach(() => {
  cleanup();
  localStorage.clear();
});

const OFF: ChatOptions = { sheet: false, steps: false, gxStopped: false, done: false, restore: false, composer: true };

const session = (over: Partial<SessionInfo> = {}): SessionInfo => ({
  id: "s-a",
  project: "gx",
  dir: "/x",
  harness: "claude",
  ticket: "gx:a/01",
  design: true,
  status: { state: "idle" },
  status_changed_at: new Date().toISOString(),
  run_started_at: new Date().toISOString(),
  unread: {},
  actionable: {},
  auto_advance: true,
  ...over,
});

const chat = (over: Partial<SessionChatResponse> = {}): SessionChatResponse => ({
  conversation_id: "c1",
  messages: [{ id: "m1", role: "agent", kind: "text", text: "hello there", at: new Date().toISOString() }],
  round: null,
  background: [],
  ...over,
});

function render(s: SessionInfo, c: SessionChatResponse, options: ChatOptions, done?: boolean) {
  const state = { snapshot: { sessions: [s] }, connected: true, loadedOnce: true, tick: 0 } as unknown as ConnectionState;
  const conn = { getState: () => state, subscribe: () => () => {}, onEvent: () => () => {} } as unknown as Connection;
  const router = createMemoryRouter([
    {
      path: "/",
      element: (
        <DesignChat
          session={s}
          chat={c}
          refetch={() => {}}
          done={done}
          actions={{ address: "gx:a" }}
          questions={false}
          onQuestions={() => {}}
          options={options}
        />
      ),
    },
  ]);
  renderThemed(
    <ConnectionContext.Provider value={conn}>
      <RouterProvider router={router} />
    </ConnectionContext.Provider>,
  );
}

test("a busy session shows a plain Working row when steps are off", async () => {
  render(session({ status: { state: "working" } }), chat(), OFF);
  expect(await screen.findByText("Working")).toBeTruthy();
});

test("a round shows no sheet and no toggle when the sheet is off", async () => {
  const round = [{ label: "1", title: "t", body: "Which way?", kind: "options" as const, options: [{ id: "a", label: "Alpha" }] }];
  render(session(), chat({ round }), OFF);
  await screen.findByText("hello there");
  expect(screen.queryByText("Which way?")).toBeNull();
  expect(screen.queryByRole("group", { name: "View" })).toBeNull();
  cleanup();
  render(session(), chat({ round }), ALL_OPTIONS);
  expect(await screen.findByRole("group", { name: "View" })).toBeTruthy();
});

test("a gx-raised Actionable shows no gx stopped when gxStopped is off", async () => {
  const s = session({ actionable: { gx_raised: true, reason: "never started" } });
  render(s, chat(), OFF);
  await screen.findByText("hello there");
  expect(screen.queryByLabelText("gx stopped")).toBeNull();
  cleanup();
  render(s, chat(), ALL_OPTIONS);
  expect(await screen.findByLabelText("gx stopped")).toBeTruthy();
});

test("done is ignored when the done option is off", async () => {
  const s = session({ failure: { kind: "ended", label: "Session ended", text: "" } as SessionFailure });
  render(s, chat(), OFF, true);
  expect(await screen.findByText("Session ended")).toBeTruthy();
});

const BUTTONS: [string, string[]][] = [
  ["crashed", []],
  ["ended", []],
  ["auth", ["Retry"]],
  ["stalled", ["Screen"]],
  ["blocked", ["Screen"]],
  ["rate-limit", []],
];

test.each(BUTTONS)("the %s failure notice has the buttons %j when restore is off", async (kind, expected) => {
  const failure = { kind, label: `Failure ${kind}`, text: "" } as SessionFailure;
  render(session({ failure }), chat(), OFF);
  await screen.findByText(`Failure ${kind}`);
  const region = screen.getByRole("region", { name: "Session failure" });
  const names = Array.from(region.querySelectorAll("button")).map((b) => b.textContent);
  expect(names).toEqual(expected);
});
