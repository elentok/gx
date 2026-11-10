import { cleanup, screen } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { afterEach, expect, test } from "vitest";
import { ConnectionContext, type Connection, type ConnectionState } from "../connection";
import { appRoutes } from "../routes";
import { renderThemed } from "../ui/render";
import type { FetchArchived } from "./archive";

afterEach(cleanup);

const state = { snapshot: undefined, connected: true, loadedOnce: true, liveSnapshot: true, tick: 0, streams: 0 } as unknown as ConnectionState;
const conn = { getState: () => state, subscribe: () => () => {}, onEvent: () => () => {}, refresh: () => {} } as unknown as Connection;

function renderArchive(archived: FetchArchived) {
  sessionStorage.clear();
  const router = createMemoryRouter(appRoutes({ archived, epicList: async () => ({ epics: [], follow_ups: [] }) }), { initialEntries: ["/archive"] });
  renderThemed(
    <ConnectionContext.Provider value={conn}>
      <RouterProvider router={router} />
    </ConnectionContext.Provider>,
  );
}

test("the page shows a spinner before data, then the empty state", async () => {
  let resolve!: () => void;
  renderArchive(() => new Promise((r) => (resolve = () => r({ epics: [] }))));
  expect(await screen.findByRole("status", { name: "Loading archive" })).toBeTruthy();
  resolve();
  expect(await screen.findByText("Nothing is archived")).toBeTruthy();
  expect(screen.queryByRole("status", { name: "Loading archive" })).toBeNull();
});

test("the header has a back link to the Epics page", async () => {
  renderArchive(async () => ({ epics: [] }));
  const back = await screen.findByRole("link", { name: "← Epics" });
  expect(back.getAttribute("href")).toBe("#/epics");
  expect(screen.getByRole("heading", { level: 1, name: "Archive" })).toBeTruthy();
});
