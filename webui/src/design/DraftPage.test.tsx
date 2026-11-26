import { setViMode } from "../vimode"
import { act, cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react"
import { ConfirmHost } from "../shell/ConfirmHost"
import { afterEach, describe, expect, test, vi } from "vitest"
import { MemoryRouter, Route, Routes, useLocation } from "react-router"
import { ConnectionContext, type Connection } from "../connection"
import type { DesignDraft } from "../generated/server"
import { renderThemed } from "../ui/render"
import { DeviceContext, type Device } from "../useWide"
import type { DraftResult, DraftsClient } from "./drafts"
import type { OpenDesign } from "./open"
import { DraftPage } from "./DraftPage"
import { designHref } from "../hrefs"
import { slugProblem } from "./draftSlug"
import type { DesignDraftPickResult, DraftHistoryEntry } from "../generated/ops"
import type { ListHistory, PickDesignHistory } from "../history/api"

afterEach(() => {
  cleanup()
  localStorage.clear()
  vi.useRealTimers()
})

const ok: OpenDesign = async () => ({ refused: false }) as never
const UNSAVED_KEY = "gx.newDesign.unsaved"

/** An in-memory draft store holding one draft, d1. */
function fakeDrafts(over: Partial<DesignDraft> = {}) {
  const stored: DesignDraft = {
    id: "d1",
    project: "gx",
    slug: "",
    idea: "",
    label: "Untitled design",
    images: [],
    revision: 3,
    edited_at: "",
    ...over,
  }
  let revision = stored.revision
  const save = vi.fn(async (body: { id?: string; idea: string }): Promise<DraftResult> => {
    revision++
    Object.assign(stored, { idea: body.idea, revision })
    return { kind: "ok", draft: { ...stored } }
  })
  const saveFields = vi.fn(
    async (body: { slug?: string; project?: string }): Promise<DraftResult> => {
      Object.assign(stored, body)
      stored.label = stored.slug || "Untitled design"
      return { kind: "ok", draft: { ...stored } }
    },
  )
  const client: DraftsClient = {
    read: async () => ({ kind: "ok", draft: { ...stored } }),
    save,
    saveFields,
    remove: vi.fn(async (): Promise<DraftResult> => ({ kind: "deleted" })),
    ping: vi.fn(async () => {}),
    removeIfEmpty: vi.fn(async () => {}),
    getLastProject: async () => "",
    setLastProject: vi.fn(async () => {}),
    uploadImage: vi.fn(async () => "img1"),
    removeImage: vi.fn(async () => ({})),
  }
  return { client, save, saveFields, stored }
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

async function mount(
  fake: ReturnType<typeof fakeDrafts>,
  {
    projects = ["gx", "blog"],
    device,
    pathname = "/drafts/d1",
    viMode = false,
    open = ok,
    state,
    pickHistory,
    listHistory,
    workspaces = [],
  }: {
    workspaces?: { name: string; order: number }[]
    state?: unknown
    projects?: string[]
    device?: Device
    pathname?: string
    viMode?: boolean
    open?: OpenDesign
    pickHistory?: PickDesignHistory
    listHistory?: ListHistory
  } = {},
) {
  setViMode(viMode)
  const connState = { snapshot: { sessions: [], workspaces } }
  const listeners = new Set<(address: string, type: string) => void>()
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
      <DeviceContext.Provider value={device}>
        <ConfirmHost />
        <ConnectionContext.Provider value={connection}>
          <MemoryRouter initialEntries={[{ pathname, state }]}>
            <Routes>
              <Route path="/epics" element={<Where />} />
              <Route path="/d/*" element={<Where />} />
              <Route
                path="*"
                element={
                  <DraftPage
                    id="d1"
                    projects={projects}
                    open={open}
                    drafts={fake.client}
                    pickHistory={pickHistory}
                    listHistory={listHistory}
                  />
                }
              />
            </Routes>
          </MemoryRouter>
        </ConnectionContext.Provider>
      </DeviceContext.Provider>,
    )
  })
  return {
    emit: (type: string) =>
      act(async () => {
        for (const l of listeners) l("", type)
      }),
  }
}
const type = (name: string, value: string) =>
  fireEvent.change(screen.getByRole(name === "Project" ? "combobox" : "textbox", { name }), {
    target: { value },
  })
