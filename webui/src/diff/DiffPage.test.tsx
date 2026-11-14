import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react"
import { createMemoryRouter, RouterProvider } from "react-router"
import { afterEach, expect, test, vi } from "vitest"
import { ConnectionContext, type Connection, type ConnectionState } from "../connection"
import type { DiffRange } from "../generated/server"
import { appRoutes } from "../routes"
import { renderThemed } from "../ui/render"
import { type Device, DeviceContext } from "../useWide"
import type { DiffFilesResult, FetchDiffFiles, FetchDiffPatch } from "./api"

afterEach(cleanup)

const range = (over: Partial<DiffRange> = {}): DiffRange => ({
  first_sha: "a",
  second_sha: "b",
  merge_base: "a",
  ...over,
})
const file = { path: "a.go", status: "M", added: 1, deleted: 0 }
const files = (n: number, over: Partial<DiffRange> = {}): DiffFilesResult => ({
  kind: "files",
  files: { ...range(over), files: Array.from({ length: n }, () => file) },
})

const pierre = vi.hoisted(() => ({ last: undefined as Record<string, unknown> | undefined }))
vi.mock("@pierre/diffs/react", () => ({
  PatchDiff: (p: Record<string, unknown>) => {
    pierre.last = p
    return <div data-testid="pierre" data-patch={p.patch as string} />
  },
}))

function render(
  path: string,
  fetchFiles: FetchDiffFiles,
  device?: Device,
  fetchPatch: FetchDiffPatch = async (_p, _a, _b, path) => `patch of ${path}`,
) {
  const state = {
    snapshot: { sessions: [], pending: [] },
    connected: true,
    loadedOnce: true,
    tick: 0,
  }
  const conn = {
    getState: () => state as unknown as ConnectionState,
    subscribe: () => () => {},
    onEvent: () => () => {},
    refresh: () => {},
  } as unknown as Connection
  const router = createMemoryRouter(appRoutes({ diffFiles: fetchFiles, diffPatch: fetchPatch }), {
    initialEntries: [path],
  })
  renderThemed(
    <DeviceContext.Provider value={device}>
      <ConnectionContext.Provider value={conn}>
        <RouterProvider router={router} />
      </ConnectionContext.Provider>
    </DeviceContext.Provider>,
  )
  return router
}

const base = () => screen.findByRole("textbox", { name: "Base ref" }) as Promise<HTMLInputElement>
const tip = () => screen.getByRole("textbox", { name: "Tip ref" }) as HTMLInputElement
const OPEN = "/diff?project=gx&base=main&tip=dev"

test("the fields fill from the URL with ... between them", async () => {
  render(
    OPEN,
    vi.fn(async () => files(2)),
  )
  expect((await base()).value).toBe("main")
  expect(tip().value).toBe("dev")
  expect(screen.getByTestId("ref-bar").textContent).toContain("...")
  expect(await screen.findByText("2 files")).toBeTruthy()
})

test("Reload writes the URL and asks again with the typed refs; no polling", async () => {
  const fetchFiles = vi.fn(async () => files(1))
  const router = render(OPEN, fetchFiles)
  const input = await base()
  await waitFor(() => expect(fetchFiles).toHaveBeenCalledTimes(1))
  fireEvent.change(input, { target: { value: "v1" } })
  expect(fetchFiles).toHaveBeenCalledTimes(1)
  fireEvent.click(screen.getByRole("button", { name: "Reload" }))
  await waitFor(() => expect(fetchFiles).toHaveBeenCalledTimes(2))
  expect(fetchFiles).toHaveBeenLastCalledWith("gx", "v1", "dev")
  expect(router.state.location.search).toContain("base=v1")
  await new Promise((r) => setTimeout(r, 50))
  expect(fetchFiles).toHaveBeenCalledTimes(2)
})

test("Enter reloads like the button, and Reload with unchanged refs asks again", async () => {
  const fetchFiles = vi.fn(async () => files(1))
  render(OPEN, fetchFiles)
  const input = await base()
  await waitFor(() => expect(fetchFiles).toHaveBeenCalledTimes(1))
  fireEvent.change(tip(), { target: { value: "v2" } })
  fireEvent.keyDown(tip(), { key: "Enter" })
  await waitFor(() => expect(fetchFiles).toHaveBeenCalledTimes(2))
  expect(fetchFiles).toHaveBeenLastCalledWith("gx", "main", "v2")
  fireEvent.click(screen.getByRole("button", { name: "Reload" }))
  await waitFor(() => expect(fetchFiles).toHaveBeenCalledTimes(3))
  expect(input.value).toBe("main")
})

