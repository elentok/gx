import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { afterEach, expect, test } from "vitest";
import type { Connection, ConnectionState } from "../connection";
import type { EpicList, EpicListRow, SessionPlacement, Workspace } from "../generated/server";
import { ACTIVE_KEY } from "./active";
import { useActiveWorkspace } from "./useActiveWorkspace";

// A stored "Main" left by an earlier test would satisfy the "Main" waits before the workspace list loads, so a click can land mid-load.
afterEach(() => {
  cleanup();
  localStorage.clear();
});

const list = { epics: [] as EpicListRow[], follow_ups: [] } as EpicList;
const epicRow = (name: string, workspace: string) =>
  ({ address: `demo:${name}`, addresses: [`demo:${name}`], section: "implementing", workspace }) as EpicListRow;

function fakeConnection(workspaces: Workspace[]) {
  const listeners = new Set<() => void>();
  let state = { snapshot: { sessions: [], workspaces }, connected: true, loadedOnce: true, tick: 0 } as unknown as ConnectionState;
  const conn = {
    getState: () => state,
    subscribe: (fn: () => void) => {
      listeners.add(fn);
      return () => listeners.delete(fn);
    },
    onEvent: () => () => {},
  } as unknown as Connection;
  const setWorkspaces = (next: Workspace[]) => {
    state = { ...state, snapshot: { ...state.snapshot, workspaces: next } } as ConnectionState;
    act(() => listeners.forEach((fn) => fn()));
  };
  const setPlacements = (placements: SessionPlacement[]) => {
    state = { ...state, snapshot: { ...state.snapshot, placements } } as ConnectionState;
    act(() => listeners.forEach((fn) => fn()));
  };
  return { conn, setWorkspaces, setPlacements };
}

function Probe({ conn }: { conn: Connection }) {
  const { active, setActive } = useActiveWorkspace(conn, async () => list);
  return (
    <>
      <div data-testid="active">{active}</div>
      <button onClick={() => setActive("Side")}>pick</button>
    </>
  );
}

const MAIN: Workspace = { name: "Main", default: true, members: 0 };
const SIDE: Workspace = { name: "Side", members: 0 };

function setup(workspaces: Workspace[], route = "/stats") {
  const fake = fakeConnection(workspaces);
  const router = createMemoryRouter([{ path: "*", element: <Probe conn={fake.conn} /> }], { initialEntries: [route] });
  render(<RouterProvider router={router} />);
  return { ...fake, router };
}

test("a workspaces-changed refresh that removes the active workspace drops to Default and stores it", async () => {
  localStorage.setItem(ACTIVE_KEY, "Side");
  const { setWorkspaces } = setup([MAIN, SIDE]);
  await waitFor(() => expect(screen.getByTestId("active").textContent).toBe("Side"));

  setWorkspaces([MAIN]);

  await waitFor(() => expect(screen.getByTestId("active").textContent).toBe("Main"));
  expect(localStorage.getItem(ACTIVE_KEY)).toBe("Main");
});

test("picking a workspace stores it", async () => {
  setup([MAIN, SIDE]);
  await waitFor(() => expect(screen.getByTestId("active").textContent).toBe("Main"));

  act(() => screen.getByText("pick").click());

  await waitFor(() => expect(screen.getByTestId("active").textContent).toBe("Side"));
  expect(localStorage.getItem(ACTIVE_KEY)).toBe("Side");
});

test("picking a workspace on an item page sticks until another item is opened", async () => {
  list.epics = [epicRow("blog", "Main"), epicRow("ops", "Main"), epicRow("side", "Side")];
  const { router } = setup([MAIN, SIDE], "/e/demo:blog");
  await waitFor(() => expect(screen.getByTestId("active").textContent).toBe("Main"));

  act(() => screen.getByText("pick").click());
  await waitFor(() => expect(screen.getByTestId("active").textContent).toBe("Side"));
  expect(localStorage.getItem(ACTIVE_KEY)).toBe("Side");

  await act(() => router.navigate("/e/demo:ops"));
  await waitFor(() => expect(screen.getByTestId("active").textContent).toBe("Main"));
  expect(localStorage.getItem(ACTIVE_KEY)).toBe("Main");
  list.epics = [];
});

test("an agent opened before its placement is known stays pending and applies the epic's workspace once it arrives", async () => {
  list.epics = [epicRow("blog", "Side")];
  localStorage.setItem(ACTIVE_KEY, "Main");
  const { setPlacements } = setup([MAIN, SIDE], "/agents/fix1");
  await waitFor(() => expect(screen.getByTestId("active").textContent).toBe("Main"));

  setPlacements([{ session: "fix1", kind: "epic", epic: "demo:blog" }]);

  await waitFor(() => expect(screen.getByTestId("active").textContent).toBe("Side"));
  expect(localStorage.getItem(ACTIVE_KEY)).toBe("Side");
  list.epics = [];
});

test("an agent placed in Other leaves the workspace unchanged", async () => {
  localStorage.setItem(ACTIVE_KEY, "Side");
  const { setPlacements } = setup([MAIN, SIDE], "/agents/fix2");
  await waitFor(() => expect(screen.getByTestId("active").textContent).toBe("Side"));

  setPlacements([{ session: "fix2", kind: "other" }]);

  await waitFor(() => expect(screen.getByTestId("active").textContent).toBe("Side"));
  expect(localStorage.getItem(ACTIVE_KEY)).toBe("Side");
});
