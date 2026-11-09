import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { createMemoryRouter, RouterProvider, useLocation } from "react-router";
import { afterEach, expect, test, vi } from "vitest";
import { session } from "../agents/agentsTestKit";
import { ConnectionContext, type Connection, type ConnectionState } from "../connection";
import {
  CardKindDesign,
  CardKindImplementation,
  SectionFinished,
  SectionImplementing,
  SectionNeedsYou,
  SectionOnHold,
  SectionReadyToDesign,
  type EpicListRow,
  type SessionPlacement,
} from "../generated/server";
import { Modal } from "../ui/modal/Modal";
import { renderThemed } from "../ui/render";
import { JumpPalette } from "./JumpPalette";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const row = (name: string, over: Partial<EpicListRow>): EpicListRow =>
  ({
    address: `p:${name}`,
    project: "p",
    name,
    kind: "loop",
    state: "idle",
    done: 1,
    total: 4,
    queued: false,
    queueable: false,
    blocked_by_epics: [],
    workspace: "Default",
    card_kind: CardKindImplementation,
    section: SectionImplementing,
    status_words: "Idle",
    ...over,
  }) as EpicListRow;

const rows = [
  row("blog", {}),
  row("shop", { card_kind: CardKindDesign }),
  row("away", { workspace: "Work" }),
  row("away-waiting", { workspace: "Work", section: SectionNeedsYou, state_pill: "Question" }),
  row("frozen", { section: SectionOnHold }),
  row("frozen-away", { workspace: "Work", section: SectionOnHold }),
  row("unstarted", { card_kind: CardKindDesign, section: SectionReadyToDesign }),
  row("shipped", { section: SectionFinished }),
];
const placements: SessionPlacement[] = [
  { session: "chat1", kind: "chat", workspace: "Default" },
  { session: "chat2", kind: "chat", workspace: "Work" },
  { session: "a1", kind: "epic", epic: "p:blog" },
  { session: "a2", kind: "other" },
  { session: "a3", kind: "other" },
];
const sessions = [
  session("chat1", "p", { name: "my-chat" }),
  session("chat2", "p", { name: "far-chat" }),
  session("a1", "p", { name: "builder" }),
  session("a2", "p", { name: "waiting-oneoff", actionable: { reason: "asks" } }),
  session("a3", "p", { name: "quiet-oneoff" }),
];

function Where() {
  return <div data-testid="where">{useLocation().pathname}</div>;
}

