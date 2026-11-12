import { act, cleanup, screen, waitFor } from "@testing-library/react"
import { createMemoryRouter, RouterProvider } from "react-router"
import { afterEach, describe, expect, test, vi } from "vitest"
import { session } from "../agents/agentsTestKit"
import { ConnectionContext, type Connection, type ConnectionState } from "../connection"
import type { DesignDraft } from "../generated/server"
import { renderThemed } from "../ui/render"
import { NewItemHost } from "./NewItemHost"
import { closeNewItem, openNewAgent, openNewDesign } from "./newItemOpen"

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

function draftsBody(u: string, draft?: DesignDraft) {
  if (!draft) return []
  if (u.includes("/read")) return { draft }
  if (u.includes("last-project")) return { project: "" }
  return [{ ...draft, image_count: 0 }]
}

function mount(path: string, restored?: DesignDraft) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string) => {
      const u = String(url)
      const body = u.includes("worktrees")
        ? projects.map((project) => ({ project, root: `/${project}`, worktrees: [], tiers: {} }))
        : u.includes("drafts")
          ? draftsBody(u, restored)
          : { epics, follow_ups: [], projects }
      return new Response(JSON.stringify(body), { status: 200 })
    }),
  )
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
      <RouterProvider router={router} />
    </ConnectionContext.Provider>,
  )
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
  ])("from %s both forms start on %s", async (path, want) => {
    mount(path)
    act(() => openNewDesign())
    await projectIs(want)
    act(closeNewItem)
    act(openNewAgent)
    await projectIs(want)
  })

  test.each(["/stats", "/settings"])("on %s the forms keep today's rule", async (path) => {
    mount(path)
    act(() => openNewDesign())
    await projectIs("alpha")
    act(closeNewItem)
    act(openNewAgent)
    await projectIs("alpha")
  })

  test("the one-project Epics filter wins over the first project where no page has one", async () => {
    localStorage.setItem("gx.epics.project", JSON.stringify(["gamma"]))
    mount("/epics")
    act(() => openNewDesign())
    await projectIs("gamma")
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

  describe("a restored design draft keeps its own project", () => {
    const restored: DesignDraft = {
      id: "d1",
      project: "alpha",
      slug: "",
      idea: "hello",
      images: [],
      revision: 1,
      edited_at: "",
    }

    test("over the route's project", async () => {
      mount("/e/beta:feat", restored)
      act(() => openNewDesign())
      await projectIs("alpha")
    })

    test("over the Epics page's filtered project", async () => {
      localStorage.setItem("gx.epics.project", JSON.stringify(["gamma"]))
      mount("/epics", restored)
      act(() => openNewDesign())
      await projectIs("alpha")
    })
  })
})
