import { cleanup, screen } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { afterEach, expect, test } from "vitest";
import canned from "../canned-detail.json";
import { ConnectionContext, type Connection, type ConnectionState } from "../connection";
import type { DetailRun, RelatedTicket, TicketDetail } from "../generated/server";
import { appRoutes } from "../routes";
import { renderThemed } from "../ui/render";
import type { FetchDetail } from "./detail";
import { formatStart } from "./runs";

afterEach(cleanup);

const plain: TicketDetail = {
  ...(canned as TicketDetail),
  epic: "gx:demo",
  map_epic: false,
  status: "open",
  body: "",
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
};

const state = { connected: true, loadedOnce: true, tick: 0, snapshot: { tickets: [], epics: [] } } as unknown as ConnectionState;
const conn = { getState: () => state, subscribe: () => () => {}, onEvent: () => () => {} } as unknown as Connection;

function renderTicket(detail: TicketDetail) {
  const fetchDetail: FetchDetail = async () => ({ kind: "ok", detail });
  const router = createMemoryRouter(appRoutes({ ticket: fetchDetail }), { initialEntries: ["/t/gx:demo/12"] });
  return renderThemed(
    <ConnectionContext.Provider value={conn}>
      <RouterProvider router={router} />
    </ConnectionContext.Provider>,
  );
}

const headings = () => screen.getAllByRole("heading", { level: 2 }).map((h) => h.textContent);
const related = (relation: string, address: string, status: string): RelatedTicket => ({
  relation,
  address,
  linkable: address.includes(":"),
  title: `Title of ${address.slice(address.indexOf(":") + 1)}`,
  status,
});

test("Related shows reason and status, links to the ticket page, and names other epics without the project", async () => {
  renderTicket({
    ...plain,
    body: "Some text.",
    related: [
      related("blocker", "gx:demo/03", "done"),
      related("blocker", "gx:other-epic/07", "open"),
      related("parent", "gx:demo/02", "done"),
      related("fork", "gx:demo/13", "open"),
    ],
  });
  await screen.findByText("Title of demo/03");
  expect(headings()).toEqual(["Related", "Description"]);
  const section = screen.getByRole("region", { name: "Related" });
  expect(section.textContent).toContain("03");
  expect(section.textContent).toContain("other-epic/07");
  expect(section.textContent).not.toContain("gx:");
  expect(section.textContent).toContain("blocks this · done");
  expect(section.textContent).toContain("forked from · done");
  expect(section.textContent).toContain("fork · open");
  expect(screen.getByRole("link", { name: "Title of other-epic/07" }).getAttribute("href")).toBe("#/t/gx:other-epic/07");
});

test("Related tones each status word like the epic page", async () => {
  renderTicket({
    ...plain,
    related: [
      related("blocker", "gx:demo/03", "needs-answer"),
      related("blocker", "gx:demo/04", "needs-repair"),
      related("blocker", "gx:demo/05", "done"),
      related("blocker", "gx:demo/06", "error"),
    ],
  });
  await screen.findByText("Title of demo/03");
  const toneOf = (title: string) => screen.getByText(title).closest("[data-tone]")!.getAttribute("data-tone");
  expect(toneOf("Title of demo/03")).toBe("paused");
  expect(toneOf("Title of demo/04")).toBe("deadlocked");
  expect(toneOf("Title of demo/05")).toBe("done");
  expect(toneOf("Title of demo/06")).toBe("deadlocked");
});

test("Description renders the body as markdown without the Paused section", async () => {
  renderTicket({
    ...plain,
    body: "Intro **bold**.\n\n## Paused\n\nwaiting on a person\n\n## Comments\n\nA comment.",
  });
  await screen.findByText("bold");
  const section = screen.getByRole("region", { name: "Description" });
  expect(section.querySelector("strong")?.textContent).toBe("bold");
  expect(section.textContent).toContain("A comment.");
  expect(section.textContent).not.toContain("waiting on a person");
  expect(section.textContent).not.toContain("Paused");
});

test("a closed map ticket shows Resolution then Question, with no repeated headings and no stat line", async () => {
  renderTicket({
    ...plain,
    map_epic: true,
    status: "done",
    stats: undefined,
    body: "## Question\n\nWhich store?\n\n## Resolution\n\nUse Redis.",
  });
  await screen.findByText("Which store?");
  expect(headings()).toEqual(["Resolution", "Question"]);
  expect(screen.getByRole("region", { name: "Resolution" }).textContent).toBe("ResolutionUse Redis.");
  expect(screen.getByRole("region", { name: "Question" }).textContent).toBe("QuestionWhich store?");
});

test("an open map ticket starts with Question", async () => {
  renderTicket({ ...plain, map_epic: true, body: "## Question\n\nWhich store?" });
  await screen.findByText("Which store?");
  expect(headings()).toEqual(["Question"]);
});

test("empty sections are hidden and a ticket with nothing says so", async () => {
  renderTicket(plain);
  await screen.findByText("Nothing here yet");
  expect(screen.queryAllByRole("heading", { level: 2 })).toEqual([]);
});

test("a closed map ticket with no resolution text hides Resolution", async () => {
  renderTicket({ ...plain, map_epic: true, status: "done", body: "## Question\n\nWhich store?\n\n## Resolution\n" });
  await screen.findByText("Which store?");
  expect(headings()).toEqual(["Question"]);
});

const run = (start: string, over: Partial<DetailRun> = {}): DetailRun => ({
  start,
  duration: 90,
  outcome: "merged",
  start_kind: "new-session",
  ...over,
});

test("Runs lists each run in order with start, duration, outcome and session word", async () => {
  const runs = [
    run("2026-01-01T09:00:00Z", { outcome: "paused", pause_label: "Needs an answer", duration: 600 }),
    run("2026-01-01T09:30:00Z", { start_kind: "same-session" }),
    run("2026-01-01T10:00:00Z", { outcome: "agent-lost", duration: undefined }),
    run("2026-01-01T10:30:00Z", { outcome: "agent-gone-after-restart", duration: undefined }),
  ];
  renderTicket({ ...plain, runs });
  const section = await screen.findByRole("region", { name: "Runs" });
  const lines = screen.getAllByRole("listitem").map((l) => l.textContent);
  expect(lines).toEqual([
    `${formatStart(runs[0].start)} · 10m · paused · Needs an answer · new session`,
    `${formatStart(runs[1].start)} · 1m · merged · same session`,
    `${formatStart(runs[2].start)} · agent lost · new session`,
    `${formatStart(runs[3].start)} · agent gone after server restart, no work to keep, back in the queue · new session`,
  ]);
  expect(section.textContent).not.toMatch(/resumed|claim|reopen/i);
});

test("Runs is hidden for a ticket with no runs and for a map ticket", async () => {
  renderTicket({ ...plain, body: "Some text." });
  await screen.findByText("Some text.");
  expect(screen.queryByRole("region", { name: "Runs" })).toBeNull();
  cleanup();
  renderTicket({ ...plain, map_epic: true, body: "## Question\n\nWhich store?", runs: [run("2026-01-01T09:00:00Z")] });
  await screen.findByText("Which store?");
  expect(screen.queryByRole("region", { name: "Runs" })).toBeNull();
});
