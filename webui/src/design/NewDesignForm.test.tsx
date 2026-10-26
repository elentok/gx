import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { afterEach, expect, test, vi } from "vitest";
import { ConnectionContext, type Connection, type ConnectionState } from "../connection";
import type { EpicListRow, SessionInfo, SessionResult } from "../generated/server";
import { appRoutes, type RouteFetchers } from "../routes";
import { renderThemed } from "../ui/render";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

const row = (name: string, project = "gx", over: Partial<EpicListRow> = {}): EpicListRow => ({
  address: `${project}:${name}`,
  project,
  name,
  kind: "map",
  state: "designing",
  done: 0,
  total: 0,
  queued: false,
  queueable: false,
  blocked_by_epics: [],
  ...over,
});

const session = (over: Partial<SessionInfo> = {}): SessionInfo => ({
  id: "s1",
  name: "idea",
  project: "gx",
  dir: "/x",
  harness: "claude",
  ticket: "gx:idea",
  design: true,
  status: { state: "idle" },
  status_changed_at: "2026-10-23T10:00:00.123456789Z",
  run_started_at: "2026-10-23T10:00:00.123456789Z",
  unread: {},
  actionable: {},
  auto_advance: true,
  ...over,
});

const exited = (over: Partial<SessionInfo> = {}) => session({ status: { state: "exited" }, ...over });

function render(path: string, epics: EpicListRow[], sessions: SessionInfo[] = [], over: Partial<RouteFetchers> = {}) {
  const state = { snapshot: { sessions }, connected: true, loadedOnce: true, tick: 0 } as unknown as ConnectionState;
  const conn = { getState: () => state, subscribe: () => () => {}, onEvent: () => () => {}, refresh: () => {} } as unknown as Connection;
  const router = createMemoryRouter(
    appRoutes({
      epicList: async () => ({ epics, follow_ups: [] }),
      epic: async () => ({ kind: "missing" }) as never,
      ...over,
    }),
    { initialEntries: [path] },
  );
  renderThemed(
    <ConnectionContext.Provider value={conn}>
      <RouterProvider router={router} />
    </ConnectionContext.Provider>,
  );
  return router;
}

const started: SessionResult = { session: session() };

async function fill(slug: string, idea: string) {
  fireEvent.change(await screen.findByLabelText("Epic name"), { target: { value: slug } });
  fireEvent.change(screen.getByLabelText("Rough idea"), { target: { value: idea } });
}

// The form opens from "New session" on a Design that has no map yet and whose session has exited.
async function openForm() {
  fireEvent.click(await screen.findByRole("button", { name: "New session" }));
  await screen.findByLabelText("Epic name");
}

test("New session on an Exited session-only Design opens the form with its name", async () => {
  render("/d/gx:idea", [], [exited()]);
  fireEvent.click(await screen.findByRole("button", { name: "New session" }));
  expect(await screen.findByRole("form", { name: "New Design" })).toBeTruthy();
  expect((screen.getByLabelText("Epic name") as HTMLInputElement).value).toBe("idea");
});

test("submit sends the exact request and shows the new Design", async () => {
  const openDesign = vi.fn(async () => started);
  const router = render("/d/gx:old", [], [exited({ ticket: "gx:old" })], { openDesign });
  await openForm();
  await fill("idea", "make it fast");
  fireEvent.click(screen.getByRole("button", { name: "Start design" }));
  await waitFor(() => expect(router.state.location.pathname).toBe("/d/gx:idea"));
  expect(openDesign).toHaveBeenCalledWith({ epic: "idea", new_design: true, idea: "make it fast", project: "gx" });
});

test("the project select shows only with several projects and starts on the current Design's project", async () => {
  const openDesign = vi.fn(async () => started);
  render("/d/other:idea", [row("a")], [exited({ project: "other", ticket: "other:idea" })], { openDesign });
  await openForm();
  const select = (await screen.findByLabelText("Project")) as HTMLSelectElement;
  expect(select.value).toBe("other");
  await fill("idea", "x");
  fireEvent.click(screen.getByRole("button", { name: "Start design" }));
  await waitFor(() => expect(openDesign).toHaveBeenCalledWith(expect.objectContaining({ project: "other" })));
  cleanup();
  render("/d/gx:idea", [row("a")], [exited()]);
  await openForm();
  expect(screen.queryByLabelText("Project")).toBeNull();
});

test("a refusal shows the server's message and keeps the form", async () => {
  const openDesign = vi.fn(async () => ({ refused: true, reason: "epic-exists", message: "gx already has an epic idea" }) as SessionResult);
  const router = render("/d/gx:old", [], [exited({ ticket: "gx:old" })], { openDesign });
  await openForm();
  await fill("idea", "x");
  fireEvent.click(screen.getByRole("button", { name: "Start design" }));
  expect((await screen.findByRole("alert")).textContent).toBe("gx already has an epic idea");
  expect((screen.getByLabelText("Epic name") as HTMLInputElement).value).toBe("idea");
  expect(router.state.location.pathname).toBe("/d/gx:old");
});

test("the open form and its text survive a reload, and Cancel drops them", async () => {
  render("/d/gx:idea", [], [exited()]);
  await openForm();
  await fill("idea", "make it fast");
  cleanup();
  render("/d/gx:idea", [], [exited()]);
  expect(((await screen.findByLabelText("Epic name")) as HTMLInputElement).value).toBe("idea");
  expect((screen.getByLabelText("Rough idea") as HTMLTextAreaElement).value).toBe("make it fast");
  fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
  cleanup();
  render("/d/gx:idea", [], [exited()]);
  await screen.findByRole("button", { name: "New session" });
  expect(screen.queryByRole("form", { name: "New Design" })).toBeNull();
});
