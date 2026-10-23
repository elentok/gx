import { act, cleanup } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { afterEach, expect, test, vi } from "vitest";
import { ConnectionContext, type Connection, type ConnectionState } from "../connection";
import type { SessionInfo } from "../generated/server";
import { appRoutes } from "../routes";
import { renderThemed } from "../ui/render";
import type { PostFocus } from "./focus";

afterEach(cleanup);

const session = (id: string, changed: string): SessionInfo => ({
  id,
  project: "gx",
  dir: "/x",
  harness: "claude",
  ticket: "gx:a/01",
  design: true,
  status: { state: "idle" } as SessionInfo["status"],
  status_changed_at: changed,
  run_started_at: changed,
  unread: {},
  actionable: {},
});

function setVisibility(v: "visible" | "hidden") {
  Object.defineProperty(document, "visibilityState", { value: v, configurable: true });
  document.dispatchEvent(new Event("visibilitychange"));
}

function render(focus: PostFocus) {
  let state = {
    snapshot: { sessions: [session("s-a", "2026-01-01T00:00:00Z")] },
    connected: true,
    loadedOnce: true,
    tick: 0,
    streams: 1,
  } as unknown as ConnectionState;
  const listeners = new Set<() => void>();
  const conn = {
    clientId: "tab-1",
    getState: () => state,
    subscribe: (l: () => void) => (listeners.add(l), () => listeners.delete(l)),
    onEvent: () => () => {},
  } as unknown as Connection;
  const update = (patch: Partial<ConnectionState>) => {
    state = { ...state, ...patch };
    listeners.forEach((l) => l());
  };
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
        detail: { address: "gx:a", kind: "map", summary: "", done: 0, total: 0, agent_time: 0, cost_total: 0, rows: [], queueable: false, queued: false, steps: [] },
      }),
      chat: async () => ({ kind: "ok", detail: { conversation_id: "c1", messages: [], round: null, background: [] } }),
      focus,
    }),
    { initialEntries: ["/d/gx:a"] },
  );
  renderThemed(
    <ConnectionContext.Provider value={conn}>
      <RouterProvider router={router} />
    </ConnectionContext.Provider>,
  );
  return { update, router };
}

test("reports on open, on visibility change and on a session switch", async () => {
  setVisibility("visible");
  const focus = vi.fn<PostFocus>(async () => ({}));
  const { update } = render(focus);
  await vi.waitFor(() => expect(focus).toHaveBeenCalledWith({ client: "tab-1", focused: true, session: "s-a" }));

  act(() => setVisibility("hidden"));
  await vi.waitFor(() => expect(focus).toHaveBeenLastCalledWith({ client: "tab-1", focused: false, session: "s-a" }));

  act(() =>
    update({ snapshot: { sessions: [session("s-a", "2026-01-01T00:00:00Z"), session("s-b", "2026-02-01T00:00:00Z")] } as ConnectionState["snapshot"] }),
  );
  await vi.waitFor(() => expect(focus).toHaveBeenLastCalledWith({ client: "tab-1", focused: false, session: "s-b" }));
  setVisibility("visible");
});

test("leaving the Design page reports no focus and no session", async () => {
  setVisibility("visible");
  const focus = vi.fn<PostFocus>(async () => ({}));
  const { update, router } = render(focus);
  await vi.waitFor(() => expect(focus).toHaveBeenLastCalledWith({ client: "tab-1", focused: true, session: "s-a" }));

  act(() =>
    update({ snapshot: { sessions: [session("s-a", "2026-01-01T00:00:00Z"), session("s-b", "2026-02-01T00:00:00Z")] } as ConnectionState["snapshot"] }),
  );
  await vi.waitFor(() => expect(focus).toHaveBeenLastCalledWith({ client: "tab-1", focused: true, session: "s-b" }));
  expect(focus).not.toHaveBeenCalledWith({ client: "tab-1", focused: false, session: "" });

  await act(() => router.navigate("/"));
  await vi.waitFor(() => expect(focus).toHaveBeenLastCalledWith({ client: "tab-1", focused: false, session: "" }));
});
