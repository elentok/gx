import { setViMode } from "../vimode"
import { act, cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react"
import { ConfirmHost } from "../shell/ConfirmHost"
import { afterEach, describe, expect, test, vi } from "vitest"
import { MemoryRouter, Route, Routes, useLocation } from "react-router"
import { ConnectionContext, type Connection } from "../connection"
import {
  EventChatDraftsChanged,
  type ChatDraft,
  type ChatDraftSaveRequest,
  type ProjectWorktrees,
} from "../generated/server"
import { renderThemed } from "../ui/render"
import { WideContext } from "../useWide"
import { ChatDraftPage } from "./ChatDraftPage"
import { chatDraftImages } from "./draftImageApi"
import type { LaunchDefaults } from "./chatDraftDefaults"
import type { ChatDraftOutcome, ChatDraftsClient, ChatStartOutcome } from "./chatDrafts"
import type { ChatDraftPickResult, DraftHistoryEntry } from "../generated/ops"
import type { ListHistory, PickChatHistory } from "../history/api"

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
    source_dir: "",
    worktree: "split-auth-abc123",
    worktree_path: "",
    start_error: "",
    revision: 3,
    edited_at: "",
    label: "split-auth",
    images: [],
    ...over,
  }
  const save = vi.fn(async (body: ChatDraftSaveRequest): Promise<ChatDraftOutcome> => {
    const { id: _id, revision: _revision, ...rest } = body
    Object.assign(stored, rest, { revision: stored.revision + 1 })
    return { kind: "ok", draft: { ...stored } }
  })
  const remove = vi.fn(async (): Promise<ChatDraftOutcome> => ({ kind: "deleted" }))
  const read = vi.fn(async (): Promise<ChatDraftOutcome> => ({ kind: "ok", draft: { ...stored } }))
  const start = vi.fn(
    async (): Promise<ChatStartOutcome> => ({ kind: "started", session: "sess-9" }),
  )
  const client: ChatDraftsClient = { create: vi.fn(async () => "c2"), read, save, remove, start }
  return { client, read, save, remove, start, stored }
}

/** Shows where the router is, so a redirect is visible. */
function Where() {
  const { pathname, state } = useLocation()
  const s = state as { note?: string; creating?: boolean } | null
  return (
    <p data-testid="where">
      {pathname} {s?.note}
      {s?.creating ? "creating" : ""}
    </p>
  )
}
const where = () => screen.getByTestId("where").textContent

let claudeTiers: Record<string, string> = { worker: "opus" }
const trees = async (): Promise<ProjectWorktrees[]> => [
  {
    project: "gx",
    root: "/r/gx",
    worktrees: ["/r/gx-wt/main", "/r/gx-wt/feature-x"],
    tiers: {},
    tier_models: { claude: claudeTiers },
  },
  { project: "other", root: "/r/other", worktrees: [], tiers: {} },
]

async function mount(
  fake: ReturnType<typeof fakeDrafts>,
  viMode = false,
  defaults: LaunchDefaults = { model: "", effort: "" },
  wide = true,
  history: { pickHistory?: PickChatHistory; listHistory?: ListHistory } = {},
  workspaces: { name: string; order: number }[] = [],
) {
  const listeners = new Set<(address: string, type: string) => void>()
  setViMode(viMode)
  const connState = { snapshot: { sessions: [], workspaces } }
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
      <WideContext.Provider value={wide}>
        <ConfirmHost />
        <ConnectionContext.Provider value={connection}>
          <MemoryRouter initialEntries={["/chat-drafts/c1"]}>
            <Routes>
              <Route path="/epics" element={<Where />} />
              <Route path="/agents/*" element={<Where />} />
              <Route
                path="*"
                element={
                  <ChatDraftPage
                    id="c1"
                    drafts={fake.client}
                    fetchTrees={trees}
                    fetchDefaults={async () => defaults}
                    {...history}
                  />
                }
              />
            </Routes>
          </MemoryRouter>
        </ConnectionContext.Provider>
      </WideContext.Provider>,
    )
  })
  return {
    emit: (type: string) =>
      act(async () => {
        for (const l of listeners) l("", type)
      }),
  }
}
const brief = () => screen.getByRole("textbox", { name: "Brief" }) as HTMLTextAreaElement
const typeBrief = (value: string) => fireEvent.change(brief(), { target: { value } })
const wait = (ms: number) => act(async () => void vi.advanceTimersByTime(ms))

