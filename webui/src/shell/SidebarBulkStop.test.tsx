import { act, cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { afterEach, expect, test, vi } from "vitest";
import { session } from "../agents/agentsTestKit";
import { ConnectionContext, type Connection, type ConnectionState } from "../connection";
import { CardKindImplementation, SectionImplementing, type EpicListRow, type SessionInfo, type SessionPlacement } from "../generated/server";
import { renderThemed } from "../ui/render";
import { DeviceContext } from "../useWide";
import { AppShell } from "./AppShell";

const toast = vi.hoisted(() => vi.fn());
vi.mock("../ui/toaster", async (orig) => ({ ...(await orig<typeof import("../ui/toaster")>()), showToast: toast }));

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  localStorage.clear();
  toast.mockClear();
});

const blog = {
  address: "p:blog",
  project: "p",
  name: "Blog",
  kind: "loop",
  state: "idle",
  done: 1,
  total: 4,
  queued: false,
  queueable: false,
  blocked_by_epics: [],
  workspace: "Default",
  card_kind: CardKindImplementation,
  section: SectionImplementing,
  status_words: "Idle",
} as EpicListRow;

const placements: SessionPlacement[] = ["a1", "a2", "a3"].map((session) => ({ session, kind: "other" }));
const sessions: SessionInfo[] = [
  session("a1", "p", { name: "alpha" }),
  session("a2", "p", { name: "bravo" }),
  session("a3", "p", { name: "gone", status: { state: "exited" } }),
];

type Call = { route: string; body: { targets: { id: string; state?: string }[]; yes?: boolean } };

function setup(reply: (c: Call) => unknown = (c) => ({ results: c.body.targets.map((t) => ({ target: t.id, status: "done" })) })) {
  const calls: Call[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string, init?: RequestInit) => {
      const route = /\/v1\/sessions\/(\w+)/.exec(String(url))?.[1];
      if (!route) return new Response(JSON.stringify({ epics: [blog], follow_ups: [] }), { status: 200 });
      const call = { route, body: JSON.parse(String(init?.body)) };
      calls.push(call);
      return new Response(JSON.stringify(reply(call)), { status: 200 });
    }),
  );
  let state = {
    snapshot: { needs_you_total: 0, workspaces: [{ name: "Default", default: true, members: 0 }], sessions, placements },
    connected: true,
    loadedOnce: true,
    tick: 0,
  } as unknown as ConnectionState;
  const listeners = new Set<() => void>();
  const conn = {
    getState: () => state,
    subscribe: (l: () => void) => {
      listeners.add(l);
      return () => listeners.delete(l);
    },
    onEvent: () => () => {},
  } as unknown as Connection;
  /** The session leaves the snapshot, as when another client removes it. */
  const drop = (id: string) => {
    state = { ...state, snapshot: { ...state.snapshot, sessions: sessions.filter((s) => s.id !== id) }, tick: state.tick + 1 } as ConnectionState;
    act(() => listeners.forEach((l) => l()));
  };
  const router = createMemoryRouter([{ element: <AppShell />, children: [{ path: "*", element: <div /> }] }], { initialEntries: ["/"] });
  renderThemed(
    <DeviceContext.Provider value="desktop">
      <ConnectionContext.Provider value={conn}>
        <RouterProvider router={router} />
      </ConnectionContext.Provider>
    </DeviceContext.Provider>,
  );
  return Object.assign(calls, { drop });
}

const body = () => within(screen.getByTestId("sidebar-body"));
const box = (name: string) => body().getByRole("checkbox", { name }) as HTMLInputElement;
const bar = () => within(document.querySelector("[data-part='select-bar']") as HTMLElement);

async function selectRows(...names: string[]) {
  await body().findByRole("link", { name: /Blog/ });
  fireEvent.click(screen.getByRole("button", { name: "Select" }));
  fireEvent.click(body().getByRole("button", { name: "Other" }));
  for (const n of names) fireEvent.click(box(`Select ${n}`));
}

const token = (id: string, live = true) => `${id}:2026-01-01T00:00:00Z:${live ? "live" : "exited"}`;

test("Stop confirms once, lists the rows, and posts the token of each live row; the Exited row is skipped not live", async () => {
  const calls = setup();
  await selectRows("alpha", "bravo", "gone");
  const stop = bar().getByRole("button", { name: "Stop 2 of 3" });
  expect(stop.title).toBe("Runs on alpha, bravo\ngone: not live");
  fireEvent.click(stop);
  const dialog = await screen.findByRole("dialog");
  expect(dialog.textContent).toContain("alpha, bravo");
  expect(calls).toHaveLength(0);
  fireEvent.click(within(dialog).getByRole("button", { name: "Stop" }));
  await waitFor(() => expect(calls).toHaveLength(1));
  expect(calls[0]).toEqual({
    route: "stop",
    body: { targets: [{ id: "a1", state: token("a1") }, { id: "a2", state: token("a2") }], yes: false },
  });
});

test("Remove names the live rows in the confirm and sends yes with the tokens", async () => {
  const calls = setup();
  await selectRows("alpha", "gone");
  fireEvent.click(bar().getByRole("button", { name: "Remove 2" }));
  const dialog = await screen.findByRole("dialog");
  expect(dialog.textContent).toContain("Live and stopped first: alpha.");
  fireEvent.click(within(dialog).getByRole("button", { name: "Remove" }));
  await waitFor(() => expect(calls).toHaveLength(1));
  expect(calls[0]).toEqual({
    route: "remove",
    body: { targets: [{ id: "a1", state: token("a1") }, { id: "a3", state: token("a3", false) }], yes: true },
  });
});

test("Keep on the Remove confirm sends nothing and keeps the selection", async () => {
  const calls = setup();
  await selectRows("alpha");
  fireEvent.click(bar().getByRole("button", { name: "Remove 1" }));
  fireEvent.click(within(await screen.findByRole("dialog")).getByRole("button", { name: "Keep" }));
  expect(calls).toHaveLength(0);
  expect(screen.getByTestId("select-count").textContent).toBe("1 selected");
});

test("stale and queue-session results stay selected and toast their reasons; done rows leave", async () => {
  setup((c) => ({
    results: c.body.targets.map((t) =>
      t.id === "a1"
        ? { target: t.id, status: "refused", reason: "stale", message: "changed, reselect" }
        : { target: t.id, status: "refused", reason: "queue-session", message: "managed by the queue" },
    ),
  }));
  await selectRows("alpha", "bravo");
  fireEvent.click(bar().getByRole("button", { name: "Stop 2" }));
  fireEvent.click(within(await screen.findByRole("dialog")).getByRole("button", { name: "Stop" }));
  await waitFor(() => expect(toast).toHaveBeenCalledTimes(2));
  expect(toast.mock.calls.map(([t]) => t.title)).toEqual(["alpha: changed, reselect", "bravo: managed by the queue"]);
  expect(screen.getByTestId("select-count").textContent).toBe("2 selected");
  expect(box("Select alpha").checked).toBe(true);
});

test("a row that leaves the list leaves the selection", async () => {
  const { drop } = setup();
  await selectRows("alpha", "bravo");
  drop("a2");
  await waitFor(() => expect(screen.getByTestId("select-count").textContent).toBe("1 selected"));
  expect(body().queryByRole("checkbox", { name: "Select bravo" })).toBeNull();
});