const wait = (ms: number) => act(async () => void vi.advanceTimersByTime(ms))
const start = () => screen.getByRole("button", { name: "Start design" }) as HTMLButtonElement
const startBlocked = () => start().getAttribute("aria-disabled") === "true"
/** The text Start's aria-describedby points at, or null without a reason. */
const startReason = () => {
  const id = start().getAttribute("aria-describedby")
  return id ? document.getElementById(id)?.textContent : null
}

test("the page has its header, fields and footer, and no dialog", async () => {
  await mount(fakeDrafts())
  expect(screen.getByRole("heading", { name: "Untitled design" })).toBeTruthy()
  for (const name of ["Rename", "Remove draft", "Start design"])
    expect(screen.getByRole("button", { name })).toBeTruthy()
  expect(screen.getByRole("combobox", { name: "Project" })).toBeTruthy()
  expect(screen.getByRole("textbox", { name: "Epic slug" })).toBeTruthy()
  expect(screen.getByRole("textbox", { name: "Prompt" })).toBeTruthy()
  expect(screen.queryByRole("dialog")).toBeNull()
  expect(screen.queryByRole("button", { name: "Close" })).toBeNull()
  expect(screen.queryByRole("textbox", { name: "Draft name" })).toBeNull()
  expect(document.title).toBe("Untitled design · Design draft · gx")
})

test("one project hides the select", async () => {
  await mount(fakeDrafts(), { projects: ["gx"] })
  expect(screen.queryByRole("combobox", { name: "Project" })).toBeNull()
})

test("a draft that cannot be read shows the empty state", async () => {
  const fake = fakeDrafts()
  fake.client.read = async () => {
    throw new Error("down")
  }
  await mount(fake)
  expect(screen.getByText("Draft not found")).toBeTruthy()
})

test("a bookmarked draft that is gone goes to the Epics page with a note", async () => {
  const fake = fakeDrafts()
  fake.client.read = async () => ({ kind: "gone" })
  await mount(fake)
  expect(where()).toBe("/epics This draft was removed.")
})

test("a bookmarked draft that was started goes to its Design page", async () => {
  const fake = fakeDrafts()
  fake.client.read = async () => ({ kind: "gone", started: "gx:fresh" })
  await mount(fake)
  expect(where()).toBe(`${designHref("gx:fresh").slice(1)} `)
})

test("an empty prompt or slug blocks Start with no reason text; a bad slug names itself", async () => {
  await mount(fakeDrafts())
  expect(startBlocked()).toBe(true)
  expect(startReason()).toBeNull()
  expect(screen.queryByText("Write a prompt first")).toBeNull()
  type("Prompt", "an idea")
  expect(startBlocked()).toBe(true)
  expect(startReason()).toBeNull()
  expect(screen.queryByText("Name the epic to start.")).toBeNull()
  type("Epic slug", "My Epic")
  expect(startBlocked()).toBe(true)
  expect(startReason()).toBe(slugProblem("My Epic"))
  type("Epic slug", "my-epic")
  expect(startBlocked()).toBe(false)
  expect(startReason()).toBeNull()
  expect(start().getAttribute("disabled")).toBeNull()
})

test("the footer holds only buttons: no reason text, Remove is an icon, Start does not wrap", async () => {
  await mount(fakeDrafts({ slug: "My Epic", idea: "y" }))
  const remove = screen.getByRole("button", { name: "Remove draft" })
  expect(remove.getAttribute("title")).toBe("Remove draft")
  expect(remove.textContent).toBe("")
  expect(getComputedStyle(start()).whiteSpace).toBe("nowrap")
})

test("a blocked Start does nothing on click or Enter in the slug field", async () => {
  const open = vi.fn<OpenDesign>(async () => ({ refused: false }) as never)
  await mount(fakeDrafts({ slug: "", idea: "an idea" }), { open })
  await act(async () => void fireEvent.click(start()))
  const slug = screen.getByRole("textbox", { name: "Epic slug" })
  await act(async () => void fireEvent.submit(slug.closest("form")!))
  expect(open).not.toHaveBeenCalled()
})

test("a blocked Start with a reason opens a tooltip on focus; with no reason it has none", async () => {
  await mount(fakeDrafts({ slug: "My Epic", idea: "y" }))
  await act(async () => start().focus())
  expect(await screen.findAllByText(slugProblem("My Epic")!)).not.toHaveLength(0)
  expect(document.querySelector("[role=tooltip]")).toBeTruthy()
  cleanup()
  await mount(fakeDrafts())
  await act(async () => start().focus())
  expect(document.querySelector("[role=tooltip]")).toBeNull()
})