const startButton = () => screen.getByRole("button", { name: "Start chat" }) as HTMLButtonElement
const startBlocked = () => startButton().getAttribute("aria-disabled") === "true"
/** The text Start's aria-describedby points at, or null without a reason. */
const startReason = () => {
  const id = startButton().getAttribute("aria-describedby")
  return id ? document.getElementById(id)?.textContent : null
}
const clickStart = () => act(async () => fireEvent.click(startButton()))

test("Start posts the draft and goes to the new chat", async () => {
  const fake = fakeDrafts()
  await mount(fake)
  await clickStart()
  expect(fake.start).toHaveBeenCalledWith("c1", undefined)
  expect(where()).toBe("/agents/sess-9 creating")
})

test("Cmd+Enter in the brief starts the chat", async () => {
  const fake = fakeDrafts()
  await mount(fake)
  await act(async () => fireEvent.keyDown(brief(), { key: "Enter", metaKey: true }))
  expect(fake.start).toHaveBeenCalledWith("c1", undefined)
  expect(where()).toBe("/agents/sess-9 creating")
})

test("Ctrl+Enter in the brief starts the chat", async () => {
  const fake = fakeDrafts()
  await mount(fake)
  await act(async () => fireEvent.keyDown(brief(), { key: "Enter", ctrlKey: true }))
  expect(fake.start).toHaveBeenCalledWith("c1", undefined)
})

test("plain Enter and Cmd+Enter on a blank brief do not start", async () => {
  const fake = fakeDrafts({ brief: "   " })
  await mount(fake)
  await act(async () => fireEvent.keyDown(brief(), { key: "Enter", metaKey: true }))
  await act(async () => fireEvent.keyDown(brief(), { key: "Enter" }))
  expect(fake.start).not.toHaveBeenCalled()
})

test("Start is blocked for a blank brief only, never while typing, with no reason text", async () => {
  vi.useFakeTimers()
  await mount(fakeDrafts())
  expect(startBlocked()).toBe(false)
  expect(startButton().getAttribute("disabled")).toBeNull()
  typeBrief("   ")
  expect(startBlocked()).toBe(true)
  expect(startReason()).toBeNull()
  expect(screen.queryByText("Write a brief to start.")).toBeNull()
  typeBrief("Something")
  expect(startBlocked()).toBe(false)
  expect(startReason()).toBeNull()
  expect(screen.getByRole("status").textContent).toBe("")
})

test("Start inside the debounce window sends the typed text with the start and saves nothing", async () => {
  vi.useFakeTimers()
  const fake = fakeDrafts()
  await mount(fake)
  typeBrief("last words")
  await act(async () => fireEvent.click(startButton()))
  expect(fake.start).toHaveBeenCalledTimes(1)
  expect(fake.start).toHaveBeenCalledWith("c1", { brief: "last words" })
  expect(fake.save).not.toHaveBeenCalled()
})

test("Cmd+Enter does nothing while Start is blocked, and a click on blocked Start does not start", async () => {
  const fake = fakeDrafts({ brief: "" })
  await mountOpen(fake)
  await act(async () => fireEvent.keyDown(brief(), { key: "Enter", metaKey: true }))
  await act(async () => fireEvent.click(startButton()))
  expect(fake.start).not.toHaveBeenCalled()
})

test("the footer's Remove is an icon button named Remove draft, and Start does not wrap", async () => {
  await mount(fakeDrafts())
  const remove = screen.getByRole("button", { name: "Remove draft" })
  expect(remove.getAttribute("title")).toBe("Remove draft")
  expect(remove.textContent).toBe("")
  expect(getComputedStyle(startButton()).whiteSpace).toBe("nowrap")
})

test("a blocked Start with a reason opens a tooltip on focus", async () => {
  const upload = vi.spyOn(chatDraftImages, "upload").mockImplementation(() => new Promise(() => {}))
  await mount(fakeDrafts())
  await act(async () =>
    fireEvent.paste(brief(), { clipboardData: { files: [png()], types: ["Files"] } }),
  )
  await waitFor(() => expect(startReason()).toBe("Uploading images..."))
  await act(async () => startButton().focus())
  expect(await screen.findAllByText("Uploading images...")).not.toHaveLength(0)
  expect(document.querySelector("[role=tooltip]")).toBeTruthy()
  upload.mockRestore()
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
  expect(where()).toBe("/agents/sess-9 creating")
})

