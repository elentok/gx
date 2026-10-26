import { cleanup, fireEvent, screen, within } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { afterEach, expect, test } from "vitest";
import { ConnectionContext, type Connection, type ConnectionState } from "../connection";
import type { EpicDetail, EpicList, EpicListRow } from "../generated/server";
import { appRoutes } from "../routes";
import { renderThemed } from "../ui/render";

afterEach(cleanup);

const row = (name: string, over: Partial<EpicListRow> = {}): EpicListRow => ({
  address: `gx:${name}`,
  project: "gx",
  name,
  kind: "loop",
  state: "running",
  done: 0,
  total: 3,
  queued: false,
  queueable: false,
  blocked_by_epics: [],
  card_kind: "implementation",
  section: "running",
  addresses: [`gx:${name}`],
  ...over,
});

const detailOf = (address: string, kind: EpicDetail["kind"]): EpicDetail => ({
  address,
  kind,
  summary: "",
  done: 0,
  total: 0,
  agent_time: 0,
  cost_total: 0,
  stats: {},
  rows: [],
  queueable: false,
  queued: false,
});

function render(path: string, epics: EpicListRow[], kind: EpicDetail["kind"] = "loop") {
  const state = { snapshot: { sessions: [] }, connected: true, loadedOnce: true, tick: 0 } as unknown as ConnectionState;
  const conn = { getState: () => state, subscribe: () => () => {}, onEvent: () => () => {} } as unknown as Connection;
  const list: EpicList = { epics, follow_ups: [] };
  const router = createMemoryRouter(
    appRoutes({
      epicList: async () => list,
      epic: async (address) => ({ kind: "ok", detail: detailOf(address, kind) }),
    }),
    { initialEntries: [path] },
  );
  renderThemed(
    <ConnectionContext.Provider value={conn}>
      <RouterProvider router={router} />
    </ConnectionContext.Provider>,
  );
}

const joined = row("blog-impl", { addresses: ["gx:blog-impl", "gx:blog"], card_kind: "design" });

test("the epic page has a back link to /epics and a title that opens the jump menu", async () => {
  render("/e/gx:blog-impl", [joined, row("other")]);
  const title = await screen.findByRole("heading", { level: 1 });
  expect(screen.getByRole("link", { name: "← Epics" }).getAttribute("href")).toBe("/epics");
  fireEvent.click(within(title).getByRole("button", { name: /^blog-impl/ }));
  expect(await screen.findByRole("dialog")).toBeTruthy();
});

test("a Design's epic page links to its -impl page", async () => {
  render("/e/gx:blog", [joined], "map");
  const link = await screen.findByRole("link", { name: "Open implementation epic →" });
  expect(link.getAttribute("href")).toBe("#/e/gx:blog-impl");
});

test("an -impl epic page links back to the Design", async () => {
  render("/e/gx:blog-impl", [joined]);
  const link = await screen.findByRole("link", { name: "Open design →" });
  expect(link.getAttribute("href")).toBe("#/e/gx:blog");
});

test("an -impl epic with no Design shows no link", async () => {
  render("/e/gx:solo-impl", [row("solo-impl")]);
  await screen.findByRole("heading", { level: 1 });
  expect(screen.queryByRole("link", { name: /Open (design|implementation epic)/ })).toBeNull();
});
