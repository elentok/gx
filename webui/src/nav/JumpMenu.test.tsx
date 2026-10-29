import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { createMemoryRouter, RouterProvider, useLocation } from "react-router";
import { afterEach, expect, test, vi } from "vitest";
import { ConnectionContext, type Connection, type ConnectionState } from "../connection";
import type { EpicListRow } from "../generated/server";
import { renderThemed } from "../ui/render";
import { JumpMenu, jumpHref, RowList, type ListRowData, type ListSection } from "./JumpMenu";

afterEach(cleanup);

const row = (name: string, over: Partial<EpicListRow> = {}): EpicListRow => ({
  address: `demo:${name}`,
  project: "demo",
  name,
  kind: "loop",
  state: "running",
  done: 1,
  total: 4,
  queued: false,
  queueable: false,
  blocked_by_epics: [],
  addresses: [`demo:${name}`],
  card_kind: "implementation",
  section: "implementing",
  status_words: "Implementing...",
  ring_progress: 25,
  ...over,
});

const ROWS = [
  row("question", { section: "needs-you", state_pill: "Question", status_words: "Waiting" }),
  row("other-question", { section: "needs-you", state_pill: "Question" }),
  row("current", { section: "implementing" }),
  row("loop-epic", { section: "implementing" }),
  row("blog", { card_kind: "design", section: "designing" }),
  row("fix-it", { card_kind: "one-off", section: "implementing", address: "demo:one/01", addresses: ["demo:one/01"] }),
  row("unstarted", { card_kind: "design", section: "ready-to-design", status_words: "Not started" }),
  row("old-epic", { section: "finished", state: "finished", status_words: "Merged" }),
];

function Where() {
  return <div data-testid="where">{useLocation().pathname}</div>;
}

function setup(epics: EpicListRow[] = ROWS) {
  const router = createMemoryRouter([
    {
      path: "*",
      element: (
        <>
          <JumpMenu name="current" current="demo:current" epics={epics} />
          <Where />
        </>
      ),
    },
  ]);
  const state = { snapshot: { sessions: [] }, connected: true, loadedOnce: true, tick: 0 } as unknown as ConnectionState;
  const conn = { getState: () => state, subscribe: () => () => {}, onEvent: () => () => {} } as unknown as Connection;
  renderThemed(
    <ConnectionContext.Provider value={conn}>
      <RouterProvider router={router} />
    </ConnectionContext.Provider>,
  );
}

const titleButton = () => screen.findByRole("button", { name: /^current/ });
const names = () => screen.queryAllByRole("option").map((o) => within(o).getAllByText(/./)[0].textContent);

test("a click opens the menu with the filter focused", async () => {
  setup();
  fireEvent.click(await titleButton());
  expect(document.activeElement).toBe(screen.getByRole("combobox", { name: "Jump to epic" }));
});

test("Ctrl+K and ⌘K open the menu", async () => {
  setup();
  await titleButton();
  fireEvent.keyDown(window, { key: "k", ctrlKey: true });
  expect(await screen.findByRole("dialog", { name: "Jump to epic" })).toBeTruthy();
  fireEvent.keyDown(window, { key: "Escape" });
  expect(screen.queryByRole("dialog")).toBeNull();
  fireEvent.keyDown(window, { key: "k", metaKey: true });
  expect(await screen.findByRole("dialog")).toBeTruthy();
});

test("the title shows no count", async () => {
  setup();
  expect((await titleButton()).textContent).toBe("current▾⌘K");
  expect(screen.queryByTestId("waiting-count")).toBeNull();
});

test("Ready to design and Finished show only while typing, and typing searches finished epics", async () => {
  setup();
  fireEvent.click(await titleButton());
  expect(screen.queryByText("unstarted")).toBeNull();
  expect(screen.queryByText("old-epic")).toBeNull();
  const input = screen.getByRole("combobox");
  fireEvent.change(input, { target: { value: "old" } });
  expect(screen.getByText("old-epic")).toBeTruthy();
  fireEvent.change(input, { target: { value: "unst" } });
  expect(screen.getByText("unstarted")).toBeTruthy();
});

test("the current epic is marked, and a Needs-you row reads its pill", async () => {
  setup();
  fireEvent.click(await titleButton());
  const options = screen.getAllByRole("option");
  expect(options.find((o) => o.hasAttribute("aria-current"))?.textContent).toContain("current");
  expect(within(options[0]).getByText("demo · Question")).toBeTruthy();
});

test("Down and Enter open the next row", async () => {
  setup();
  fireEvent.click(await titleButton());
  const input = screen.getByRole("combobox");
  expect(names()[0]).toBe("question");
  fireEvent.keyDown(input, { key: "ArrowDown" });
  fireEvent.keyDown(input, { key: "Enter" });
  await waitFor(() => expect(screen.getByTestId("where").textContent).toBe("/e/demo:other-question"));
  expect(screen.queryByRole("dialog")).toBeNull();
});

