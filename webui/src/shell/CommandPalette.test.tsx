import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { createMemoryRouter, RouterProvider, useLocation } from "react-router";
import { afterEach, expect, test, vi } from "vitest";
import { session } from "../agents/agentsTestKit";
import { ConnectionContext, type Connection, type ConnectionState } from "../connection";
import { CardActionAnswer, CardKindFollowUps, CardKindImplementation, CardKindOneOff, SectionNeedsYou, type EpicListRow } from "../generated/server";
import { Modal } from "../ui/modal/Modal";
import { renderThemed } from "../ui/render";
import { CommandPalette } from "./CommandPalette";
import { JumpPalette } from "./JumpPalette";
import { NewItemHost } from "./NewItemHost";
import { closeNewItem } from "./newItemOpen";
import { ShellContext, type Shell } from "./ShellContext";

afterEach(() => {
  closeNewItem();
  localStorage.clear();
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

function Where() {
  return <div data-testid="where">{useLocation().pathname}</div>;
}

const projects = ["alpha", "beta", "gamma"];

const card = (name: string, day: number, over: Partial<EpicListRow> = {}): EpicListRow =>
  ({
    address: `p:${name}`,
    project: "p",
    name,
    workspace: "Default",
    card_kind: CardKindImplementation,
    section: SectionNeedsYou,
    waiting_since: `2026-01-0${day}T00:00:00Z`,
    ...over,
  }) as EpicListRow;

const defaultEpics = [{ address: "beta:feat", project: "beta" }] as EpicListRow[];

function setup({ narrow = false, mac = false, path = "/stats", afk = false, epics = defaultEpics, follow_ups = [] as unknown[] } = {}) {
  vi.spyOn(navigator, "platform", "get").mockReturnValue(mac ? "MacIntel" : "Linux x86_64");
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string) => {
      const u = String(url);
      const body = u.includes("worktrees")
        ? projects.map((project) => ({ project, root: `/${project}`, worktrees: [], tiers: {} }))
        : u.includes("drafts")
          ? []
          : { epics, follow_ups, projects };
      return new Response(JSON.stringify(body), { status: 200 });
    }),
  );
  if (narrow) vi.stubGlobal("matchMedia", () => ({ matches: false, addEventListener() {}, removeEventListener() {} }));
  const state = { snapshot: { sessions: [session("s1", "gamma")], placements: [], workspaces: [], afk }, connected: true, loadedOnce: true, tick: 0 } as unknown as ConnectionState;
  const conn = { getState: () => state, subscribe: () => () => {}, onEvent: () => () => {} } as unknown as Connection;
  const router = createMemoryRouter(
    [
      {
        path: "*",
        element: (
          <>
            <input aria-label="field" />
            <JumpPalette active="Default" />
            <ShellContext.Provider value={{ workspace: "Default", closeSliding() {} } as Shell}>
              <CommandPalette />
            </ShellContext.Provider>
            <NewItemHost />
            <Where />
          </>
        ),
      },
    ],
    { initialEntries: [path] },
  );
  renderThemed(
    <ConnectionContext.Provider value={conn}>
      <RouterProvider router={router} />
    </ConnectionContext.Provider>,
  );
}

const palette = () => screen.queryByRole("dialog", { name: "Commands" });
const jump = () => screen.queryByRole("dialog", { name: "Jump to" });
const names = () => screen.queryAllByRole("option").map((o) => within(o).getAllByText(/./)[0].textContent);
const where = () => screen.getByTestId("where").textContent;
const press = (init: KeyboardEventInit, target: Element | Window = window) => fireEvent.keyDown(target, init);

test("Ctrl+E opens off a Mac, Cmd+E does nothing; the reverse on a Mac", () => {
  setup();
  press({ key: "e", metaKey: true });
  expect(palette()).toBeNull();
  press({ key: "e", ctrlKey: true });
  expect(palette()).toBeTruthy();
  cleanup();
  setup({ mac: true });
  press({ key: "e", ctrlKey: true });
  expect(palette()).toBeNull();
  press({ key: "e", metaKey: true });
  expect(palette()).toBeTruthy();
});

