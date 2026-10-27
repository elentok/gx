import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import type { ChecksStatus, ProjectChecks } from "../checks/status";
import { ConnectionContext, type Connection, type ConnectionState } from "../connection";
import type { EpicList, EpicListRow } from "../generated/server";
import { appRoutes } from "../routes";
import { renderThemed } from "../ui/render";

beforeEach(() => localStorage.clear());
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
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

const epic = (project: string): EpicListRow => ({
  address: `${project}:work`,
  project,
  name: "work",
  kind: "loop",
  state: "ready",
  done: 0,
  total: 3,
  queued: false,
  queueable: true,
  blocked_by_epics: [],
  card_kind: "implementation",
  section: "ready-to-queue",
  addresses: [`${project}:work`],
});

const project = (name: string, over: Partial<ProjectChecks> = {}, recommended = 0): ProjectChecks => ({
  project: name,
  missing: true,
  trunk: "main",
  save_targets: ["repo", "project.json"],
  error: "",
  kinds: ["format", "lint", "typecheck", "test"].map((kind, i) => ({
    kind,
    state: "gx-default",
    default: "",
    candidates: [],
    ...(i < recommended ? { recommended: {} } : {}),
  })),
  ...over,
});

function renderPage(checks: () => Promise<ChecksStatus>, onSetUp = vi.fn()) {
  const list: EpicList = { epics: [epic("demo"), epic("auto")], follow_ups: [] };
  const router = createMemoryRouter(appRoutes({ epicList: async () => list, checksStatus: checks, onSetUpChecks: onSetUp }), {
    initialEntries: ["/epics"],
  });
  renderThemed(
    <ConnectionContext.Provider value={conn}>
      <RouterProvider router={router} />
    </ConnectionContext.Provider>,
  );
  return onSetUp;
}

const both = async (): Promise<ChecksStatus> => ({ projects: [project("demo", {}, 1), project("auto")] });
const banner = () => screen.findByRole("region", { name: "Missing check config" });
const noBanner = async () => {
  await screen.findByRole("combobox", { name: "Project" });
  await waitFor(() => expect(screen.queryByRole("region", { name: "Missing check config" })).toBeNull());
};

test("one filtered project gets its own sentence, the suggestion count and two buttons", async () => {
  localStorage.setItem("gx.epics.project", "demo");
  const onSetUp = renderPage(both);
  const b = await banner();
  expect(b.textContent).toContain("demo has no check config.");
  expect(b.textContent).toContain("Agents run gx's built-in checks until you set it up.");
  expect(b.textContent).toContain("gx found 1 command to suggest.");
  expect(within(b).queryByText(/auto/)).toBeNull();
  expect(within(b).getByRole("button", { name: "Not now" })).toBeTruthy();
  fireEvent.click(within(b).getByRole("button", { name: "Set up checks" }));
  expect(onSetUp).toHaveBeenCalledWith("demo");
});

test("All projects shows one banner with a Set up checks button per missing project", async () => {
  const onSetUp = renderPage(both);
  const b = await banner();
  expect(b.textContent).toContain("2 projects have no check config.");
  expect(within(b).getAllByRole("listitem").map((li) => li.textContent?.replace("Set up checks", ""))).toEqual(["demo", "auto"]);
  fireEvent.click(within(b).getByRole("button", { name: "Set up checks for auto" }));
  expect(onSetUp).toHaveBeenCalledWith("auto");
});

test("no banner when nothing is missing", async () => {
  renderPage(async () => ({ projects: [project("demo", { missing: false })] }));
  await noBanner();
});

test("no banner when the filtered project has config", async () => {
  localStorage.setItem("gx.epics.project", "auto");
  renderPage(async () => ({ projects: [project("demo"), project("auto", { missing: false })] }));
  await noBanner();
});

test("a failed read shows no banner", async () => {
  renderPage(async () => {
    throw new Error("boom");
  });
  await noBanner();
});

test("Not now hides the banner and calls no write", async () => {
  const fetchMock = vi.fn();
  vi.stubGlobal("fetch", fetchMock);
  renderPage(both);
  const b = await banner();
  fireEvent.click(within(b).getByRole("button", { name: "Not now" }));
  expect(screen.queryByRole("region", { name: "Missing check config" })).toBeNull();
  expect(fetchMock).not.toHaveBeenCalled();
  expect(localStorage.length).toBe(0);
});
