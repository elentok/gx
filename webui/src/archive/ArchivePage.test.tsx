import { act, cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { afterEach, expect, test, vi } from "vitest";
import type { ArchivedEpicRow } from "../generated/server";
import { ConnectionContext, type Connection, type ConnectionState } from "../connection";
import { appRoutes } from "../routes";
import { renderThemed } from "../ui/render";
import type { FetchArchived } from "./archive";

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

const state = { snapshot: undefined, connected: true, loadedOnce: true, liveSnapshot: true, tick: 0, streams: 0 } as unknown as ConnectionState;
const conn = { getState: () => state, subscribe: () => () => {}, onEvent: () => () => {}, refresh: () => {} } as unknown as Connection;

function eventConn() {
  const listeners = new Set<() => void>();
  const c = {
    ...conn,
    onEvent: (l: () => void) => {
      listeners.add(l);
      return () => listeners.delete(l);
    },
  } as unknown as Connection;
  return { conn: c, emit: () => listeners.forEach((l) => l()) };
}

function renderArchive(archived: FetchArchived, withConn: Connection = conn) {
  sessionStorage.clear();
  const router = createMemoryRouter(appRoutes({ archived, epicList: async () => ({ epics: [], follow_ups: [] }) }), { initialEntries: ["/archive"] });
  renderThemed(
    <ConnectionContext.Provider value={withConn}>
      <RouterProvider router={router} />
    </ConnectionContext.Provider>,
  );
  return router;
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

const arow = (name: string, over: Partial<ArchivedEpicRow> = {}): ArchivedEpicRow => ({
  address: `gx:${name}`,
  project: "gx",
  name,
  title: `Title ${name}`,
  archived_at: "2026-03-05T12:00:00Z",
  kind: "loop",
  addresses: [`gx:${name}`],
  workspace: "work",
  ...over,
});

const names = () => screen.getAllByRole("listitem").map((li) => li.getAttribute("aria-label"));

/** Answers the unarchive POST per target. */
const stubUnarchive = (answer: (target: string) => object) => {
  const bodies: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string, init?: RequestInit) => {
      if (!String(url).includes("/v1/epics/unarchive")) return new Response("not stubbed", { status: 404 });
      const body = JSON.parse(String(init?.body)) as { targets: string[] };
      bodies.push(body);
      return new Response(JSON.stringify({ results: body.targets.map((t) => ({ target: t, status: "done", ...answer(t) })) }), { status: 200 });
    }),
  );
  return bodies;
};

test("rows keep the server's order and show their fields; a no-date row shows no date", async () => {
  renderArchive(async () => ({ epics: [arow("b"), arow("a", { kind: "map" }), arow("old", { archived_at: undefined })] }));
  await screen.findByText("Title b");
  expect(names()).toEqual(["Title b", "Title a", "Title old"]);
  const [b, a, old] = screen.getAllByRole("listitem");
  expect(b.textContent).toContain("gx · Epic · ");
  expect(b.textContent).toContain("2026");
  expect(b.textContent).toContain("work");
  expect(a.textContent).toContain("Design");
  expect(old.textContent).toContain("gx · Epic · work");
  expect(old.textContent).not.toMatch(/\d{4}/);
});

test("a click or Enter on a row opens its detail route", async () => {
  const router = renderArchive(async () => ({ epics: [arow("a"), arow("b")] }));
  fireEvent.click(await screen.findByText("Title a"));
  await waitFor(() => expect(router.state.location.pathname).toBe("/archive/gx:a"));
  await act(() => router.navigate("/archive"));
  fireEvent.keyDown(await screen.findByRole("listitem", { name: "Title b" }), { key: "Enter" });
  await waitFor(() => expect(router.state.location.pathname).toBe("/archive/gx:b"));
});

test("the row menu Unarchive posts and the row leaves; a clash names the new address", async () => {
  const post = stubUnarchive((t) => (t === "gx:b" ? { new_address: "gx:b-2" } : {}));
  renderArchive(async () => ({ epics: [arow("a"), arow("b")] }));
  await screen.findByText("Title a");
  fireEvent.click(within(screen.getByRole("listitem", { name: "Title b" })).getByRole("button", { name: "More actions" }));
  fireEvent.click(await screen.findByText("Unarchive"));
  await waitFor(() => expect(names()).toEqual(["Title a"]));
  expect(post).toEqual([{ targets: ["gx:b"] }]);
  expect(await screen.findByText("Unarchived as b-2")).toBeTruthy();
});

test("select mode offers Unarchive only; refused rows stay selected, done rows leave", async () => {
  const post = stubUnarchive((t) => (t === "gx:b" ? { status: "refused", reason: "gone", message: "not in the archive" } : {}));
  renderArchive(async () => ({ epics: [arow("a"), arow("b")] }));
  await screen.findByText("Title a");
  fireEvent.click(within(screen.getByTestId("page-header-actions")).getByRole("button", { name: "Select" }));
  fireEvent.click(screen.getByRole("checkbox", { name: "Select Title a" }));
  fireEvent.click(screen.getByRole("checkbox", { name: "Select Title b" }));
  const bar = screen.getByRole("button", { name: "Unarchive 2" }).parentElement!;
  expect(within(bar).getAllByRole("button").map((b) => b.textContent)).toEqual(["Unarchive 2"]);
  fireEvent.click(screen.getByRole("button", { name: "Unarchive 2" }));
  await waitFor(() => expect(names()).toEqual(["Title b"]));
  expect(post).toEqual([{ targets: ["gx:a", "gx:b"] }]);
  expect((screen.getByRole("checkbox", { name: "Select Title b" }) as HTMLInputElement).checked).toBe(true);
  expect(await screen.findByText("Title b: not in the archive")).toBeTruthy();
});

test("an event refetches and updates the rows; a burst refetches once", async () => {
  const { conn: c, emit } = eventConn();
  let list = [arow("a")];
  const fetchList = vi.fn(async () => ({ epics: list }));
  renderArchive(fetchList, c);
  await screen.findByText("Title a");
  expect(fetchList).toHaveBeenCalledTimes(1);
  list = [arow("c"), arow("a")];
  vi.useFakeTimers();
  emit();
  emit();
  emit();
  await act(() => vi.advanceTimersByTimeAsync(1000));
  vi.useRealTimers();
  expect(fetchList).toHaveBeenCalledTimes(2);
  await waitFor(() => expect(names()).toEqual(["Title c", "Title a"]));
});
