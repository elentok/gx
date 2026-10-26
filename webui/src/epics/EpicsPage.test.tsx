import { act, cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { afterEach, expect, test, vi } from "vitest";
import { ConnectionContext, type Connection, type ConnectionState } from "../connection";
import type { EpicList, EpicListRow } from "../generated/server";
import type { OpenDesign } from "../design/open";
import { appRoutes } from "../routes";
import { renderThemed } from "../ui/render";
import type { FetchEpicList } from "./list";

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

const state: ConnectionState = {
  snapshot: undefined as unknown as ConnectionState["snapshot"],
  connected: true,
  loadedOnce: true,
  tick: 0,
  streams: 0,
};
const conn = {
  getState: () => state,
  subscribe: () => () => {},
  onEvent: () => () => {},
  refresh: () => {},
} as unknown as Connection;

const row = (name: string, over: Partial<EpicListRow> = {}): EpicListRow => ({
  address: `gx:${name}`,
  project: "gx",
  name,
  kind: "loop",
  state: "ready",
  done: 0,
  total: 3,
  queued: false,
  queueable: true,
  blocked_by_epics: [],
  card_kind: "implementation",
  section: "ready-to-queue",
  addresses: [`gx:${name}`],
  ...over,
});

const listOf = (epics: EpicListRow[], over: Partial<EpicList> = {}): EpicList => ({ epics, follow_ups: [], ...over });

function eventConn() {
  const listeners = new Set<() => void>();
  const c = {
    getState: () => state,
    subscribe: () => () => {},
    onEvent: (l: () => void) => {
      listeners.add(l);
      return () => listeners.delete(l);
    },
  } as unknown as Connection;
  return { conn: c, emit: () => listeners.forEach((l) => l()) };
}

function renderEpics(fetchList: FetchEpicList, withConn: Connection = conn, openDesign?: OpenDesign) {
  const router = createMemoryRouter(appRoutes({ epicList: fetchList, openDesign }), { initialEntries: ["/epics"] });
  const view = renderThemed(
    <ConnectionContext.Provider value={withConn}>
      <RouterProvider router={router} />
    </ConnectionContext.Provider>,
  );
  return { ...view, router };
}

const filterBox = () => screen.findByRole("searchbox", { name: "Filter epics" });

const loaded = async (list: EpicList) => {
  renderEpics(async () => list);
  await filterBox();
  await waitFor(() => expect(screen.queryByRole("status", { name: "Loading epics" })).toBeNull());
};

const mixed = () =>
  listOf([
    row("need", { section: "needs-you" }),
    row("busy", { section: "in-progress" }),
    row("rdy", { section: "ready-to-queue" }),
    row("idea", { kind: "map", card_kind: "design", section: "ready-to-design" }),
    row("done-a", { state: "finished", section: "finished" }),
    row("done-b", { state: "finished", section: "finished" }),
  ]);

const regionNames = () => screen.getAllByRole("region").map((r) => r.getAttribute("aria-labelledby"));
const toggle = (title: string) => screen.getByRole("button", { name: new RegExp(`^.?\\s*${title}`) });

test("five sections show in order with counts, Ready to design and Finished start collapsed", async () => {
  await loaded(mixed());
  expect(regionNames()).toEqual([
    "epics-needs-you",
    "epics-ready-to-queue",
    "epics-in-progress",
    "epics-ready-to-design",
    "epics-finished",
  ]);
  expect(toggle("Needs you").getAttribute("aria-expanded")).toBe("true");
  expect(toggle("Ready to design").getAttribute("aria-expanded")).toBe("false");
  expect(toggle("Finished").getAttribute("aria-expanded")).toBe("false");
  expect(toggle("Finished").textContent).toContain("2");
  expect(screen.getByRole("link", { name: "need" })).toBeTruthy();
  expect(screen.queryByRole("link", { name: "idea" })).toBeNull();
  expect(screen.queryByRole("link", { name: "done-a" })).toBeNull();
});

test("a toggle opens a collapsed section and closes an open one", async () => {
  await loaded(mixed());
  fireEvent.click(toggle("Finished"));
  expect(toggle("Finished").getAttribute("aria-expanded")).toBe("true");
  expect(screen.getByRole("link", { name: "done-a" })).toBeTruthy();
  fireEvent.click(toggle("Needs you"));
  expect(screen.queryByRole("link", { name: "need" })).toBeNull();
});

test("Finished shows the first recently-finished cards, the count follows", async () => {
  await loaded({ ...mixed(), recently_finished: 1 });
  expect(toggle("Finished").textContent).toContain("1");
  fireEvent.click(toggle("Finished"));
  expect(screen.getByRole("link", { name: "done-a" })).toBeTruthy();
  expect(screen.queryByRole("link", { name: "done-b" })).toBeNull();
});

test("a few hundred finished epics load, show the cap, and filter past it", async () => {
  const many = Array.from({ length: 300 }, (_, i) => row(`done-${i}`, { state: "finished", section: "finished" }));
  await loaded(listOf(many, { recently_finished: 20 }));
  fireEvent.click(toggle("Finished"));
  expect(screen.getAllByRole("link", { name: /^done-/ })).toHaveLength(20);

  fireEvent.change(await filterBox(), { target: { value: "done-29" } });
  expect(screen.getAllByRole("link", { name: /^done-/ })).toHaveLength(11);
});

test("the filter opens a matching collapsed section, hides the others, and reaches past the cap", async () => {
  await loaded({ ...mixed(), recently_finished: 1 });
  fireEvent.change(await filterBox(), { target: { value: "DONE-B" } });
  expect(regionNames()).toEqual(["epics-finished"]);
  expect(screen.getByRole("link", { name: "done-b" })).toBeTruthy();
  expect(toggle("Finished").textContent).toContain("1");
  fireEvent.change(await filterBox(), { target: { value: "" } });
  expect(regionNames()).toHaveLength(5);
  expect(screen.queryByRole("link", { name: "done-b" })).toBeNull();
});

const twoProjects = () =>
  listOf([
    row("g1", { project: "gx" }),
    row("o1", { project: "other" }),
    row("fu", { project: "other", card_kind: "follow-ups" as never }),
  ]);
const projectSelect = () => screen.getByRole("combobox", { name: "Project" }) as HTMLSelectElement;

test("the project select shows only with two or more projects, defaulting to All projects", async () => {
  await loaded(mixed());
  expect(screen.queryByRole("combobox", { name: "Project" })).toBeNull();
  cleanup();
  await loaded(twoProjects());
  expect(projectSelect().value).toBe("");
  expect(within(projectSelect()).getByRole("option", { name: "All projects" })).toBeTruthy();
  expect(screen.getAllByRole("link", { name: /^(g1|o1|fu)$/ })).toHaveLength(3);
});

test("the project select filters every card, follow-ups included, combined with text, and is stored", async () => {
  await loaded(twoProjects());
  fireEvent.change(projectSelect(), { target: { value: "other" } });
  expect(screen.queryByRole("link", { name: "g1" })).toBeNull();
  expect(screen.getByRole("link", { name: "o1" })).toBeTruthy();
  expect(screen.getByRole("link", { name: "fu" })).toBeTruthy();
  fireEvent.change(await filterBox(), { target: { value: "o1" } });
  expect(screen.queryByRole("link", { name: "fu" })).toBeNull();
  expect(localStorage.getItem("gx.epics.project")).toBe("other");
});

test("a stored project is restored", async () => {
  localStorage.setItem("gx.epics.project", "gx");
  await loaded(twoProjects());
  expect(projectSelect().value).toBe("gx");
  expect(screen.queryByRole("link", { name: "o1" })).toBeNull();
});

test("a stale stored project acts as All projects without being rewritten", async () => {
  localStorage.setItem("gx.epics.project", "gone");
  await loaded(twoProjects());
  expect(projectSelect().value).toBe("");
  expect(screen.getAllByRole("link", { name: /^(g1|o1|fu)$/ })).toHaveLength(3);
  expect(localStorage.getItem("gx.epics.project")).toBe("gone");
});

test("a filter with no match says No match", async () => {
  await loaded(mixed());
  fireEvent.change(await filterBox(), { target: { value: "zzz" } });
  expect(screen.getByText("No match")).toBeTruthy();
  expect(screen.queryByRole("region")).toBeNull();
});

test("no epics shows the empty state with a New Design button", async () => {
  await loaded(listOf([]));
  expect(screen.getByText("No epics yet.")).toBeTruthy();
  expect(screen.getAllByRole("button", { name: "New Design" })).toHaveLength(1);
});

test("the follow-ups counts line is gone", async () => {
  await loaded(listOf([row("x")], { follow_ups: [{ project: "gx", open: 3 }] }));
  expect(screen.queryByText(/Follow-ups/)).toBeNull();
});

test("New Design opens the form in a dialog, Close closes it", async () => {
  await loaded(mixed());
  expect(screen.queryByRole("dialog")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "New Design" }));
  const dialog = screen.getByRole("dialog", { name: "New Design" });
  expect(within(dialog).getByRole("textbox", { name: "Epic name" })).toBeTruthy();
  fireEvent.click(within(dialog).getByRole("button", { name: "Close" }));
  expect(screen.queryByRole("dialog")).toBeNull();
});

