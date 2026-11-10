import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { createMemoryRouter, RouterProvider } from "react-router";
import { afterEach, expect, test, vi } from "vitest";
import { session } from "../agents/agentsTestKit";
import { ConnectionContext, type Connection, type ConnectionState } from "../connection";
import { CardKindDesign, CardKindImplementation, SectionImplementing, SectionOnHold, type EpicListRow, type SessionPlacement } from "../generated/server";
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
  row("p:blog", { name: "Blog" }),
  row("p:shop", { name: "Shop", card_kind: CardKindDesign }),
  row("p:old", { name: "Old", section: SectionOnHold }),
];
const placements: SessionPlacement[] = [{ session: "s3", kind: "chat", workspace: "Default" }];

type Reply = (route: string, body: Record<string, unknown>) => unknown;

function setup(reply: Reply) {
  const calls: { route: string; body: Record<string, unknown> }[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string, init?: RequestInit) => {
      const route = /\/v1\/workspaces\/(\w+)/.exec(String(url))?.[1];
      if (!route) return new Response(JSON.stringify({ epics: rows, follow_ups: [] }), { status: 200 });
      const body = JSON.parse(String(init?.body));
      calls.push({ route, body });
      return new Response(JSON.stringify(reply(route, body)), { status: 200 });
    }),
  );
  const state = {
    snapshot: {
      needs_you_total: 0,
      workspaces: [
        { name: "Default", default: true, members: 0 },
        { name: "Work", members: 0 },
      ],
      sessions: [session("s3", "p", { name: "talk" })],
      placements,
    },
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

const done = (target: string, extra = {}) => ({ target, status: "done", ...extra });
const body = () => within(screen.getByTestId("sidebar-body"));
const box = (name: string) => body().getByRole("checkbox", { name }) as HTMLInputElement;
const bar = () => within(document.querySelector("[data-part='select-bar']") as HTMLElement);

async function selectRows(...names: string[]) {
  await body().findByRole("link", { name: /Blog/ });
  fireEvent.click(screen.getByRole("button", { name: "Select" }));
  if (names.includes("Old")) fireEvent.click(body().getByRole("button", { name: /On hold/ }));
  for (const n of names) fireEvent.click(box(`Select ${n}`));
}

test("buttons show only when they fit, labelled n of m, with a tooltip naming rows and skip reasons", async () => {
  setup(() => ({ results: [] }));
  await selectRows("Blog", "Shop", "Old");
  expect(bar().getByRole("button", { name: "Move 3" })).toBeTruthy();
  const hold = bar().getByRole("button", { name: "Hold 2 of 3" });
  expect(hold.title).toBe("Runs on Blog, Shop\nOld: already on hold");
  const unhold = bar().getByRole("button", { name: "Take off hold 1 of 3" });
  expect(unhold.title).toBe("Runs on Old\nBlog: not on hold\nShop: not on hold");
});

test("Take off hold and Hold stay hidden when no selected row fits", async () => {
  setup(() => ({ results: [] }));
  await selectRows("Blog");
  expect(bar().queryByRole("button", { name: /Take off hold/ })).toBeNull();
});

test("Move opens a menu and posts one targets call", async () => {
  const calls = setup((_, b) => ({ results: (b.targets as string[]).map((t) => done(t)) }));
  await selectRows("Blog", "talk");
  fireEvent.click(bar().getByRole("button", { name: "Move 2" }));
  fireEvent.click(await screen.findByRole("menuitem", { name: "Work" }));
  await waitFor(() => expect(calls).toHaveLength(1));
  expect(calls[0]).toEqual({ route: "move", body: { name: "Work", targets: ["p:blog", "s3"] } });
});

test("Hold dry-runs, and with no live sessions sends one real call without yes and no confirm", async () => {
  const calls = setup((_, b) => ({ results: (b.targets as string[]).map((t) => done(t)) }));
  await selectRows("Blog", "Shop");
  fireEvent.click(bar().getByRole("button", { name: "Hold 2" }));
  await waitFor(() => expect(calls).toHaveLength(2));
  expect(calls[0].body).toEqual({ targets: ["p:blog", "p:shop"], dry_run: true });
  expect(calls[1].body).toEqual({ targets: ["p:blog", "p:shop"] });
  expect(screen.queryByRole("dialog")).toBeNull();
  // Held rows stay in On hold, so they stay selected.
  expect(screen.getByTestId("select-count").textContent).toBe("2 selected");
});

test("Hold asks when a dry run lists sessions, and sends the real call with yes only after the confirm", async () => {
  const calls = setup((_, b) => ({
    results: (b.targets as string[]).map((t) => (b.dry_run ? { target: t, status: "fits", sessions: t === "p:shop" ? ["designer"] : [] } : done(t))),
  }));
  await selectRows("Blog", "Shop");
  fireEvent.click(bar().getByRole("button", { name: "Hold 2" }));
  const dialog = await screen.findByRole("dialog");
  expect(dialog.textContent).toContain("designer");
  expect(calls).toHaveLength(1);
  fireEvent.click(within(dialog).getByRole("button", { name: "Hold and end sessions" }));
  await waitFor(() => expect(calls).toHaveLength(2));
  expect(calls[1].body).toEqual({ targets: ["p:blog", "p:shop"], yes: true });
});

test("Cancel on the confirm sends no real call and keeps the selection", async () => {
  const calls = setup((_, b) => ({ results: (b.targets as string[]).map((t) => ({ target: t, status: "fits", sessions: ["designer"] })) }));
  await selectRows("Blog");
  fireEvent.click(bar().getByRole("button", { name: "Hold 1" }));
  fireEvent.click(within(await screen.findByRole("dialog")).getByRole("button", { name: "Cancel" }));
  expect(calls).toHaveLength(1);
  expect(screen.getByTestId("select-count").textContent).toBe("1 selected");
});

test("Take off hold runs on held rows only, with no question", async () => {
  const calls = setup((_, b) => ({ results: (b.targets as string[]).map((t) => done(t)) }));
  await selectRows("Blog", "Old");
  fireEvent.click(bar().getByRole("button", { name: "Take off hold 1 of 2" }));
  await waitFor(() => expect(calls).toHaveLength(1));
  expect(calls[0]).toEqual({ route: "unhold", body: { targets: ["p:old"] } });
  expect(screen.queryByRole("dialog")).toBeNull();
  expect(screen.getByTestId("select-count").textContent).toBe("2 selected");
  expect(toast.mock.calls.some(([t]) => /Not run/.test(t.title) && /Blog: not on hold/.test(t.description))).toBe(true);
});

test("refused rows stay selected and a toast gives each reason", async () => {
  setup((_, b) => ({
    results: (b.targets as string[]).map((t) => (t === "p:shop" ? { target: t, status: "refused", reason: "hold-queued", message: "It is queued." } : done(t))),
  }));
  await selectRows("Blog", "Shop");
  fireEvent.click(bar().getByRole("button", { name: "Hold 2" }));
  await waitFor(() => expect(toast).toHaveBeenCalled());
  expect(toast.mock.calls[0][0]).toMatchObject({ type: "error", title: "Shop: It is queued." });
  expect(screen.getByTestId("select-count").textContent).toBe("2 selected");
  expect(box("Select Shop").checked).toBe(true);
});
