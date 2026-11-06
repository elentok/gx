import { cleanup, fireEvent, screen, within } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { afterEach, expect, test, vi } from "vitest";
import canned from "../canned-detail.json";
import { ConnectionContext, type Connection, type ConnectionState } from "../connection";
import type { FetchScreen } from "../design/screen";
import type { PendingRow, TicketDetail } from "../generated/server";
import { appRoutes } from "../routes";
import { renderThemed } from "../ui/render";
import type { FetchDetail } from "./detail";

afterEach(cleanup);

const state = { connected: true, loadedOnce: true, tick: 0, snapshot: { tickets: [], epics: [] } } as unknown as ConnectionState;
const conn = { getState: () => state, subscribe: () => () => {}, onEvent: () => () => {} } as unknown as Connection;

const base: TicketDetail = { ...(canned as TicketDetail), runs: [], related: [], epic: "gx:demo" };

async function renderTicket(detail: TicketDetail, ticketScreen?: FetchScreen, pending: PendingRow[] = []) {
  state.snapshot = { tickets: [], epics: [], pending } as unknown as ConnectionState["snapshot"];
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

test("the ticket header shows the check-lock waiting label as a status label, and nothing without it", async () => {
  const claimed = { ...base, status: "claimed" };
  await renderTicket({ ...claimed, waiting_label: "waiting for the check lock, held by run-1" });
  expect(document.querySelector("[data-waiting-label]")?.textContent).toBe("waiting for the check lock, held by run-1");
  expect(screen.queryByText(/Paused/)).toBeNull();
  cleanup();

  await renderTicket(claimed);
  expect(document.querySelector("[data-waiting-label]")).toBeNull();
});

// Replaces the ticket-page half of e2e flow 148 (Long-running badge).
test("the ticket header shows the Long-running badge only for a marked ticket", async () => {
  await renderTicket({ ...base, long_ticket_turns: 120, long_ticket_threshold: 100 });
  expect(screen.getByText("Long-running").getAttribute("title")).toBe("120 assistant turns (threshold 100)");
  cleanup();

  await renderTicket(base);
  expect(screen.queryByText("Long-running")).toBeNull();
});

test("the ticket page shows the launch step in the verdict slot, and the cap wait as Waiting for a slot", async () => {
  const claimed = { ...base, status: "claimed", stats: { ...base.stats, launch_step: { label: "installing dependencies", position: 2 } } };
  await renderTicket(claimed, undefined, [{ address: "gx:demo/12", agent: "", verdict: "concurrency cap", reason: "epic cap 3/2" }]);
  expect(document.querySelectorAll("[data-verdict]")).toHaveLength(1);
  expect(screen.getByText("Launching: installing dependencies (2/4)")).toBeTruthy();
  cleanup();

  await renderTicket(base, undefined, [{ address: "gx:demo/12", agent: "", verdict: "concurrency cap", reason: "epic cap 3/2" }]);
  expect(screen.getByText("Waiting for a slot: epic cap 3/2")).toBeTruthy();
});

const minutesAgo = (m: number) => new Date(Date.now() - m * 60_000).toISOString();

test("a launching ticket shows the claimed_at timer beside the Launching label, and the timer stays once the run is working", async () => {
  const stats = { ...base.stats, elapsed: 0, claimed_at: minutesAgo(5) };
  await renderTicket({ ...base, status: "claimed", stats: { ...stats, launch_step: { label: "starting agent", position: 3 } } });
  expect(screen.getByText("Launching: starting agent (3/4)")).toBeTruthy();
  expect(screen.getByText("5m")).toBeTruthy();
  cleanup();

  // Working: the step is gone, the label with it, and the same claimed_at clock keeps counting.
  await renderTicket({ ...base, status: "claimed", stats });
  expect(screen.queryByText(/Launching/)).toBeNull();
  expect(screen.getByText("5m")).toBeTruthy();
});

test("a launch step change re-renders the ticket page with no reload", async () => {
  const listeners: ((a?: string) => void)[] = [];
  const evConn = { ...conn, onEvent: (l: (a?: string) => void) => (listeners.push(l), () => {}) } as unknown as Connection;
  const stepped = (position: number, label: string): TicketDetail => ({
    ...base,
    status: "claimed",
    stats: { ...base.stats, launch_step: { label, position } },
  });
  let current = stepped(2, "installing dependencies");
  const fetchDetail: FetchDetail = async () => ({ kind: "ok", detail: current });
  const router = createMemoryRouter(appRoutes({ ticket: fetchDetail }), { initialEntries: ["/t/gx:demo/12"] });
  renderThemed(
    <ConnectionContext.Provider value={evConn}>
      <RouterProvider router={router} />
    </ConnectionContext.Provider>,
  );
  await screen.findByText("Launching: installing dependencies (2/4)");

  for (const [position, label] of [[3, "starting agent"], [4, "waiting for the agent"]] as const) {
    current = stepped(position, label);
    listeners.forEach((l) => l("gx:demo/12"));
    await screen.findByText(`Launching: ${label} (${position}/4)`);
  }
  expect(screen.queryByText(/installing dependencies/)).toBeNull();
});

test("the ticket page shows the ticket's verdict under the header,and nothing without a pending row", async () => {
  const mine: PendingRow = { address: "gx:demo/12", agent: "", verdict: "disk low", reason: "/tmp has 5G free" };
  await renderTicket(base, undefined, [{ ...mine, address: "gx:demo/13", verdict: "concurrency cap" }]);
  expect(document.querySelector("[data-verdict]")).toBeNull();
  cleanup();

  await renderTicket(base, undefined, [mine]);
  expect(screen.getByText("disk low: /tmp has 5G free")).toBeTruthy();
});