test("Start posts new_design with the draft's fields", async () => {
  const fake = fakeDrafts({ slug: "fresh", idea: "an idea" })
  const open = vi.fn<OpenDesign>(async () => ({ refused: false }) as never)
  await mount(fake, { open })
  await act(async () => void fireEvent.click(start()))
  expect(open).toHaveBeenCalledWith(
    expect.objectContaining({ epic: "fresh", new_design: true, idea: "an idea", project: "gx" }),
  )
})

test("typing saves after 800 ms with the revision and only the prompt", async () => {
  vi.useFakeTimers()
  const fake = fakeDrafts()
  await mount(fake)
  type("Prompt", "hello")
  expect(screen.getByRole("status").textContent).toBe("")
  await wait(799)
  expect(fake.save).not.toHaveBeenCalled()
  expect(screen.getByRole("status").textContent).toBe("")
  await wait(1)
  expect(fake.save).toHaveBeenCalledWith({ id: "d1", idea: "hello", revision: 3 })
  expect(fake.saveFields).not.toHaveBeenCalled()
})

test("a slug or project goes through saveFields, never the text save, and moves the title", async () => {
  vi.useFakeTimers()
  const fake = fakeDrafts({ idea: "text" })
  await mount(fake)
  type("Epic slug", "my-epic")
  await wait(800)
  expect(fake.saveFields).toHaveBeenCalledWith({ id: "d1", slug: "my-epic" })
  expect(fake.save).not.toHaveBeenCalled()
  expect(document.title).toBe("my-epic · Design draft · gx")

  type("Project", "blog")
  await wait(800)
  expect(fake.saveFields).toHaveBeenLastCalledWith({ id: "d1", project: "blog" })

  // A text save after that does not carry the slug.
  type("Prompt", "more")
  await wait(800)
  expect(fake.save).toHaveBeenCalledWith({ id: "d1", idea: "more", revision: 3 })
})

test("a slug edit after an outside project change never writes the old project back", async () => {
  vi.useFakeTimers()
  const fake = fakeDrafts({ idea: "text" })
  await mount(fake)
  type("Epic slug", "first")
  await wait(800)
  fake.stored.project = "elsewhere"
  type("Epic slug", "second")
  await wait(800)
  expect(fake.saveFields).toHaveBeenLastCalledWith({ id: "d1", slug: "second" })
  expect(fake.stored.project).toBe("elsewhere")
})

test("an invalid slug shows its problem and writes nothing; an empty one is written", async () => {
  vi.useFakeTimers()
  const fake = fakeDrafts({ slug: "old", label: "old" })
  await mount(fake)
  type("Epic slug", "Bad Slug")
  await wait(800)
  expect(screen.getByRole("alert").textContent).toContain("is not a slug")
  expect(fake.saveFields).not.toHaveBeenCalled()
  type("Epic slug", "")
  await wait(800)
  expect(fake.saveFields).toHaveBeenCalledWith({ id: "d1", slug: "" })
})

test("a page opened by New design takes back focus that a closing menu returns to its trigger", async () => {
  const trigger = document.createElement("button")
  document.body.append(trigger)
  await mount(fakeDrafts(), { state: { focusPrompt: true } })
  const prompt = screen.getByRole("textbox", { name: "Prompt" })
  expect(document.activeElement).toBe(prompt)
  act(() => trigger.focus())
  expect(document.activeElement).toBe(prompt)
  // A key press ends the window: focus the person moves later stays where it goes.
  fireEvent.keyDown(document.body, { key: "Tab" })
  act(() => trigger.focus())
  expect(document.activeElement).toBe(trigger)
  trigger.remove()
})

test("leaving the page during the focus hold clears its timer and document listeners", async () => {
  const added = vi.spyOn(document, "addEventListener")
  const removed = vi.spyOn(document, "removeEventListener")
  const setT = vi.spyOn(globalThis, "setTimeout")
  const clearT = vi.spyOn(globalThis, "clearTimeout")
  try {
    await mount(fakeDrafts(), { state: { focusPrompt: true } })
    const holdIndex = setT.mock.calls.findIndex(([, ms]) => ms === 2000)
    expect(holdIndex).toBeGreaterThanOrEqual(0)
    const timer = setT.mock.results[holdIndex].value
    const events = ["focusin", "pointerdown", "keydown"]
    // The hold adds its listeners right after starting its timer.
    const after = setT.mock.invocationCallOrder[holdIndex]
    const held = added.mock.calls.filter(
      ([type], i) => events.includes(type) && added.mock.invocationCallOrder[i] > after,
    )
    expect(held).toHaveLength(3)
    cleanup()
    expect(clearT).toHaveBeenCalledWith(timer)
    for (const [type, handler] of held) {
      expect(removed.mock.calls.some(([t, h]) => t === type && h === handler)).toBe(true)
    }
  } finally {
    vi.restoreAllMocks()
  }
})

