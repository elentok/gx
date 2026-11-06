import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { afterEach, expect, test } from "vitest";
import type { Connection, ConnectionState } from "../connection";
import type { EpicList, EpicListRow, Workspace } from "../generated/server";
import { ACTIVE_KEY } from "./active";
import { useActiveWorkspace } from "./useActiveWorkspace";

afterEach(cleanup);

const list = { epics: [] as EpicListRow[], follow_ups: [] } as EpicList;

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
  return { conn, setWorkspaces };
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

function setup(workspaces: Workspace[]) {
  const fake = fakeConnection(workspaces);
  const router = createMemoryRouter([{ path: "*", element: <Probe conn={fake.conn} /> }], { initialEntries: ["/stats"] });
  render(<RouterProvider router={router} />);
  return fake;
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
