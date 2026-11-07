import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { createMemoryRouter, RouterProvider, useLocation } from "react-router";
import { afterEach, expect, test } from "vitest";
import type { EpicListRow } from "../generated/server";
import { renderThemed } from "../ui/render";
import { jumpHref, Menu, type ListRowData, type ListSection } from "./JumpMenu";

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

test("a joined card with a Design question opens the Design page", () => {
  const joined = row("blog", {
    address: "demo:blog",
    addresses: ["demo:blog", "demo:blog-impl"],
    actions: [{ kind: "answer", label: "Answer", target: "demo:blog", link: "#/d/demo:blog" }],
  });
  expect(jumpHref(joined)).toBe("#/d/demo:blog");
  expect(jumpHref(row("loop-epic"))).toBe("#/e/demo:loop-epic");
  expect(jumpHref(row("blog", { card_kind: "design" }))).toBe("#/d/demo:blog");
});

function Where() {
  return <div data-testid="where">{useLocation().pathname}</div>;
}

const plain = (key: string, over: Partial<ListRowData> = {}): ListRowData => ({
  key,
  title: key,
  subtitle: `sub ${key}`,
  href: `#/x/${key}`,
  searchText: key,
  ...over,
});

const names = () => screen.queryAllByRole("option").map((o) => within(o).getAllByText(/./)[0].textContent);

function setupMenu(extra: Partial<React.ComponentProps<typeof Menu>> = {}) {
  const sections: ListSection[] = [
    { key: "b", title: "Second", rows: [plain("beta", { current: true, trailing: <span>extra-mark</span> })] },
    { key: "a", title: "First", rows: [plain("alpha", { searchText: "alpha hidden-word" }), plain("gamma")] },
    { key: "h", title: "Hidden", hiddenUntilSearch: true, rows: [plain("late")] },
  ];
  const router = createMemoryRouter([
    {
      path: "*",
      element: (
        <>
          <Menu sections={sections} label="Pick" placeholder="Filter" phone={false} onClose={() => {}} {...extra} />
          <Where />
        </>
      ),
    },
  ]);
  renderThemed(<RouterProvider router={router} />);
}

test("a menu keeps the caller's section order, hides hiddenUntilSearch sections and shows current and trailing marks", async () => {
  setupMenu();
  await screen.findByRole("dialog", { name: "Pick" });
  expect(screen.getAllByRole("group").map((g) => g.getAttribute("aria-label"))).toEqual(["Second", "First"]);
  expect(names()).toEqual(["betacurrent", "alpha", "gamma"]);
  const beta = screen.getAllByRole("option")[0];
  expect(beta.hasAttribute("aria-current")).toBe(true);
  expect(within(beta).getByText("extra-mark")).toBeTruthy();
});

test("typing searches the caller's search text, reveals hidden sections, and Enter opens the cursor row's link", async () => {
  setupMenu();
  const input = await screen.findByRole("combobox", { name: "Pick" });
  fireEvent.change(input, { target: { value: "late" } });
  expect(names()).toEqual(["late"]);
  fireEvent.change(input, { target: { value: "hidden-word" } });
  expect(names()).toEqual(["alpha"]);
  fireEvent.keyDown(input, { key: "Enter" });
  await waitFor(() => expect(screen.getByTestId("where").textContent).toBe("/x/alpha"));
});

test("on a phone the menu has a close button", async () => {
  let closed = 0;
  setupMenu({ phone: true, onClose: () => closed++ });
  fireEvent.click(await screen.findByRole("button", { name: "Close" }));
  expect(closed).toBe(1);
});