test("nothing is fetched while a ref is empty", async () => {
  const fetchFiles = vi.fn(async () => files(1))
  render("/diff?project=gx&base=main", fetchFiles)
  await base()
  await new Promise((r) => setTimeout(r, 30))
  expect(fetchFiles).not.toHaveBeenCalled()
})

const notice = () => screen.findByTestId("notice")
const editable = () => {
  for (const name of ["Base ref", "Tip ref"]) {
    const input = screen.getByRole("textbox", { name }) as HTMLInputElement
    fireEvent.change(input, { target: { value: "zzz" } })
    expect(input.value).toBe("zzz")
  }
}

test("an unknown ref names the field, marks it invalid and stays editable", async () => {
  render(OPEN, async () => ({ kind: "unknown-ref", side: "second" }))
  const n = await notice()
  expect(n.textContent).toContain('Unknown ref "dev"')
  expect(tip().getAttribute("aria-invalid")).toBe("true")
  expect((await base()).getAttribute("aria-invalid")).toBeNull()
  editable()
})

test("an empty range says No changes", async () => {
  render("/diff?project=gx&base=main&tip=main", async () => files(0))
  const n = await notice()
  expect(n.textContent).toContain("No changes")
  expect(n.textContent).toContain("Nothing differs between main and main.")
  editable()
})

test("no common history is a neutral notice like No changes", async () => {
  render(OPEN, async () => files(0, { no_common_history: true }))
  const n = await notice()
  expect(n.textContent).toContain("No common history")
  expect(n.getAttribute("data-tone")).toBe("muted")
  editable()
})

test("an epic with empty refs shows No diff recorded and fetches nothing", async () => {
  const fetchFiles = vi.fn(async () => files(1))
  render("/diff?project=gx&epic=gx:e", fetchFiles)
  const n = await notice()
  expect(n.textContent).toContain("No diff recorded")
  expect(n.textContent).toContain(
    "gx has no exact range for this epic. Type two refs above to compare them.",
  )
  expect(n.getAttribute("data-tone")).toBe("waiting")
  expect(fetchFiles).not.toHaveBeenCalled()
  await base()
  editable()
})

test("unavailable commits show an amber notice in the same area", async () => {
  render(OPEN, async () => files(0, { commits_unavailable: true }))
  const n = await notice()
  expect(n.textContent).toContain("Commits not available in this clone")
  expect(n.getAttribute("data-tone")).toBe("waiting")
  editable()
})

const named = (...paths: string[]): DiffFilesResult => ({
  kind: "files",
  files: { ...range(), files: paths.map((path) => ({ ...file, path })) },
})
const TREE = () => named("README.md", "src/a.go", "src/b.go") // tree order: a.go, b.go, README.md
const counter = () => screen.getByTestId("file-nav").textContent
const press = (key: string, target: Element = document.body) => fireEvent.keyDown(target, { key })

test("n and p walk the visible rows with wrap-around and the counter follows", async () => {
  const router = render(OPEN, async () => TREE())
  await screen.findByTestId("file-nav")
  expect(counter()).toContain("1 of 3")
  press("n")
  await waitFor(() => expect(counter()).toContain("2 of 3"))
  expect(router.state.location.search).toContain("file=src%2Fb.go")
  press("n")
  press("n")
  await waitFor(() => expect(counter()).toContain("1 of 3"))
  press("p")
  await waitFor(() => expect(counter()).toContain("3 of 3"))
  fireEvent.click(screen.getByRole("button", { name: "Next" }))
  await waitFor(() => expect(counter()).toContain("1 of 3"))
})

test("a filter narrows the counter and p/n do nothing while it has focus", async () => {
  render(OPEN, async () => TREE())
  await screen.findByTestId("file-nav")
  const input = screen.getByTestId("file-filter")
  fireEvent.change(input, { target: { value: "src" } })
  expect(counter()).toContain("1 of 2")
  press("n", input)
  expect(counter()).toContain("1 of 2")
  press("n")
  await waitFor(() => expect(counter()).toContain("2 of 2"))
})

test("the selected file is in the URL and a fresh render from that URL picks it", async () => {
  render(`${OPEN}&file=src%2Fb.go`, async () => TREE())
  await screen.findByTestId("file-nav")
  expect(counter()).toContain("2 of 3")
  cleanup()
  render(`${OPEN}&file=nope.go`, async () => TREE())
  await screen.findByTestId("file-nav")
  expect(counter()).toContain("1 of 3")
})

