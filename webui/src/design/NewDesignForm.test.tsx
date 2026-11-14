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

async function mount(
  fake: ReturnType<typeof fakeDrafts>,
  {
    projects = ["gx", "blog"],
    device,
    pathname = "/drafts/d1",
    viMode = false,
    open = ok,
  }: {
    projects?: string[]
    device?: Device
    pathname?: string
    viMode?: boolean
    open?: OpenDesign
  } = {},
) {
  const connState = { snapshot: { sessions: [], vi_mode: viMode } }
  const connection = {
    refresh: () => {},
    getState: () => connState,
    subscribe: () => () => {},
    onEvent: () => () => {},
  } as unknown as Connection
  await act(async () => {
    renderThemed(
      <DeviceContext.Provider value={device}>
        <ConnectionContext.Provider value={connection}>
          <MemoryRouter initialEntries={[pathname]}>
            <DraftPage id="d1" projects={projects} open={open} drafts={fake.client} />
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

test("Start posts new_design with the draft's fields", async () => {
  const fake = fakeDrafts({ slug: "fresh", idea: "an idea" })
  const open = vi.fn<OpenDesign>(async () => ({ refused: false }) as never)
  await mount(fake, { open })
  await act(async () => void fireEvent.click(start()))
  expect(open).toHaveBeenCalledWith(
    expect.objectContaining({ epic: "fresh", new_design: true, idea: "an idea", project: "gx" }),
  )
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
  expect(start().disabled).toBe(false)
  let land: (id: string) => void = () => {}
  vi.mocked(fake.client.uploadImage).mockImplementationOnce(
    () => new Promise<string>((ok) => (land = ok)),
  )
  addImages(png())
  await screen.findByText("Wait for the upload to finish")
  expect(start().disabled).toBe(true)
  await act(async () => land("i9"))
  expect(screen.queryByText("Wait for the upload to finish")).toBeNull()
  expect(start().disabled).toBe(false)

  vi.mocked(fake.client.uploadImage).mockClear()
  addImages(big())
  await screen.findByText("Too large, max 20 MB")
  expect(screen.getByText("Remove the failed image first")).toBeTruthy()
  expect(start().disabled).toBe(true)
  expect(fake.client.uploadImage).not.toHaveBeenCalled()
  fireEvent.click(screen.getAllByRole("button", { name: "Remove image" }).at(-1)!)
  expect(start().disabled).toBe(false)
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