test("Rename focuses the slug field", async () => {
  await mount(fakeDrafts())
  fireEvent.click(screen.getByRole("button", { name: "Rename" }))
  expect(document.activeElement).toBe(screen.getByRole("textbox", { name: "Epic slug" }))
})

test("a failed save shows Not saved, keeps one fallback by draft id and the next visit saves it", async () => {
  vi.useFakeTimers()
  const fake = fakeDrafts()
  fake.save.mockRejectedValue(new Error("down"))
  await mount(fake)
  type("Prompt", "kept")
  await wait(800)
  expect(screen.getByText("Not saved, retrying")).toBeTruthy()
  type("Prompt", "kept more")
  await wait(5000)
  expect(Object.keys(JSON.parse(localStorage.getItem(UNSAVED_KEY) ?? "{}"))).toEqual(["d1"])
  cleanup()

  fake.save.mockImplementation(async (body) => {
    Object.assign(fake.stored, { idea: body.idea })
    return { kind: "ok", draft: { ...fake.stored } }
  })
  await mount(fake)
  expect(fake.save).toHaveBeenLastCalledWith({ id: "d1", idea: "kept more", revision: 3 })
  expect(localStorage.getItem(UNSAVED_KEY)).toBeNull()
})

const project = () => (screen.getByRole("combobox", { name: "Project" }) as HTMLSelectElement).value
const blank = { project: "" }

test("a blank draft starts on the page's project, then the one-project filter, the last used, then the first", async () => {
  const projects = ["blog", "gx", "web"]
  const withLast = (last: string) => {
    const fake = fakeDrafts(blank)
    fake.client.getLastProject = async () => last
    return fake
  }
  await mount(withLast("gx"), { projects, pathname: "/e/web:x" })
  expect(project()).toBe("web")
  cleanup()
  localStorage.setItem("gx.epics.project", JSON.stringify(["gx"]))
  await mount(withLast("web"), { projects })
  expect(project()).toBe("gx")
  localStorage.clear()
  cleanup()
  await mount(withLast("web"), { projects })
  expect(project()).toBe("web")
  cleanup()
  await mount(withLast(""), { projects })
  expect(project()).toBe("blog")
  cleanup()
  await mount(withLast("gone"), { projects })
  expect(project()).toBe("blog")
})

test("a filter of several projects does not steer a blank draft", async () => {
  localStorage.setItem("gx.epics.project", JSON.stringify(["gx", "web"]))
  await mount(fakeDrafts(blank), { projects: ["blog", "gx", "web"] })
  expect(project()).toBe("blog")
})

test("a saved draft keeps its project against the page", async () => {
  await mount(fakeDrafts({ project: "gx" }), { pathname: "/e/blog:x" })
  expect(project()).toBe("gx")
})

test("a draft whose project is gone moves to another with a note", async () => {
  await mount(fakeDrafts({ project: "old" }))
  expect(project()).toBe("gx")
  expect(screen.getByText("old no longer exists; moved to gx")).toBeTruthy()
})

test("with a vi snapshot the prompt is the code editor, labelled Prompt", async () => {
  await mount(fakeDrafts(), { viMode: true })
  expect(await screen.findByLabelText("Prompt")).toBeTruthy()
  expect(screen.queryByRole("textbox", { name: "Prompt" })?.tagName).not.toBe("TEXTAREA")
})

const png = (name = "a.png") =>
  new File([new Uint8Array([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a])], name, {
    type: "image/png",
  })
const addImages = (...files: File[]) =>
  fireEvent.change(screen.getByTestId("image-input"), { target: { files } })
const big = () => {
  const f = png("big.png")
  Object.defineProperty(f, "size", { value: 21 * 1024 * 1024 })
  return f
}

