import { cleanup, fireEvent, screen, within } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { afterEach, expect, test, vi } from "vitest";
import canned from "../canned-detail.json";
import { ConnectionContext, type Connection, type ConnectionState } from "../connection";
import type { FetchScreen } from "../design/screen";
import type { TicketDetail } from "../generated/server";
import { appRoutes } from "../routes";
import { renderThemed } from "../ui/render";
import type { FetchDetail } from "./detail";

afterEach(cleanup);

const state = { connected: true, loadedOnce: true, tick: 0, snapshot: { tickets: [], epics: [] } } as unknown as ConnectionState;
const conn = { getState: () => state, subscribe: () => () => {}, onEvent: () => () => {} } as unknown as Connection;

const base: TicketDetail = { ...(canned as TicketDetail), runs: [], related: [], epic: "gx:demo" };

async function renderTicket(detail: TicketDetail, ticketScreen?: FetchScreen) {
  const fetchDetail: FetchDetail = async () => ({ kind: "ok", detail });
  const router = createMemoryRouter(appRoutes({ ticket: fetchDetail, ticketScreen }), { initialEntries: ["/t/gx:demo/12"] });
  renderThemed(
    <ConnectionContext.Provider value={conn}>
      <RouterProvider router={router} />
    </ConnectionContext.Provider>,
  );
  await screen.findByRole("heading", { level: 1 });
}

test("a ticket with a pty agent has a Screen button that opens the screen by ticket address", async () => {
  const fetchRows = vi.fn<FetchScreen>(async () => ({ kind: "ok", detail: { rows: ["agent row"] } }));
  await renderTicket({ ...base, has_screen: true }, fetchRows);

  fireEvent.click(screen.getByRole("button", { name: "Screen" }));
  const dialog = await screen.findByRole("dialog", { name: "Screen" });
  await within(dialog).findByText(/agent row/);
  expect(fetchRows).toHaveBeenCalledWith("gx:demo/12");
});

test("a ticket without a pty agent has no Screen button", async () => {
  await renderTicket({ ...base, has_screen: undefined });

  expect(screen.queryByRole("button", { name: "Screen" })).toBeNull();
});
