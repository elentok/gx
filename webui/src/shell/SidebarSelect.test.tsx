import { act, cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
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
  row("p:blog", { name: "Blog" }),
  row("p:shop", { name: "Shop", card_kind: CardKindDesign }),
  row("p:old", { name: "Old", section: SectionOnHold }),
  row("p:older", { name: "Older", section: SectionOnHold }),
];
const placements: SessionPlacement[] = [
  { session: "s1", kind: "epic", epic: "p:blog" },
  { session: "s2", kind: "other" },
  { session: "s3", kind: "chat", workspace: "Default" },
  { session: "s6", kind: "epic", epic: "p:blog" },
];
const sessions = () => [
  session("s1", "p", { name: "builder" }),
  session("s2", "p", { name: "scratch", status: { state: "exited" } }),
  session("s3", "p", { name: "talk" }),
  session("s6", "p", { name: "queuer", queue: true, tier: "worker" }),
];

function setup() {
  vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({ epics: rows, follow_ups: [] }), { status: 200 })));
  const listeners = new Set<() => void>();
  let state = {
    snapshot: { needs_you_total: 0, workspaces: [{ name: "Default", default: true, members: 0 }], sessions: sessions(), placements },
    connected: true,
    loadedOnce: true,
    tick: 0,
  } as unknown as ConnectionState;
  const conn = {
    getState: () => state,
    subscribe: (cb: () => void) => {
      listeners.add(cb);
      return () => listeners.delete(cb);
    },
    onEvent: () => () => {},
  } as unknown as Connection;
  const router = createMemoryRouter([{ element: <AppShell />, children: [{ path: "*", element: <div /> }] }], { initialEntries: ["/"] });
  renderThemed(
    <DeviceContext.Provider value="desktop">
      <ConnectionContext.Provider value={conn}>
        <RouterProvider router={router} />
      </ConnectionContext.Provider>
    </DeviceContext.Provider>,
  );
  return {
    removeSession(id: string) {
      state = {
        ...state,
        snapshot: {
          ...state.snapshot,
          sessions: state.snapshot!.sessions!.filter((s) => s.id !== id),
          placements: state.snapshot!.placements!.filter((p) => p.session !== id),
        },
      } as ConnectionState;
      act(() => listeners.forEach((l) => l()));
    },
  };
}

const body = () => within(screen.getByTestId("sidebar-body"));
const box = (name: string) => body().getByRole("checkbox", { name }) as HTMLInputElement;
const enterSelect = async () => {
  await body().findByRole("link", { name: /Blog/ });
  fireEvent.click(screen.getByRole("button", { name: "Select" }));
};
const count = () => screen.getByTestId("select-count").textContent;

test("Select shows a checkbox on every row with a menu, nested agents and Other rows included, and none on a Queue agent", async () => {
  setup();
  await enterSelect();
  expect(box("Select Blog")).toBeTruthy();
  expect(box("Select Shop")).toBeTruthy();
  expect(box("Select talk")).toBeTruthy();
  fireEvent.click(body().getByRole("button", { name: "Expand Blog" }));
  expect(box("Select builder")).toBeTruthy();
  expect(body().queryByRole("checkbox", { name: "Select queuer" })).toBeNull();
  fireEvent.click(body().getByRole("button", { name: /Other/ }));
  expect(box("Select scratch")).toBeTruthy();
});

test("Space on a Queue agent shows a toast and selects nothing", async () => {
  setup();
  await enterSelect();
  fireEvent.click(body().getByRole("button", { name: "Expand Blog" }));
  fireEvent.keyDown(body().getByRole("link", { name: /queuer/ }), { key: " " });
  expect(toast).toHaveBeenCalledTimes(1);
  expect(toast.mock.calls[0][0].title).toMatch(/queue/i);
  expect(count()).toBe("0 selected");
});

