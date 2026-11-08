import { cleanup, fireEvent, screen } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { afterEach, expect, test, vi } from "vitest";
import { ConnectionContext, type Connection, type ConnectionState } from "../connection";
import { PageHeader } from "../page/PageHeader";
import { renderThemed } from "../ui/render";
import { DeviceContext, type Device } from "../useWide";
import { AppShell } from "./AppShell";

afterEach(cleanup);

function setup(device: Device, needsYou = 0) {
  const state = { snapshot: { needs_you_total: needsYou, build: "v1.2.3" }, connected: true, loadedOnce: true, tick: 0 } as unknown as ConnectionState;
  const conn = { getState: () => state, subscribe: () => () => {}, onEvent: () => () => {} } as unknown as Connection;
  const router = createMemoryRouter(
    [
      {
        element: <AppShell />,
        children: [
          {
            path: "/",
            element: (
              <>
                <PageHeader title="Page" />
                <a href="#/x">item</a>
              </>
            ),
          },
        ],
      },
    ],
    { initialEntries: ["/"] },
  );
  renderThemed(
    <DeviceContext.Provider value={device}>
      <ConnectionContext.Provider value={conn}>
        <RouterProvider router={router} />
      </ConnectionContext.Provider>
    </DeviceContext.Provider>,
  );
}

// A hidden aside has no accessible name in the testing library, so find it by its label.
const sidebar = () => document.querySelector<HTMLElement>('aside[aria-label="Sidebar"]')!;
const isOpen = () => sidebar().getAttribute("data-open") === "true";
const mascot = () => screen.queryByRole("button", { name: "Show sidebar" });

test("desktop docks the sidebar at 280px, with the brand, the build and a hide button", () => {
  setup("desktop");
  expect(isOpen()).toBe(true);
  expect(getComputedStyle(sidebar()).width).toBe("280px");
  expect(sidebar().querySelector("img")?.getAttribute("src")).toBe("/gx-logo-wide.png");
  expect(screen.getByText("v1.2.3")).toBeTruthy();
  expect(mascot()).toBeNull();
});

test("desktop: hiding the sidebar shows the mascot, which brings it back", () => {
  setup("desktop");
  fireEvent.click(screen.getByRole("button", { name: "Hide sidebar" }));
  expect(isOpen()).toBe(false);
  fireEvent.click(mascot()!);
  expect(isOpen()).toBe(true);
  expect(mascot()).toBeNull();
});

test.each(["tablet", "phone"] as const)("%s: closed by default, the mascot opens it with a scrim, picking a link closes it", (device) => {
  setup(device);
  expect(isOpen()).toBe(false);
  expect(screen.queryByTestId("scrim")).toBeNull();
  fireEvent.click(mascot()!);
  expect(isOpen()).toBe(true);
  expect(sidebar().getAttribute("data-device")).toBe(device);
  expect(screen.getByTestId("scrim")).toBeTruthy();
  fireEvent.click(screen.getByTestId("scrim"));
  expect(isOpen()).toBe(false);
  fireEvent.click(mascot()!);
  const link = document.createElement("a");
  sidebar().appendChild(link);
  fireEvent.click(link);
  expect(isOpen()).toBe(false);
});

test.each([
  ["tablet", false],
  ["phone", false],
  ["desktop", true],
] as const)("%s: More settings in the gear menu closes a sliding sidebar only (stays open: %s)", async (device, staysOpen) => {
  setup(device);
  if (device !== "desktop") fireEvent.click(mascot()!);
  fireEvent.click(screen.getByRole("button", { name: "Menu" }));
  fireEvent.click(await screen.findByRole("menuitem", { name: "More settings..." }));
  expect(isOpen()).toBe(staysOpen);
});

test("the amber dot shows only while the sidebar is closed and something needs me", () => {
  setup("tablet", 2);
  expect(screen.getByTestId("mascot-dot")).toBeTruthy();
  fireEvent.click(mascot()!);
  expect(screen.queryByTestId("mascot-dot")).toBeNull();
});

test("no dot when nothing needs me", () => {
  setup("tablet", 0);
  expect(screen.queryByTestId("mascot-dot")).toBeNull();
});

test.each(["desktop", "tablet", "phone"] as const)("%s: the header search button opens the jump menu", async (device) => {
  vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({ epics: [], follow_ups: [] }), { status: 200 })));
  setup(device);
  if (device !== "desktop") fireEvent.click(mascot()!);
  fireEvent.click(screen.getByRole("button", { name: "Search" }));
  expect(await screen.findByRole("dialog", { name: "Jump to" })).toBeTruthy();
  fireEvent.keyDown(window, { key: "Escape" });
  expect(screen.queryByRole("dialog")).toBeNull();
});

test.each([
  ["MacIntel", "Search (⌘K)"],
  ["Linux x86_64", "Search (Ctrl+K)"],
])("the search button's tooltip names the platform key (%s)", (platform, title) => {
  vi.spyOn(navigator, "platform", "get").mockReturnValue(platform);
  setup("desktop");
  expect(screen.getByRole("button", { name: "Search" }).getAttribute("title")).toBe(title);
  vi.restoreAllMocks();
});
