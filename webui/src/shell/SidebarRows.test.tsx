import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react"
import { createMemoryRouter, RouterProvider, useLocation } from "react-router"
import { afterEach, expect, test, vi } from "vitest"
import { session } from "../agents/agentsTestKit"
import { SessionActions } from "../agents/SessionActions"
import { ConnectionContext, type Connection, type ConnectionState } from "../connection"
import {
  CardKindDesign,
  CardKindImplementation,
  SectionImplementing,
  SectionOnHold,
  type EpicListRow,
  type SessionPlacement,
} from "../generated/server"
import { renderThemed } from "../ui/render"
import { DeviceContext, type Device } from "../useWide"
import { AppShell } from "./AppShell"

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
  localStorage.clear()
})

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
  }) as EpicListRow

const rows = [
  row("p:blog", { name: "Blog", status_words: "Waiting on review" }),
  row("p:shop", { name: "Shop", card_kind: CardKindDesign }),
  row("p:old", { name: "Old", section: SectionOnHold, status_words: "Held" }),
]
const placements: SessionPlacement[] = [
  { session: "s1", kind: "epic", epic: "p:blog" },
  { session: "s2", kind: "other" },
  { session: "s3", kind: "chat", workspace: "Default" },
  { session: "s4", kind: "epic", epic: "p:shop" },
  { session: "s5", kind: "epic", epic: "p:blog" },
  { session: "s6", kind: "epic", epic: "p:blog" },
]

function Page() {
  return <div data-testid="page-path">{useLocation().pathname}</div>
}

function setup(device: Device, path = "/") {
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () => new Response(JSON.stringify({ epics: rows, follow_ups: [] }), { status: 200 }),
    ),
  )
  const state = {
    snapshot: {
      needs_you_total: 0,
      workspaces: [
        { name: "Default", default: true, members: 0 },
        { name: "Work", members: 0 },
      ],
      sessions: [
        session("s1", "p", { name: "builder" }),
        session("s2", "p", { name: "scratch", status: { state: "exited" } }),
        session("s3", "p", { name: "talk" }),
        session("s4", "p", { name: "designer" }),
        session("s5", "p", { name: "fixer", fix_of: "p:blog/03", status: { state: "working" } }),
        session("s6", "p", { name: "queuer", queue: true, tier: "worker" }),
      ],
      placements,
    },
    connected: true,
    loadedOnce: true,
    tick: 0,
  } as unknown as ConnectionState
  const conn = {
    getState: () => state,
    subscribe: () => () => {},
    onEvent: () => () => {},
  } as unknown as Connection
  const router = createMemoryRouter(
    [{ element: <AppShell />, children: [{ path: "*", element: <Page /> }] }],
    { initialEntries: [path] },
  )
  renderThemed(
    <DeviceContext.Provider value={device}>
      <ConnectionContext.Provider value={conn}>
        <RouterProvider router={router} />
      </ConnectionContext.Provider>
    </DeviceContext.Provider>,
  )
}

const body = () => within(screen.getByTestId("sidebar-body"))

test("rows show the name and status words; the open epic is current and stays collapsed", async () => {
  setup("desktop", "/e/p:blog")
  const link = await body().findByRole("link", { name: /Blog/ })
  expect(link.textContent).toContain("Waiting on review")
  expect(link.getAttribute("href")).toBe("#/e/p:blog")
  expect(link.getAttribute("aria-current")).toBe("page")
  expect(body().queryByRole("link", { name: /builder/ })).toBeNull()
})

test("clicking an epic row navigates and leaves it collapsed; the chevron toggles both ways", async () => {
  setup("desktop", "/")
  const link = await body().findByRole("link", { name: /Blog/ })
  fireEvent.click(link)
  await waitFor(() => expect(window.location.hash).toBe("#/e/p:blog"))
  expect(body().queryByRole("link", { name: /builder/ })).toBeNull()
  fireEvent.click(body().getByRole("button", { name: "Expand Blog" }))
  expect(body().getByRole("link", { name: /builder/ })).toBeTruthy()
  fireEvent.click(body().getByRole("button", { name: "Collapse Blog" }))
  expect(body().queryByRole("link", { name: /builder/ })).toBeNull()
})

