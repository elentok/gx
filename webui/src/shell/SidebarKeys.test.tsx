import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { useRef } from "react";
import { createMemoryRouter, MemoryRouter, RouterProvider } from "react-router";
import { afterEach, expect, test, vi } from "vitest";
import { session } from "../agents/agentsTestKit";
import { ConnectionContext, type Connection, type ConnectionState } from "../connection";
import { CardKindImplementation, SectionImplementing, SectionOnHold, SessionBlocked, type EpicListRow, type SessionPlacement } from "../generated/server";
import { renderThemed } from "../ui/render";
import { DeviceContext } from "../useWide";
import { AppShell } from "./AppShell";
import { closeNewItem, openNewAgent } from "./newItemOpen";
import type { Shell } from "./ShellContext";
import { useSidebarKeys } from "./SidebarKeys";

const toast = vi.hoisted(() => vi.fn());
vi.mock("../ui/toaster", async (orig) => ({ ...(await orig<typeof import("../ui/toaster")>()), showToast: toast }));

afterEach(() => {
  cleanup();
  toast.mockClear();
  closeNewItem();
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

async function setup(withChat = false, withQueue = false) {
  vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({ epics: rows, follow_ups: [] }), { status: 200 })));
  const state = {
    snapshot: {
      needs_you_total: 0,
      workspaces: [{ name: "Default", default: true, members: 0 }],
      sessions: [
        session("s1", "p", { name: "builder" }),
        session("s2", "p", { name: "scratch" }),
        ...(withChat ? [session("s3", "p", { name: "talk" })] : []),
        ...(withQueue ? [session("s6", "p", { name: "queuer", queue: true, tier: "worker" })] : []),
      ],
      placements: [
        ...placements,
        ...(withChat ? [{ session: "s3", kind: "chat", workspace: "Default" }] : []),
        ...(withQueue ? [{ session: "s6", kind: "epic", epic: "p:blog" }] : []),
      ],
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
  return router;
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

test("Enter on a chat navigates with the prompt focus request; on an epic row it only navigates", async () => {
  const router = await setup(true);
  focus(body().getByRole("link", { name: /Blog/ }));
  press("Enter");
  await waitFor(() => expect(window.location.hash).toBe("#/e/p:blog"));
  expect(router.state.location.state).toBeNull();
  focus(body().getByRole("link", { name: /talk/ }));
  press("Enter");
  await waitFor(() => expect(router.state.location.state).toEqual({ focusPrompt: true }));
  expect(router.state.location.pathname).toBe("/agents/s3");
});

test("a click on a chat row sends no focus request", async () => {
  const router = await setup(true);
  fireEvent.click(body().getByRole("link", { name: /talk/ }));
  expect(router.state.location.state).toBeNull();
});

// A sidebar whose sessions and epic rows can change after mount; `change` swaps them and notifies as the server would.
async function setupLive() {
  let epics = rows;
  let sessions = ["alpha", "beta", "gamma"].map((n, i) => session(`c${i}`, "p", { name: n }));
  vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({ epics, follow_ups: [] }), { status: 200 })));
  const subs = new Set<() => void>();
  const events = new Set<() => void>();
  const build = () =>
    ({
      snapshot: {
        needs_you_total: 0,
        workspaces: [{ name: "Default", default: true, members: 0 }],
        sessions,
        placements: sessions.map((s) => ({ session: s.id, kind: "chat", workspace: "Default" })),
      },
      connected: true,
      loadedOnce: true,
      tick: 0,
    }) as unknown as ConnectionState;
  let state = build();
  const conn = {
    getState: () => state,
    subscribe: (l: () => void) => (subs.add(l), () => subs.delete(l)),
    onEvent: (l: () => void) => (events.add(l), () => events.delete(l)),
  } as unknown as Connection;
  const router = createMemoryRouter([{ element: <AppShell />, children: [{ path: "*", element: <div>page</div> }] }], { initialEntries: ["/"] });
  renderThemed(
    <DeviceContext.Provider value="desktop">
      <ConnectionContext.Provider value={conn}>
        <RouterProvider router={router} />
      </ConnectionContext.Provider>
    </DeviceContext.Provider>,
  );
  await body().findByRole("link", { name: /alpha/ });
  return {
    change(next: { sessions?: typeof sessions; epics?: EpicListRow[] }) {
      sessions = next.sessions ?? sessions;
      epics = next.epics ?? epics;
      state = build();
      act(() => {
        subs.forEach((l) => l());
        events.forEach((l) => l());
      });
    },
    sessions: () => sessions,
  };
}

