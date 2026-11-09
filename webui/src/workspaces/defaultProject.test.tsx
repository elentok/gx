import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { session } from "../agents/agentsTestKit";
import { ConnectionContext, type Connection, type ConnectionState } from "../connection";
import { defaultProject, pageProject, useDefaultProject } from "./defaultProject";

const getLastProject = vi.hoisted(() => vi.fn(async () => ""));
vi.mock("../design/drafts", async (importOriginal) => ({ ...(await importOriginal<object>()), getLastProject }));

beforeEach(() => {
  localStorage.clear();
  getLastProject.mockImplementation(async () => "");
});
afterEach(cleanup);

const sessions = [session("abc", "chatproj")];

test("page project comes from each address kind and a chat's session", () => {
  expect(pageProject("/e/p:x", sessions)).toBe("p");
  expect(pageProject("/d/p:x", sessions)).toBe("p");
  expect(pageProject("/t/p:x/01", sessions)).toBe("p");
  expect(pageProject("/agents/abc", sessions)).toBe("chatproj");
  expect(pageProject("/agents/gone", sessions)).toBeUndefined();
});

test("Epics, Stats, Settings and the agents list have no page project", () => {
  for (const path of ["/", "/epics", "/stats", "/settings", "/agents"]) expect(pageProject(path, sessions)).toBeUndefined();
});

const pick = (over: Partial<Parameters<typeof defaultProject>[0]>) =>
  defaultProject({ pathname: "/epics", sessions, projects: ["a", "b", "c"], ...over });

test("the page project wins, then a one-project filter, then last used, then the first", () => {
  localStorage.setItem("gx.epics.project", JSON.stringify(["b"]));
  expect(pick({ pathname: "/e/c:x", lastProject: "a" })).toBe("c");
  expect(pick({ lastProject: "a" })).toBe("b");
  localStorage.clear();
  expect(pick({ lastProject: "c" })).toBe("c");
  expect(pick({})).toBe("a");
});

test("a filter of two or none, or with one project only, is skipped", () => {
  localStorage.setItem("gx.epics.project", JSON.stringify(["b", "c"]));
  expect(pick({ lastProject: "c" })).toBe("c");
  localStorage.setItem("gx.epics.project", JSON.stringify(["gone"]));
  expect(pick({ lastProject: "c" })).toBe("c");
  localStorage.setItem("gx.epics.project", JSON.stringify(["b"]));
  expect(pick({ projects: ["b"] })).toBe("b");
  expect(pick({ projects: ["a", "z"], lastProject: "z" })).toBe("z");
});

test("a candidate missing from the list is skipped at each step", () => {
  localStorage.setItem("gx.epics.project", JSON.stringify(["gone"]));
  expect(pick({ pathname: "/e/nope:x", lastProject: "nope" })).toBe("a");
  expect(pick({ pathname: "/agents/abc", lastProject: "b" })).toBe("b");
});

test("the hook resolves the route, the snapshot and the server's last project", async () => {
  getLastProject.mockImplementation(async () => "c");
  const state = { snapshot: { sessions }, connected: true, loadedOnce: true, tick: 0 } as unknown as ConnectionState;
  const conn = { getState: () => state, subscribe: () => () => {}, onEvent: () => () => {} } as unknown as Connection;
  function Probe() {
    return <span>{useDefaultProject(["a", "b", "c"])}</span>;
  }
  const router = createMemoryRouter([{ path: "*", element: <Probe /> }], { initialEntries: ["/stats"] });
  render(
    <ConnectionContext.Provider value={conn}>
      <RouterProvider router={router} />
    </ConnectionContext.Provider>,
  );
  await waitFor(() => expect(screen.getByText("c")).toBeTruthy());
});
