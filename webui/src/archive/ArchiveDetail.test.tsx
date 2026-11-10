import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { afterEach, expect, test, vi } from "vitest";
import { ConnectionContext, type Connection, type ConnectionState } from "../connection";
import type { EpicDetail } from "../generated/server";
import { appRoutes } from "../routes";
import { renderThemed } from "../ui/render";

const bulkUnarchive = vi.hoisted(() => vi.fn());
vi.mock("../select/bulkWrites", async (orig) => ({ ...(await orig<object>()), bulkUnarchive }));

afterEach(() => {
  cleanup();
  bulkUnarchive.mockReset();
});

const detail = (over: Partial<EpicDetail> = {}): EpicDetail => ({
  address: "gx:blog-impl",
  kind: "loop",
  summary: "",
  done: 1,
  total: 1,
  agent_time: 0,
  cost_total: 0,
  stats: {},
  rows: [{ address: "gx:blog-impl/01", title: "title 01", rendered_status: "done" }] as EpicDetail["rows"],
  queueable: true,
  queued: false,
  state: "finished",
  archived_at: "2026-03-04T10:00:00Z",
  ...over,
});

function render(path: string, d: EpicDetail | "not-found") {
  const state = { snapshot: { sessions: [], pending: [] }, connected: true, loadedOnce: true, tick: 0 } as unknown as ConnectionState;
  const conn = { getState: () => state, subscribe: () => () => {}, onEvent: () => () => {}, refresh: () => {} } as unknown as Connection;
  const router = createMemoryRouter(
    appRoutes({
      epicList: async () => ({ epics: [], follow_ups: [] }),
      // The active fetcher must stay unused on this route.
      epic: async () => {
        throw new Error("active detail fetched");
      },
      archivedEpic: async () => (d === "not-found" ? { kind: "not-found" } : { kind: "ok", detail: d }),
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

test("shows the Archived stamp and Unarchive as the only action", async () => {
  render("/archive/gx:blog-impl", detail());
  expect(await screen.findByTestId("archived-stamp")).toBeTruthy();
  const names = screen.getAllByRole("button").map((b) => b.textContent);
  expect(names).toContain("Unarchive");
  for (const gone of ["Queue", "Hold", "Merge", "Fix", "Open design"]) {
    expect(names.some((n) => n?.startsWith(gone))).toBe(false);
  }
});

test("ticket rows are plain text, not links", async () => {
  render("/archive/gx:blog-impl", detail());
  expect(await screen.findByText("title 01")).toBeTruthy();
  expect(screen.queryByRole("link", { name: /title 01/ })).toBeNull();
});

test("an archived Design's -impl button goes to the archive detail", async () => {
  const router = render("/archive/gx:blog", detail({ address: "gx:blog", kind: "map", impl_address: "gx:blog-impl", rows: [], total: 0 }));
  fireEvent.click(await screen.findByRole("button", { name: "Open implementation epic" }));
  await waitFor(() => expect(router.state.location.pathname).toBe("/archive/gx:blog-impl"));
});

test("Unarchive posts and navigates to the renamed active epic", async () => {
  bulkUnarchive.mockResolvedValue([{ target: "gx:blog-impl", status: "done", new_address: "gx:blog-impl-2" }]);
  const router = render("/archive/gx:blog-impl", detail());
  fireEvent.click(await screen.findByRole("button", { name: "Unarchive" }));
  await waitFor(() => expect(router.state.location.pathname).toBe("/e/gx:blog-impl-2"));
  expect(bulkUnarchive).toHaveBeenCalledWith(["gx:blog-impl"]);
});

test("an unknown address shows the not-found state", async () => {
  render("/archive/gx:nope", "not-found");
  expect(await screen.findByText("Epic not found")).toBeTruthy();
});
