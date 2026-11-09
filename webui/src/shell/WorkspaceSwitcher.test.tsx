import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { afterEach, expect, test, vi } from "vitest";
import { ConnectionContext, type Connection, type ConnectionState } from "../connection";
import { SectionImplementing, type EpicListRow, type WorkspaceResult } from "../generated/server";
import { renderThemed } from "../ui/render";
import { DeviceContext } from "../useWide";
import { ACTIVE_KEY } from "../workspaces/active";
import { AppShell } from "./AppShell";
import { NewWorkspaceDialog, type CreateWorkspace } from "./WorkspaceSwitcher";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  localStorage.clear();
});

const row = (address: string, workspace: string): EpicListRow =>
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
    workspace,
    card_kind: "implementation",
    section: SectionImplementing,
    status_words: "Idle",
  }) as EpicListRow;

function setup(create: CreateWorkspace = async () => ({}), onManage = () => {}) {
  // Like the server's event: the snapshot lists a created workspace once the write is accepted.
  const created: CreateWorkspace = async (name) => {
    const res = await create(name);
    if (!res.refused) state.snapshot!.workspaces!.push({ name, members: 0 });
    return res;
  };
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => new Response(JSON.stringify({ epics: [row("p:alpha", "Default"), row("p:beta", "Side")], follow_ups: [] }), { status: 200 })),
  );
  const state = {
    snapshot: {
      needs_you_total: 3,
      workspaces: [
        { name: "Default", default: true, members: 1, needs_you: 2 },
        { name: "Side", members: 1, needs_you: 1 },
      ],
      sessions: [],
      placements: [],
    },
    connected: true,
    loadedOnce: true,
    tick: 0,
  } as unknown as ConnectionState;
  const conn = { getState: () => state, subscribe: () => () => {}, onEvent: () => () => {} } as unknown as Connection;
  const router = createMemoryRouter([{ element: <AppShell createWorkspace={created} onManageWorkspaces={onManage} />, children: [{ path: "*", element: <div>page</div> }] }]);
  renderThemed(
    <DeviceContext.Provider value="desktop">
      <ConnectionContext.Provider value={conn}>
        <RouterProvider router={router} />
      </ConnectionContext.Provider>
    </DeviceContext.Provider>,
  );
}

const body = () => within(screen.getByTestId("sidebar-body"));
const openMenu = async () => {
  fireEvent.click(await screen.findByRole("button", { name: /^Workspace:/ }));
  return screen.findByRole("menu");
};

test("the switcher is a boxed button with a ▾ and the inset focus ring", async () => {
  setup();
  const trigger = await screen.findByRole("button", { name: /^Workspace:/ });
  expect(trigger.getAttribute("data-focus-ring")).toBe("inset");
  expect(trigger.textContent).toContain("▾");
});

test("the menu lists every workspace in order with its Needs-you count; picking one switches the rows and storage", async () => {
  setup();
  await body().findByRole("link", { name: /p:alpha/ });
  const menu = within(await openMenu());
  const items = menu.getAllByRole("menuitem").map((i) => i.textContent);
  expect(items).toEqual(["Defaultdefault2 needs you", "Side1 needs you","New workspace…", "Manage workspaces"]);
  fireEvent.click(menu.getByRole("menuitem", { name: /Side/ }));
  await waitFor(() => expect(body().getByRole("link", { name: /p:beta/ })).toBeTruthy());
  expect(body().queryByRole("link", { name: /p:alpha/ })).toBeNull();
  expect(localStorage.getItem(ACTIVE_KEY)).toBe("Side");
});

test("New workspace… posts the trimmed name, makes it active and closes the dialog", async () => {
  const create = vi.fn(async (): Promise<WorkspaceResult> => ({}));
  setup(create);
  fireEvent.click(within(await openMenu()).getByRole("menuitem", { name: "New workspace…" }));
  fireEvent.change(await screen.findByLabelText("Workspace name"), { target: { value: " Fresh " } });
  fireEvent.click(screen.getByRole("button", { name: "Create" }));
  await waitFor(() => expect(create).toHaveBeenCalledWith("Fresh"));
  await waitFor(() => expect(screen.queryByLabelText("Workspace name")).toBeNull());
  expect(localStorage.getItem(ACTIVE_KEY)).toBe("Fresh");
});

test.each([
  ["desktop", true],
  ["phone", false],
] as const)("the New workspace dialog on a %s: Name focused is %s", (device, focused) => {
  renderThemed(
    <DeviceContext.Provider value={device}>
      <NewWorkspaceDialog onCreated={() => {}} onClose={() => {}} />
    </DeviceContext.Provider>,
  );
  expect(document.activeElement === screen.getByLabelText("Workspace name")).toBe(focused);
});

test("a clash refusal shows inline and keeps the dialog open", async () => {
  const create = vi.fn(async (): Promise<WorkspaceResult> => ({ refused: true, message: "A workspace named Side already exists." }));
  setup(create);
  fireEvent.click(within(await openMenu()).getByRole("menuitem", { name: "New workspace…" }));
  fireEvent.change(await screen.findByLabelText("Workspace name"), { target: { value: "Side" } });
  fireEvent.click(screen.getByRole("button", { name: "Create" }));
  expect((await screen.findByRole("alert")).textContent).toBe("A workspace named Side already exists.");
  expect(screen.getByLabelText("Workspace name")).toBeTruthy();
  expect(localStorage.getItem(ACTIVE_KEY)).not.toBe("Side");
});

test("the footer + New item opens the same dialog", async () => {
  setup();
  fireEvent.click(await screen.findByRole("button", { name: "+ New" }));
  fireEvent.click(await screen.findByRole("menuitem", { name: "New workspace…" }));
  expect(await screen.findByLabelText("Workspace name")).toBeTruthy();
});

test("Manage workspaces calls its callback", async () => {
  const onManage = vi.fn();
  setup(undefined, onManage);
  fireEvent.click(within(await openMenu()).getByRole("menuitem", { name: "Manage workspaces" }));
  await waitFor(() => expect(onManage).toHaveBeenCalled());
});
