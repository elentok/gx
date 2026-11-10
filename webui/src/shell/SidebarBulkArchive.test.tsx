import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { afterEach, expect, test, vi } from "vitest";
import { ConnectionContext, type Connection, type ConnectionState } from "../connection";
import { CardKindDesign, CardKindImplementation, ProgressDone, SectionFinished, SectionImplementing, type EpicListRow } from "../generated/server";
import { renderThemed } from "../ui/render";
import { DeviceContext } from "../useWide";
import { AppShell } from "./AppShell";

const toast = vi.hoisted(() => vi.fn());
vi.mock("../ui/toaster", async (orig) => ({ ...(await orig<typeof import("../ui/toaster")>()), showToast: toast }));

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  localStorage.clear();
  toast.mockClear();
});

const row = (address: string, over: Partial<EpicListRow>): EpicListRow =>
  ({
    address,
    project: "p",
    name: address,
    kind: "loop",
    state: "idle",
    done: 4,
    total: 4,
    queued: false,
    queueable: false,
    blocked_by_epics: [],
    workspace: "Default",
    card_kind: CardKindImplementation,
    section: SectionFinished,
    status_words: "Finished",
    ...over,
  }) as EpicListRow;

const rows = [
  row("p:blog", { name: "Blog" }),
  row("p:shop", { name: "Shop", card_kind: CardKindDesign, progress_step: ProgressDone }),
  row("p:mid", { name: "Mid", section: SectionImplementing }),
];

type Call = { dry_run?: boolean; yes?: boolean; targets: string[] };
type Reply = (body: Call) => unknown;

function setup(reply: Reply) {
  const calls: Call[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string, init?: RequestInit) => {
      if (!String(url).includes("/v1/epics/archive")) return new Response(JSON.stringify({ epics: rows, follow_ups: [] }), { status: 200 });
      const body = JSON.parse(String(init?.body)) as Call;
      calls.push(body);
      return new Response(JSON.stringify(reply(body)), { status: 200 });
    }),
  );
  const state = {
    snapshot: { needs_you_total: 0, workspaces: [{ name: "Default", default: true, members: 0 }], sessions: [], placements: [] },
    connected: true,
    loadedOnce: true,
    tick: 0,
  } as unknown as ConnectionState;
  const conn = { getState: () => state, subscribe: () => () => {}, onEvent: () => () => {} } as unknown as Connection;
  const router = createMemoryRouter([{ element: <AppShell />, children: [{ path: "*", element: <div /> }] }], { initialEntries: ["/"] });
  renderThemed(
    <DeviceContext.Provider value="desktop">
      <ConnectionContext.Provider value={conn}>
        <RouterProvider router={router} />
      </ConnectionContext.Provider>
    </DeviceContext.Provider>,
  );
  return calls;
}

const fits = (target: string, extra = {}) => ({ target, status: "fits", on_trunk: true, ...extra });
const body = () => within(screen.getByTestId("sidebar-body"));
const box = (name: string) => body().getByRole("checkbox", { name }) as HTMLInputElement;
const bar = () => within(document.querySelector("[data-part='select-bar']") as HTMLElement);

async function selectRows(...names: string[]) {
  await body().findByRole("link", { name: /Blog/ });
  fireEvent.click(screen.getByRole("button", { name: "Select" }));
  for (const n of names) fireEvent.click(box(`Select ${n}`));
}

const dryOnly = (b: Call) => (b.dry_run ? { results: b.targets.map((t) => fits(t)) } : { results: b.targets.map((t) => ({ target: t, status: "done" })) });

test("the button reads n of m and fits only finished loop epics and finished Designs", async () => {
  setup(dryOnly);
  await selectRows("Blog", "Shop", "Mid");
  expect(bar().getByRole("button", { name: "Archive 2 of 3" })).toBeTruthy();
});