const control = (name: string) =>
  screen.getByLabelText(name) as HTMLInputElement | HTMLSelectElement
/** Harness, Model and Effort live in the settings popover; tests that use them mount with it open. */
const openSettings = () =>
  act(async () => void fireEvent.click(screen.getByRole("button", { name: "Chat settings" })))
async function mountOpen(...args: Parameters<typeof mount>) {
  const mounted = await mount(...args)
  await openSettings()
  return mounted
}
/** Opens a combobox and clicks the option with this text. */
const pick = (name: string, option: string) => {
  fireEvent.focus(control(name))
  fireEvent.click(screen.getByRole("option", { name: new RegExp(`^${option}`) }))
}
/** The text of the option a chip shows. */
const chipText = (name: string) => {
  const select = control(name) as HTMLSelectElement
  return select.options[select.selectedIndex].text
}

test("the settings and the brief render and Start is enabled", async () => {
  await mountOpen(fakeDrafts())
  expect(control("Project").value).toBe("gx")
  expect(control("Folder").value).toBe("split-auth-abc123")
  expect(control("Harness").value).toBe("claude")
  expect(control("Model").value).toBe("opus")
  expect(control("Effort").value).toBe("high")
  expect(control("Name").value).toBe("split-auth")
  expect(brief().value).toBe("Split the auth work out.")
  expect(startBlocked()).toBe(false)
})

test("a long Name ends in an ellipsis instead of a hard clip", async () => {
  await mountOpen(fakeDrafts())
  expect(getComputedStyle(control("Name")).textOverflow).toBe("ellipsis")
})

test("only tiers with a resolved model are offered as models", async () => {
  await mountOpen(fakeDrafts())
  const values = Array.from((control("Model") as HTMLSelectElement).options).map((o) => o.value)
  // The worker tier is the default row's model, so the pinned draft keeps the one row.
  expect(values).toEqual(["opus"])
})

test("a tier that names the default model, or another tier's model, is not listed twice", async () => {
  claudeTiers = {
    light: "haiku",
    worker: "claude-sonnet-5-5",
    strongest: "claude-sonnet-5-5-20260101",
  }
  try {
    await mountOpen(fakeDrafts({ model: "", effort: "" }), false, {
      model: "claude-sonnet-5-5",
      effort: "high",
    })
    const select = control("Model") as HTMLSelectElement
    const rows = Array.from(select.options).map((o) => [o.value, o.textContent])
    expect(rows).toEqual([
      ["", "Claude Sonnet 5.5"],
      ["haiku", "Claude Haiku"],
    ])
    expect(select.value).toBe("")
  } finally {
    claudeTiers = { worker: "opus" }
  }
})

test("a model pinned to what the default also is stays selected, once", async () => {
  claudeTiers = { worker: "claude-sonnet-5-5" }
  try {
    await mountOpen(fakeDrafts({ model: "claude-sonnet-5-5", effort: "" }), false, {
      model: "claude-sonnet-5-5",
      effort: "high",
    })
    const select = control("Model") as HTMLSelectElement
    expect(Array.from(select.options).map((o) => o.value)).toEqual(["claude-sonnet-5-5"])
    expect(select.value).toBe("claude-sonnet-5-5")
  } finally {
    claudeTiers = { worker: "opus" }
  }
})

test.each([
  ["Harness", "codex", { harness: "codex" }],
  ["Model", "", { model: "" }],
  ["Effort", "max", { effort: "max" }],
  ["Name", "renamed", { name: "renamed" }],
])("changing %s saves once with the revision and only that field", async (label, value, body) => {
  vi.useFakeTimers()
  const fake = fakeDrafts()
  await mount(fake)
  if (label !== "Name") await openSettings()
  fireEvent.change(control(label), { target: { value } })
  await wait(800)
  expect(fake.save).toHaveBeenCalledTimes(1)
  expect(fake.save).toHaveBeenCalledWith({
    id: "c1",
    brief: "Split the auth work out.",
    revision: 3,
    ...body,
  })
})

