import { cleanup, screen, within } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { afterEach, expect, test } from "vitest";
import canned from "../canned-detail.json";
import { ConnectionContext, type Connection, type ConnectionState } from "../connection";
import type { TicketDetail } from "../generated/server";
import { appRoutes } from "../routes";
import { renderThemed } from "../ui/render";
import type { FetchDetail } from "./detail";

afterEach(cleanup);

const path = "/tickets/gx/demo/issues/12-broken.md";

const broken: TicketDetail = {
  ...(canned as TicketDetail),
  title: "Broken",
  epic: "gx:demo",
  map_epic: false,
  status: "error",
  type: undefined,
  body: undefined,
  stats: undefined,
  related: [],
  runs: [],
  pause_text: undefined,
  rounds: [],
  latest: undefined,
  chat: [],
  state: "",
  actions: [],
  permission: undefined,
  answered: false,
  error_reason: "blocked_by names a ticket that does not exist",
  path,
};

const state = { connected: true, loadedOnce: true, tick: 0, snapshot: { tickets: [], epics: [] } } as unknown as ConnectionState;
const conn = { getState: () => state, subscribe: () => () => {}, onEvent: () => () => {} } as unknown as Connection;

async function renderTicket(detail: TicketDetail) {
  const fetchDetail: FetchDetail = async () => ({ kind: "ok", detail });
  const router = createMemoryRouter(appRoutes(fetchDetail), { initialEntries: ["/t/gx:demo/12"] });
  renderThemed(
    <ConnectionContext.Provider value={conn}>
      <RouterProvider router={router} />
    </ConnectionContext.Provider>,
  );
  return screen.findByRole("group", { name: "Action card" });
}

test("a file that fails to parse shows the reduced page and an action card with the parser error", async () => {
  // Shaped like server/detail.go:brokenDetail: the server owns the phrase, the parser text rides separately.
  const card = await renderTicket({ ...broken, error_reason: "ticket file format is invalid", parse_error: "yaml: line 3: bad indent" });

  expect(screen.getByRole("heading", { level: 1 }).textContent).toBe("Broken");
  expect(screen.getByText("error")).toBeTruthy();
  expect(screen.getByRole("link", { name: "demo" })).toBeTruthy();
  expect(within(card).getByText(/format is invalid/)).toBeTruthy();
  expect(within(card).getByText(/yaml: line 3: bad indent/)).toBeTruthy();
  expect(within(card).getByText(path)).toBeTruthy();
  expect(within(card).queryAllByRole("button")).toHaveLength(0);
  expect(screen.queryByRole("heading", { level: 2 })).toBeNull();
  expect(screen.queryByText("Nothing here yet")).toBeNull();
});

test("a graph error shows the full page plus the action card with the reason and path", async () => {
  const card = await renderTicket({ ...broken, body: "Some description.", type: "implement" });

  expect(within(card).getByText(/blocked_by names a ticket/)).toBeTruthy();
  expect(within(card).getByText(path)).toBeTruthy();
  expect(within(card).queryAllByRole("button")).toHaveLength(0);
  expect(screen.getByText("Some description.")).toBeTruthy();
  expect(screen.getByText("implement")).toBeTruthy();
});
