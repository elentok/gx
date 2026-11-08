import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { useRef } from "react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { afterEach, expect, test, vi } from "vitest";
import { session } from "../agents/agentsTestKit";
import { requestPromptFocus } from "../agents/promptFocus";
import { ConnectionContext, type Connection, type ConnectionState } from "../connection";
import { CardKindImplementation, SectionImplementing, SectionOnHold, type EpicListRow, type SessionPlacement } from "../generated/server";
import { renderThemed } from "../ui/render";
import { DeviceContext } from "../useWide";
import { AppShell } from "./AppShell";
import type { Shell } from "./ShellContext";
import { useSidebarKeys } from "./SidebarKeys";

vi.mock("../agents/promptFocus", () => ({ requestPromptFocus: vi.fn(), usePromptFocus: () => {} }));

afterEach(() => {
  vi.mocked(requestPromptFocus).mockClear();
  cleanup();
  vi.unstubAllGlobals();
  localStorage.clear();
  window.location.hash = "";
});

const row = (address: string, over: Partial<EpicListRow>): EpicListRow =>
  ({
    address,
    project: "p",
    name: address,
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

const rows = [row("p:blog", { name: "Blog" }), row("p:shop", { name: "Shop" }), row("p:old", { name: "Old", section: SectionOnHold })];
const placements: SessionPlacement[] = [
  { session: "s1", kind: "epic", epic: "p:blog" },
  { session: "s2", kind: "other" },
];

async function setup(withChat = false) {
  vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify({ epics: rows, follow_ups: [] }), { status: 200 })));
  const state = {
    snapshot: {
      needs_you_total: 0,
      workspaces: [{ name: "Default", default: true, members: 0 }],
      sessions: [session("s1", "p", { name: "builder" }), session("s2", "p", { name: "scratch" }), ...(withChat ? [session("s3", "p", { name: "talk" })] : [])],
      placements: withChat ? [...placements, { session: "s3", kind: "chat", workspace: "Default" }] : placements,
    },
    connected: true,
    loadedOnce: true,
    tick: 0,
  } as unknown as ConnectionState;
  const conn = { getState: () => state, subscribe: () => () => {}, onEvent: () => () => {} } as unknown as Connection;
  const router = createMemoryRouter([{ element: <AppShell />, children: [{ path: "*", element: <div>page</div> }] }], { initialEntries: ["/"] });
  renderThemed(
    <DeviceContext.Provider value="desktop">
      <ConnectionContext.Provider value={conn}>
        <RouterProvider router={router} />
      </ConnectionContext.Provider>
    </DeviceContext.Provider>,
  );
  await within(screen.getByTestId("sidebar-body")).findByRole("link", { name: /Blog/ });
}

const body = () => within(screen.getByTestId("sidebar-body"));
const aside = () => document.querySelector('aside[aria-label="Sidebar"]') as HTMLElement;
const press = (key: string) => fireEvent.keyDown(document.activeElement ?? document.body, { key });
const focus = (el: HTMLElement) => el.focus();

test("Cmd/Ctrl+B toggles the sidebar and focuses a row when it opens", async () => {
  await setup();
  fireEvent.keyDown(document.body, { key: "b", ctrlKey: true });
  await waitFor(() => expect(aside().getAttribute("data-open")).toBe("false"));
  fireEvent.keyDown(document.body, { key: "b", ctrlKey: true });
  await waitFor(() => expect(aside().getAttribute("data-open")).toBe("true"));
  expect(document.activeElement).toBe(body().getByRole("link", { name: /Blog/ }));
});

test("Down/Up and j/k walk the rows, through On hold and Other once expanded", async () => {
  await setup();
  fireEvent.click(body().getByRole("button", { name: "On hold (1)" }));
  fireEvent.click(body().getByRole("button", { name: "Other" }));
  focus(body().getByRole("link", { name: /Blog/ }));
  press("ArrowDown");
  expect(document.activeElement).toBe(body().getByRole("link", { name: /Shop/ }));
  press("j");
  expect(document.activeElement).toBe(body().getByRole("button", { name: "On hold (1)" }));
  press("j");
  expect(document.activeElement).toBe(body().getByRole("link", { name: /Old/ }));
  press("j");
  expect(document.activeElement).toBe(body().getByRole("button", { name: "Other" }));
  press("j");
  expect(document.activeElement).toBe(body().getByRole("link", { name: /scratch/ }));
  press("k");
  press("ArrowUp");
  expect(document.activeElement).toBe(body().getByRole("link", { name: /Old/ }));
});

