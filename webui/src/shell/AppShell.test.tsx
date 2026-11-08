import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { afterEach, describe, expect, test, vi } from "vitest";
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

describe("closing the sidebar closes its menus", () => {
  const closers: [string, () => void][] = [
    ["the toggle", () => fireEvent.click(screen.getByRole("button", { name: "Hide sidebar" }))],
    ["the scrim", () => fireEvent.click(screen.getByTestId("scrim"))],
    ["Ctrl+B", () => fireEvent.keyDown(window, { key: "b", ctrlKey: true })],
    ["a swipe", () => swipe(sidebar(), [200, 300], [100, 300])],
  ];
  test.each(closers)("%s closes an open gear menu", async (_, close) => {
    setup("phone");
    fireEvent.click(mascot()!);
    fireEvent.click(screen.getByRole("button", { name: "Menu" }));
    await screen.findByRole("menuitem", { name: "More settings..." });
    close();
    expect(isOpen()).toBe(false);
    await waitFor(() => expect(screen.queryByRole("menuitem", { name: "More settings..." })).toBeNull());
  });

  test("a dialog from a menu item stays open and blocks Cmd+B and a swipe", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({ epics: [], follow_ups: [], projects: [] }), { status: 200 })));
    setup("phone");
    fireEvent.click(mascot()!);
    fireEvent.click(screen.getByRole("button", { name: "+ New" }));
    fireEvent.click(await screen.findByRole("menuitem", { name: "New design" }));
    await screen.findByRole("dialog");
    fireEvent.keyDown(window, { key: "b", ctrlKey: true });
    swipe(sidebar(), [200, 300], [100, 300]);
    expect(isOpen()).toBe(true);
    expect(screen.getByRole("dialog")).toBeTruthy();
    vi.unstubAllGlobals();
  });
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

function swipe(target: Element, from: [number, number], to: [number, number]) {
  fireEvent.touchStart(target, { touches: [{ clientX: from[0], clientY: from[1] }] });
  fireEvent.touchEnd(target, { changedTouches: [{ clientX: to[0], clientY: to[1] }] });
}

describe("swipe", () => {
  afterEach(() => vi.unstubAllGlobals());
  const standalone = () => vi.stubGlobal("matchMedia", (q: string) => ({
      matches: q === "(display-mode: standalone)",
      addEventListener: () => {},
      removeEventListener: () => {},
    }));

  test.each(["tablet", "phone"] as const)("%s: swipe left on the sidebar or the scrim closes it", (device) => {
    setup(device);
    fireEvent.click(mascot()!);
    swipe(sidebar(), [200, 300], [100, 310]);
    expect(isOpen()).toBe(false);
    fireEvent.click(mascot()!);
    swipe(screen.getByTestId("scrim"), [300, 300], [200, 300]);
    expect(isOpen()).toBe(false);
  });

  test.each([
    ["under 40px", [200, 300], [165, 300]],
    ["mostly vertical", [200, 300], [150, 500]],
    ["rightward", [200, 300], [300, 300]],
  ] as const)("a %s move does not close it", (_, from, to) => {
    setup("phone");
    fireEvent.click(mascot()!);
    swipe(sidebar(), [...from], [...to]);
    expect(isOpen()).toBe(true);
  });

  test("a tap on a row still navigates and closes through the click", () => {
    setup("phone");
    fireEvent.click(mascot()!);
    const link = document.createElement("a");
    sidebar().appendChild(link);
    swipe(link, [100, 100], [102, 101]);
    expect(isOpen()).toBe(true);
    fireEvent.click(link);
    expect(isOpen()).toBe(false);
  });

  test("a horizontal scroller that can still scroll that way swallows the gesture", () => {
    setup("phone");
    fireEvent.click(mascot()!);
    const strip = document.createElement("div");
    strip.style.overflowX = "auto";
    Object.defineProperties(strip, { scrollWidth: { value: 500 }, clientWidth: { value: 200 } });
    sidebar().appendChild(strip);
    swipe(strip, [200, 300], [100, 300]);
    expect(isOpen()).toBe(true);
    // Scrolled to its end, the same swipe closes the sidebar.
    strip.scrollLeft = 300;
    swipe(strip, [200, 300], [100, 300]);
    expect(isOpen()).toBe(false);
  });

  test("nothing swipes while docked", () => {
    setup("desktop");
    swipe(sidebar(), [200, 300], [100, 300]);
    expect(isOpen()).toBe(true);
  });

  test.each([
    ["without a standalone signal", false, false],
    ["as a home-screen app", true, true],
  ])("a rightward edge swipe %s", (_, isStandalone, opens) => {
    if (isStandalone) standalone();
    setup("phone");
    swipe(document.body, [5, 300], [100, 300]);
    expect(isOpen()).toBe(opens);
  });

  test("navigator.standalone counts as a standalone signal, and only from the edge", () => {
    vi.stubGlobal("navigator", { ...navigator, standalone: true });
    setup("phone");
    swipe(document.body, [100, 300], [200, 300]);
    expect(isOpen()).toBe(false);
    swipe(document.body, [5, 300], [100, 300]);
    expect(isOpen()).toBe(true);
  });
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
