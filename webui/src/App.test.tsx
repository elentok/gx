import { act, fireEvent, screen, waitFor } from "@testing-library/react"
import { afterEach, describe, expect, test, vi } from "vitest"
import { App } from "./App"
import { readRoundDraft, writeRoundDraft } from "./drafts"
import type { Snapshot } from "./snapshot"
import { renderThemed } from "./ui/render"

const snapshotListing = (...ids: string[]) =>
  ({
    seq: 1,
    budget: {
      day: "",
      total: 0,
      soft_limit: 0,
      hard_limit: 0,
      budget_paused: false,
      hard_latched: false,
    },
    health: {
      cpu: { level: 0, state: "unknown", reason: "waiting", read_at: "" },
      memory: { level: 0, state: "unknown", reason: "waiting", read_at: "" },
      disk: { level: 0, state: "unknown", reason: "waiting", read_at: "" },
    },
    tickets: [],
    pending: [],
    sessions: ids.map((id) => ({ id })),
    epics: [],
    recently_finished: [],
  }) as unknown as Snapshot

test("only a live snapshot deletes the drafts of sessions it does not list", async () => {
  writeRoundDraft("a", "r", { n: 1 })
  writeRoundDraft("b", "r", { n: 2 })

  let release: (s: Snapshot) => void = () => {}
  const live = new Promise<Snapshot>((resolve) => (release = resolve))
  renderThemed(
    <App
      deps={{
        cache: { load: () => snapshotListing("a"), save: () => {} },
        fetchSnapshot: () => live,
        openStream: () => new Promise(() => {}),
      }}
    />,
  )

  expect(readRoundDraft("b", "r")).toEqual({ n: 2 })

  await act(async () => release(snapshotListing("a")))
  await waitFor(() => expect(readRoundDraft("b", "r")).toBeNull())
  expect(readRoundDraft("a", "r")).toEqual({ n: 1 })
})

test("a live snapshot with a null session list renders and deletes no drafts", async () => {
  writeRoundDraft("a", "r", { n: 1 })

  const unreadable = { ...snapshotListing(), sessions: null } as unknown as Snapshot
  let release: (s: Snapshot) => void = () => {}
  const live = new Promise<Snapshot>((resolve) => (release = resolve))
  renderThemed(
    <App
      deps={{
        cache: { load: () => snapshotListing("a"), save: () => {} },
        fetchSnapshot: () => live,
        openStream: () => new Promise(() => {}),
      }}
    />,
  )

  await act(async () => release(unreadable))
  expect(readRoundDraft("a", "r")).toEqual({ n: 1 })
})

describe("the Backlog page", () => {
  const backlog = {
    projects: [
      {
        project: "gx",
        workspaces: [],
        areas: [
          { address: "gx:web", name: "web", open: 3 },
          { address: "gx:docs", name: "docs", open: 0 },
          {
            address: "gx:cli",
            name: "cli",
            open: 2,
            unfinished: { map: "gx:cli-design", to_move: 1 },
          },
        ],
      },
      { project: "empty", workspaces: [], areas: [] },
    ],
  }

  type Post = { url: string; body: unknown }
  function renderBacklog(answer: (post: Post) => unknown) {
    const posts: Post[] = []
    vi.stubGlobal(
      "fetch",
      vi.fn(async (url: string, init?: RequestInit) => {
        if (!init?.method) return new Response(JSON.stringify(backlog))
        const post = { url, body: JSON.parse(String(init.body)) }
        posts.push(post)
        return new Response(JSON.stringify(answer(post)))
      }),
    )
    window.location.hash = "#/backlog"
    renderThemed(
      <App
        deps={{
          cache: { load: () => snapshotListing(), save: () => {} },
          fetchSnapshot: async () => snapshotListing(),
          openStream: () => new Promise(() => {}),
        }}
      />,
    )
    return posts
  }

  afterEach(() => {
    vi.unstubAllGlobals()
    window.location.hash = ""
  })

  test("lists areas with open counts and disables Start Design at zero", async () => {
    renderBacklog(() => ({}))

    expect(await screen.findByText("3 open")).toBeTruthy()
    expect(screen.queryByRole("region", { name: "empty" })).toBeNull()
    const buttons = screen.getAllByRole("button", { name: "Start Design" })
    expect(buttons.map((b) => (b as HTMLButtonElement).disabled)).toEqual([false, true, false])
  })

  test("Start posts the area and hint, and opens the chat", async () => {
    const posts = renderBacklog(() => ({ session: { id: "chat-1" } }))

    fireEvent.click((await screen.findAllByRole("button", { name: "Start Design" }))[0])
    fireEvent.change(screen.getByLabelText("Hint"), { target: { value: " tidy " } })
    fireEvent.click(screen.getByRole("button", { name: "Start" }))

    await waitFor(() => expect(window.location.hash).toBe("#/agents/chat-1"))
    expect(posts).toEqual([
      {
        url: "/v1/backlog/start-design",
        body: { project: "gx", area: "gx:web", hint: "tidy" },
      },
    ])
  })

  test("a refusal shows its message and keeps the dialog", async () => {
    renderBacklog(() => ({ refused: true, reason: "no-open-tickets", message: "Nothing to move." }))

    fireEvent.click((await screen.findAllByRole("button", { name: "Start Design" }))[0])
    fireEvent.click(screen.getByRole("button", { name: "Start" }))

    expect((await screen.findByRole("alert")).textContent).toBe("Nothing to move.")
    expect(screen.getByRole("dialog")).toBeTruthy()
    expect(window.location.hash).toBe("#/backlog")
  })

  test("Finish move posts the map without opening a chat", async () => {
    const posts = renderBacklog(() => ({ address: "gx:cli-design" }))

    fireEvent.click(await screen.findByRole("button", { name: "Finish move" }))

    await waitFor(() => expect(posts).toHaveLength(1))
    expect(posts[0]).toEqual({
      url: "/v1/backlog/finish-move",
      body: { project: "gx", area: "gx:cli", map: "gx:cli-design" },
    })
    expect(screen.queryByRole("dialog")).toBeNull()
    expect(window.location.hash).toBe("#/backlog")
  })
})