test("the New Design project starts on the page's filtered project", async () => {
  localStorage.setItem("gx.epics.project", "blog");
  await loaded(listOf([row("a"), row("b", { project: "blog", address: "blog:b" })]));
  fireEvent.click(screen.getByRole("button", { name: "New Design" }));
  const dialog = screen.getByRole("dialog", { name: "New Design" });
  expect((within(dialog).getByRole("combobox", { name: "Project" }) as HTMLSelectElement).value).toBe("blog");
});

test("Start posts new_design and opens the new Design page", async () => {
  const open = vi.fn(async () => ({ refused: false }) as never);
  const { router } = renderEpics(async () => mixed(), conn, open);
  await filterBox();
  fireEvent.click(await screen.findByRole("button", { name: "New Design" }));
  const dialog = screen.getByRole("dialog", { name: "New Design" });
  fireEvent.change(within(dialog).getByRole("textbox", { name: "Epic name" }), { target: { value: "fresh" } });
  fireEvent.change(within(dialog).getByRole("textbox", { name: "Rough idea" }), { target: { value: "an idea" } });
  await act(async () => fireEvent.click(within(dialog).getByRole("button", { name: "Start design" })));
  expect(open).toHaveBeenCalledWith({ epic: "fresh", new_design: true, idea: "an idea", project: "gx" });
  await waitFor(() => expect(router.state.location.pathname).toBe("/d/gx:fresh"));
});