test("opens from a focused text box with the filter focused, and a second press closes", () => {
  setup();
  const field = screen.getByLabelText("field");
  field.focus();
  press({ key: "e", ctrlKey: true, bubbles: true }, field);
  expect(palette()).toBeTruthy();
  expect(document.activeElement).toBe(screen.getByRole("combobox"));
  press({ key: "e", ctrlKey: true });
  expect(palette()).toBeNull();
});

test("does nothing under a Modal", () => {
  setup();
  renderThemed(
    <Modal label="Over" onClose={() => {}}>
      body
    </Modal>,
  );
  press({ key: "e", ctrlKey: true });
  expect(palette()).toBeNull();
});

test("never stacks with Cmd+K, in either order", async () => {
  setup();
  press({ key: "e", ctrlKey: true });
  press({ key: "k", ctrlKey: true });
  expect(palette()).toBeTruthy();
  expect(jump()).toBeNull();
  press({ key: "e", ctrlKey: true });
  press({ key: "k", ctrlKey: true });
  expect(await screen.findByRole("dialog", { name: "Jump to" })).toBeTruthy();
  press({ key: "e", ctrlKey: true });
  expect(palette()).toBeNull();
});

test("Escape and a click outside close without running anything", () => {
  setup();
  press({ key: "e", ctrlKey: true });
  press({ key: "Escape" });
  expect(palette()).toBeNull();
  press({ key: "e", ctrlKey: true });
  fireEvent.mouseDown(document.body);
  expect(palette()).toBeNull();
  expect(where()).toBe("/stats");
});

test("rows are in fixed order, hold no epic or session rows, and filter fuzzily", () => {
  setup();
  press({ key: "e", ctrlKey: true });
  expect(names()).toEqual(["New design", "New chat", "Next question", "Go to Epics", "Go to Agents", "Go to Stats", "Go to Settings", "Toggle AFK"]);
  fireEvent.change(screen.getByRole("combobox"), { target: { value: "go ep" } });
  expect(names()[0]).toBe("Go to Epics");
  fireEvent.change(screen.getByRole("combobox"), { target: { value: "newdes" } });
  expect(names()[0]).toBe("New design");
  fireEvent.change(screen.getByRole("combobox"), { target: { value: "new chat" } });
  expect(names()[0]).toBe("New chat");
});

const projectSelect = async () => ((await screen.findByRole("combobox", { name: "Project" })) as HTMLSelectElement).value;

test.each([
  ["ndes", "/stats", "alpha"],
  ["ndes", "/e/beta:feat", "beta"],
  ["nc", "/stats", "alpha"],
  ["nc", "/agents/s1", "gamma"],
])("typing %s then Enter on %s opens the form on %s over the same page", async (filter, path, project) => {
  setup({ path });
  press({ key: "e", ctrlKey: true });
  const box = screen.getByRole("combobox");
  fireEvent.change(box, { target: { value: filter } });
  fireEvent.keyDown(box, { key: "Enter" });
  expect(palette()).toBeNull();
  await waitFor(async () => expect(await projectSelect()).toBe(project));
  expect(where()).toBe(path);
  closeNewItem();
  await waitFor(() => expect(screen.queryByRole("combobox", { name: "Project" })).toBeNull());
  expect(where()).toBe(path);
});

test("Down and Enter run the chosen command", () => {
  setup();
  press({ key: "e", ctrlKey: true });
  const box = screen.getByRole("combobox");
  for (let i = 0; i < 4; i++) fireEvent.keyDown(box, { key: "ArrowDown" });
  fireEvent.keyDown(box, { key: "Enter" });
  expect(where()).toBe("/agents");
  expect(palette()).toBeNull();
});