test("outside select mode rows have no checkboxes and a row's ... menu still opens", async () => {
  setup("desktop", "/")
  const shop = await body().findByRole("link", { name: /Shop/ })
  expect(body().queryAllByRole("checkbox")).toEqual([])
  fireEvent.click(within(shop.closest("li")!).getByRole("button", { name: "More actions" }))
  expect((await screen.findAllByRole("menuitem")).length).toBeGreaterThan(0)
})

test("chat and epic icons sit in the same fixed, centering slot", async () => {
  setup("desktop", "/")
  const epic = within(await body().findByRole("link", { name: /Blog/ }))
  const chat = within(await body().findByRole("link", { name: /talk/ }))
  const slots = [epic.getByTestId("icon-slot"), chat.getByTestId("icon-slot")]
  for (const slot of slots) {
    const style = getComputedStyle(slot)
    expect(style.width).toBe("26px")
    expect(style.display).toBe("flex")
    expect(style.justifyContent).toBe("center")
    expect(style.alignItems).toBe("center")
  }
  expect(epic.getByTestId("kind-dot")).toBeTruthy()
  expect(chat.getByLabelText("Chat")).toBeTruthy()
})

test("a Fix agent row shows the wrench in its status colour; Queue and plain agent rows have none", async () => {
  setup("desktop", "/")
  await body().findByRole("link", { name: /Blog/ })
  fireEvent.click(body().getByRole("button", { name: "Expand Blog" }))
  const fixer = within(body().getByRole("link", { name: /fixer/ }))
  const working = getComputedStyle(fixer.getByText("Working")).color
  expect(working).not.toBe("")
  expect(getComputedStyle(fixer.getByTestId("fix-wrench")).color).toBe(working)
  expect(
    within(body().getByRole("link", { name: /queuer/ })).queryByTestId("fix-wrench"),
  ).toBeNull()
  expect(
    within(body().getByRole("link", { name: /builder/ })).queryByTestId("fix-wrench"),
  ).toBeNull()
})

test("a nested agent row has the spacer slot and no icon; truncation styles sit on its title", async () => {
  setup("desktop", "/")
  await body().findByRole("link", { name: /Blog/ })
  fireEvent.click(body().getByRole("button", { name: "Expand Blog" }))
  const builder = within(body().getByRole("link", { name: /builder/ }))
  expect(getComputedStyle(builder.getByTestId("icon-slot")).width).toBe("26px")
  expect(builder.queryByLabelText("Chat")).toBeNull()
  expect(builder.queryByTestId("kind-dot")).toBeNull()
  const title = builder.getByText("builder")
  expect(getComputedStyle(title).textOverflow).toBe("ellipsis")
  expect(getComputedStyle(title).whiteSpace).toBe("nowrap")
  expect(getComputedStyle(title).fontSize).toBe("13px")
  // Fix agent: wrench sits in the same slot.
  const fixer = within(body().getByRole("link", { name: /fixer/ }))
  expect(fixer.getByTestId("icon-slot").contains(fixer.getByTestId("fix-wrench"))).toBe(true)
})

test("the open nested agent row is current with the top-level tint and edge; only the last guide stops at the middle", async () => {
  setup("desktop", "/agents/s1")
  await body().findByRole("link", { name: /Blog/ })
  fireEvent.click(body().getByRole("button", { name: "Expand Blog" }))
  const link = body().getByRole("link", { name: /builder/ })
  expect(link.getAttribute("aria-current")).toBe("page")
  const frame = link.parentElement!
  const parentFrame = body().getByRole("link", { name: /Blog/ }).parentElement!
  expect(getComputedStyle(frame).backgroundColor).toBe(
    getComputedStyle(parentFrame).backgroundColor,
  )
  expect(getComputedStyle(frame).borderLeftWidth).toBe("3px")
  expect(body().getByRole("link", { name: /fixer/ }).getAttribute("aria-current")).toBeNull()
  const guides = body().getAllByTestId("nested-guide")
  expect(getComputedStyle(guides[0]).bottom).toBe("0px")
  expect(getComputedStyle(guides[guides.length - 1]).bottom).toBe("50%")
})