test.each([
  ["Project", "other", { project: "other" }],
  ["Folder", "Trunk checkout", { folder: "trunk" }],
  ["Folder", "Source folder", { folder: "source" }],
])("picking %s %s from its combobox saves only that field", async (label, option, body) => {
  vi.useFakeTimers()
  const fake = fakeDrafts()
  await mount(fake)
  pick(label, option)
  await wait(800)
  expect(fake.save).toHaveBeenCalledTimes(1)
  expect(fake.save).toHaveBeenCalledWith({
    id: "c1",
    brief: "Split the auth work out.",
    revision: 3,
    ...body,
  })
})

test("typing in Folder filters the list and offers a new worktree", async () => {
  vi.useFakeTimers()
  const fake = fakeDrafts({ folder: "source" })
  await mount(fake)
  fireEvent.focus(control("Folder"))
  fireEvent.change(control("Folder"), { target: { value: "trunk" } })
  expect(screen.queryByRole("option", { name: /Source folder/ })).toBeNull()
  expect(screen.getByRole("option", { name: /Trunk checkout/ })).toBeTruthy()
  fireEvent.change(control("Folder"), { target: { value: "fresh" } })
  fireEvent.click(screen.getByRole("option", { name: /Create worktree/ }))
  await wait(800)
  expect(fake.save).toHaveBeenCalledWith({
    id: "c1",
    brief: "Split the auth work out.",
    revision: 3,
    folder: "worktree",
    worktree: "fresh",
  })
})

test("Folder lists the project's existing worktrees and picking one saves its path", async () => {
  vi.useFakeTimers()
  const fake = fakeDrafts({ folder: "source" })
  await mount(fake)
  fireEvent.focus(control("Folder"))
  // The trunk worktree is the Trunk checkout row, not a second row.
  expect(screen.queryByRole("option", { name: /^main/ })).toBeNull()
  fireEvent.change(control("Folder"), { target: { value: "feat" } })
  fireEvent.click(screen.getByRole("option", { name: /feature-x/ }))
  await wait(800)
  expect(fake.save).toHaveBeenCalledWith({
    id: "c1",
    brief: "Split the auth work out.",
    revision: 3,
    folder: "dir",
    dir: "/r/gx-wt/feature-x",
  })
})

test("the title is the name: editing it saves the name", async () => {
  vi.useFakeTimers()
  const fake = fakeDrafts({ name: "" })
  await mount(fake)
  expect((control("Name") as HTMLInputElement).placeholder).toBe("Untitled chat")
  fireEvent.change(control("Name"), { target: { value: "auth" } })
  await wait(800)
  expect(fake.save).toHaveBeenCalledWith({
    id: "c1",
    brief: "Split the auth work out.",
    revision: 3,
    name: "auth",
  })
})

test("an unset model and effort show what the chat will launch with, never Default", async () => {
  await mountOpen(fakeDrafts({ model: "", effort: "" }), false, {
    model: "claude-opus-4-5",
    effort: "high",
  })
  expect(chipText("Model")).toMatch(/Opus/)
  expect(chipText("Effort")).toBe("high")
  expect(screen.queryByText("Default")).toBeNull()
})

test("an unresolved default leaves the plain model name and shows Auto for effort", async () => {
  claudeTiers = {}
  try {
    await mountOpen(fakeDrafts({ model: "", effort: "" }))
    expect(chipText("Model")).toBe("Model")
    expect(chipText("Effort")).toBe("Auto")
    expect((control("Effort") as HTMLSelectElement).disabled).toBe(true)
  } finally {
    claudeTiers = { worker: "opus" }
  }
})

test("an unset model shows the worker tier's model, not the probe's default", async () => {
  await mountOpen(fakeDrafts({ model: "", effort: "" }), false, {
    model: "claude-haiku-4-5",
    effort: "",
  })
  expect(chipText("Model")).toBe("Claude Opus")
})

test("picking an effort with no model also saves the default model", async () => {
  vi.useFakeTimers()
  const fake = fakeDrafts({ model: "", effort: "" })
  await mountOpen(fake, false, { model: "claude-opus-4-5", effort: "" })
  fireEvent.change(control("Effort"), { target: { value: "max" } })
  await wait(800)
  expect(fake.save).toHaveBeenCalledWith({
    id: "c1",
    brief: "Split the auth work out.",
    revision: 3,
    effort: "max",
    model: "opus",
  })
})

