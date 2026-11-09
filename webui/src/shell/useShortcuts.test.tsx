import { act, cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { afterEach, expect, test, vi } from "vitest";
import { session } from "../agents/agentsTestKit";
import { ConnectionContext, type Connection, type ConnectionState } from "../connection";
import { CardKindDesign, CardKindImplementation, SectionImplementing, SectionOnHold, type EpicListRow } from "../generated/server";
import { renderThemed } from "../ui/render";
import { DeviceContext } from "../useWide";
import { ACTIVE_KEY } from "../workspaces/active";
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
  row("p:blog", { name: "Blog" }),
  row("p:shop", { name: "Shop", card_kind: CardKindDesign }),
  row("p:old", { name: "Old", section: SectionOnHold, status_words: "Held" }),
];

let pageRenders = 0;
function Page() {
  pageRenders++;
  return <div>page</div>;
}

function setup(sort?: string) {
  vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({ epics: rows, follow_ups: [] }), { status: 200 })));
  const state = {
    snapshot: {
      needs_you_total: 0,
      workspaces: [{ name: "Default", default: true, members: 0, sort }, { name: "Work", members: 0 }],
      sessions: [session("s2", "p", { name: "scratch" })],
      placements: [{ session: "s2", kind: "other" }],
    },
    connected: true,
    loadedOnce: true,
    tick: 0,
  } as unknown as ConnectionState;
  const conn = { getState: () => state, subscribe: () => () => {}, onEvent: () => () => {} } as unknown as Connection;
  const router = createMemoryRouter([{ element: <AppShell />, children: [{ path: "*", element: <Page /> }] }], { initialEntries: ["/"] });
  renderThemed(
    <DeviceContext.Provider value="desktop">
      <ConnectionContext.Provider value={conn}>
        <RouterProvider router={router} />
      </ConnectionContext.Provider>
    </DeviceContext.Provider>,
  );
  return router;
}

const body = () => within(screen.getByTestId("sidebar-body"));
const press = (init: KeyboardEventInit, target: Element | Window = window) => act(() => void fireEvent.keyDown(target, init));
// The listeners attach in effects that can land late under load, and a key pressed before then is lost, so the press repeats until `check` holds.
const pressUntil = (init: KeyboardEventInit, check: () => void) =>
  waitFor(() => {
    press(init);
    check();
  });

test("Alt+1 opens the first own row, reading the digit from the physical code", async () => {
  const router = setup();
  await body().findByRole("link", { name: /Blog/ });
  await pressUntil({ key: "¡", code: "Digit1", altKey: true }, () => expect(router.state.location.pathname).toBe("/e/p:blog"));
  await pressUntil({ key: "™", code: "Digit2", altKey: true }, () => expect(router.state.location.pathname).toBe("/d/p:shop"));
});

test("Alt+1 opens the first row of the sorted list", async () => {
  rows[1] = { ...rows[1], last_activity_at: "2026-01-01T00:00:00Z" };
  const router = setup("activity");
  await body().findByRole("link", { name: /Blog/ });
  await pressUntil({ key: "¡", code: "Digit1", altKey: true }, () => expect(router.state.location.pathname).toBe("/d/p:shop"));
  rows[1] = { ...rows[1], last_activity_at: undefined };
});

test("numbers show while Alt is held, only on own rows, and vanish on keyup and on blur", async () => {
  setup();
  await body().findByRole("link", { name: /Blog/ });
  expect(body().queryAllByTestId("row-number")).toHaveLength(0);
  await pressUntil({ key: "Alt", code: "AltLeft", altKey: true }, () =>
    expect(body().getAllByTestId("row-number").map((n) => n.textContent)).toEqual(["1", "2"]),
  );
  act(() => void fireEvent.keyUp(window, { key: "Alt", code: "AltLeft" }));
  expect(body().queryAllByTestId("row-number")).toHaveLength(0);
  press({ key: "Alt", code: "AltLeft", altKey: true });
  act(() => void fireEvent.blur(window));
  expect(body().queryAllByTestId("row-number")).toHaveLength(0);
});

test("Alt changes only the menu slot: status words stay and no page component re-renders", async () => {
  setup();
  await body().findByRole("link", { name: /Blog/ });
  const words = () => body().getAllByRole("link").map((l) => l.textContent);
  const before = words();
  const renders = pageRenders;
  await pressUntil({ key: "Alt", code: "AltLeft", altKey: true }, () => expect(body().getAllByTestId("row-number")).toHaveLength(2));
  expect(words()).toEqual(before);
  act(() => void fireEvent.keyUp(window, { key: "Alt", code: "AltLeft" }));
  expect(words()).toEqual(before);
  expect(pageRenders).toBe(renders);
});

test("Alt held does nothing while a text field has focus", async () => {
  setup();
  await body().findByRole("link", { name: /Blog/ });
  const input = document.createElement("input");
  document.body.appendChild(input);
  input.focus();
  press({ key: "Alt", code: "AltLeft", altKey: true, bubbles: true }, input);
  expect(body().queryAllByTestId("row-number")).toHaveLength(0);
  input.remove();
});

test("Alt+3 does nothing: held rows have no number", async () => {
  const router = setup();
  await body().findByRole("link", { name: /Blog/ });
  press({ key: "3", code: "Digit3", altKey: true });
  expect(router.state.location.pathname).toBe("/");
});

test("Alt+Shift+2 picks the second workspace", async () => {
  setup();
  await body().findByRole("link", { name: /Blog/ });
  await pressUntil({ key: "@", code: "Digit2", altKey: true, shiftKey: true }, () => expect(localStorage.getItem(ACTIVE_KEY)).toBe("Work"));
});

test("nothing fires while a text field has focus", async () => {
  const router = setup();
  await body().findByRole("link", { name: /Blog/ });
  const input = document.createElement("input");
  document.body.appendChild(input);
  input.focus();
  press({ key: "1", code: "Digit1", altKey: true, bubbles: true }, input);
  press({ key: "@", code: "Digit2", altKey: true, shiftKey: true, bubbles: true }, input);
  expect(router.state.location.pathname).toBe("/");
  expect(localStorage.getItem(ACTIVE_KEY)).not.toBe("Work");
  input.remove();
});
