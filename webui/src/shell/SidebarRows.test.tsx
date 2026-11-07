import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { afterEach, expect, test, vi } from "vitest";
import { session } from "../agents/agentsTestKit";
import { ConnectionContext, type Connection, type ConnectionState } from "../connection";
import {
  CardKindDesign,
  CardKindImplementation,
  SectionImplementing,
  SectionOnHold,
  type EpicListRow,
  type SessionPlacement,
} from "../generated/server";
import { renderThemed } from "../ui/render";
import { DeviceContext, type Device } from "../useWide";
import { AppShell } from "./AppShell";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  localStorage.clear();
});

const row = (address: string, over: Partial<EpicListRow>): EpicListRow =>
  ({
    address,
    project: "p",
    name: address,
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
    ...over,
  }) as EpicListRow;

const rows = [
  row("p:blog", { name: "Blog", status_words: "Waiting on review" }),
  row("p:shop", { name: "Shop", card_kind: CardKindDesign }),
  row("p:old", { name: "Old", section: SectionOnHold, status_words: "Held" }),
];
const placements: SessionPlacement[] = [
  { session: "s1", kind: "epic", epic: "p:blog" },
  { session: "s2", kind: "other" },
  { session: "s3", kind: "chat", workspace: "Default" },
  { session: "s4", kind: "epic", epic: "p:shop" },
];

function setup(device: Device, path = "/") {
  vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({ epics: rows, follow_ups: [] }), { status: 200 })));
  const state = {
    snapshot: {
      needs_you_total: 0,
      workspaces: [{ name: "Default", default: true, members: 0 }, { name: "Work", members: 0 }],
      sessions: [
        session("s1", "p", { name: "builder" }),
        session("s2", "p", { name: "scratch" }),
        session("s3", "p", { name: "talk" }),
        session("s4", "p", { name: "designer" }),
      ],
      placements,
    },
    connected: true,
    loadedOnce: true,
    tick: 0,
  } as unknown as ConnectionState;
  const conn = { getState: () => state, subscribe: () => () => {}, onEvent: () => () => {} } as unknown as Connection;
  const router = createMemoryRouter([{ element: <AppShell />, children: [{ path: "*", element: <div>page</div> }] }], { initialEntries: [path] });
  renderThemed(
    <DeviceContext.Provider value={device}>
      <ConnectionContext.Provider value={conn}>
        <RouterProvider router={router} />
      </ConnectionContext.Provider>
    </DeviceContext.Provider>,
  );
}

const body = () => within(screen.getByTestId("sidebar-body"));

test("rows show the name and status words; the open epic is current and stays collapsed", async () => {
  setup("desktop", "/e/p:blog");
  const link = await body().findByRole("link", { name: /Blog/ });
  expect(link.textContent).toContain("Waiting on review");
  expect(link.getAttribute("href")).toBe("#/e/p:blog");
  expect(link.getAttribute("aria-current")).toBe("page");
  expect(body().queryByRole("link", { name: /builder/ })).toBeNull();
});

test("clicking an epic row navigates and leaves it collapsed; the chevron toggles both ways", async () => {
  setup("desktop", "/");
  const link = await body().findByRole("link", { name: /Blog/ });
  fireEvent.click(link);
  await waitFor(() => expect(window.location.hash).toBe("#/e/p:blog"));
  expect(body().queryByRole("link", { name: /builder/ })).toBeNull();
  fireEvent.click(body().getByRole("button", { name: "Expand Blog" }));
  expect(body().getByRole("link", { name: /builder/ })).toBeTruthy();
  fireEvent.click(body().getByRole("button", { name: "Collapse Blog" }));
  expect(body().queryByRole("link", { name: /builder/ })).toBeNull();
});

test("opening another page leaves an expanded row expanded", async () => {
  setup("desktop", "/");
  await body().findByRole("link", { name: /Blog/ });
  fireEvent.click(body().getByRole("button", { name: "Expand Blog" }));
  fireEvent.click(body().getByRole("link", { name: /Shop/ }));
  await waitFor(() => expect(window.location.hash).toBe("#/d/p:shop"));
  expect(body().getByRole("link", { name: /builder/ })).toBeTruthy();
});

test("a Design is one row: no chevron and no nested session, even when open", async () => {
  setup("desktop", "/d/p:shop");
  await body().findByRole("link", { name: /Shop/ });
  expect(body().queryByRole("button", { name: /(Expand|Collapse) Shop/ })).toBeNull();
  expect(body().queryByRole("link", { name: /designer/ })).toBeNull();
});

test("On hold is collapsed with a count and opens on click; Other lists its rows", async () => {
  setup("desktop");
  await body().findByRole("link", { name: /Blog/ });
  expect(body().queryByRole("link", { name: /Old/ })).toBeNull();
  fireEvent.click(body().getByRole("button", { name: "On hold (1)" }));
  expect(body().getByRole("link", { name: /Old/ })).toBeTruthy();
  expect(body().queryByRole("link", { name: /scratch/ })).toBeNull();
  fireEvent.click(body().getByRole("button", { name: "Other" }));
  expect(body().getByRole("link", { name: /scratch/ })).toBeTruthy();
});

test("a chat row reads Chat before its name", async () => {
  setup("desktop");
  await body().findByRole("link", { name: /Blog/ });
  expect(body().getByRole("link", { name: /^Chat talk/ })).toBeTruthy();
});

test("a row's ... menu opens and Move posts the other workspace", async () => {
  const fetchMock = vi.fn(async (url: string, _init?: RequestInit) =>
    new Response(JSON.stringify(String(url).includes("/v1/workspaces/") ? { workspaces: [] } : { epics: rows, follow_ups: [] }), { status: 200 }),
  );
  setup("desktop");
  vi.stubGlobal("fetch", fetchMock);
  const shop = await body().findByRole("link", { name: /Shop/ });
  fireEvent.click(within(shop.closest("li")!).getByRole("button", { name: "More actions" }));
  fireEvent.click(await screen.findByRole("menuitem", { name: "Move to workspace" }));
  fireEvent.click(await screen.findByRole("button", { name: "Work" }));
  await waitFor(() => {
    const call = fetchMock.mock.calls.find(([u]) => String(u).includes("/v1/workspaces/move"));
    expect(JSON.parse((call![1] as RequestInit).body as string)).toEqual({ name: "Work", key: "p:shop" });
  });
});

test("on a tablet a row click closes the sidebar", async () => {
  setup("tablet");
  fireEvent.click(screen.getByRole("button", { name: "Show sidebar" }));
  const link = await body().findByRole("link", { name: /Shop/ });
  fireEvent.click(link);
  await waitFor(() => expect(document.querySelector('aside[aria-label="Sidebar"]')!.getAttribute("data-open")).toBe("false"));
});