const png = (name = "a.png") =>
  new File([new Uint8Array([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a])], name, {
    type: "image/png",
  })

test("Add image opens the file picker, and a chosen file uploads to the draft", async () => {
  const upload = vi.spyOn(chatDraftImages, "upload").mockResolvedValue("img1")
  await mount(fakeDrafts())
  const click = vi.spyOn(screen.getByTestId("image-input"), "click")
  fireEvent.click(screen.getByRole("button", { name: "Add image" }))
  expect(click).toHaveBeenCalled()
  await act(async () =>
    fireEvent.change(screen.getByTestId("image-input"), { target: { files: [png()] } }),
  )
  await screen.findByText("1 of 10 images")
  expect(upload).toHaveBeenCalledWith("c1", expect.any(File))
  upload.mockRestore()
})

test("a pasted image uploads, and Start waits for an upload in flight", async () => {
  let finish: (id: string) => void = () => {}
  const upload = vi
    .spyOn(chatDraftImages, "upload")
    .mockImplementation(() => new Promise((resolve) => (finish = resolve)))
  await mount(fakeDrafts())
  await act(async () =>
    fireEvent.paste(brief(), { clipboardData: { files: [png()], types: ["Files"] } }),
  )
  await waitFor(() => expect(upload).toHaveBeenCalledWith("c1", expect.any(File)))
  expect(startBlocked()).toBe(true)
  expect(startReason()).toBe("Uploading images...")
  await act(async () => finish("img1"))
  expect(startBlocked()).toBe(false)
  upload.mockRestore()
})

test("a stale revision on a settings change shows the banner", async () => {
  vi.useFakeTimers()
  const fake = fakeDrafts()
  fake.save.mockResolvedValueOnce({ kind: "conflict" })
  await mountOpen(fake)
  fireEvent.change(control("Effort"), { target: { value: "low" } })
  await wait(800)
  expect(screen.getByText("This draft changed on another device.")).toBeTruthy()
})

test("an empty draft opens with the placeholder, a disabled Start and its blocker", async () => {
  await mountOpen(
    fakeDrafts({ brief: "", name: "", model: "", effort: "", label: "Untitled chat" }),
  )
  expect(brief().value).toBe("")
  expect(brief().placeholder).toBe("What should the new chat start from?")
  expect(startBlocked()).toBe(true)
  expect(startReason()).toBeNull()
  expect(control("Model").value).toBe("")
  // The draft has no brief and no images, so Remove asks nothing.
  await act(async () => fireEvent.click(screen.getByRole("button", { name: "Remove draft" })))
  expect(screen.queryByRole("alertdialog")).toBeNull()
  await waitFor(() => expect(where()).toBe("/epics This draft was removed."))
})

test("once Start made a worktree the folder is that worktree and cannot be edited", async () => {
  await mount(fakeDrafts({ worktree_path: "/work/split-auth-abc123" }))
  expect(screen.queryByLabelText("Folder")).toBeNull()
  expect(screen.getByText("Worktree /work/split-auth-abc123")).toBeTruthy()
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
  expect(screen.getByText("Not saved, retrying")).toBeTruthy()
  await wait(5000)
  expect(fake.save).toHaveBeenCalledTimes(2)
  expect(screen.queryByText("Not saved, retrying")).toBeNull()
  expect(fake.stored.brief).toBe("Mine")
})

test("Remove asks, then deletes the draft and goes to the Epics page with the note", async () => {
  const fake = fakeDrafts()
  await mount(fake)
  fireEvent.click(screen.getByRole("button", { name: "Remove draft" }))
  const popover = await screen.findByRole("alertdialog")
  expect(fake.remove).not.toHaveBeenCalled()
  fireEvent.click(within(popover).getByRole("button", { name: /^Cancel/ }))
  await waitFor(() => expect(screen.queryByRole("alertdialog")).toBeNull())
  expect(fake.remove).not.toHaveBeenCalled()
  fireEvent.click(screen.getByRole("button", { name: "Remove draft" }))
  const again = await screen.findByRole("alertdialog")
  await act(async () => fireEvent.click(within(again).getByRole("button", { name: /^Remove/ })))
  await waitFor(() => expect(fake.remove).toHaveBeenCalledWith("c1"))
  expect(where()).toBe("/epics This draft was removed.")
})

