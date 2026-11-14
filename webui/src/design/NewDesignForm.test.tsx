import { act, cleanup, fireEvent, screen } from "@testing-library/react"
import { afterEach, expect, test, vi } from "vitest"
import { MemoryRouter } from "react-router"
import { ConnectionContext, type Connection } from "../connection"
import type { DesignDraft } from "../generated/server"
import { renderThemed } from "../ui/render"
import { DeviceContext, type Device } from "../useWide"
import type { DraftResult, DraftsClient } from "./drafts"
import type { OpenDesign } from "./open"
import { DraftPage } from "./NewDesignForm"

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
    async (body: { slug: string; project: string }): Promise<DraftResult> => {
      Object.assign(stored, body, { label: body.slug || "Untitled design" })
      return { kind: "ok", draft: { ...stored } }
    },
  )
  const client: DraftsClient = {
    list: async () => [],
    read: async () => ({ kind: "ok", draft: { ...stored } }),
    save,
    saveFields,
    remove: async () => ({ kind: "deleted" }),
    getLastProject: async () => "",
    setLastProject: vi.fn(async () => {}),
    uploadImage: vi.fn(async () => "img1"),
    removeImage: vi.fn(async () => ({})),
  }
  return { client, save, saveFields, stored }
}

const connState = { snapshot: { sessions: [] } }
const connection = {
  refresh: () => {},
  getState: () => connState,
  subscribe: () => () => {},
  onEvent: () => () => {},
} as unknown as Connection

async function mount(
  fake: ReturnType<typeof fakeDrafts>,
  { projects = ["gx", "blog"], device }: { projects?: string[]; device?: Device } = {},
) {
  await act(async () => {
    renderThemed(
      <DeviceContext.Provider value={device}>
        <ConnectionContext.Provider value={connection}>
          <MemoryRouter initialEntries={["/drafts/d1"]}>
            <DraftPage id="d1" projects={projects} open={ok} drafts={fake.client} />
          </MemoryRouter>
        </ConnectionContext.Provider>
      </DeviceContext.Provider>,
    )
  })
}
const type = (name: string, value: string) =>
  fireEvent.change(screen.getByRole(name === "Project" ? "combobox" : "textbox", { name }), {
    target: { value },
  })
const wait = (ms: number) => act(async () => void vi.advanceTimersByTime(ms))
const start = () => screen.getByRole("button", { name: "Start design" }) as HTMLButtonElement

test("the page has its header, fields and footer, and no dialog", async () => {
  await mount(fakeDrafts())
  expect(screen.getByRole("heading", { name: "Untitled design" })).toBeTruthy()
  for (const name of ["Rename", "Remove", "Start design"])
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

test("a draft that is gone shows the empty state", async () => {
  const fake = fakeDrafts()
  fake.client.read = async () => ({ kind: "gone" })
  await mount(fake)
  expect(screen.getByText("Draft not found")).toBeTruthy()
})

test("Start names its reason: prompt, then epic, then the slug", async () => {
  await mount(fakeDrafts())
  expect(screen.getByText("Write a prompt first")).toBeTruthy()
  expect(start().disabled).toBe(true)
  type("Prompt", "an idea")
  expect(screen.getByText("Name the epic to start.")).toBeTruthy()
  type("Epic slug", "my-epic")
  expect(start().disabled).toBe(false)
})

test("the reason sits above the footer on a phone", async () => {
  await mount(fakeDrafts(), { device: "phone" })
  const reason = screen.getByText("Write a prompt first")
  expect(reason.parentElement).not.toBe(start().parentElement)
})

test("typing saves after 800 ms with the revision and only the prompt", async () => {
  vi.useFakeTimers()
  const fake = fakeDrafts()
  await mount(fake)
  type("Prompt", "hello")
  expect(screen.getByText("Saving...")).toBeTruthy()
  await wait(799)
  expect(fake.save).not.toHaveBeenCalled()
  await wait(1)
  expect(fake.save).toHaveBeenCalledWith({ id: "d1", idea: "hello", revision: 3 })
  expect(fake.saveFields).not.toHaveBeenCalled()
  expect(screen.getByText(/Saved/)).toBeTruthy()
})

test("a slug or project goes through saveFields, never the text save, and moves the title", async () => {
  vi.useFakeTimers()
  const fake = fakeDrafts({ idea: "text" })
  await mount(fake)
  type("Epic slug", "my-epic")
  await wait(800)
  expect(fake.saveFields).toHaveBeenCalledWith({ id: "d1", project: "gx", slug: "my-epic" })
  expect(fake.save).not.toHaveBeenCalled()
  expect(document.title).toBe("my-epic · Design draft · gx")

  type("Project", "blog")
  await wait(800)
  expect(fake.saveFields).toHaveBeenLastCalledWith({ id: "d1", project: "blog", slug: "my-epic" })

  // A text save after that does not carry the slug.
  type("Prompt", "more")
  await wait(800)
  expect(fake.save).toHaveBeenCalledWith({ id: "d1", idea: "more", revision: 3 })
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
  expect(fake.saveFields).toHaveBeenCalledWith({ id: "d1", project: "gx", slug: "" })
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
  expect(screen.getByText("Not saved")).toBeTruthy()
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