test("the bar replaces the footer and Done restores it and clears the selection", async () => {
  setup();
  expect(document.querySelector("[data-part='sidebar-footer']")).toBeTruthy();
  await enterSelect();
  expect(document.querySelector("[data-part='sidebar-footer']")).toBeNull();
  fireEvent.click(box("Select Blog"));
  expect(count()).toBe("1 selected");
  fireEvent.click(screen.getByRole("button", { name: "Done" }));
  expect(document.querySelector("[data-part='sidebar-footer']")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Select" }));
  expect(count()).toBe("0 selected");
});

test("a selected row is tinted, and the selection survives navigation", async () => {
  setup();
  await enterSelect();
  fireEvent.click(box("Select Blog"));
  const frame = () => body().getByRole("link", { name: /Blog/ }).parentElement!;
  expect(frame().hasAttribute("data-selected")).toBe(true);
  fireEvent.click(body().getByRole("link", { name: /Shop/ }));
  await waitFor(() => expect(window.location.hash).toBe("#/d/p:shop"));
  expect(count()).toBe("1 selected");
  expect(frame().hasAttribute("data-selected")).toBe(true);
});

test("Select all skips a collapsed On hold section; Select none clears", async () => {
  setup();
  await enterSelect();
  fireEvent.click(screen.getByRole("button", { name: "Select all" }));
  expect(count()).toBe("3 selected");
  fireEvent.click(body().getByRole("button", { name: /On hold/ }));
  expect(box("Select Old").checked).toBe(false);
  fireEvent.click(screen.getByRole("button", { name: "Select all" }));
  expect(count()).toBe("5 selected");
  fireEvent.click(screen.getByRole("button", { name: "Select none" }));
  expect(count()).toBe("0 selected");
});

test("section checkboxes select a whole section and show a mixed state", async () => {
  setup();
  await enterSelect();
  fireEvent.click(body().getByRole("button", { name: /On hold/ }));
  const section = box("Select all in On hold (2)");
  fireEvent.click(box("Select Old"));
  expect(section.indeterminate).toBe(true);
  fireEvent.click(section);
  expect(box("Select Older").checked).toBe(true);
  expect(section.indeterminate).toBe(false);
  expect(section.checked).toBe(true);
  fireEvent.click(section);
  expect(count()).toBe("0 selected");
});

test("a collapsed section's checkbox selects its hidden rows and shows all", async () => {
  setup();
  await enterSelect();
  const section = box("Select all in On hold (2)");
  fireEvent.click(section);
  expect(count()).toBe("2 selected");
  expect(section.checked).toBe(true);
});

test("Escape with focus outside the sidebar leaves the sidebar mode alone", async () => {
  setup();
  await enterSelect();
  const outside = document.body.appendChild(document.createElement("button"));
  outside.focus();
  fireEvent.keyDown(outside, { key: "Escape" });
  expect(count()).toBe("0 selected");
  outside.remove();
});

test("Escape with focus on a sidebar control ends the mode", async () => {
  setup();
  await enterSelect();
  const control = screen.getByRole("button", { name: "Select all" });
  control.focus();
  fireEvent.keyDown(control, { key: "Escape" });
  expect(screen.queryByTestId("select-count")).toBeNull();
});

test("`s` with focus on a sidebar button that is not a row enters select mode", async () => {
  setup();
  await body().findByRole("link", { name: /Blog/ });
  const btn = document.querySelector<HTMLElement>("aside button:not([data-nav])")!;
  btn.focus();
  fireEvent.keyDown(btn, { key: "s" });
  expect(screen.getByTestId("select-count")).toBeTruthy();
});

test("Escape ends the mode", async () => {
  setup();
  await enterSelect();
  fireEvent.keyDown(document.body, { key: "Escape" });
  expect(screen.queryByTestId("select-count")).toBeNull();
  expect(screen.getByRole("button", { name: "Select" })).toBeTruthy();
});

test("a row that leaves the list leaves the selection", async () => {
  const ctl = setup();
  await enterSelect();
  fireEvent.click(body().getByRole("button", { name: "Expand Blog" }));
  fireEvent.click(box("Select builder"));
  fireEvent.click(box("Select talk"));
  expect(count()).toBe("2 selected");
  ctl.removeSession("s1");
  await waitFor(() => expect(count()).toBe("1 selected"));
});