function setup({ path = "/agents", narrow = false } = {}) {
  vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({ epics: rows, follow_ups: [] }), { status: 200 })));
  if (narrow) vi.stubGlobal("matchMedia", () => ({ matches: false, addEventListener() {}, removeEventListener() {} }));
  const state = { snapshot: { sessions, placements, workspaces: [{ name: "Default", default: true, members: 3 }, { name: "Work", members: 2 }] }, connected: true, loadedOnce: true, tick: 0 } as unknown as ConnectionState;
  const conn = { getState: () => state, subscribe: () => () => {}, onEvent: () => () => {} } as unknown as Connection;
  const router = createMemoryRouter(
    [
      {
        path: "*",
        element: (
          <>
            <input aria-label="field" />
            <JumpPalette active="Default" />
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

const open = async () => {
  await waitFor(() => expect(fetch).toHaveBeenCalled());
  fireEvent.keyDown(window, { key: "k", ctrlKey: true });
  return screen.findByRole("dialog", { name: "Jump to" });
};
const names = () => screen.queryAllByRole("option").map((o) => within(o).getAllByText(/./)[0].textContent);
const option = (name: string) => screen.getAllByRole("option").find((o) => o.textContent?.startsWith(name))!;
const type = (value: string) => fireEvent.change(screen.getByRole("combobox"), { target: { value } });

test("Ctrl+K and ⌘K toggle the menu on any page, Escape closes it", async () => {
  setup({ path: "/epics" });
  await open();
  fireEvent.keyDown(window, { key: "Escape" });
  expect(screen.queryByRole("dialog")).toBeNull();
  fireEvent.keyDown(window, { key: "k", metaKey: true });
  expect(await screen.findByRole("dialog")).toBeTruthy();
  fireEvent.keyDown(window, { key: "k", metaKey: true });
  expect(screen.queryByRole("dialog")).toBeNull();
});

test("Ctrl+K opens from inside a text field", async () => {
  setup();
  await waitFor(() => expect(fetch).toHaveBeenCalled());
  const field = screen.getByLabelText("field");
  field.focus();
  fireEvent.keyDown(field, { key: "k", ctrlKey: true, bubbles: true });
  expect(await screen.findByRole("dialog", { name: "Jump to" })).toBeTruthy();
});

test("Ctrl+K does nothing under a Modal, and Escape closes only the Modal", async () => {
  setup();
  await waitFor(() => expect(fetch).toHaveBeenCalled());
  const onClose = vi.fn();
  const modal = renderThemed(
    <Modal label="Over" onClose={onClose}>
      body
    </Modal>,
  );
  fireEvent.keyDown(window, { key: "k", ctrlKey: true });
  expect(screen.queryByRole("dialog", { name: "Jump to" })).toBeNull();
  modal.unmount();
  await open();
  renderThemed(
    <Modal label="Over" onClose={onClose}>
      body
    </Modal>,
  );
  fireEvent.keyDown(window, { key: "Escape" });
  expect(onClose).toHaveBeenCalledTimes(1);
  expect(screen.getByRole("dialog", { name: "Jump to" })).toBeTruthy();
});

test("the empty menu is the active workspace, then Needs you elsewhere", async () => {
  setup();
  await open();
  await waitFor(() => expect(names()).toContain("blog"));
  expect(screen.getAllByRole("group").map((g) => g.getAttribute("aria-label"))).toEqual(["Designs", "Epics", "Chats", "Needs you elsewhere"]);
  expect(names()).toEqual(["shop", "blog", "my-chat", "away-waiting", "waiting-oneoff"]);
  expect(within(option("away-waiting")).getByText("Work")).toBeTruthy();
  expect(within(option("waiting-oneoff")).getByText("Needs you")).toBeTruthy();
});

test("typing finds held, Ready to design, Finished items, agents and other workspaces", async () => {
  setup();
  await open();
  await waitFor(() => expect(names()).toContain("blog"));
  type("frozen");
  expect(names()).toEqual(["frozen", "frozen-away"]);
  expect(within(option("frozen")).getByText("On hold")).toBeTruthy();
  type("unstart");
  expect(names()).toEqual(["unstarted"]);
  type("shipped");
  expect(names()).toEqual(["shipped"]);
  type("builder");
  expect(names()).toEqual(["builder"]);
  expect(within(option("builder")).getByText("blog")).toBeTruthy();
  type("away");
  expect(names()).toEqual(["away-waiting", "away", "frozen-away"]);
  expect(within(option("away")).getByText("Work")).toBeTruthy();
  type("far-chat");
  expect(names()).toEqual(["far-chat"]);
  type("quiet");
  expect(names()).toEqual(["quiet-oneoff"]);
});

test("a fuzzy query that is not a substring finds an epic", async () => {
  setup();
  await open();
  await waitFor(() => expect(names()).toContain("blog"));
  type("bg");
  expect(names()[0]).toBe("blog");
});

test("Enter opens the cursor row", async () => {
  setup();
  await open();
  await waitFor(() => expect(names()).toContain("blog"));
  fireEvent.keyDown(screen.getByRole("combobox"), { key: "Enter" });
  await waitFor(() => expect(screen.getByTestId("where").textContent).toBe("/d/p:shop"));
  expect(screen.queryByRole("dialog")).toBeNull();
});

test("a click outside closes the popover, and a phone gets a Close button instead", async () => {
  setup();
  await open();
  fireEvent.mouseDown(document.body);
  expect(screen.queryByRole("dialog")).toBeNull();
});

test("a mouse-down inside a menu portal outside the popover does not close it", async () => {
  setup();
  await open();
  const menu = document.body.appendChild(document.createElement("div"));
  menu.setAttribute("role", "menu");
  fireEvent.mouseDown(menu);
  expect(screen.getByRole("dialog", { name: "Jump to" })).toBeTruthy();
  menu.remove();
});

test("on a phone the menu is a full screen with a Close button", async () => {
  setup({ narrow: true });
  await open();
  fireEvent.mouseDown(document.body);
  expect(screen.getByRole("dialog")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: "Close" }));
  expect(screen.queryByRole("dialog")).toBeNull();
});

const menuOf = (name: string) => within(option(name)).queryByRole("button", { name: "More actions" });
const posts = () => vi.mocked(fetch).mock.calls.filter(([, init]) => init?.method === "POST");

test("Design, epic and chat rows have a row menu; agent and One-off rows do not", async () => {
  setup();
  await open();
  await waitFor(() => expect(names()).toContain("blog"));
  expect(menuOf("shop")).toBeTruthy();
  expect(menuOf("blog")).toBeTruthy();
  expect(menuOf("my-chat")).toBeTruthy();
  expect(menuOf("waiting-oneoff")).toBeNull();
  type("builder");
  expect(menuOf("builder")).toBeNull();
  type("quiet");
  expect(menuOf("quiet-oneoff")).toBeNull();
});

test("Move from a row posts and does not open the row", async () => {
  setup();
  await open();
  await waitFor(() => expect(names()).toContain("blog"));
  fireEvent.click(menuOf("blog")!);
  fireEvent.click(await screen.findByRole("menuitem", { name: "Move to workspace" }));
  fireEvent.click(await screen.findByRole("menuitem", { name: /^Work/ }));
  await waitFor(() => expect(posts()).toHaveLength(1));
  expect(String(posts()[0][0])).toContain("/v1/workspaces/move");
  expect(JSON.parse(String(posts()[0][1]?.body))).toEqual({ name: "Work", key: "p:blog" });
  expect(screen.getByTestId("where").textContent).toBe("/agents");
});

test("Hold from a row posts, and a held row offers Take off hold", async () => {
  setup();
  await open();
  await waitFor(() => expect(names()).toContain("blog"));
  fireEvent.click(menuOf("blog")!);
  fireEvent.click(await screen.findByRole("menuitem", { name: "Hold" }));
  await waitFor(() => expect(posts()).toHaveLength(1));
  expect(String(posts()[0][0])).toContain("/v1/workspaces/hold");
  type("frozen");
  fireEvent.click(menuOf("frozen")!);
  expect(await screen.findByRole("menuitem", { name: "Take off hold" })).toBeTruthy();
});
