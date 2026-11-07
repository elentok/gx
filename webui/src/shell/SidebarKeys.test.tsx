import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { afterEach, expect, test, vi } from "vitest";
import { session } from "../agents/agentsTestKit";
import { ConnectionContext, type Connection, type ConnectionState } from "../connection";
import { CardKindImplementation, SectionImplementing, SectionOnHold, type EpicListRow, type SessionPlacement } from "../generated/server";
import { renderThemed } from "../ui/render";
import { DeviceContext } from "../useWide";
import { AppShell } from "./AppShell";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  localStorage.clear();
  window.location.hash = "";
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

const rows = [row("p:blog", { name: "Blog" }), row("p:shop", { name: "Shop" }), row("p:old", { name: "Old", section: SectionOnHold })];
const placements: SessionPlacement[] = [
  { session: "s1", kind: "epic", epic: "p:blog" },
  { session: "s2", kind: "other" },
];

async function setup() {
  vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({ epics: rows, follow_ups: [] }), { status: 200 })));
  const state = {
    snapshot: {
      needs_you_total: 0,
      workspaces: [{ name: "Default", default: true, members: 0 }],
      sessions: [session("s1", "p", { name: "builder" }), session("s2", "p", { name: "scratch" })],
      placements,
    },
    connected: true,
    loadedOnce: true,
    tick: 0,
  } as unknown as ConnectionState;
  const conn = { getState: () => state, subscribe: () => () => {}, onEvent: () => () => {} } as unknown as Connection;
  const router = createMemoryRouter([{ element: <AppShell />, children: [{ path: "*", element: <div>page</div> }] }], { initialEntries: ["/"] });
  renderThemed(
    <DeviceContext.Provider value="desktop">
      <ConnectionContext.Provider value={conn}>
        <RouterProvider router={router} />
      </ConnectionContext.Provider>
    </DeviceContext.Provider>,
  );
  await within(screen.getByTestId("sidebar-body")).findByRole("link", { name: /Blog/ });
}

const body = () => within(screen.getByTestId("sidebar-body"));
const aside = () => document.querySelector('aside[aria-label="Sidebar"]') as HTMLElement;
const press = (key: string) => fireEvent.keyDown(document.activeElement ?? document.body, { key });
const focus = (el: HTMLElement) => el.focus();

test("Cmd/Ctrl+B toggles the sidebar and focuses a row when it opens", async () => {
  await setup();
  fireEvent.keyDown(document.body, { key: "b", ctrlKey: true });
  await waitFor(() => expect(aside().getAttribute("data-open")).toBe("false"));
  fireEvent.keyDown(document.body, { key: "b", ctrlKey: true });
  await waitFor(() => expect(aside().getAttribute("data-open")).toBe("true"));
  expect(document.activeElement).toBe(body().getByRole("link", { name: /Blog/ }));
});

test("Down/Up and j/k walk the rows, through On hold and Other once expanded", async () => {
  await setup();
  fireEvent.click(body().getByRole("button", { name: "On hold (1)" }));
  fireEvent.click(body().getByRole("button", { name: "Other" }));
  focus(body().getByRole("link", { name: /Blog/ }));
  press("ArrowDown");
  expect(document.activeElement).toBe(body().getByRole("link", { name: /Shop/ }));
  press("j");
  expect(document.activeElement).toBe(body().getByRole("button", { name: "On hold (1)" }));
  press("j");
  expect(document.activeElement).toBe(body().getByRole("link", { name: /Old/ }));
  press("j");
  expect(document.activeElement).toBe(body().getByRole("button", { name: "Other" }));
  press("j");
  expect(document.activeElement).toBe(body().getByRole("link", { name: /scratch/ }));
  press("k");
  press("ArrowUp");
  expect(document.activeElement).toBe(body().getByRole("link", { name: /Old/ }));
});

test("only the focused row is a tab stop and the chevron is none", async () => {
  await setup();
  const blog = body().getByRole("link", { name: /Blog/ });
  const shop = body().getByRole("link", { name: /Shop/ });
  expect([blog.tabIndex, shop.tabIndex]).toEqual([0, -1]);
  expect(body().getByRole("button", { name: "Expand Blog" }).tabIndex).toBe(-1);
  focus(shop);
  await waitFor(() => expect([blog.tabIndex, shop.tabIndex]).toEqual([-1, 0]));
});

test("l/Right expand and h/Left collapse an epic's agents and a section", async () => {
  await setup();
  focus(body().getByRole("link", { name: /Blog/ }));
  press("l");
  expect(body().getByRole("link", { name: /builder/ })).toBeTruthy();
  press("h");
  expect(body().queryByRole("link", { name: /builder/ })).toBeNull();
  press("ArrowRight");
  expect(body().getByRole("link", { name: /builder/ })).toBeTruthy();
  press("ArrowLeft");
  expect(body().queryByRole("link", { name: /builder/ })).toBeNull();
  focus(body().getByRole("button", { name: "On hold (1)" }));
  press("l");
  expect(body().getByRole("link", { name: /Old/ })).toBeTruthy();
  press("h");
  expect(body().queryByRole("link", { name: /Old/ })).toBeNull();
});

test("Enter opens the focused row", async () => {
  await setup();
  focus(body().getByRole("link", { name: /Shop/ }));
  press("Enter");
  await waitFor(() => expect(window.location.hash).toBe("#/e/p:shop"));
});

test("vim keys do nothing outside the sidebar or in a text field", async () => {
  await setup();
  const blog = body().getByRole("link", { name: /Blog/ });
  focus(blog);
  fireEvent.keyDown(document.body, { key: "j" });
  expect(document.activeElement).toBe(blog);
  const input = document.createElement("input");
  aside().appendChild(input);
  input.focus();
  fireEvent.keyDown(input, { key: "j" });
  expect(document.activeElement).toBe(input);
});