test("Add image opens the file picker, and a chosen file uploads at once with a count", async () => {
  const fake = fakeDrafts()
  await mount(fake)
  const click = vi.spyOn(screen.getByTestId("image-input"), "click")
  fireEvent.click(screen.getByRole("button", { name: "Add image" }))
  expect(click).toHaveBeenCalled()
  addImages(png())
  await screen.findByText("1 of 10 images")
  expect(fake.client.uploadImage).toHaveBeenCalledWith("d1", expect.any(File))
})

test("a pasted image uploads at once", async () => {
  const fake = fakeDrafts()
  await mount(fake)
  fireEvent.paste(screen.getByRole("textbox", { name: "Prompt" }), {
    clipboardData: { files: [png()], types: ["Files"] },
  })
  await screen.findByText("1 of 10 images")
  expect(fake.client.uploadImage).toHaveBeenCalledTimes(1)
})

test("Start names a held upload and a failed one, and a refused file is never uploaded", async () => {
  const fake = fakeDrafts({ slug: "x", idea: "y" })
  await mount(fake)
  expect(startBlocked()).toBe(false)
  let land: (id: string) => void = () => {}
  vi.mocked(fake.client.uploadImage).mockImplementationOnce(
    () => new Promise<string>((ok) => (land = ok)),
  )
  addImages(png())
  await waitFor(() => expect(startReason()).toBe("Wait for the upload to finish"))
  expect(startBlocked()).toBe(true)
  await act(async () => land("i9"))
  expect(startReason()).toBeNull()
  expect(startBlocked()).toBe(false)

  vi.mocked(fake.client.uploadImage).mockClear()
  addImages(big())
  await screen.findByText("Too large, max 20 MB")
  expect(startReason()).toBe("Remove the failed image first")
  expect(startBlocked()).toBe(true)
  expect(fake.client.uploadImage).not.toHaveBeenCalled()
  fireEvent.click(screen.getAllByRole("button", { name: "Remove image" }).at(-1)!)
  expect(startBlocked()).toBe(false)
})

test("a conflict shows the banner with its two ways out", async () => {
  vi.useFakeTimers()
  const fake = fakeDrafts()
  fake.save.mockResolvedValue({ kind: "conflict" })
  await mount(fake)
  type("Prompt", "mine")
  await wait(800)
  expect(screen.getByText("This draft changed on another device.")).toBeTruthy()
  for (const name of ["Load theirs", "Keep mine"])
    expect(screen.getByRole("button", { name })).toBeTruthy()
})

test("Load theirs takes the server's copy and Keep mine saves over it", async () => {
  vi.useFakeTimers()
  const fake = fakeDrafts()
  fake.save.mockResolvedValueOnce({ kind: "conflict" })
  await mount(fake)
  type("Prompt", "mine")
  await wait(800)
  fake.stored.idea = "theirs"
  fake.stored.revision = 9
  await act(async () => void fireEvent.click(screen.getByRole("button", { name: "Load theirs" })))
  expect((screen.getByRole("textbox", { name: "Prompt" }) as HTMLTextAreaElement).value).toBe(
    "theirs",
  )
  expect(screen.queryByText("This draft changed on another device.")).toBeNull()

  fake.save.mockResolvedValueOnce({ kind: "conflict" })
  type("Prompt", "mine again")
  await wait(800)
  await act(async () => void fireEvent.click(screen.getByRole("button", { name: "Keep mine" })))
  await wait(0)
  expect(fake.save).toHaveBeenLastCalledWith({ id: "d1", idea: "mine again", revision: 9 })
})

test("a slug changed elsewhere updates the field and raises no banner", async () => {
  const fake = fakeDrafts({ slug: "old", label: "old" })
  const page = await mount(fake)
  Object.assign(fake.stored, { slug: "renamed", label: "renamed" })
  await page.emit("design-drafts-changed")
  expect((screen.getByRole("textbox", { name: "Epic slug" }) as HTMLInputElement).value).toBe(
    "renamed",
  )
  expect(screen.queryByRole("alert")).toBeNull()
})

test("Remove on an empty draft deletes at once and goes to the Epics page", async () => {
  const fake = fakeDrafts()
  await mount(fake)
  await act(async () => void fireEvent.click(screen.getByRole("button", { name: "Remove draft" })))
  expect(screen.queryByText("Remove this draft and its images?")).toBeNull()
  expect(fake.client.remove).toHaveBeenCalledTimes(1)
  expect(fake.client.remove).toHaveBeenCalledWith("d1")
  expect(where()).toBe("/epics ")
})

