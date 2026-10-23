import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { afterEach, expect, test, vi } from "vitest";
import canned from "../canned-detail.json";
import { ConnectionContext, type Connection, type ConnectionState } from "../connection";
import type { TicketDetail } from "../generated/server";
import { appRoutes } from "../routes";
import { renderThemed } from "../ui/render";
import type { FetchDetail } from "./detail";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const base = { ...canned, epic: "gx:demo", map_epic: false, related: [], runs: [] } as TicketDetail;
const plain: TicketDetail = {
  ...base,
  status: "open",
  pause_reason: undefined,
  rounds: [],
  latest: undefined,
  chat: [],
  state: "",
  actions: [],
};

// The state object must be stable: useSyncExternalStore loops on a fresh one per call.
const state = { connected: true, loadedOnce: true, tick: 0, snapshot: { tickets: [], epics: [] } } as unknown as ConnectionState;
const conn = {
  getState: () => state,
  subscribe: () => () => {},
  onEvent: () => () => {},
} as unknown as Connection;

function renderTicket(current: () => TicketDetail) {
  const fetchDetail: FetchDetail = async () => ({ kind: "ok", detail: current() });
  const router = createMemoryRouter(appRoutes(fetchDetail), { initialEntries: ["/t/gx:demo/12"] });
  return renderThemed(
    <ConnectionContext.Provider value={conn}>
      <RouterProvider router={router} />
    </ConnectionContext.Provider>,
  );
}

const card = () => screen.queryByRole("group", { name: "Action card" });

test("an open question shows the card first under the header, with its chat and rounds", async () => {
  renderTicket(() => base);
  const c = await screen.findByRole("group", { name: "Action card" });
  const header = screen.getByRole("heading", { level: 1 });
  expect(header.compareDocumentPosition(c) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  expect(c.textContent).toContain("Should the cache live in Redis or in process memory?");
  expect(c.textContent).toContain("Earlier rounds (2)");
  expect(c.querySelector('[role="form"]')).toBeTruthy();
});

test("a permission prompt on a running ticket shows the card", async () => {
  const detail: TicketDetail = {
    ...plain,
    status: "claimed",
    permission: { request_id: "r1", tool: "Bash", description: "run ls" },
  };
  renderTicket(() => detail);
  const c = await screen.findByRole("group", { name: "Action card" });
  expect(c.textContent).toContain("Needs approval");
  expect(screen.getByRole("button", { name: "Allow once" })).toBeTruthy();
});

test("a Paused ticket shows its pause text in the card next to its action", async () => {
  const detail: TicketDetail = {
    ...plain,
    status: "needs-repair",
    pause_reason: "merge-conflict",
    pause_text: "Conflict in main.go",
    state: "tok-2",
    actions: ["resume"],
  };
  renderTicket(() => detail);
  const c = await screen.findByRole("group", { name: "Action card" });
  expect(c.textContent).toContain("Conflict in main.go");
  expect(screen.getByRole("button", { name: "Resolve conflict" })).toBeTruthy();
});

test("an Answered ticket keeps the last question and the answer", async () => {
  const detail: TicketDetail = {
    ...base,
    status: "open",
    answered: true,
    latest: { ...base.latest!, outcome: "answered", answer: { source: "web", text: "Redis" } },
    chat: [...base.chat, { from: "web", text: "Redis" }],
    actions: [],
  };
  renderTicket(() => detail);
  const c = await screen.findByRole("group", { name: "Action card" });
  expect(c.textContent).toContain("Should the cache live in Redis or in process memory?");
  expect(c.textContent).toContain("Redis");
});

test("a sent answer keeps its result on screen after the refetch", async () => {
  let detail = base;
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => {
      detail = {
        ...base,
        status: "open",
        answered: true,
        latest: { ...base.latest!, outcome: "answered", answer: { source: "web", text: "Redis" } },
        actions: [],
      };
      return new Response(JSON.stringify({ outcome: "queued" }));
    }),
  );
  renderTicket(() => detail);
  fireEvent.change(await screen.findByLabelText("Your answer"), { target: { value: "Redis" } });
  fireEvent.click(screen.getByRole("button", { name: "Send answer" }));
  const result = await screen.findByRole("status", { name: "Answer result" });
  expect(result.textContent).toContain("Answer sent · queued");
  await waitFor(() => expect(screen.queryByLabelText("Your answer")).toBeNull());
  expect(screen.getByRole("status", { name: "Answer result" })).toBeTruthy();
  expect(card()).toBeTruthy();
});

test("a sent action keeps its result on screen after the refetch", async () => {
  let detail: TicketDetail = {
    ...plain,
    status: "needs-answer",
    pause_reason: "manual",
    state: "tok-3",
    actions: ["resume"],
  };
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => {
      detail = { ...plain, status: "open", actions: [] };
      return new Response(JSON.stringify({ data: { ticket: "gx:demo/12", status: "reopened" } }));
    }),
  );
  renderTicket(() => detail);
  fireEvent.click(await screen.findByRole("button", { name: "Resume" }));
  const result = await screen.findByRole("status", { name: "Resume result" });
  expect(result.textContent).toContain("reopened");
  await waitFor(() => expect(screen.queryByRole("button", { name: "Resume" })).toBeNull());
  expect(screen.getByRole("status", { name: "Resume result" })).toBeTruthy();
});

test("a done ticket and a plain open ticket show no card and no earlier rounds", async () => {
  renderTicket(() => ({ ...base, status: "done", rounds: base.rounds, latest: { ...base.latest!, outcome: "answered" }, actions: [] }));
  await screen.findByRole("heading", { level: 1 });
  expect(card()).toBeNull();
  expect(screen.queryByText(/Earlier rounds/)).toBeNull();
  cleanup();
  renderTicket(() => plain);
  await screen.findByRole("heading", { level: 1 });
  expect(card()).toBeNull();
  expect(screen.queryByText(/Earlier rounds/)).toBeNull();
});