test("an empty chat draft removes with no confirm", async () => {
  const fake = fakeDrafts({ brief: "", images: [] })
  await mount(fake)
  await act(async () => fireEvent.click(screen.getByRole("button", { name: "Remove draft" })))
  expect(screen.queryByRole("alertdialog")).toBeNull()
  await waitFor(() => expect(fake.remove).toHaveBeenCalledWith("c1"))
})

test("with workspaces the ⋯ lists Rename, Move to workspace, Remove, and Remove is red", async () => {
  await mount(fakeDrafts(), false, { model: "", effort: "" }, true, {}, [
    { name: "Default", order: 0 },
    { name: "Work", order: 1 },
  ])
  fireEvent.click(screen.getByRole("button", { name: "More actions" }))
  const items = await screen.findAllByRole("menuitem")
  expect(items.map((i) => i.textContent)).toEqual(["Rename", "Move to workspace›", "Remove"])
  expect(getComputedStyle(items[2]!).color).not.toBe(getComputedStyle(items[0]!).color)
})

test("a chat draft with an image and no brief asks before Remove", async () => {
  const fake = fakeDrafts({ brief: "", images: ["img1"] })
  await mount(fake)
  await act(async () => fireEvent.click(screen.getByRole("button", { name: "Remove draft" })))
  expect(await screen.findByRole("alertdialog")).toBeTruthy()
  expect(fake.remove).not.toHaveBeenCalled()
})

test("Rename from the ⋯ focuses the Name field", async () => {
  await mount(fakeDrafts())
  fireEvent.click(screen.getByRole("button", { name: "More actions" }))
  fireEvent.click(await screen.findByRole("menuitem", { name: "Rename" }))
  await waitFor(() => expect(document.activeElement).toBe(screen.getByLabelText("Name")))
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
  expect(where()).toBe("/agents/sess-1 creating")
})

test("a chat-drafts-changed event that reads gone does not leave the page while Start runs", async () => {
  const fake = fakeDrafts()
  let finish!: (r: ChatStartOutcome) => void
  fake.start.mockImplementationOnce(() => new Promise((r) => (finish = r)))
  const { emit } = await mount(fake)
  await clickStart()
  fake.read.mockResolvedValueOnce({ kind: "gone" })
  await emit(EventChatDraftsChanged)
  expect(screen.queryByTestId("where")).toBeNull()
  await act(async () => finish({ kind: "gone", started: "sess-4" }))
  expect(where()).toBe("/agents/sess-4 creating")
})

test("a late save answer after Start changes nothing, and no save goes out while Start runs", async () => {
  vi.useFakeTimers()
  const fake = fakeDrafts()
  let answer!: (r: ChatDraftOutcome) => void
  fake.save.mockImplementationOnce(() => new Promise((r) => (answer = r)))
  let finish!: (r: ChatStartOutcome) => void
  fake.start.mockImplementationOnce(() => new Promise((r) => (finish = r)))
  await mount(fake)
  typeBrief("first")
  await wait(800)
  expect(fake.save).toHaveBeenCalledTimes(1)
  typeBrief("second")
  await clickStart()
  expect(fake.start).toHaveBeenCalledWith("c1", { brief: "second" })
  await act(async () => answer({ kind: "gone", started: "sess-7" }))
  expect(screen.queryByTestId("where")).toBeNull()
  await wait(5000)
  await act(async () => void window.dispatchEvent(new Event("pagehide")))
  expect(fake.save).toHaveBeenCalledTimes(1)
  await act(async () => finish({ kind: "started", session: "sess-9" }))
  expect(where()).toBe("/agents/sess-9 creating")
})

test("a late conflict from a save sent before Start raises no banner", async () => {
  vi.useFakeTimers()
  const fake = fakeDrafts()
  let answer!: (r: ChatDraftOutcome) => void
  fake.save.mockImplementationOnce(() => new Promise((r) => (answer = r)))
  fake.start.mockImplementationOnce(() => new Promise(() => {}))
  await mount(fake)
  typeBrief("first")
  await wait(800)
  await clickStart()
  await act(async () => answer({ kind: "conflict" }))
  expect(screen.queryByText("This draft changed on another device.")).toBeNull()
})