test.each([
  ["a prompt", { idea: "text" }],
  ["a slug", { slug: "x", label: "x" }],
])(
  "Remove on a draft with %s asks first; Keep removes nothing, Remove deletes",
  async (_n, over) => {
    const fake = fakeDrafts(over)
    await mount(fake)
    fireEvent.click(screen.getByRole("button", { name: "Remove draft" }))
    const popover = await screen.findByRole("alertdialog")
    expect(screen.queryByText("Remove this draft and its images?")).toBeNull()
    expect(within(popover).queryByRole("button", { name: "Keep" })).toBeNull()
    expect(fake.client.remove).not.toHaveBeenCalled()
    fireEvent.click(within(popover).getByRole("button", { name: /^Cancel/ }))
    await waitFor(() => expect(screen.queryByRole("alertdialog")).toBeNull())
    expect(fake.client.remove).not.toHaveBeenCalled()
    fireEvent.click(screen.getByRole("button", { name: "Remove draft" }))
    const again = await screen.findByRole("alertdialog")
    await act(
      async () => void fireEvent.click(within(again).getByRole("button", { name: /^Remove/ })),
    )
    await waitFor(() => expect(fake.client.remove).toHaveBeenCalledTimes(1))
    expect(where()).toBe("/epics ")
  },
)

test("the title row ends with a More actions menu: Rename, then Remove (Move needs a workspace)", async () => {
  await mount(fakeDrafts())
  const more = screen.getByRole("button", { name: "More actions" })
  const row = more.closest("div")!
  expect(row.lastElementChild?.contains(more) || row.lastElementChild === more).toBe(true)
  fireEvent.click(more)
  const items = (await screen.findAllByRole("menuitem")).map((i) => i.textContent)
  expect(items.filter((t) => ["Rename", "Move to workspace", "Remove"].includes(t ?? ""))).toEqual([
    "Rename",
    "Remove",
  ])
})

test("with workspaces the ⋯ lists Rename, Move to workspace, Remove, and Remove is red", async () => {
  await mount(fakeDrafts(), {
    workspaces: [
      { name: "Default", order: 0 },
      { name: "Work", order: 1 },
    ],
  })
  fireEvent.click(screen.getByRole("button", { name: "More actions" }))
  const items = await screen.findAllByRole("menuitem")
  expect(items.map((i) => i.textContent)).toEqual(["Rename", "Move to workspace›", "Remove"])
  expect(getComputedStyle(items[2]!).color).not.toBe(getComputedStyle(items[0]!).color)
})

test("Rename from the ⋯ focuses the slug field", async () => {
  await mount(fakeDrafts())
  fireEvent.click(screen.getByRole("button", { name: "More actions" }))
  fireEvent.click(await screen.findByRole("menuitem", { name: "Rename" }))
  await waitFor(() => expect(document.activeElement).toBe(screen.getByLabelText("Epic slug")))
})

test("Remove on a draft with only an image asks first", async () => {
  const fake = fakeDrafts()
  await mount(fake)
  addImages(png())
  await screen.findByText("1 of 10 images")
  fireEvent.click(screen.getByRole("button", { name: "Remove draft" }))
  expect(await screen.findByRole("alertdialog")).toBeTruthy()
})

test("a good Start goes to the Design page with the draft id and no workspace", async () => {
  const fake = fakeDrafts({ slug: "fresh", idea: "an idea" })
  const open = vi.fn<OpenDesign>(
    async () => ({ refused: false, start_error: "no session" }) as never,
  )
  await mount(fake, { open })
  await act(async () => void fireEvent.click(start()))
  const body = open.mock.calls[0][0]
  expect(body.draft_id).toBe("d1")
  expect(body).not.toHaveProperty("workspace")
  expect(where()).toBe(`${designHref("gx:fresh").slice(1)} `)
})

test("a refused Start keeps the text and the draft saves again", async () => {
  vi.useFakeTimers()
  const fake = fakeDrafts({ slug: "fresh", idea: "an idea" })
  const open = vi.fn<OpenDesign>(async () => ({ refused: true, reason: "has an epic" }) as never)
  await mount(fake, { open })
  await act(async () => void fireEvent.click(start()))
  expect((screen.getByRole("textbox", { name: "Prompt" }) as HTMLTextAreaElement).value).toBe(
    "an idea",
  )
  type("Prompt", "more")
  await wait(800)
  expect(fake.save).toHaveBeenCalledWith(expect.objectContaining({ idea: "more" }))
  expect(fake.client.remove).not.toHaveBeenCalled()
})