test("a Design opens on its Design page, a loop epic on its epic page, a one-off on its ticket", async () => {
  setup();
  const pick = async (name: string) => {
    fireEvent.click(await titleButton());
    fireEvent.click(screen.getByText(name));
  };
  await pick("blog");
  await waitFor(() => expect(screen.getByTestId("where").textContent).toBe("/d/demo:blog"));
  await pick("loop-epic");
  await waitFor(() => expect(screen.getByTestId("where").textContent).toBe("/e/demo:loop-epic"));
  await pick("fix-it");
  await waitFor(() => expect(screen.getByTestId("where").textContent).toBe("/t/demo:one/01"));
});

test("a joined card with a Design question opens the Design page", () => {
  const joined = row("blog", {
    address: "demo:blog",
    addresses: ["demo:blog", "demo:blog-impl"],
    actions: [{ kind: "answer", label: "Answer", target: "demo:blog", link: "#/d/demo:blog" }],
  });
  expect(jumpHref(joined)).toBe("#/d/demo:blog");
  expect(jumpHref(row("loop-epic"))).toBe("#/e/demo:loop-epic");
});

test("Esc and a click outside close the menu", async () => {
  setup();
  fireEvent.click(await titleButton());
  fireEvent.keyDown(screen.getByRole("combobox"), { key: "Escape" });
  expect(screen.queryByRole("dialog")).toBeNull();
  fireEvent.click(await titleButton());
  fireEvent.mouseDown(document.body);
  expect(screen.queryByRole("dialog")).toBeNull();
});

const plain = (key: string, over: Partial<ListRowData> = {}): ListRowData => ({
  key,
  title: key,
  subtitle: `sub ${key}`,
  href: `#/x/${key}`,
  searchText: key,
  ...over,
});

function setupList(props: Partial<React.ComponentProps<typeof RowList>> = {}) {
  const sections: ListSection[] = [
    { key: "b", title: "Second", rows: [plain("beta", { current: true, trailing: <span>extra-mark</span> })] },
    { key: "a", title: "First", rows: [plain("alpha", { searchText: "alpha hidden-word" }), plain("gamma")] },
  ];
  const router = createMemoryRouter([
    {
      path: "*",
      element: (
        <>
          <RowList name="Rows" label="Pick" placeholder="Filter" sections={sections} {...props} />
          <Where />
        </>
      ),
    },
  ]);
  renderThemed(<RouterProvider router={router} />);
}

test("a row list keeps the caller's section order and shows current and trailing marks", async () => {
  setupList();
  fireEvent.click(await screen.findByRole("button", { name: /^Rows/ }));
  expect(screen.getAllByRole("group").map((g) => g.getAttribute("aria-label"))).toEqual(["Second", "First"]);
  expect(names()).toEqual(["betacurrent", "alpha", "gamma"]);
  const beta = screen.getAllByRole("option")[0];
  expect(beta.hasAttribute("aria-current")).toBe(true);
  expect(within(beta).getByText("extra-mark")).toBeTruthy();
});

test("a row list searches the caller's search text, and Enter opens the cursor row's link", async () => {
  setupList();
  fireEvent.click(await screen.findByRole("button", { name: /^Rows/ }));
  const input = screen.getByRole("combobox", { name: "Pick" });
  fireEvent.change(input, { target: { value: "hidden-word" } });
  expect(names()).toEqual(["alpha"]);
  fireEvent.keyDown(input, { key: "Enter" });
  await waitFor(() => expect(screen.getByTestId("where").textContent).toBe("/x/alpha"));
});

test("a row list can turn the Cmd-K listener off", async () => {
  setupList({ hotkey: false });
  const title = await screen.findByRole("button", { name: /^Rows/ });
  expect(title.textContent).not.toContain("⌘K");
  fireEvent.keyDown(window, { key: "k", metaKey: true });
  expect(screen.queryByRole("dialog")).toBeNull();
});

test("on a phone the menu is a sheet with a close button and no ⌘K hint", async () => {
  vi.stubGlobal("matchMedia", () => ({ matches: false, addEventListener() {}, removeEventListener() {} }));
  try {
    setup();
    const title = await titleButton();
    expect(title.textContent).not.toContain("⌘K");
    fireEvent.click(title);
    fireEvent.mouseDown(document.body);
    expect(screen.getByRole("dialog")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Close" }));
    expect(screen.queryByRole("dialog")).toBeNull();
  } finally {
    vi.unstubAllGlobals();
  }
});