test("on a phone the Files and Diff tabs show one pane each and picking a file opens Diff", async () => {
  render(OPEN, async () => TREE(), "phone")
  const files = await screen.findByRole("tab", { name: "Files" })
  expect(screen.getByRole("tab", { name: "Diff" })).toBeTruthy()
  expect(screen.queryByTestId("file-nav")).toBeNull()
  fireEvent.click(files)
  fireEvent.click(screen.getAllByTestId("tree-file")[1])
  expect(await screen.findByTestId("file-nav")).toBeTruthy()
  expect(screen.queryByTestId("file-tree")).toBeNull()
})

test("desktop has no tabs", async () => {
  render(OPEN, async () => TREE())
  await screen.findByTestId("file-nav")
  expect(screen.queryByRole("tab")).toBeNull()
})

test("the Split/Unified switch is in the ref bar", async () => {
  render(OPEN, async () => files(1))
  await base()
  const toggle = screen.getByTestId("view-toggle")
  expect(screen.getByTestId("ref-bar").contains(toggle)).toBe(true)
  expect(screen.getByRole("button", { name: "Split" }).getAttribute("aria-pressed")).toBe("true")
  fireEvent.click(screen.getByRole("button", { name: "Unified" }))
  expect(screen.getByRole("button", { name: "Unified" }).getAttribute("aria-pressed")).toBe("true")
})

const style = () => (pierre.last?.options as { diffStyle: string }).diffStyle

test("the selected file's patch is fetched alone, by the resolved SHAs", async () => {
  const fetchPatch = vi.fn<FetchDiffPatch>(async (_p, _a, _b, path) => `patch of ${path}`)
  render(OPEN, async () => TREE(), undefined, fetchPatch)
  const body = await screen.findByTestId("pierre")
  expect(body.getAttribute("data-patch")).toBe("patch of src/a.go")
  expect(fetchPatch).toHaveBeenCalledTimes(1)
  expect(fetchPatch).toHaveBeenCalledWith("gx", "a", "b", "src/a.go")
  press("n")
  await waitFor(() =>
    expect(screen.getByTestId("pierre").getAttribute("data-patch")).toBe("patch of src/b.go"),
  )
  expect(fetchPatch).toHaveBeenCalledTimes(2)
  expect(screen.getAllByTestId("file-card")).toHaveLength(1)
})

test("a failed patch shows a retry that fetches again", async () => {
  const fetchPatch = vi
    .fn<FetchDiffPatch>()
    .mockRejectedValueOnce(new Error("boom"))
    .mockResolvedValue("ok")
  render(OPEN, async () => files(1), undefined, fetchPatch)
  fireEvent.click(await screen.findByRole("button", { name: "Retry" }))
  expect((await screen.findByTestId("pierre")).getAttribute("data-patch")).toBe("ok")
  expect(fetchPatch).toHaveBeenCalledTimes(2)
})

test("a renamed file shows both paths in its header", async () => {
  const renamed: DiffFilesResult = {
    kind: "files",
    files: { ...range(), files: [{ ...file, path: "new.go", old_path: "old.go", status: "R" }] },
  }
  render(OPEN, async () => renamed)
  const header = await screen.findByTestId("file-card-header")
  expect(header.textContent).toContain("old.go → new.go")
})

test("large, generated and binary files get a header and no body or fetch", async () => {
  const fetchPatch = vi.fn<FetchDiffPatch>(async () => "x")
  const flagged: DiffFilesResult = {
    kind: "files",
    files: { ...range(), files: [{ ...file, path: "big.go", large: true }] },
  }
  render(OPEN, async () => flagged, undefined, fetchPatch)
  expect(await screen.findByTestId("file-card-header")).toBeTruthy()
  expect(screen.queryByTestId("pierre")).toBeNull()
  expect(fetchPatch).not.toHaveBeenCalled()
})

test("split on desktop, unified on phone, and the switch changes the view and survives a file change", async () => {
  render(OPEN, async () => TREE())
  await screen.findByTestId("pierre")
  expect(style()).toBe("split")
  fireEvent.click(screen.getByRole("button", { name: "Unified" }))
  await waitFor(() => expect(style()).toBe("unified"))
  press("n")
  await waitFor(() =>
    expect(screen.getByTestId("pierre").getAttribute("data-patch")).toBe("patch of src/b.go"),
  )
  expect(style()).toBe("unified")
  cleanup()
  render(OPEN, async () => TREE(), "phone")
  fireEvent.click((await screen.findAllByTestId("tree-file"))[0])
  await screen.findByTestId("pierre")
  expect(style()).toBe("unified")
})