test("the page pings the lease on open and every 30 s, and stops after unmount", async () => {
  vi.useFakeTimers()
  const fake = fakeDrafts()
  await mount(fake)
  expect(fake.client.ping).toHaveBeenCalledTimes(1)
  expect(fake.client.ping).toHaveBeenCalledWith("d1")
  await wait(30_000)
  expect(fake.client.ping).toHaveBeenCalledTimes(2)
  await wait(30_000)
  expect(fake.client.ping).toHaveBeenCalledTimes(3)
  cleanup()
  await wait(60_000)
  expect(fake.client.ping).toHaveBeenCalledTimes(3)
})

const leaveApp = async () => {
  cleanup()
  await act(async () => void (await Promise.resolve()))
}

describe("leaving the page", () => {
  test("an empty draft sends one remove-if-empty", async () => {
    const fake = fakeDrafts()
    await mount(fake)
    await leaveApp()
    expect(fake.client.removeIfEmpty).toHaveBeenCalledTimes(1)
    expect(fake.client.removeIfEmpty).toHaveBeenCalledWith("d1")
  })

  test("a pagehide before the unmount sends none", async () => {
    const fake = fakeDrafts()
    await mount(fake)
    window.dispatchEvent(new Event("pagehide"))
    await leaveApp()
    expect(fake.client.removeIfEmpty).not.toHaveBeenCalled()
  })

  test("a draft with a slug sends none; clearing the slug and leaving sends one", async () => {
    const withSlug = fakeDrafts({ slug: "x", label: "x" })
    await mount(withSlug)
    await leaveApp()
    expect(withSlug.client.removeIfEmpty).not.toHaveBeenCalled()

    const cleared = fakeDrafts({ slug: "x", label: "x" })
    await mount(cleared)
    type("Epic slug", "")
    await leaveApp()
    expect(cleared.client.removeIfEmpty).toHaveBeenCalledTimes(1)
  })

  test("unmounting after Remove or Start sends none", async () => {
    const removed = fakeDrafts()
    await mount(removed)
    await act(
      async () => void fireEvent.click(screen.getByRole("button", { name: "Remove draft" })),
    )
    await leaveApp()
    expect(removed.client.removeIfEmpty).not.toHaveBeenCalled()

    const started = fakeDrafts({ slug: "fresh", idea: "an idea" })
    await mount(started)
    await act(async () => void fireEvent.click(start()))
    await leaveApp()
    expect(started.client.removeIfEmpty).not.toHaveBeenCalled()
  })
})

describe("a draft that vanished", () => {
  test("a design-drafts-changed event and a gone read go to the Epics page with the note", async () => {
    const fake = fakeDrafts()
    const page = await mount(fake)
    fake.client.read = async () => ({ kind: "gone" })
    await page.emit("design-drafts-changed")
    expect(where()).toBe("/epics This draft was removed.")
  })

  test("a Start record sends the page to the Design page", async () => {
    const fake = fakeDrafts()
    const page = await mount(fake)
    fake.client.read = async () => ({ kind: "gone", started: "gx:fresh" })
    await page.emit("design-drafts-changed")
    expect(where()).toBe(`${designHref("gx:fresh").slice(1)} `)
  })

  test("a failed ping re-reads", async () => {
    vi.useFakeTimers()
    const fake = fakeDrafts()
    await mount(fake)
    vi.mocked(fake.client.ping).mockRejectedValue(new Error("gone"))
    fake.client.read = async () => ({ kind: "gone" })
    await wait(30_000)
    expect(where()).toBe("/epics This draft was removed.")
  })

  test("a save against a gone draft redirects, with no banner", async () => {
    vi.useFakeTimers()
    const fake = fakeDrafts()
    fake.save.mockResolvedValue({ kind: "gone", started: "gx:fresh" })
    await mount(fake)
    type("Prompt", "mine")
    fake.client.read = async () => ({ kind: "gone", started: "gx:fresh" })
    await wait(800)
    expect(where()).toBe(`${designHref("gx:fresh").slice(1)} `)
  })
})