test("opening another page leaves an expanded row expanded", async () => {
  setup("desktop", "/")
  await body().findByRole("link", { name: /Blog/ })
  fireEvent.click(body().getByRole("button", { name: "Expand Blog" }))
  fireEvent.click(body().getByRole("link", { name: /Shop/ }))
  await waitFor(() => expect(window.location.hash).toBe("#/d/p:shop"))
  expect(body().getByRole("link", { name: /builder/ })).toBeTruthy()
})

test("a Design is one row: no chevron and no nested session, even when open", async () => {
  setup("desktop", "/d/p:shop")
  await body().findByRole("link", { name: /Shop/ })
  expect(body().queryByRole("button", { name: /(Expand|Collapse) Shop/ })).toBeNull()
  expect(body().queryByRole("link", { name: /designer/ })).toBeNull()
})

test("On hold is collapsed with a count and opens on click; Other lists its rows", async () => {
  setup("desktop")
  await body().findByRole("link", { name: /Blog/ })
  expect(body().queryByRole("link", { name: /Old/ })).toBeNull()
  fireEvent.click(body().getByRole("button", { name: "On hold (1)" }))
  expect(body().getByRole("link", { name: /Old/ })).toBeTruthy()
  expect(body().queryByRole("link", { name: /scratch/ })).toBeNull()
  fireEvent.click(body().getByRole("button", { name: "Other" }))
  const scratch = within(body().getByRole("link", { name: /scratch/ }))
  // No parent row: no guide line.
  expect(body().queryAllByTestId("nested-guide")).toHaveLength(0)
  expect(scratch.queryByLabelText("Chat")).toBeNull()
})

test("the Other section's open state survives a reload", async () => {
  setup("desktop")
  await body().findByRole("link", { name: /Blog/ })
  fireEvent.click(body().getByRole("button", { name: "Other" }))
  expect(localStorage.getItem("gx.sidebar.other-open")).toBe("1")
  cleanup()
  setup("desktop")
  await body().findByRole("link", { name: /Blog/ })
  expect(await body().findByRole("link", { name: /scratch/ })).toBeTruthy()
})

test("a chat row reads Chat before its name", async () => {
  setup("desktop")
  await body().findByRole("link", { name: /Blog/ })
  expect(body().getByRole("link", { name: /^Chat talk/ })).toBeTruthy()
})

test("a row's ... menu opens and Move posts the other workspace", async () => {
  const fetchMock = vi.fn(
    async (url: string, _init?: RequestInit) =>
      new Response(
        JSON.stringify(
          String(url).includes("/v1/workspaces/")
            ? { results: [{ target: "p:shop", status: "done" }] }
            : { epics: rows, follow_ups: [] },
        ),
        { status: 200 },
      ),
  )
  setup("desktop")
  vi.stubGlobal("fetch", fetchMock)
  const shop = await body().findByRole("link", { name: /Shop/ })
  fireEvent.click(within(shop.closest("li")!).getByRole("button", { name: "More actions" }))
  fireEvent.click(await screen.findByRole("menuitem", { name: "Move to workspace" }))
  fireEvent.click(await screen.findByRole("menuitem", { name: /^Work/ }))
  await waitFor(() => {
    const call = fetchMock.mock.calls.find(([u]) => String(u).includes("/v1/workspaces/move"))
    expect(JSON.parse((call![1] as RequestInit).body as string)).toEqual({
      name: "Work",
      targets: ["p:shop"],
    })
  })
})

test("a chat row's menu offers Move, Stop and Remove", async () => {
  setup("desktop")
  const chat = await body().findByRole("link", { name: /^Chat talk/ })
  fireEvent.click(within(chat.closest("li")!).getByRole("button", { name: "More actions" }))
  await screen.findAllByRole("menuitem")
  expect(screen.getAllByRole("menuitem").map((i) => i.textContent)).toEqual([
    "Move to workspace›",
    "Stop",
    "Remove",
  ])
})