const blocked = (s: ReturnType<typeof session>) => ({ ...s, status: { state: SessionBlocked }, actionable: { reason: "ask" } }) as typeof s;
const order = () => body().getAllByRole("link").map((a) => a.textContent ?? "");
const indexOfName = (name: RegExp) => order().findIndex((t) => name.test(t));

test("a chat flipping to Needs you reorders the rows while focus stays on the same element", async () => {
  const live = await setupLive();
  const beta = body().getByRole("link", { name: /beta/ });
  focus(beta);
  expect(indexOfName(/gamma/)).toBeGreaterThan(indexOfName(/alpha/));
  live.change({ sessions: live.sessions().map((s) => (s.id === "c2" ? blocked(s) : s)) });
  await waitFor(() => expect(indexOfName(/gamma/)).toBeLessThan(indexOfName(/alpha/)));
  expect(document.activeElement).toBe(beta);
  expect(beta.tabIndex).toBe(0);
});

test("focus follows the key when an epic moves into On hold", async () => {
  const live = await setupLive();
  fireEvent.click(body().getByRole("button", { name: "On hold (1)" }));
  focus(body().getByRole("link", { name: /Shop/ }));
  live.change({ epics: rows.map((r) => (r.address === "p:shop" ? { ...r, section: SectionOnHold } : r)) });
  await body().findByRole("button", { name: "On hold (2)" });
  await waitFor(() => {
    const shop = body().getByRole("link", { name: /Shop/ });
    expect(document.activeElement).toBe(shop);
    expect(shop.closest("[data-nav-section]") ?? shop.closest("li")?.parentElement).toBe(body().getByRole("link", { name: /Old/ }).closest("li")?.parentElement);
  });
});

test("an open row menu stays on its row through a reorder and closes when the row leaves", async () => {
  const live = await setupLive();
  const gammaLi = () => body().getByRole("link", { name: /gamma/ }).closest("li")!;
  fireEvent.click(within(gammaLi()).getByRole("button", { name: "More actions" }));
  await screen.findAllByRole("menuitem");
  live.change({ sessions: live.sessions().map((s) => (s.id === "c2" ? blocked(s) : s)) });
  await waitFor(() => expect(indexOfName(/gamma/)).toBeLessThan(indexOfName(/alpha/)));
  expect(within(gammaLi()).getByRole("button", { name: "More actions" }).getAttribute("aria-expanded")).toBe("true");
  expect(screen.getAllByRole("menuitem").length).toBeGreaterThan(0);
  live.change({ sessions: live.sessions().filter((s) => s.id !== "c2") });
  await waitFor(() => expect(screen.queryAllByRole("menuitem")).toHaveLength(0));
});

const Harness = ({ keys }: { keys: string[] }) => (
  <MemoryRouter>
    <Rows keys={keys} />
  </MemoryRouter>
);

function Rows({ keys }: { keys: string[] }) {
  const aside = useRef<HTMLElement>(null);
  useSidebarKeys(aside, { sidebarOpen: true, toggle: () => {} } as unknown as Shell);
  return (
    <aside ref={aside}>
      <ul data-nav-list="" tabIndex={-1} data-testid="list">
        {keys.map((k) => (
          <li key={k}>
            <a href={`#/${k}`} data-nav={`row:${k}`}>
              {k}
            </a>
          </li>
        ))}
      </ul>
    </aside>
  );
}

const link = (name: string) => screen.getByRole("link", { name });