describe("History", () => {
  const entry: DraftHistoryEntry = {
    id: "e1",
    savedAt: new Date().toISOString(),
    text: "Rework the sidebar",
    imageCount: 0,
    imagesDropped: false,
    project: "gx",
    slug: "sidebar",
    name: "",
    harness: "",
    model: "",
    effort: "",
    folder: "",
    composer: "",
  }
  const listHistory = vi.fn<ListHistory>(async () => ({
    list: "design-drafts",
    max: 50,
    entries: [entry],
  }))
  const pickResult = (): DesignDraftPickResult => ({
    notices: ["The epic name was cleared."],
    draft: {
      id: "d1",
      project: "gx",
      slug: "",
      idea: "Rework the sidebar",
      label: "Untitled design",
      images: [],
      revision: 10,
      edited_at: "",
    },
  })
  const history = () => screen.getByRole("button", { name: "History" })
  const restore = () => screen.getByRole("button", { name: "Restore" }) as HTMLButtonElement
  // The phone popup picks in two steps: tap a row, then Restore.
  const phone = { device: "phone", listHistory } as const
  const openPopup = async () => {
    await act(async () => void fireEvent.click(history()))
    await act(async () => void fireEvent.click(screen.getByText("Rework the sidebar")))
  }
  const prompt = () => screen.getByRole("textbox", { name: "Prompt" }) as HTMLTextAreaElement

  test("a wide page shows the History text and a phone shows the icon only", async () => {
    await mount(fakeDrafts())
    expect(history().textContent).toBe("History")
    cleanup()
    await mount(fakeDrafts(), { device: "phone" })
    expect(history().textContent).toBe("")
    expect(history().getAttribute("aria-label")).toBe("History")
  })

  test("the button opens the Design drafts list", async () => {
    vi.useFakeTimers()
    await mount(fakeDrafts(), { listHistory })
    await act(async () => void fireEvent.click(history()))
    expect(listHistory).toHaveBeenCalledWith({ list: "design-drafts" })
    expect(screen.getByText("Rework the sidebar")).toBeTruthy()
    expect(screen.queryByText("Chat drafts")).toBeNull()
  })

  test("a pick sends the revision, fills the page and leaves it saved with the Prompt focused", async () => {
    vi.useFakeTimers()
    const fake = fakeDrafts()
    const pickHistory = vi.fn<PickDesignHistory>(async () => pickResult())
    await mount(fake, { ...phone, pickHistory })
    await openPopup()
    await act(async () => void fireEvent.click(restore()))
    expect(pickHistory).toHaveBeenCalledWith({ entry_id: "e1", draft_id: "d1", revision: 3 })
    expect(prompt().value).toBe("Rework the sidebar")
    expect(screen.getByText("The epic name was cleared.")).toBeTruthy()
    expect((screen.getByRole("combobox", { name: "Project" }) as HTMLSelectElement).value).toBe(
      "gx",
    )
    expect(screen.queryByRole("button", { name: "Restore" })).toBeNull()
    expect(document.activeElement).toBe(prompt())
    expect(screen.getByRole("status").textContent).toBe("")
    await wait(800)
    expect(fake.save).not.toHaveBeenCalled()
    type("Prompt", "more")
    await wait(800)
    expect(fake.save).toHaveBeenCalledWith({ id: "d1", idea: "more", revision: 10 })
  })

  test("Restore stays enabled while the person types", async () => {
    vi.useFakeTimers()
    const pickHistory = vi.fn<PickDesignHistory>(async () => pickResult())
    await mount(fakeDrafts(), { ...phone, pickHistory })
    type("Prompt", "typing")
    await openPopup()
    expect(restore().disabled).toBe(false)
    expect(screen.queryByText("Saving…")).toBeNull()
  })

  test("a revision conflict closes the popup and raises the banner, text unchanged", async () => {
    vi.useFakeTimers()
    const pickHistory = vi.fn<PickDesignHistory>(async () => ({
      refused: true,
      reason: "revision-conflict",
      notices: [],
    }))
    await mount(fakeDrafts({ idea: "mine" }), { ...phone, pickHistory })
    await openPopup()
    await act(async () => void fireEvent.click(restore()))
    expect(screen.queryByRole("button", { name: "Restore" })).toBeNull()
    expect(screen.getByText("This draft changed on another device.")).toBeTruthy()
    for (const name of ["Load theirs", "Keep mine"])
      expect(screen.getByRole("button", { name })).toBeTruthy()
    expect(prompt().value).toBe("mine")
  })
})
