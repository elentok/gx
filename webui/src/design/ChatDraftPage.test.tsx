import { act, cleanup, fireEvent, screen } from "@testing-library/react"
import { afterEach, expect, test, vi } from "vitest"
import { MemoryRouter, Route, Routes, useLocation } from "react-router"
import { ConnectionContext, type Connection } from "../connection"
import { EventChatDraftsChanged, type ChatDraft } from "../generated/server"
import { renderThemed } from "../ui/render"
import { ChatDraftPage } from "./ChatDraftPage"
import type { ChatDraftOutcome, ChatDraftsClient, ChatStartOutcome } from "./chatDrafts"

afterEach(() => {
  cleanup()
  vi.useRealTimers()
})

/** An in-memory chat draft store holding one draft, c1. */
function fakeDrafts(over: Partial<ChatDraft> = {}) {
  const stored: ChatDraft = {
    id: "c1",
    project: "gx",
    name: "split-auth",
    harness: "claude",
    model: "opus",
    effort: "high",
    brief: "Split the auth work out.",
    folder: "worktree",
    dir: "",
    worktree: "split-auth-abc123",
    worktree_path: "",
    start_error: "",
    revision: 3,
    edited_at: "",
    label: "split-auth",
    ...over,
  }
  const save = vi.fn(async (body: { brief: string }): Promise<ChatDraftOutcome> => {
    Object.assign(stored, { brief: body.brief, revision: stored.revision + 1 })
    return { kind: "ok", draft: { ...stored } }
  })
  const remove = vi.fn(async (): Promise<ChatDraftOutcome> => ({ kind: "deleted" }))
  const read = vi.fn(async (): Promise<ChatDraftOutcome> => ({ kind: "ok", draft: { ...stored } }))
  const start = vi.fn(
    async (): Promise<ChatStartOutcome> => ({ kind: "started", session: "sess-9" }),
  )
  const client: ChatDraftsClient = { read, save, remove, start }
  return { client, read, save, remove, start, stored }
}

/** Shows where the router is, so a redirect is visible. */
function Where() {
  const { pathname, state } = useLocation()
  return (
    <p data-testid="where">
      {pathname} {(state as { note?: string } | null)?.note}
    </p>
  )
}
const where = () => screen.getByTestId("where").textContent

async function mount(fake: ReturnType<typeof fakeDrafts>, viMode = false) {
  const listeners = new Set<(address: string, type: string) => void>()
  const connState = { snapshot: { sessions: [], vi_mode: viMode } }
  const connection = {
    refresh: () => {},
    getState: () => connState,
    subscribe: () => () => {},
    onEvent: (l: (address: string, type: string) => void) => {
      listeners.add(l)
      return () => listeners.delete(l)
    },
  } as unknown as Connection
  await act(async () => {
    renderThemed(
      <ConnectionContext.Provider value={connection}>
        <MemoryRouter initialEntries={["/chat-drafts/c1"]}>
          <Routes>
            <Route path="/epics" element={<Where />} />
            <Route path="/agents/*" element={<Where />} />
            <Route path="*" element={<ChatDraftPage id="c1" drafts={fake.client} />} />
          </Routes>
        </MemoryRouter>
      </ConnectionContext.Provider>,
    )
  })
  return { emit: (type: string) => act(async () => listeners.forEach((l) => l("", type))) }
}
const brief = () => screen.getByRole("textbox", { name: "Brief" }) as HTMLTextAreaElement
const typeBrief = (value: string) => fireEvent.change(brief(), { target: { value } })
const wait = (ms: number) => act(async () => void vi.advanceTimersByTime(ms))

const startButton = () => screen.getByRole("button", { name: "Start chat" }) as HTMLButtonElement
const clickStart = () => act(async () => fireEvent.click(startButton()))

test("Start posts the draft and goes to the new chat", async () => {
  const fake = fakeDrafts()
  await mount(fake)
  await clickStart()
  expect(fake.start).toHaveBeenCalledWith("c1")
  expect(where()).toBe("/agents/sess-9 ")
})

test("Start is disabled for a blank brief and while a save is in flight, and says why", async () => {
  vi.useFakeTimers()
  await mount(fakeDrafts())
  expect(startButton().disabled).toBe(false)
  typeBrief("   ")
  expect(startButton().disabled).toBe(true)
  expect(screen.getByText("Write a brief to start.")).toBeTruthy()
  typeBrief("Something")
  expect(startButton().disabled).toBe(true)
  expect(screen.getByText("Saving the brief...")).toBeTruthy()
  await wait(800)
  expect(startButton().disabled).toBe(false)
})