test("a reorder keeps focus on the same row, and the tab stop with it", async () => {
  const { rerender } = render(<Harness keys={["a", "b", "c"]} />);
  link("b").focus();
  rerender(<Harness keys={["c", "b", "a"]} />);
  await waitFor(() => expect(document.activeElement).toBe(link("b")));
  rerender(<Harness keys={["b", "a", "c"]} />);
  await waitFor(() => expect(document.activeElement).toBe(link("b")));
  expect(["a", "b", "c"].map((k) => link(k).tabIndex)).toEqual([-1, 0, -1]);
});

test("when the focused row leaves, focus lands on the row at its old index, clamped, then on the list", async () => {
  const { rerender } = render(<Harness keys={["a", "b", "c"]} />);
  link("b").focus();
  rerender(<Harness keys={["a", "c"]} />);
  await waitFor(() => expect(document.activeElement).toBe(link("c")));
  rerender(<Harness keys={["a"]} />);
  await waitFor(() => expect(document.activeElement).toBe(link("a")));
  rerender(<Harness keys={[]} />);
  await waitFor(() => expect(document.activeElement).toBe(screen.getByTestId("list")));
});

test("a reorder does not steal focus from outside the sidebar", async () => {
  const { rerender } = render(<Harness keys={["a", "b"]} />);
  link("a").focus();
  const input = document.createElement("input");
  document.body.appendChild(input);
  input.focus();
  await Promise.resolve();
  rerender(<Harness keys={["b", "a"]} />);
  await Promise.resolve();
  expect(document.activeElement).toBe(input);
  input.remove();
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

const box = (name: string) => body().getByRole("checkbox", { name }) as HTMLInputElement;

test("`s` enters select mode from a row and Escape leaves it", async () => {
  await setup();
  focus(body().getByRole("link", { name: /Blog/ }));
  press("s");
  expect(box("Select Blog").checked).toBe(false);
  press("Escape");
  expect(body().queryByRole("checkbox", { name: "Select Blog" })).toBeNull();
});

test("Space does nothing outside select mode and toggles the focused row in it", async () => {
  await setup();
  focus(body().getByRole("link", { name: /Blog/ }));
  press(" ");
  expect(body().queryByRole("checkbox", { name: "Select Blog" })).toBeNull();
  press("s");
  press(" ");
  expect(box("Select Blog").checked).toBe(true);
  press(" ");
  expect(box("Select Blog").checked).toBe(false);
});

test("Enter opens the row outside select mode and toggles it in select mode", async () => {
  await setup();
  focus(body().getByRole("link", { name: /Blog/ }));
  press("s");
  press("Enter");
  expect(box("Select Blog").checked).toBe(true);
  expect(window.location.hash).toBe("");
  press("Escape");
  press("Enter");
  await waitFor(() => expect(window.location.hash).toBe("#/e/p:blog"));
});

test("Space on a Queue agent shows the reason toast and selects nothing", async () => {
  await setup(false, true);
  focus(body().getByRole("link", { name: /Blog/ }));
  press("l");
  focus(body().getByRole("link", { name: /queuer/ }));
  press("s");
  press(" ");
  expect(toast).toHaveBeenCalledTimes(1);
  expect(toast.mock.calls[0][0].title).toMatch(/queue/i);
  expect(screen.getByTestId("select-count").textContent).toBe("0 selected");
});

test("typing `shsl` in the New chat name field starts no mode", async () => {
  await setup();
  // The form's worktree list comes from the same fetch the sidebar used for its epics.
  vi.stubGlobal("fetch", vi.fn(async () => new Response("[]", { status: 200 })));
  act(openNewAgent);
  const input = await screen.findByRole("textbox", { name: "Name" });
  input.focus();
  for (const key of "shsl") fireEvent.keyDown(input, { key });
  expect(document.activeElement).toBe(input);
  expect(body().queryByRole("checkbox")).toBeNull();
});

test("select keys are ignored while a menu is open", async () => {
  await setup();
  fireEvent.click(within(body().getByRole("link", { name: /Blog/ }).closest("li")!).getByRole("button", { name: "More actions" }));
  const item = (await screen.findAllByRole("menuitem"))[0];
  item.focus();
  fireEvent.keyDown(item, { key: "s" });
  expect(body().queryByRole("checkbox")).toBeNull();
});