test.each([
  ["Go to Epics", "/epics"],
  ["Go to Agents", "/agents"],
  ["Go to Stats", "/stats"],
  ["Go to Settings", "/settings"],
])("clicking %s navigates", (title, path) => {
  setup();
  press({ key: "e", ctrlKey: true });
  fireEvent.click(screen.getAllByRole("option").find((o) => o.textContent?.startsWith(title))!);
  expect(where()).toBe(path);
});

test.each([
  [true, "AFK is on", false],
  [false, "AFK is off", true],
])("Toggle AFK with afk=%s says %s and posts on=%s once, then closes", async (afk, subtitle, on) => {
  setup({ afk });
  press({ key: "e", ctrlKey: true });
  const row = screen.getAllByRole("option").find((o) => o.textContent?.startsWith("Toggle AFK"))!;
  expect(row.textContent).toContain(subtitle);
  fireEvent.click(row);
  expect(palette()).toBeNull();
  const posts = () => vi.mocked(fetch).mock.calls.filter(([u]) => String(u).includes("/v1/afk"));
  await waitFor(() => expect(posts()).toHaveLength(1));
  expect(JSON.parse(String(posts()[0][1]?.body))).toEqual({ on });
});

const nextRow = () => screen.getAllByRole("option").find((o) => o.textContent?.startsWith("Next question"))!;

const waiting = [
  card("old", 1),
  card("new", 3, { actions: [{ kind: CardActionAnswer, link: "#/answer/new" }] as EpicListRow["actions"] }),
  card("mid", 2),
  card("far", 9, { workspace: "Work" }),
];

test("Next question: from a plain page Enter opens the newest card's Answer link", async () => {
  setup({ epics: waiting });
  await waitFor(() => expect(vi.mocked(fetch)).toHaveBeenCalled());
  press({ key: "e", ctrlKey: true });
  await waitFor(() => expect(nextRow().textContent).toContain("new · 3 waiting"));
  const box = screen.getByRole("combobox");
  fireEvent.change(box, { target: { value: "needs you" } });
  expect(names()).toEqual(["Next question"]);
  fireEvent.keyDown(box, { key: "Enter" });
  expect(where()).toBe("/answer/new");
  expect(palette()).toBeNull();
});

test("Next question: from a card's page it goes to the next one, a plain card to its page", async () => {
  setup({ epics: waiting, path: "/e/p:mid" });
  press({ key: "e", ctrlKey: true });
  await waitFor(() => expect(nextRow().textContent).toContain("old · 3 waiting"));
  fireEvent.click(nextRow());
  expect(where()).toBe("/e/p:old");
});

test.each([
  ["a One-off card", card("chore", 2, { card_kind: CardKindOneOff, address: "p:t1" }), "chore · 1 waiting"],
  ["a Follow-ups card", card("", 2, { card_kind: CardKindFollowUps, name: "", waiting_since: undefined }), "p · 1 waiting"],
])("Next question subtitle names %s", async (_n, row, subtitle) => {
  setup({ epics: [row], follow_ups: [{ project: "p", workspaces: ["Default"] }] });
  press({ key: "e", ctrlKey: true });
  await waitFor(() => expect(nextRow().textContent).toContain(subtitle));
});

test("Next question with nothing waiting stays in place and Enter leaves the route", async () => {
  setup({ epics: [card("away", 1, { workspace: "Work" })] });
  press({ key: "e", ctrlKey: true });
  await waitFor(() => expect(vi.mocked(fetch)).toHaveBeenCalled());
  expect(nextRow().textContent).toContain("Nothing needs you");
  const box = screen.getByRole("combobox");
  fireEvent.change(box, { target: { value: "next" } });
  fireEvent.keyDown(box, { key: "Enter" });
  expect(where()).toBe("/stats");
});

test("full screen at a phone width", () => {
  setup({ narrow: true });
  press({ key: "e", ctrlKey: true });
  expect(palette()).toBeTruthy();
  fireEvent.mouseDown(document.body);
  expect(palette()).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: /close/i }));
  expect(palette()).toBeNull();
});