test("Start does not start while the conflict banner shows", async () => {
  vi.useFakeTimers()
  const fake = fakeDrafts()
  fake.save.mockResolvedValueOnce({ kind: "conflict" })
  await mount(fake)
  typeBrief("Mine")
  await wait(800)
  expect(screen.getByText("This draft changed on another device.")).toBeTruthy()
  await clickStart()
  await act(async () => fireEvent.keyDown(brief(), { key: "Enter", metaKey: true }))
  expect(fake.start).not.toHaveBeenCalled()
})

test("a refused edit keeps the typed text, and the next autosave uses the re-read revision", async () => {
  vi.useFakeTimers()
  const fake = fakeDrafts()
  fake.start.mockImplementationOnce(async () => {
    fake.stored.revision = 8
    return { kind: "failed", message: "no such model" }
  })
  await mount(fake)
  typeBrief("typed")
  await clickStart()
  expect(screen.getByRole("alert").textContent).toContain("no such model")
  expect(brief().value).toBe("typed")
  // The refused edit left the draft as it was, so the text is still unsaved and goes out now.
  expect(fake.save).toHaveBeenCalledTimes(1)
  expect(fake.save).toHaveBeenLastCalledWith({ id: "c1", brief: "typed", revision: 8 })
  await clickStart()
  expect(fake.start).toHaveBeenCalledTimes(2)
  expect(where()).toBe("/agents/sess-9 creating")
})

test("changed settings go in Start's edit and unchanged ones stay out", async () => {
  vi.useFakeTimers()
  const fake = fakeDrafts()
  await mountOpen(fake)
  fireEvent.change(control("Effort"), { target: { value: "max" } })
  await clickStart()
  expect(fake.save).not.toHaveBeenCalled()
  expect(fake.start).toHaveBeenCalledWith("c1", {
    brief: "Split the auth work out.",
    effort: "max",
  })
})

test("a chat-drafts-changed event that finds the draft gone leaves the page", async () => {
  const fake = fakeDrafts()
  const { emit } = await mount(fake)
  fake.read.mockResolvedValueOnce({ kind: "gone" })
  await emit(EventChatDraftsChanged)
  expect(where()).toBe("/epics This draft was removed.")
})

test("on a wide screen the settings are one pill too, and the paperclip is in the footer", async () => {
  await mount(fakeDrafts())
  expect(screen.getByRole("button", { name: "Chat settings" })).toBeTruthy()
  expect(screen.queryByRole("combobox", { name: "Effort" })).toBeNull()
  await openSettings()
  for (const name of ["Harness", "Model", "Effort"]) expect(control(name)).toBeTruthy()
  const attach = screen.getByRole("button", { name: "Add image" })
  expect(control("Name").parentElement?.parentElement?.lastElementChild?.contains(attach)).toBe(
    true,
  )
})

test("on a phone one pill sums up the settings, and its sheet changes them", async () => {
  await mount(
    fakeDrafts({ model: "", effort: "" }),
    false,
    {
      model: "claude-opus-4-5",
      effort: "high",
    },
    false,
  )
  expect(screen.queryByRole("combobox", { name: "Effort" })).toBeNull()
  const pill = screen.getByRole("button", { name: "Chat settings" })
  expect(pill.textContent).toMatch(/Opus.*· high/)
  await openSettings()
  // A popover, not a bottom sheet.
  expect(screen.queryByTestId("grab-handle")).toBeNull()
  expect(screen.getByRole("dialog", { name: "Chat settings" })).toBeTruthy()
  const effort = screen.getByRole("combobox", { name: "Effort" })
  fireEvent.change(effort, { target: { value: "max" } })
  expect(screen.getByRole("button", { name: "Chat settings" }).textContent).toMatch(/· max/)
  // A loaded run can drop the first Escape before the popover is ready for it; Escape on an open
  // popover is idempotent, so poll the whole action.
  await waitFor(() => {
    const dialog = screen.queryByRole("dialog", { name: "Chat settings" })
    if (dialog) fireEvent.keyDown(dialog, { key: "Escape" })
    expect(screen.queryByRole("combobox", { name: "Effort" })).toBeNull()
  })
})

