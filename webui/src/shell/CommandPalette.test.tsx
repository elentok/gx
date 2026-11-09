import { cleanup, fireEvent, screen, within } from "@testing-library/react";
import { createMemoryRouter, RouterProvider, useLocation } from "react-router";
import { afterEach, expect, test, vi } from "vitest";
import { ConnectionContext, type Connection, type ConnectionState } from "../connection";
import { Modal } from "../ui/modal/Modal";
import { renderThemed } from "../ui/render";
import { CommandPalette } from "./CommandPalette";
import { JumpPalette } from "./JumpPalette";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

function Where() {
  return <div data-testid="where">{useLocation().pathname}</div>;
}

function setup({ narrow = false, mac = false } = {}) {
  vi.spyOn(navigator, "platform", "get").mockReturnValue(mac ? "MacIntel" : "Linux x86_64");
  vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({ epics: [], follow_ups: [] }), { status: 200 })));
  if (narrow) vi.stubGlobal("matchMedia", () => ({ matches: false, addEventListener() {}, removeEventListener() {} }));
  const state = { snapshot: { sessions: [], placements: [], workspaces: [] }, connected: true, loadedOnce: true, tick: 0 } as unknown as ConnectionState;
  const conn = { getState: () => state, subscribe: () => () => {}, onEvent: () => () => {} } as unknown as Connection;
  const router = createMemoryRouter(
    [
      {
        path: "*",
        element: (
          <>
            <input aria-label="field" />
            <JumpPalette active="Default" />
            <CommandPalette />
            <Where />
          </>
        ),
      },
    ],
    { initialEntries: ["/stats"] },
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
  expect(names()).toEqual(["Go to Epics", "Go to Agents", "Go to Stats", "Go to Settings"]);
  fireEvent.change(screen.getByRole("combobox"), { target: { value: "go ep" } });
  expect(names()[0]).toBe("Go to Epics");
});

test("Down and Enter run the chosen command", () => {
  setup();
  press({ key: "e", ctrlKey: true });
  const box = screen.getByRole("combobox");
  fireEvent.keyDown(box, { key: "ArrowDown" });
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

test("full screen at a phone width", () => {
  setup({ narrow: true });
  press({ key: "e", ctrlKey: true });
  expect(palette()).toBeTruthy();
  fireEvent.mouseDown(document.body);
  expect(palette()).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: /close/i }));
  expect(palette()).toBeNull();
});