test("Archive posts a dry run first and confirms even with no live Session and no warning", async () => {
  const calls = setup(dryOnly);
  await selectRows("Blog", "Shop");
  fireEvent.click(bar().getByRole("button", { name: "Archive 2" }));
  const dialog = await screen.findByRole("dialog");
  expect(calls).toEqual([{ targets: ["p:blog", "p:shop"], dry_run: true }]);
  expect(within(dialog).getAllByRole("listitem").map((li) => li.textContent)).toEqual(["Blog", "Shop"]);
  expect(dialog.textContent).not.toContain("Live sessions");
  expect(dialog.textContent).not.toContain("trunk");
});

test("the confirm lists live sessions and one warning per epic not on trunk; Confirm sends one real call with yes", async () => {
  const calls = setup((b) =>
    b.dry_run ? { results: [fits("p:blog", { on_trunk: false, sessions: ["s1", "s2"] }), fits("p:shop")] } : { results: b.targets.map((t) => ({ target: t, status: "done" })) },
  );
  await selectRows("Blog", "Shop");
  fireEvent.click(bar().getByRole("button", { name: "Archive 2" }));
  const dialog = await screen.findByRole("dialog");
  expect(dialog.textContent).toContain("Live sessions will stop: s1, s2.");
  expect(dialog.textContent).toContain("Blog is not merged to trunk.");
  expect(dialog.textContent).not.toContain("Shop is not merged");
  fireEvent.click(within(dialog).getByRole("button", { name: "Archive" }));
  await waitFor(() => expect(calls).toHaveLength(2));
  expect(calls[1]).toEqual({ targets: ["p:blog", "p:shop"], yes: true });
  await waitFor(() => expect(screen.queryByTestId("select-count")).toBeNull());
});

test("Cancel sends no real call and keeps the selection", async () => {
  const calls = setup(dryOnly);
  await selectRows("Blog");
  fireEvent.click(bar().getByRole("button", { name: "Archive 1" }));
  fireEvent.click(within(await screen.findByRole("dialog")).getByRole("button", { name: "Cancel" }));
  expect(calls).toHaveLength(1);
  expect(screen.getByTestId("select-count").textContent).toBe("1 selected");
});

test("a clash rename shows in the toast as Archived as ...-2", async () => {
  setup((b) => (b.dry_run ? dryOnly(b) : { results: [{ target: "p:blog", status: "done", new_address: "p:blog-2" }, { target: "p:shop", status: "done" }] }));
  await selectRows("Blog", "Shop");
  fireEvent.click(bar().getByRole("button", { name: "Archive 2" }));
  fireEvent.click(within(await screen.findByRole("dialog")).getByRole("button", { name: "Archive" }));
  await waitFor(() => expect(toast).toHaveBeenCalled());
  expect(toast.mock.calls.map(([t]) => t.title)).toContain("Archived as blog-2");
});

test("rows the dry run refuses are listed with the reason, do not run, and stay selected with a toast", async () => {
  const calls = setup((b) =>
    b.dry_run
      ? { results: [{ target: "p:blog", status: "refused", reason: "impl-not-archivable", message: "impl epic not finished" }, fits("p:shop")] }
      : { results: b.targets.map((t) => ({ target: t, status: "done" })) },
  );
  await selectRows("Blog", "Shop");
  fireEvent.click(bar().getByRole("button", { name: "Archive 2" }));
  const dialog = await screen.findByRole("dialog");
  expect(dialog.textContent).toContain("Blog will not run: impl epic not finished.");
  fireEvent.click(within(dialog).getByRole("button", { name: "Archive" }));
  await waitFor(() => expect(calls).toHaveLength(2));
  expect(calls[1]).toEqual({ targets: ["p:shop"], yes: true });
  await waitFor(() => expect(toast.mock.calls.map(([t]) => t.title)).toEqual(["Blog: impl epic not finished"]));
  await waitFor(() => expect(screen.getByTestId("select-count").textContent).toBe("1 selected"));
  expect(box("Select Blog").checked).toBe(true);
});