const menuLabels = () => screen.getAllByRole("menuitem").map((i) => i.textContent)
const expandBlog = async () => {
  await body().findByRole("link", { name: /Blog/ })
  fireEvent.click(body().getByRole("button", { name: "Expand Blog" }))
}

test("a nested agent's menu equals the agent page's, with Stop only while live; no Move or Promote", async () => {
  setup("desktop")
  await expandBlog()
  fireEvent.click(
    within(body().getByRole("link", { name: /builder/ }).parentElement!).getByRole("button", {
      name: "More actions",
    }),
  )
  await screen.findAllByRole("menuitem")
  const nested = menuLabels()
  expect(nested).toContain("Stop")
  expect(nested).toContain("Remove")
  expect(nested.join()).not.toMatch(/Move|Promote|Hold/)
  cleanup()
  renderThemed(
    <SessionActions session={session("s1", "p", { name: "builder" })} onRemoved={() => {}} />,
  )
  fireEvent.click(screen.getByRole("button", { name: "More actions" }))
  await screen.findAllByRole("menuitem")
  expect(menuLabels()).toEqual(nested)
})

test("an exited nested agent has no Stop; a Queue agent has no menu button", async () => {
  setup("desktop")
  await expandBlog()
  expect(
    within(body().getByRole("link", { name: /queuer/ }).parentElement!).queryByRole("button", {
      name: "More actions",
    }),
  ).toBeNull()
  fireEvent.click(body().getByRole("button", { name: "Other" }))
  const scratch = within(body().getByRole("link", { name: /scratch/ }).parentElement!)
  fireEvent.click(scratch.getByRole("button", { name: "More actions" }))
  await screen.findAllByRole("menuitem")
  expect(menuLabels()).toContain("Remove")
  expect(menuLabels()).not.toContain("Stop")
})

test("removing the open nested agent from its row confirms, then goes to the agents list", async () => {
  const fetchMock = vi.fn(async (url: string, _init?: RequestInit) => {
    const u = String(url)
    if (u.includes("/v1/sessions/remove"))
      return new Response(JSON.stringify({ results: [{ target: "s1", status: "done" }] }), {
        status: 200,
      })
    return new Response(JSON.stringify({ epics: rows, follow_ups: [] }), { status: 200 })
  })
  setup("desktop", "/agents/s1")
  vi.stubGlobal("fetch", fetchMock)
  await expandBlog()
  fireEvent.click(
    within(body().getByRole("link", { name: /builder/ }).parentElement!).getByRole("button", {
      name: "More actions",
    }),
  )
  fireEvent.click(await screen.findByRole("menuitem", { name: "Remove" }))
  fireEvent.click(await screen.findByRole("button", { name: "Remove" }))
  await waitFor(() =>
    expect(fetchMock.mock.calls.some(([u]) => String(u).includes("/v1/sessions/remove"))).toBe(
      true,
    ),
  )
  await waitFor(() => expect(screen.getByTestId("page-path").textContent).toBe("/agents"))
})

test("a nested row owns its menu button apart from its parent; a nested row never shows a number", async () => {
  setup("desktop")
  await expandBlog()
  const parentFrame = body().getByRole("link", { name: /Blog/ }).parentElement!
  const nestedFrame = body().getByRole("link", { name: /builder/ }).parentElement!
  expect(parentFrame.contains(nestedFrame)).toBe(false)
  const menu = within(nestedFrame).getByRole("button", { name: "More actions" })
  expect(parentFrame.contains(menu)).toBe(false)
  fireEvent.keyDown(window, { key: "Alt", altKey: true })
  await waitFor(() => expect(within(parentFrame).getByTestId("row-number")).toBeTruthy())
  expect(within(nestedFrame).queryByTestId("row-number")).toBeNull()
})