describe("History", () => {
  const entry: DraftHistoryEntry = {
    id: "e1",
    savedAt: new Date().toISOString(),
    text: "Restored brief",
    imageCount: 1,
    imagesDropped: false,
    project: "other",
    slug: "",
    name: "restored-name",
    harness: "codex",
    model: "gpt",
    effort: "low",
    folder: "source",
    composer: "",
  }
  const listHistory = vi.fn<ListHistory>(async () => ({
    list: "chat-drafts",
    max: 50,
    entries: [entry],
  }))
  // The server keeps the page's settings, so the entry's other values must not show.
  const pickResult = (): ChatDraftPickResult => ({
    notices: ["1 image missing"],
    draft: {
      ...fakeDrafts().stored,
      name: "restored-name",
      brief: "Restored brief",
      revision: 10,
      images: ["img1"],
      label: "restored-name",
    },
  })
  const history = () => screen.getByRole("button", { name: "History" })
  const restore = () => screen.getByRole("button", { name: "Restore" }) as HTMLButtonElement
  // The phone popup picks in two steps: tap a row, then Restore.
  const openPopup = async () => {
    await act(async () => void fireEvent.click(history()))
    await act(async () => void fireEvent.click(screen.getByText("Restored brief")))
  }
  const mountPhone = (fake: ReturnType<typeof fakeDrafts>, pickHistory?: PickChatHistory) =>
    mount(fake, false, undefined, false, { listHistory, pickHistory })

  test("a wide page shows the History text and a phone shows the icon only", async () => {
    await mount(fakeDrafts())
    expect(history().textContent).toBe("History")
    cleanup()
    await mountPhone(fakeDrafts())
    expect(history().textContent).toBe("")
    expect(history().getAttribute("aria-label")).toBe("History")
  })

  test("the button opens the Chat drafts list only", async () => {
    vi.useFakeTimers()
    await mount(fakeDrafts(), false, undefined, true, { listHistory })
    await act(async () => void fireEvent.click(history()))
    expect(listHistory).toHaveBeenCalledWith({ list: "chat-drafts" })
    expect(screen.getByText("Chat drafts")).toBeTruthy()
    expect(screen.getByText("Restored brief")).toBeTruthy()
  })

  test("a pick sends the revision, adopts brief, images and name, and leaves the page saved", async () => {
    vi.useFakeTimers()
    const fake = fakeDrafts()
    const pickHistory = vi.fn<PickChatHistory>(async () => pickResult())
    await mountPhone(fake, pickHistory)
    await openPopup()
    await act(async () => void fireEvent.click(restore()))
    expect(pickHistory).toHaveBeenCalledWith({ entry_id: "e1", draft_id: "c1", revision: 3 })
    expect(brief().value).toBe("Restored brief")
    expect((screen.getByRole("textbox", { name: "Name" }) as HTMLInputElement).value).toBe(
      "restored-name",
    )
    expect(screen.getAllByRole("button", { name: "Remove image" })).toHaveLength(1)
    expect(screen.getByText("1 image missing")).toBeTruthy()
    expect(screen.queryByRole("button", { name: "Restore" })).toBeNull()
    expect(document.activeElement).toBe(brief())
    expect(screen.getByRole("status").textContent).toBe("")
    await wait(800)
    expect(fake.save).not.toHaveBeenCalled()
    typeBrief("more")
    await wait(800)
    expect(fake.save).toHaveBeenCalledWith({ id: "c1", brief: "more", revision: 10 })
  })

  test("Restore stays enabled while the person types", async () => {
    vi.useFakeTimers()
    const pickHistory = vi.fn<PickChatHistory>(async () => pickResult())
    await mountPhone(fakeDrafts(), pickHistory)
    typeBrief("typing")
    await openPopup()
    expect(restore().disabled).toBe(false)
    expect(screen.queryByText("Saving…")).toBeNull()
  })

  test("a revision conflict closes the popup and raises the banner, brief unchanged", async () => {
    vi.useFakeTimers()
    const pickHistory = vi.fn<PickChatHistory>(async () => ({
      refused: true,
      reason: "revision-conflict",
      notices: [],
    }))
    await mountPhone(fakeDrafts(), pickHistory)
    await openPopup()
    await act(async () => void fireEvent.click(restore()))
    expect(screen.queryByRole("button", { name: "Restore" })).toBeNull()
    expect(screen.getByText("This draft changed on another device.")).toBeTruthy()
    for (const name of ["Load theirs", "Keep mine"])
      expect(screen.getByRole("button", { name })).toBeTruthy()
    expect(brief().value).toBe("Split the auth work out.")
  })
})
