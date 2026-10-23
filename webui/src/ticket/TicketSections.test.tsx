import { cleanup, screen } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { afterEach, expect, test } from "vitest";
import canned from "../canned-detail.json";
import { ConnectionContext, type Connection, type ConnectionState } from "../connection";
import type { RelatedTicket, TicketDetail } from "../generated/server";
import { appRoutes } from "../routes";
import { renderThemed } from "../ui/render";
import type { FetchDetail } from "./detail";

afterEach(cleanup);

const plain: TicketDetail = {
  ...(canned as TicketDetail),
  epic: "gx:demo",
  map_epic: false,
  status: "open",
  body: "",
  related: [],
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
  const router = createMemoryRouter(appRoutes(fetchDetail), { initialEntries: ["/t/gx:demo/12"] });
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