test("a failed Start shows the error and the worktree, keeps the brief, and Start posts again", async () => {
  const fake = fakeDrafts()
  fake.start.mockImplementationOnce(async () => {
    fake.stored.worktree_path = "/work/split-auth-abc123"
    return { kind: "failed", message: "harness not found" }
  })
  await mount(fake)
  await clickStart()
  expect(screen.getByRole("alert").textContent).toContain("harness not found")
  expect(screen.getByRole("alert").textContent).toContain(
    "Start made a worktree at /work/split-auth-abc123; pressing Start again runs there",
  )
  expect(brief().value).toBe("Split the auth work out.")
  expect(fake.remove).not.toHaveBeenCalled()
  await clickStart()
  expect(fake.start).toHaveBeenCalledTimes(2)
  expect(where()).toBe("/agents/sess-9 ")
})

test("the facts and the brief render and Start is enabled", async () => {
  await mount(fakeDrafts())
  for (const text of [
    "gx",
    "claude",
    "opus",
    "high",
    "split-auth",
    "New worktree split-auth-abc123",
  ])
    expect(screen.getAllByText(text).length).toBeGreaterThan(0)
  expect(brief().value).toBe("Split the auth work out.")
  expect(startButton().disabled).toBe(false)
})

test("typing saves after 800 ms with the draft's revision", async () => {
  vi.useFakeTimers()
  const fake = fakeDrafts()
  await mount(fake)
  typeBrief("A new brief")
  await wait(799)
  expect(fake.save).not.toHaveBeenCalled()
  await wait(1)
  expect(fake.save).toHaveBeenCalledWith({ id: "c1", brief: "A new brief", revision: 3 })
  typeBrief("A newer brief")
  await wait(800)
  expect(fake.save).toHaveBeenLastCalledWith({ id: "c1", brief: "A newer brief", revision: 4 })
})

test("a stale revision shows the banner and Load theirs takes the server's copy", async () => {
  vi.useFakeTimers()
  const fake = fakeDrafts()
  fake.save.mockResolvedValueOnce({ kind: "conflict" })
  await mount(fake)
  fake.stored.brief = "Their brief"
  fake.stored.revision = 9
  typeBrief("Mine")
  await wait(800)
  expect(screen.getByText("This draft changed on another device.")).toBeTruthy()
  await act(async () => fireEvent.click(screen.getByRole("button", { name: "Load theirs" })))
  expect(brief().value).toBe("Their brief")
  expect(screen.queryByText("This draft changed on another device.")).toBeNull()
  typeBrief("Edited after")
  await wait(800)
  expect(fake.save).toHaveBeenLastCalledWith({ id: "c1", brief: "Edited after", revision: 9 })
})

test("Keep mine saves the brief on the server's revision", async () => {
  vi.useFakeTimers()
  const fake = fakeDrafts()
  fake.save.mockResolvedValueOnce({ kind: "conflict" })
  await mount(fake)
  fake.stored.revision = 9
  typeBrief("Mine")
  await wait(800)
  await act(async () => fireEvent.click(screen.getByRole("button", { name: "Keep mine" })))
  expect(fake.save).toHaveBeenLastCalledWith({ id: "c1", brief: "Mine", revision: 9 })
})

test("a failed save shows Not saved and retries", async () => {
  vi.useFakeTimers()
  const fake = fakeDrafts()
  fake.save.mockRejectedValueOnce(new Error("offline"))
  await mount(fake)
  typeBrief("Mine")
  await wait(800)
  expect(screen.getByText("Not saved")).toBeTruthy()
  await wait(5000)
  expect(fake.save).toHaveBeenCalledTimes(2)
  expect(screen.queryByText("Not saved")).toBeNull()
  expect(fake.stored.brief).toBe("Mine")
})

test("Remove asks, then deletes the draft and goes to the Epics page with the note", async () => {
  const fake = fakeDrafts()
  await mount(fake)
  fireEvent.click(screen.getByRole("button", { name: "Remove" }))
  expect(screen.getByText("Remove this draft?")).toBeTruthy()
  expect(fake.remove).not.toHaveBeenCalled()
  fireEvent.click(screen.getByRole("button", { name: "Keep" }))
  expect(screen.queryByText("Remove this draft?")).toBeNull()
  fireEvent.click(screen.getByRole("button", { name: "Remove" }))
  await act(async () => fireEvent.click(screen.getByRole("button", { name: "Remove" })))
  expect(fake.remove).toHaveBeenCalledWith("c1")
  expect(where()).toBe("/epics This draft was removed.")
})

test("a draft that is gone goes to the Epics page with the note", async () => {
  const fake = fakeDrafts()
  fake.read.mockResolvedValueOnce({ kind: "gone" })
  await mount(fake)
  expect(where()).toBe("/epics This draft was removed.")
})

test("a draft that was started goes to its session", async () => {
  const fake = fakeDrafts()
  fake.read.mockResolvedValueOnce({ kind: "gone", started: "sess-1" })
  await mount(fake)
  expect(where()).toBe("/agents/sess-1 ")
})

test("a chat-drafts-changed event that finds the draft gone leaves the page", async () => {
  const fake = fakeDrafts()
  const { emit } = await mount(fake)
  fake.read.mockResolvedValueOnce({ kind: "gone" })
  await emit(EventChatDraftsChanged)
  expect(where()).toBe("/epics This draft was removed.")
})