test("removing the open chat from its row confirms, then leaves its page", async () => {
  const fetchMock = vi.fn(async (url: string, _init?: RequestInit) => {
    const u = String(url)
    if (u.includes("/v1/sessions/remove"))
      return new Response(JSON.stringify({ results: [{ target: "s3", status: "done" }] }), {
        status: 200,
      })
    return new Response(JSON.stringify({ epics: rows, follow_ups: [] }), { status: 200 })
  })
  setup("desktop", "/agents/s3")
  vi.stubGlobal("fetch", fetchMock)
  const chat = await body().findByRole("link", { name: /^Chat talk/ })
  fireEvent.click(within(chat.closest("li")!).getByRole("button", { name: "More actions" }))
  fireEvent.click(await screen.findByRole("menuitem", { name: "Remove" }))
  expect(
    (await screen.findByText("Stop and remove talk? Its session is deleted.")).textContent,
  ).toBeTruthy()
  expect(fetchMock.mock.calls.some(([u]) => String(u).includes("/v1/sessions/remove"))).toBe(false)
  fireEvent.click(screen.getByRole("button", { name: "Remove" }))
  await waitFor(() =>
    expect(fetchMock.mock.calls.some(([u]) => String(u).includes("/v1/sessions/remove"))).toBe(
      true,
    ),
  )
  await waitFor(() => expect(screen.getByTestId("page-path").textContent).toBe("/agents"))
})

const altDown = () => fireEvent.keyDown(window, { key: "Alt", altKey: true })
const altUp = () => fireEvent.keyUp(window, { key: "Alt" })

test("Alt with a row menu open leaves it open; the number shows, and releasing restores the button", async () => {
  setup("desktop")
  const shop = await body().findByRole("link", { name: /Shop/ })
  const li = within(shop.closest("li")!)
  fireEvent.click(li.getByRole("button", { name: "More actions" }))
  await screen.findAllByRole("menuitem")
  altDown()
  expect(screen.getAllByRole("menuitem").length).toBeGreaterThan(0)
  expect(li.getByTestId("row-number").textContent).toBe("2")
  altUp()
  expect(li.queryByTestId("row-number")).toBeNull()
  expect(li.getByRole("button", { name: "More actions" })).toBeTruthy()
})

test("Alt with the Hold dialog open leaves it open", async () => {
  const fetchMock = vi.fn(
    async (url: string) =>
      new Response(
        JSON.stringify(
          String(url).includes("/v1/workspaces/hold")
            ? {
                held: [],
                results: [
                  {
                    target: "p:shop",
                    status: "refused",
                    reason: "needs-yes",
                    sessions: ["designer"],
                  },
                ],
              }
            : { epics: rows, follow_ups: [] },
        ),
        { status: 200 },
      ),
  )
  setup("desktop")
  vi.stubGlobal("fetch", fetchMock)
  const shop = await body().findByRole("link", { name: /Shop/ })
  fireEvent.click(within(shop.closest("li")!).getByRole("button", { name: "More actions" }))
  fireEvent.click(await screen.findByRole("menuitem", { name: "Hold" }))
  await screen.findByText(/Holding Shop ends its live sessions/)
  altDown()
  expect(screen.getByText(/Holding Shop ends its live sessions/)).toBeTruthy()
  altUp()
})

test("Alt with a chat's Remove confirmation open leaves it open", async () => {
  setup("desktop")
  const chat = await body().findByRole("link", { name: /^Chat talk/ })
  fireEvent.click(within(chat.closest("li")!).getByRole("button", { name: "More actions" }))
  fireEvent.click(await screen.findByRole("menuitem", { name: "Remove" }))
  await screen.findByText("Stop and remove talk? Its session is deleted.")
  altDown()
  expect(screen.getByText("Stop and remove talk? Its session is deleted.")).toBeTruthy()
  altUp()
})

test("on a tablet a row click closes the sidebar", async () => {
  setup("tablet")
  fireEvent.click(screen.getByRole("button", { name: "Show sidebar" }))
  const link = await body().findByRole("link", { name: /Shop/ })
  fireEvent.click(link)
  await waitFor(() =>
    expect(document.querySelector('aside[aria-label="Sidebar"]')!.getAttribute("data-open")).toBe(
      "false",
    ),
  )
})