test("the first load shows a spinner under the filter", async () => {
  renderEpics(() => new Promise(() => {}));
  expect(await screen.findByRole("status", { name: "Loading epics" })).toBeTruthy();
  expect(await filterBox()).toBeTruthy();
  expect(screen.queryByRole("region")).toBeNull();
});

test("a failed first fetch shows Could not load epics, and Retry fetches again", async () => {
  let calls = 0;
  renderEpics(async () => {
    if (++calls === 1) throw new Error("down");
    return listOf([row("x")]);
  });
  await screen.findByText("Could not load epics");
  await act(async () => fireEvent.click(screen.getByRole("button", { name: "Retry" })));
  expect(await screen.findByRole("link", { name: "x" })).toBeTruthy();
  expect(screen.queryByText("Could not load epics")).toBeNull();
  expect(calls).toBe(2);
});

test("a burst of events gives one refetch, and filter text survives it", async () => {
  vi.useFakeTimers({ shouldAdvanceTime: true });
  const { conn: c, emit } = eventConn();
  const fetchList = vi.fn(async () => listOf([row("x")]));
  renderEpics(fetchList, c);
  const box = await filterBox();
  fireEvent.change(box, { target: { value: "x" } });
  await waitFor(() => expect(fetchList).toHaveBeenCalledTimes(1));
  act(() => {
    emit();
    vi.advanceTimersByTime(200);
    emit();
    vi.advanceTimersByTime(200);
    emit();
  });
  expect(fetchList).toHaveBeenCalledTimes(1);
  await act(async () => {
    vi.advanceTimersByTime(500);
  });
  expect(fetchList).toHaveBeenCalledTimes(2);
  expect((box as HTMLInputElement).value).toBe("x");
});

test("a failed refetch keeps the last list", async () => {
  vi.useFakeTimers({ shouldAdvanceTime: true });
  const { conn: c, emit } = eventConn();
  let calls = 0;
  renderEpics(async () => {
    if (++calls > 1) throw new Error("down");
    return listOf([row("keep")]);
  }, c);
  await screen.findByRole("link", { name: "keep" });
  act(() => emit());
  await act(async () => {
    vi.advanceTimersByTime(500);
  });
  expect(calls).toBe(2);
  expect(screen.getByRole("link", { name: "keep" })).toBeTruthy();
  expect(screen.queryByText("Could not load epics")).toBeNull();
});

function stateConn() {
  const subs = new Set<() => void>();
  let current = state;
  const c = {
    getState: () => current,
    subscribe: (l: () => void) => {
      subs.add(l);
      return () => subs.delete(l);
    },
    onEvent: () => () => {},
  } as unknown as Connection;
  return {
    conn: c,
    setConnected: (connected: boolean) => {
      current = { ...current, connected };
      subs.forEach((l) => l());
    },
  };
}

test("the list refetches when the connection comes back", async () => {
  const { conn: c, setConnected } = stateConn();
  let calls = 0;
  renderEpics(async () => listOf([row(++calls === 1 ? "old" : "new")]), c);
  await screen.findByRole("link", { name: "old" });
  act(() => setConnected(false));
  expect(calls).toBe(1);
  act(() => setConnected(true));
  expect(await screen.findByRole("link", { name: "new" })).toBeTruthy();
  expect(calls).toBe(2);
});

test("a failed first fetch is retried when the connection comes back", async () => {
  const { conn: c, setConnected } = stateConn();
  let calls = 0;
  renderEpics(async () => {
    if (++calls === 1) throw new Error("down");
    return listOf([row("x")]);
  }, c);
  await screen.findByText("Could not load epics");
  act(() => setConnected(false));
  act(() => setConnected(true));
  expect(await screen.findByRole("link", { name: "x" })).toBeTruthy();
  expect(calls).toBe(2);
});
