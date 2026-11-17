import { act, cleanup, screen, waitFor } from "@testing-library/react"
import { createMemoryRouter, RouterProvider } from "react-router"
import { afterEach, describe, expect, test, vi } from "vitest"
import { session } from "../agents/agentsTestKit"
import { ConnectionContext, type Connection, type ConnectionState } from "../connection"
import type { DesignDraft } from "../generated/server"
import { renderThemed } from "../ui/render"
import { NewItemHost } from "./NewItemHost"
import { startNewDesign } from "../design/newDesign"
import { closeNewItem, openNewAgent } from "./newItemOpen"
import { ShellContext, type Shell } from "./ShellContext"

afterEach(() => {
  closeNewItem()
  cleanup()
  localStorage.clear()
  vi.unstubAllGlobals()
})

const projects = ["alpha", "beta", "gamma"]
const epics = [
  { address: "beta:feat", addresses: ["beta:old-feat"], project: "beta" },
  { address: "gamma:other", project: "gamma" },
]

/** `created` is what the server answers a draft save with. */
function mount(path: string, created?: DesignDraft, closeSliding = () => {}) {
  const fetchMock = vi.fn(async (url: string, _init?: RequestInit) => {
    const u = String(url)
    const body = u.includes("worktrees")
      ? projects.map((project) => ({ project, root: `/${project}`, worktrees: [], tiers: {} }))
      : u.includes("drafts")
        ? { draft: created }
        : { epics, follow_ups: [], projects }
    return new Response(JSON.stringify(body), { status: 200 })
  })
  vi.stubGlobal("fetch", fetchMock)
  const state = {
    snapshot: { sessions: [session("s1", "gamma")] },
    connected: true,
    loadedOnce: true,
    tick: 0,
  } as unknown as ConnectionState
  const conn = {
    getState: () => state,
    subscribe: () => () => {},
    onEvent: () => () => {},
  } as unknown as Connection
  const router = createMemoryRouter([{ path: "*", element: <NewItemHost /> }], {
    initialEntries: [path],
  })
  renderThemed(
    <ConnectionContext.Provider value={conn}>
      <ShellContext.Provider value={{ closeSliding } as Shell}>
        <RouterProvider router={router} />
      </ShellContext.Provider>
    </ConnectionContext.Provider>,
  )
  return { router, fetchMock }
}

const selected = async () =>
  ((await screen.findByRole("combobox", { name: "Project" })) as HTMLSelectElement).value
const projectIs = async (want: string) => waitFor(async () => expect(await selected()).toBe(want))

describe("the project a new blank draft starts on", () => {
  test.each([
    ["/e/beta:feat", "beta"],
    ["/e/beta:old-feat", "beta"],
    ["/d/gamma:other", "gamma"],
    ["/agents/s1", "gamma"],
  ])("from %s New chat starts on %s", async (path, want) => {
    mount(path)
    act(openNewAgent)
    await projectIs(want)
  })

  test.each(["/stats", "/settings"])("on %s New chat keeps today's rule", async (path) => {
    mount(path)
    act(openNewAgent)
    await projectIs("alpha")
  })

  test("New chat uses the stored one-project Epics filter when the page has no project", async () => {
    localStorage.setItem("gx.epics.project", JSON.stringify(["gamma"]))
    mount("/epics")
    act(openNewAgent)
    await projectIs("gamma")
  })

  test("a restored chat draft with text keeps its own project", async () => {
    localStorage.setItem(
      "gx.newSession.draft",
      JSON.stringify({ project: "alpha", prompt: "hello" }),
    )
    mount("/e/beta:feat")
    act(openNewAgent)
    await projectIs("alpha")
  })
})

test("New design renders no dialog, creates a draft, opens its page and closes the sliding sidebar", async () => {
  const created: DesignDraft = {
    id: "d9",
    project: "",
    slug: "",
    idea: "",
    label: "Untitled design",
    images: [],
    revision: 1,
    edited_at: "",
  }
  const closeSliding = vi.fn()
  const { router, fetchMock } = mount("/epics", created, closeSliding)
  await act(() => startNewDesign())
  await waitFor(() => expect(router.state.location.pathname).toBe("/drafts/d9"))
  expect(screen.queryByRole("dialog")).toBeNull()
  expect(closeSliding).toHaveBeenCalledTimes(1)
  expect(router.state.location.state).toEqual({ focusPrompt: true })
  const call = fetchMock.mock.calls.find((c) => String(c[0]).endsWith("/drafts/save"))
  const init = call?.[1] as RequestInit | undefined
  expect(JSON.parse(String(init?.body))).toMatchObject({
    idea: "",
    revision: 0,
  })
})
