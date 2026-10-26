import { cleanup, fireEvent, screen, within } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { afterEach, expect, test } from "vitest";
import { ConnectionContext, type Connection, type ConnectionState } from "../connection";
import type { EpicDetail, EpicList, EpicListRow } from "../generated/server";
import { appRoutes } from "../routes";
import type { FixAgent } from "./fixAgent";
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
  extra: { sessions?: unknown[]; detail?: Partial<EpicDetail>; hang?: boolean; notFound?: boolean; fixAgent?: FixAgent } = {},
) {
  const state = {
    snapshot: { sessions: extra.sessions ?? [] },
    connected: true,
    loadedOnce: true,
    tick: 0,
  } as unknown as ConnectionState;
  const conn = { getState: () => state, subscribe: () => () => {}, onEvent: () => () => {}, refresh: () => {} } as unknown as Connection;
  const list: EpicList = { epics, follow_ups: [] };
  const router = createMemoryRouter(
    appRoutes({
      fixAgent: extra.fixAgent,
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

test("a Design's epic page has an Open implementation epic button that goes to the -impl page", async () => {
  render("/e/gx:blog", [joined], "map");
  fireEvent.click(await screen.findByRole("button", { name: "Open implementation epic" }));
  expect(await screen.findByRole("heading", { level: 1, name: /blog-impl/ })).toBeTruthy();
});

test("an -impl epic page has an Open design button that goes back to the Design", async () => {
  render("/e/gx:blog-impl", [joined]);
  fireEvent.click(await screen.findByRole("button", { name: "Open design" }));
  expect(await screen.findByRole("heading", { level: 1, name: /^blog$/ })).toBeTruthy();
});

test("an -impl epic with no Design shows no partner button", async () => {
  render("/e/gx:solo-impl", [row("solo-impl")]);
  await screen.findByRole("heading", { level: 1 });
  expect(screen.queryByRole("button", { name: /Open (design|implementation epic)/ })).toBeNull();
});

const statsOf = async () => (await screen.findByTestId("stats-line")).textContent;
const loop = (over: Partial<EpicDetail>) => ({ detail: { total: 3, wall_clock: 125, cost_total: 1.5, agent_time: 300, ...over } });

test.each([
  ["a waiting epic with no queue item", { state: "waiting" }, "Not queued · took 2m · $1.50 · 5m agent time"],
  ["a waiting queued epic", { state: "waiting", queued: true }, "Waiting for a slot · took 2m · $1.50 · 5m agent time"],
  ["a running epic", { state: "running" }, "Running · 2m · $1.50 · 5m agent time"],
  ["a finished epic", { state: "finished" }, "Finished · took 2m · $1.50 · 5m agent time"],
  ["a draft -impl epic", { state: "waiting", draft: true }, "Publishing tickets · took 2m · $1.50 · 5m agent time"],
  ["an epic that never started", { state: "waiting", queued: true, wall_clock: undefined }, "Waiting for a slot · $1.50 · 5m agent time"],
] as [string, Partial<EpicDetail>, string][])("the stats line of %s", async (_n, over, text) => {
  render("/e/gx:blog-impl", [], "loop", loop(over));
  expect(await statsOf()).toBe(text);
});

test("the stats line has no segmented bar or done count", async () => {
  render("/e/gx:blog-impl", [], "loop", loop({ state: "running", done: 1 }));
  await statsOf();
  expect(screen.queryByText(/of \d+ done/)).toBeNull();
  expect(screen.queryByLabelText("Epic progress")).toBeNull();
});

const closedRow = (elapsed: number) => ({ address: "gx:blog/01", title: "t", rendered_status: "done", elapsed });
const claimedRow = (secondsAgo: number) => ({
  address: "gx:blog/02",
  title: "t",
  rendered_status: "claimed",
  claimed_at: new Date(Date.now() - secondsAgo * 1000).toISOString(),
});

test("a Design's stats line shows the step label, the design time and the cost", async () => {
  render("/e/gx:blog", [designCard], "map", {
    detail: { steps: steps("spec"), rows: [closedRow(120)], stats: { cost: 2.5 } },
  });
  expect(await statsOf()).toBe("spec · 2m · $2.50");
});

test("a Design's stats line leaves out the time and cost when unknown, and shows the Done label when done", async () => {
  render("/e/gx:blog", [designCard], "map", { detail: { steps: steps("done", ["done"]) } });
  expect(await statsOf()).toBe("done");
});

test("a sub-minute done row counts in the design time sum", async () => {
  render("/e/gx:blog", [designCard], "map", {
    detail: { steps: steps("spec"), rows: [closedRow(30), { ...closedRow(30), address: "gx:blog/03" }] },
  });
  expect(await statsOf()).toBe("spec · 1m");
});

test("a claimed row adds its time so far only while the Design has a live session", async () => {
  const detail = { steps: steps("spec"), rows: [closedRow(60), claimedRow(600)] };
  render("/e/gx:blog", [designCard], "map", { detail, sessions: [session("working")] });
  expect(await statsOf()).toBe("spec · 11m");
  cleanup();
  render("/e/gx:blog", [designCard], "map", { detail, sessions: [session("exited")] });
  expect(await statsOf()).toBe("spec · 1m");
  cleanup();
  render("/e/gx:blog", [designCard], "map", { detail: { steps: steps("spec"), rows: [claimedRow(600)] } });
  expect(await statsOf()).toBe("spec");
});

const live = (rows: object[], kind: EpicDetail["kind"] = "loop", fixAgent?: FixAgent) =>
  render("/e/gx:blog-impl", [], kind, { fixAgent, detail: { total: rows.length, rows: rows as EpicDetail["rows"] } });
const cardOf = async (id: string) => (await screen.findByText(id)).closest("[data-indent]") as HTMLElement;
const tk = (id: string, over: object = {}) => ({ address: `gx:blog/${id}`, title: `title ${id}`, rendered_status: "open", ...over });

test("a live ticket is one link to its page with id, title and meta; paused rows have no Open link", async () => {
  live([tk("01"), tk("02", { rendered_status: "blocked", blockers: ["01"] }), tk("03", { rendered_status: "paused", reason: "needs a person" })]);
  const card = await cardOf("01");
  const links = within(card).getAllByRole("link");
  expect(links).toHaveLength(1);
  expect(links[0].getAttribute("href")).toBe("#/t/gx:blog/01");
  expect(links[0].textContent).toContain("title 01");
  expect(links[0].textContent).toContain("ready");
  expect((await cardOf("02")).textContent).toContain("after 01");
  expect((await cardOf("03")).textContent).toContain("paused · needs a person");
  expect(screen.queryByText("Open →")).toBeNull();
});

test("a fork's card is indented and the running card has the spinning dot", async () => {
  live([tk("01", { rendered_status: "claimed", claimed_at: new Date().toISOString() }), tk("02", { parent: "gx:blog/01" })]);
  expect((await cardOf("02")).getAttribute("data-indent")).toBe("1");
  expect(within(await cardOf("01")).getByRole("img", { name: "Running" })).toBeTruthy();
});

test("a needs-you card shows its question and an Answer button to the ticket page", async () => {
  live([tk("01", { rendered_status: "needs-answer", needs_answer: true, question: "Which db?" })]);
  const card = await cardOf("01");
  expect(within(card).getByText("Which db?")).toBeTruthy();
  fireEvent.click(within(card).getByRole("button", { name: "Answer" }));
  expect(await screen.findByLabelText("Loading ticket")).toBeTruthy();
});

test("a needs-repair card is expanded with its status and reason; an error card is one line", async () => {
  live([tk("01", { rendered_status: "needs-repair", reason: "dirty tree" }), tk("02", { rendered_status: "error" })]);
  const repair = await cardOf("01");
  expect(repair.textContent).toContain("needs-repair · dirty tree");
  expect(within(repair).getByRole("button", { name: "Fix it with an agent" })).toBeTruthy();
  expect(document.body.textContent).not.toContain("Fix it in the TUI");
  const error = await cardOf("02");
  expect(error.getAttribute("data-tone")).toBe("deadlocked");
  expect(error.textContent).toContain("error");
  expect(within(error).getAllByRole("link")).toHaveLength(1);
});

const repairRow = (over: object = {}) => tk("01", { rendered_status: "needs-repair", state: "tok-1", ...over });
const openFixDialog = async () => {
  fireEvent.click(within(await cardOf("01")).getByRole("button", { name: "Fix it with an agent" }));
  return within(await screen.findByRole("dialog"));
};

test("the fix dialog shows the row's fix_prompt, with or without a reason", async () => {
  live([repairRow({ reason: "dirty tree", fix_prompt: "/gx-investigate gx:blog/01 is needs-repair: from the server" })]);
  const dialog = await openFixDialog();
  expect(dialog.getByText("/gx-investigate gx:blog/01 is needs-repair: from the server")).toBeTruthy();
  expect(dialog.getByRole("button", { name: "Copy" })).toBeTruthy();
  cleanup();
  live([repairRow({ fix_prompt: "/gx-investigate gx:blog/01 is needs-repair" })]);
  expect((await openFixDialog()).getByText("/gx-investigate gx:blog/01 is needs-repair")).toBeTruthy();
});

test.each([
  [false, "Agent started. View it in the gx TUI Agents tab"],
  [true, "Agent started. View it in the gx TUI Agents tab or in the Claude app"],
])("starting the fix agent (remote control %s) says it started and drops Start", async (remote, text) => {
  const calls: unknown[] = [];
  live([repairRow()], "loop", async (body) => {
    calls.push(body);
    return { remote_control: remote };
  });
  const dialog = await openFixDialog();
  fireEvent.click(dialog.getByRole("button", { name: "Start gx agent" }));
  expect(await dialog.findByText(text)).toBeTruthy();
  expect(dialog.queryByRole("button", { name: "Start gx agent" })).toBeNull();
  expect(calls).toEqual([{ address: "gx:blog/01", state: "tok-1" }]);
});

test("a refused fix agent shows the message and keeps Copy and Start", async () => {
  live([repairRow()], "loop", async () => ({ remote_control: false, refused: true, message: "Already investigating" }));
  const dialog = await openFixDialog();
  fireEvent.click(dialog.getByRole("button", { name: "Start gx agent" }));
  expect(await dialog.findByText("Already investigating")).toBeTruthy();
  expect(dialog.getByRole("button", { name: "Copy" })).toBeTruthy();
  expect(dialog.getByRole("button", { name: "Start gx agent" })).toBeTruthy();
});

const doneTk = (id: string, minute: number, over: object = {}) =>
  tk(id, { rendered_status: "done", elapsed: 300, cost: 1.5, closed_at: `2026-01-01T10:${String(minute).padStart(2, "0")}:00Z`, ...over });
const doneIds = () => [...document.querySelectorAll("[data-indent]")].map((c) => c.textContent);

test("the done divider counts done and cancelled; it is the first item with no live rows", async () => {
  live([doneTk("01", 1), doneTk("02", 2, { rendered_status: "cancelled" })]);
  const divider = await screen.findByText("1 done, 1 cancelled");
  expect(document.querySelector("[data-done-divider]")).toBe(divider);
  expect(divider.parentElement?.firstElementChild).toBe(divider);
  expect(divider.parentElement?.previousElementSibling?.querySelector("[data-indent]")).toBeNull();
});

test("the divider follows the live cards", async () => {
  live([doneTk("01", 1), tk("02")]);
  const divider = await screen.findByText("1 done");
  expect(screen.getByText("title 02").compareDocumentPosition(divider) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
});

test("the done group shows the 3 newest, Show N more reveals the rest and Show fewer hides them", async () => {
  live([doneTk("01", 1), doneTk("02", 4), doneTk("03", 2), doneTk("04", 3), doneTk("05", 0)]);
  await screen.findByText("5 done");
  expect(doneIds().map((t) => t?.slice(0, 2))).toEqual(["02", "04", "03"]);
  fireEvent.click(screen.getByRole("button", { name: "Show 2 more" }));
  expect(doneIds()).toHaveLength(5);
  fireEvent.click(screen.getByRole("button", { name: "Show fewer" }));
  expect(doneIds()).toHaveLength(3);
});

test("a done loop card shows time and cost; a cancelled one reads cancelled; a done fork says what it forks", async () => {
  live([doneTk("01", 1, { parent: "gx:blog/09" }), doneTk("02", 2, { rendered_status: "cancelled" })]);
  expect((await cardOf("01")).textContent).toContain("5m · $1.50 · fork of 09");
  const cancelled = await cardOf("02");
  expect(cancelled.textContent).toContain("cancelled");
  expect(cancelled.getAttribute("data-tone")).toBe("muted");
});

test("a done Design card shows its time only from a minute up", async () => {
  live([doneTk("01", 1, { elapsed: 59 }), doneTk("02", 2, { elapsed: 120 })], "map");
  expect((await cardOf("01")).textContent).not.toMatch(/\d+[smh]/);
  expect((await cardOf("02")).textContent).toContain("2m");
});

const withStats = (detail: Partial<EpicDetail>, kind: EpicDetail["kind"] = "loop") =>
  render("/e/gx:blog-impl", [], kind, { detail });
const loopStats = {
  lines_added: 1200,
  lines_removed: 300,
  total_tokens: 2_500_000,
  cost: 4.5,
  lead_time: 7200,
  friction: { pauses_by_reason: { question: 2, error: 1 }, run_events: { retry: 3 }, paused_secs: 600, web_share: 0.5 },
};

test("More stats is collapsed by default with the loop preview, and opens to every stat row", async () => {
  withStats({ stats: loopStats });
  const card = await screen.findByRole("button", { name: /^More stats/ });
  expect(card.getAttribute("aria-expanded")).toBe("false");
  expect(card.textContent).toContain("+1,200 −300 lines · 2.5M tokens · lead time 2h");
  expect(document.querySelector("[data-stat-row]")).toBeNull();
  fireEvent.click(card);
  expect(card.getAttribute("aria-expanded")).toBe("true");
  const labels = [...document.querySelectorAll("[data-stat-row]")].map((r) => r.getAttribute("data-stat-row"));
  expect(labels).toEqual(["Lines", "Tokens", "Cost", "Lead time", "Pauses", "Run events", "Paused time", "Pause endings"]);
  expect(document.querySelector("[data-stat-row='Lead time']")?.textContent).toContain("created → merged to trunk");
  expect(document.querySelector("[data-stat-row='Pauses']")?.textContent).toContain("question ×2, error ×1");
  expect(document.querySelector("[data-stat-row='Pause endings']")?.textContent).toContain("50% web");
  fireEvent.click(card);
  expect(card.getAttribute("aria-expanded")).toBe("false");
});

test("a Design's More stats preview leaves out lines", async () => {
  render("/e/gx:blog", [designCard], "map", { detail: { steps: steps("spec"), stats: loopStats } });
  expect((await screen.findByRole("button", { name: /^More stats/ })).textContent).toContain("2.5M tokens · lead time 2h");
  expect(screen.queryByText(/lines/)).toBeNull();
});

test("there is no More stats card without stats", async () => {
  withStats({ stats: {} });
  await statsOf();
  expect(screen.queryByRole("button", { name: /More stats/ })).toBeNull();
});

test("a Design's cards show the ticket type then the state", async () => {
  live([tk("01", { type: "prototype" }), tk("02", { type: "grilling", rendered_status: "blocked", blockers: ["04"] })], "map");
  expect((await cardOf("01")).textContent).toContain("prototype · ready");
  expect((await cardOf("02")).textContent).toContain("grilling · after 04");
});

test("the buttons run partner first and the action last, Merge in the done tone", async () => {
  render("/e/gx:blog-impl", [joined], "loop", { detail: { state: "awaiting-merge" } });
  const buttons = await screen.findAllByRole("button", { name: /^(Open design|Merge)$/ });
  expect(buttons.map((b) => b.textContent)).toEqual(["Open design", "Merge"]);
  expect(buttons[1].getAttribute("data-tone")).toBe("done");
});

test("a draft -impl epic has a disabled Queue with its tooltip", async () => {
  render("/e/gx:blog-impl", [], "loop", { detail: { draft: true } });
  const queue = (await screen.findByRole("button", { name: "Queue" })) as HTMLButtonElement;
  expect(queue.disabled).toBe(true);
  expect(queue.parentElement?.getAttribute("title")).toBe("Queue unlocks once all tickets are published");
  expect(screen.queryByText("Still publishing tickets")).toBeNull();
});

test("the land-block reason is a notice under the bar only while paused; Merge follows isMergeable", async () => {
  render("/e/gx:blog-impl", [], "loop", { detail: { state: "paused", land_block_reason: "rebase conflict" } });
  expect((await screen.findByText("rebase conflict")).hasAttribute("data-reason")).toBe(true);
  expect(screen.getByRole("button", { name: "Merge" })).toBeTruthy();
  cleanup();
  render("/e/gx:blog-impl", [], "loop", { detail: { state: "running", land_block_reason: "stale reason" } });
  await statsOf();
  expect(screen.queryByText("stale reason")).toBeNull();
  expect(screen.queryByRole("button", { name: "Merge" })).toBeNull();
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
