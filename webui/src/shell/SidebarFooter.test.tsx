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

test("the health button and the gear carry a title tooltip", () => {
  setup({ mode: "running", health: { cpu: meter("normal"), memory: meter("normal"), disk: meter("normal") } });
  expect(screen.getByRole("button", { name: /^System health:/ }).getAttribute("title")).toMatch(/^System health:/);
  expect(screen.getByRole("button", { name: "Menu" }).getAttribute("title")).toBe("Menu");
});
