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

function render(
  path: string,
  epics: EpicListRow[],
  kind: EpicDetail["kind"] = "loop",
  extra: { sessions?: unknown[]; detail?: Partial<EpicDetail>; hang?: boolean; notFound?: boolean } = {},
) {
  const state = {
    snapshot: { sessions: extra.sessions ?? [] },
    connected: true,
    loadedOnce: true,
    tick: 0,
  } as unknown as ConnectionState;
  const conn = { getState: () => state, subscribe: () => () => {}, onEvent: () => () => {} } as unknown as Connection;
  const list: EpicList = { epics, follow_ups: [] };
  const router = createMemoryRouter(
    appRoutes({
      epicList: async () => list,
      epic: extra.hang
        ? () => new Promise(() => {})
        : extra.notFound
          ? async () => ({ kind: "not-found" })
          : async (address) => ({ kind: "ok", detail: { ...detailOf(address, kind), ...extra.detail } }),
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

test("the back link counts the other Needs-you cards from the loaded list", async () => {
  render("/e/gx:blog-impl", [
    { ...joined, section: "needs-you" },
    row("q1", { section: "needs-you" }),
    row("q2", { section: "needs-you" }),
  ]);
  expect(await screen.findByRole("link", { name: "← Epics · 2 more need you" })).toBeTruthy();
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

const ringOf = () => document.querySelector("[data-kind][data-tone]") as HTMLElement;
const steps = (current: string, done: string[] = []): EpicDetail["steps"] =>
  ["chart", "spec", "create-tickets", "done"].map((step) => ({
    step,
    label: step,
    state: done.includes(step) ? "done" : step === current ? "current" : "todo",
  }));
const session = (state: string, actionable = "") => ({
  id: "s1",
  design: true,
  ticket: "gx:blog/01",
  status: { state },
  status_changed_at: "2026-01-01T00:00:00Z",
  run_started_at: "2026-01-01T00:00:00Z",
  actionable: actionable ? { reason: actionable } : {},
});
const designCard = row("blog", { card_kind: "design", kind: "map", addresses: ["gx:blog"] });

test.each([
  ["done", { steps: steps("done", ["done"]) }, [], "done", false],
  ["a live session with an Actionable", { steps: steps("spec") }, [session("idle", "question")], "waiting", false],
  ["a working session", { steps: steps("spec") }, [session("working")], "running", true],
  ["a starting session", { steps: steps("spec") }, [session("starting")], "running", true],
  ["creating tickets", { steps: steps("create-tickets"), impl_address: "gx:blog-impl" }, [], "running", true],
  ["an idle live session", { steps: steps("spec") }, [session("idle")], "running", false],
  ["only an Exited session", { steps: steps("spec") }, [session("exited")], "muted", false],
  ["no session", { steps: steps("spec") }, [], "muted", false],
])("a Design page with %s", async (_n, detail, sessions, tone, comet) => {
  render("/e/gx:blog", [designCard], "map", { detail: { ...detail, ring_progress: 40 }, sessions });
  expect(await screen.findByText("40%")).toBeTruthy();
  expect(ringOf().getAttribute("data-kind")).toBe("design");
  expect(ringOf().getAttribute("data-tone")).toBe(tone);
  expect(!!screen.queryByTestId("comet")).toBe(comet);
});

test("a Design in Needs you reads waiting", async () => {
  render("/e/gx:blog", [{ ...designCard, section: "needs-you" }], "map", { detail: { steps: steps("spec") } });
  await screen.findByText("0%");
  expect(ringOf().getAttribute("data-tone")).toBe("waiting");
  expect(screen.queryByTestId("comet")).toBeNull();
});

test.each([
  ["running", "running", true],
  ["merging", "running", true],
  ["finished", "done", false],
  ["paused", "muted", false],
])("an -impl page whose state is %s (card shows the Design) uses its own kind and tone", async (state, tone, comet) => {
  const card = row("blog-impl", { card_kind: "design", state: "paused", addresses: ["gx:blog", "gx:blog-impl"] });
  render("/e/gx:blog-impl", [card], "loop", { detail: { state: state as EpicDetail["state"], done: 2, total: 7 } });
  expect(await screen.findByText("2/7")).toBeTruthy();
  expect(ringOf().getAttribute("data-kind")).toBe("implementation");
  expect(ringOf().getAttribute("data-tone")).toBe(tone);
  expect(!!screen.queryByTestId("comet")).toBe(comet);
});

test("an -impl page uses its card's tone and comet when the card is for this page", async () => {
  const card = row("blog-impl", { state: "running", addresses: ["gx:blog-impl", "gx:blog"] });
  render("/e/gx:blog-impl", [card], "loop", { detail: { state: "paused", done: 1, total: 2 } });
  await screen.findByText("1/2");
  expect(ringOf().getAttribute("data-tone")).toBe("running");
  expect(screen.getByTestId("comet")).toBeTruthy();
});

test("a Design page whose card shows the -impl epic uses its own kind", async () => {
  const card = row("blog-impl", { card_kind: "implementation", addresses: ["gx:blog-impl", "gx:blog"] });
  render("/e/gx:blog", [card], "map", { detail: { steps: steps("spec") } });
  await screen.findByText("0%");
  expect(ringOf().getAttribute("data-kind")).toBe("design");
  expect(ringOf().getAttribute("data-tone")).toBe("muted");
});

test("the follow-ups epic keeps its card's badge", async () => {
  render("/e/gx:follow-ups", [row("follow-ups", { card_kind: "follow-ups", addresses: ["gx:follow-ups"] })]);
  await screen.findByText("0/0");
  expect(ringOf().getAttribute("data-kind")).toBe("followups");
});

test("an empty epic reads 0/0", async () => {
  render("/e/gx:blog-impl", []);
  expect(await screen.findByText("0/0")).toBeTruthy();
});

test("the summary renders in full", async () => {
  const summary = "word ".repeat(200).trim();
  render("/e/gx:blog-impl", [], "loop", { detail: { summary } });
  expect(await screen.findByText(summary)).toBeTruthy();
});

test("the spinner shows before data and an unknown address says not found", async () => {
  render("/e/gx:blog-impl", [], "loop", { hang: true });
  expect(screen.getByLabelText("Loading epic")).toBeTruthy();
  cleanup();
  render("/e/gx:nope", [], "loop", { notFound: true });
  expect(await screen.findByText("Epic not found")).toBeTruthy();
});

test("a seeded page cache paints the header before the fetch resolves", async () => {
  sessionStorage.setItem(
    "gx.page.epic",
    JSON.stringify({ id: "gx:blog-impl", value: { ...detailOf("gx:blog-impl", "loop"), done: 2, total: 7 } }),
  );
  render("/e/gx:blog-impl", [], "loop", { hang: true });
  expect(await screen.findByText("2/7")).toBeTruthy();
  sessionStorage.clear();
});