test("only the focused row is a tab stop and the chevron is none", async () => {
  await setup();
  const blog = body().getByRole("link", { name: /Blog/ });
  const shop = body().getByRole("link", { name: /Shop/ });
  expect([blog.tabIndex, shop.tabIndex]).toEqual([0, -1]);
  expect(body().getByRole("button", { name: "Expand Blog" }).tabIndex).toBe(-1);
  focus(shop);
  await waitFor(() => expect([blog.tabIndex, shop.tabIndex]).toEqual([-1, 0]));
});

test("l/Right expand and h/Left collapse an epic's agents and a section", async () => {
  await setup();
  focus(body().getByRole("link", { name: /Blog/ }));
  press("l");
  expect(body().getByRole("link", { name: /builder/ })).toBeTruthy();
  press("h");
  expect(body().queryByRole("link", { name: /builder/ })).toBeNull();
  press("ArrowRight");
  expect(body().getByRole("link", { name: /builder/ })).toBeTruthy();
  press("ArrowLeft");
  expect(body().queryByRole("link", { name: /builder/ })).toBeNull();
  focus(body().getByRole("button", { name: "On hold (1)" }));
  press("l");
  expect(body().getByRole("link", { name: /Old/ })).toBeTruthy();
  press("h");
  expect(body().queryByRole("link", { name: /Old/ })).toBeNull();
});

test("Enter opens the focused row", async () => {
  await setup();
  focus(body().getByRole("link", { name: /Shop/ }));
  press("Enter");
  await waitFor(() => expect(window.location.hash).toBe("#/e/p:shop"));
});

test("Enter on a chat sends the prompt focus request; on an epic row it only navigates", async () => {
  await setup(true);
  focus(body().getByRole("link", { name: /Blog/ }));
  press("Enter");
  await waitFor(() => expect(window.location.hash).toBe("#/e/p:blog"));
  expect(requestPromptFocus).not.toHaveBeenCalled();
  focus(body().getByRole("link", { name: /talk/ }));
  press("Enter");
  expect(requestPromptFocus).toHaveBeenCalledTimes(1);
});

test("a click on a chat row sends no focus request", async () => {
  await setup(true);
  fireEvent.click(body().getByRole("link", { name: /talk/ }));
  expect(requestPromptFocus).not.toHaveBeenCalled();
});

function Harness({ keys }: { keys: string[] }) {
  const aside = useRef<HTMLElement>(null);
  useSidebarKeys(aside, { sidebarOpen: true, toggle: () => {} } as unknown as Shell);
  return (
    <aside ref={aside}>
      <ul data-nav-list="" tabIndex={-1} data-testid="list">
        {keys.map((k) => (
          <li key={k}>
            <a href={`#/${k}`} data-nav={`row:${k}`}>
              {k}
            </a>
          </li>
        ))}
      </ul>
    </aside>
  );
}

const link = (name: string) => screen.getByRole("link", { name });

test("a reorder keeps focus on the same row, and the tab stop with it", async () => {
  const { rerender } = render(<Harness keys={["a", "b", "c"]} />);
  link("b").focus();
  rerender(<Harness keys={["c", "b", "a"]} />);
  await waitFor(() => expect(document.activeElement).toBe(link("b")));
  rerender(<Harness keys={["b", "a", "c"]} />);
  await waitFor(() => expect(document.activeElement).toBe(link("b")));
  expect(["a", "b", "c"].map((k) => link(k).tabIndex)).toEqual([-1, 0, -1]);
});

test("when the focused row leaves, focus lands on the row at its old index, clamped, then on the list", async () => {
  const { rerender } = render(<Harness keys={["a", "b", "c"]} />);
  link("b").focus();
  rerender(<Harness keys={["a", "c"]} />);
  await waitFor(() => expect(document.activeElement).toBe(link("c")));
  rerender(<Harness keys={["a"]} />);
  await waitFor(() => expect(document.activeElement).toBe(link("a")));
  rerender(<Harness keys={[]} />);
  await waitFor(() => expect(document.activeElement).toBe(screen.getByTestId("list")));
});

test("a reorder does not steal focus from outside the sidebar", async () => {
  const { rerender } = render(<Harness keys={["a", "b"]} />);
  link("a").focus();
  const input = document.createElement("input");
  document.body.appendChild(input);
  input.focus();
  await Promise.resolve();
  rerender(<Harness keys={["b", "a"]} />);
  await Promise.resolve();
  expect(document.activeElement).toBe(input);
  input.remove();
});

test("vim keys do nothing outside the sidebar or in a text field", async () => {
  await setup();
  const blog = body().getByRole("link", { name: /Blog/ });
  focus(blog);
  fireEvent.keyDown(document.body, { key: "j" });
  expect(document.activeElement).toBe(blog);
  const input = document.createElement("input");
  aside().appendChild(input);
  input.focus();
  fireEvent.keyDown(input, { key: "j" });
  expect(document.activeElement).toBe(input);
});
