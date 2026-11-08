import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { afterEach, expect, test, vi } from "vitest";
import { ConnectionContext, type Connection, type ConnectionState } from "../connection";
import { renderThemed } from "../ui/render";
import { SidebarFooter } from "./SidebarFooter";

afterEach(cleanup);

const meter = (state: string) => ({ level: 1, state, read_at: "" });

function setup(snapshot: Record<string, unknown> = {}, connected = true, onNewWorkspace?: () => void) {
  const state = { snapshot, connected, loadedOnce: true, tick: 0 } as unknown as ConnectionState;
  const conn = { getState: () => state, subscribe: () => () => {}, onEvent: () => () => {} } as unknown as Connection;
  const router = createMemoryRouter([{ path: "/", element: <SidebarFooter onNewWorkspace={onNewWorkspace} /> }]);
  renderThemed(
    <ConnectionContext.Provider value={conn}>
      <RouterProvider router={router} />
    </ConnectionContext.Provider>,
  );
}

const warning = () => screen.queryByTitle("System health is high or critical");
const paused = () => screen.queryByTitle("Server queue is paused");
const offline = () => screen.queryByTitle("Disconnected from the server");

test("warning, paused and disconnected icons hide when they do not apply", () => {
  setup({ mode: "running", health: { cpu: meter("normal"), memory: meter("normal"), disk: meter("normal") } });
  expect(warning()).toBeNull();
  expect(paused()).toBeNull();
  expect(offline()).toBeNull();
});

test("warning, paused and disconnected icons show with their tooltips when they apply", () => {
  setup({ mode: "paused", health: { cpu: meter("normal"), memory: meter("critical"), disk: meter("normal") } }, false);
  expect(warning()).not.toBeNull();
  expect(paused()).not.toBeNull();
  expect(offline()).not.toBeNull();
});

test("the + New menu lists the three items and New workspace calls its prop", async () => {
  const onNewWorkspace = vi.fn();
  setup({}, true, onNewWorkspace);
  fireEvent.click(screen.getByRole("button", { name: "+ New" }));
  expect(await screen.findByRole("menuitem", { name: "New design" })).toBeTruthy();
  expect(screen.getByRole("menuitem", { name: "New chat" })).toBeTruthy();
  fireEvent.click(screen.getByRole("menuitem", { name: "New workspace…" }));
  await waitFor(() => expect(onNewWorkspace).toHaveBeenCalled());
});

test("the Epics badge shows the server total and is absent at zero", () => {
  setup({ needs_you_total: 3 });
  expect(screen.getByRole("link", { name: "Epics · 3 need you" })).toBeTruthy();
  cleanup();
  setup({ needs_you_total: 0 });
  expect(screen.getByRole("link", { name: "Epics" })).toBeTruthy();
});

function stubUsage(ok: boolean) {
  const summary = { mode: "limits", limits: { five_hour: { used_percentage: 10, resets_at: Date.now() / 1000 + 3600, received_at: Date.now() / 1000 } } };
  vi.stubGlobal("fetch", vi.fn(async () => (ok ? new Response(JSON.stringify(summary)) : new Response("", { status: 500 }))));
}

test("the limits tray renders without a health snapshot, and nothing renders without usage", async () => {
  stubUsage(true);
  setup({ mode: "running" });
  expect(await screen.findByRole("button", { name: "Usage limits" })).toBeTruthy();
  cleanup();
  stubUsage(false);
  setup({ mode: "running" });
  await waitFor(() => expect(vi.mocked(fetch)).toHaveBeenCalled());
  expect(screen.queryByRole("button", { name: "Usage limits" })).toBeNull();
  vi.unstubAllGlobals();
});

const followUps = (over: Record<string, unknown>) => ({ project: "gx", open: 3, bugs: 2, ideas: 1, other: 0, triage: "none", left: 0, ...over });

function stubList(follow_ups: unknown[]) {
  vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({ epics: [], follow_ups }))));
}

function setupWithRouter(snapshot: Record<string, unknown> = {}) {
  const state = { snapshot, connected: true, loadedOnce: true, tick: 0 } as unknown as ConnectionState;
  const conn = { getState: () => state, subscribe: () => () => {}, onEvent: () => () => {} } as unknown as Connection;
  const router = createMemoryRouter([
    { path: "/", element: <SidebarFooter /> },
    { path: "/epics", element: <div>epics page</div> },
    { path: "/triage/:project", element: <div>triage page</div> },
  ]);
  renderThemed(
    <ConnectionContext.Provider value={conn}>
      <RouterProvider router={router} />
    </ConnectionContext.Provider>,
  );
  return router;
}

test("the follow-ups button sums the projects and its popover has a line per project", async () => {
  stubList([followUps({ open: 12, bugs: 8, ideas: 4 }), followUps({ project: "webapp", open: 4, bugs: 3, ideas: 1 })]);
  setupWithRouter();
  fireEvent.click(await screen.findByRole("button", { name: "Follow-ups · 16 open" }));
  expect(await screen.findByRole("menuitem", { name: /gx.*8 bugs · 4 ideas/ })).toBeTruthy();
  expect(screen.getByRole("menuitem", { name: /webapp.*3 bugs · 1 idea/ })).toBeTruthy();
  vi.unstubAllGlobals();
});

test("a line opens the Epics page with the card focus, and a live Triage line opens the Triage page", async () => {
  stubList([followUps({}), followUps({ project: "webapp", triage: "waiting", left: 12 })]);
  const router = setupWithRouter();
  fireEvent.click(await screen.findByRole("button", { name: /^Follow-ups/ }));
  fireEvent.click(await screen.findByRole("menuitem", { name: /webapp.*Triage · 12 left/ }));
  await waitFor(() => expect(router.state.location.pathname).toBe("/triage/webapp"));
  await router.navigate("/");
  fireEvent.click(await screen.findByRole("button", { name: /^Follow-ups/ }));
  fireEvent.click(await screen.findByRole("menuitem", { name: /gx.*2 bugs/ }));
  await waitFor(() => expect(router.state.location.pathname).toBe("/epics"));
  expect(router.state.location.state).toEqual({ focus: "follow-ups:gx" });
  vi.unstubAllGlobals();
});

test("the health button and the gear carry a title tooltip", () => {
  setup({ mode: "running", health: { cpu: meter("normal"), memory: meter("normal"), disk: meter("normal") } });
  expect(screen.getByRole("button", { name: /^System health:/ }).getAttribute("title")).toMatch(/^System health:/);
  expect(screen.getByRole("button", { name: "Menu" }).getAttribute("title")).toBe("Menu");
});
