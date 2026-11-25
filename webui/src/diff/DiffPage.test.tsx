import { act, cleanup, fireEvent, screen, waitFor } from "@testing-library/react"
import { afterEach, expect, test, vi } from "vitest"
import type { DiffRange, DiffWorktree } from "../generated/server"
import type { Device } from "../useWide"
import type {
  DiffFilesResult,
  FetchDiffCommitPatches,
  FetchDiffFiles,
  FetchDiffPatch,
  FetchDiffWorktree,
} from "./api"
import { COMMITS, filesForCommits, renderDiff } from "./renderDiff.testutil"

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

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

const noEpic = async () => ({ kind: "not-found" as const })

function render(
  path: string,
  fetchFiles: FetchDiffFiles,
  device?: Device,
  fetchPatch: FetchDiffPatch = async (_p, _a, _b, path) => `patch of ${path}`,
  extra: Parameters<typeof renderDiff>[1] = {},
) {
  return renderDiff(
    path,
    {
      diffFiles: fetchFiles,
      diffPatch: fetchPatch,
      diffCommits: async () => ({ commits: [], truncated: false }),
      epic: noEpic,
      archivedEpic: noEpic,
      ...extra,
    },
    device,
  )
}

const base = () => screen.findByRole("textbox", { name: "Base ref" }) as Promise<HTMLInputElement>
const tip = () => screen.getByRole("textbox", { name: "Tip ref" }) as HTMLInputElement
const OPEN = "/diff?project=gx&base=main&tip=dev"

const editLink = () => screen.getByRole("button", { name: "Edit refs" })
const openEdit = async () => {
  await screen.findByTestId("range-chip")
  fireEvent.click(editLink())
  return base()
}

test("the chip shows the URL refs; the edit link opens the inputs filled from them", async () => {
  render(
    OPEN,
    vi.fn(async () => files(2)),
  )
  expect((await screen.findByTestId("range-chip")).textContent).toBe("main→dev")
  expect(screen.queryByRole("textbox", { name: "Base ref" })).toBeNull()
  expect((await openEdit()).value).toBe("main")
  expect(tip().value).toBe("dev")
  expect((await screen.findByTestId("diff-counts")).textContent).toBe("2 files +2 −0")
  expect(screen.queryByRole("button", { name: /Apply|Reload/ })).toBeNull()
})

test("Esc and a second click on the edit link close the inputs and drop the typed text", async () => {
  render(OPEN, async () => files(1))
  const input = await openEdit()
  fireEvent.change(input, { target: { value: "typed" } })
  fireEvent.keyDown(input, { key: "Escape" })
  expect(screen.queryByRole("textbox", { name: "Base ref" })).toBeNull()
  expect((await screen.findByTestId("range-chip")).textContent).toBe("main→dev")
  expect((await openEdit()).value).toBe("main")
  fireEvent.change(tip(), { target: { value: "typed" } })
  fireEvent.click(editLink())
  expect(screen.queryByRole("textbox", { name: "Tip ref" })).toBeNull()
  await openEdit()
  expect(tip().value).toBe("dev")
})

test("the header holds the range chip, the signed counts and the toggle; no Prev, Next or subtitle", async () => {
  render(OPEN, async () => files(2))
  const counts = await screen.findByTestId("diff-counts")
  const header = screen.getByTestId("diff-header")
  expect(screen.getByTestId("range-chip").textContent).toBe("main→dev")
  expect(header.contains(screen.getByTestId("range-chip"))).toBe(true)
  expect(header.contains(counts)).toBe(true)
  expect(header.contains(screen.getByTestId("view-toggle"))).toBe(true)
  expect(screen.queryByRole("button", { name: /Prev|Next/ })).toBeNull()
  expect(screen.queryByTestId("page-header-subtitle")).toBeNull()
})

test("Enter writes the URL, asks with the typed refs and closes the inputs; no polling", async () => {
  const fetchFiles = vi.fn(async () => files(1))
  const router = render(OPEN, fetchFiles)
  const input = await openEdit()
  await waitFor(() => expect(fetchFiles).toHaveBeenCalledTimes(1))
  fireEvent.change(input, { target: { value: "v1" } })
  expect(fetchFiles).toHaveBeenCalledTimes(1)
  fireEvent.keyDown(input, { key: "Enter" })
  await waitFor(() => expect(fetchFiles).toHaveBeenCalledTimes(2))
  expect(fetchFiles).toHaveBeenLastCalledWith("gx", "v1", "dev")
  expect(router.state.location.search).toContain("base=v1")
  expect(screen.queryByRole("textbox", { name: "Base ref" })).toBeNull()
  await new Promise((r) => setTimeout(r, 50))
  expect(fetchFiles).toHaveBeenCalledTimes(2)
})

test("Enter in the tip input applies, and Enter with unchanged refs asks again", async () => {
  const fetchFiles = vi.fn(async () => files(1))
  render(OPEN, fetchFiles)
  await openEdit()
  await waitFor(() => expect(fetchFiles).toHaveBeenCalledTimes(1))
  fireEvent.change(tip(), { target: { value: "v2" } })
  fireEvent.keyDown(tip(), { key: "Enter" })
  await waitFor(() => expect(fetchFiles).toHaveBeenCalledTimes(2))
  expect(fetchFiles).toHaveBeenLastCalledWith("gx", "main", "v2")
  await openEdit()
  fireEvent.keyDown(tip(), { key: "Enter" })
  await waitFor(() => expect(fetchFiles).toHaveBeenCalledTimes(3))
  expect(screen.queryByRole("textbox", { name: "Tip ref" })).toBeNull()
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
  expect(n.textContent).toContain("Fix the ref with the edit link.")
  expect(n.textContent).not.toContain("Reload")
  expect(tip().getAttribute("aria-invalid")).toBe("true")
  expect((await base()).getAttribute("aria-invalid")).toBeNull()
  editable()
})

test("an empty range says No changes", async () => {
  render("/diff?project=gx&base=main&tip=main", async () => files(0))
  const n = await notice()
  expect(n.textContent).toContain("No changes")
  expect(n.textContent).toContain("Nothing differs between main and main.")
  await openEdit()
  editable()
})

test("no common history is a neutral notice like No changes", async () => {
  render(OPEN, async () => files(0, { no_common_history: true }))
  const n = await notice()
  expect(n.textContent).toContain("No common history")
  expect(n.getAttribute("data-tone")).toBe("muted")
  await openEdit()
  editable()
})

test("an epic with empty refs shows No diff recorded and fetches nothing", async () => {
  const fetchFiles = vi.fn(async () => files(1))
  render("/diff?project=gx&epic=gx:e", fetchFiles)
  const n = await notice()
  expect(n.textContent).toContain("No diff recorded")
  expect(n.textContent).toContain(
    "gx has no exact range for this epic. Type two refs to compare them.",
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
  expect(n.textContent).toContain("Fetch them, then return to this window.")
  expect(n.textContent).not.toContain("Reload")
  expect(n.getAttribute("data-tone")).toBe("waiting")
  await openEdit()
  editable()
})

test("a failed load shows Could not load the diff; Try again calls the fetch again", async () => {
  const fetchFiles = vi.fn<FetchDiffFiles>(async () => {
    throw new Error("down")
  })
  render(OPEN, fetchFiles)
  const n = await notice()
  expect(n.textContent).toContain("Could not load the diff")
  expect(n.textContent).not.toContain("Reload")
  expect(n.getAttribute("data-tone")).toBe("deadlocked")
  await waitFor(() => expect(fetchFiles).toHaveBeenCalledTimes(1))
  fireEvent.click(screen.getByRole("button", { name: "Try again" }))
  await waitFor(() => expect(fetchFiles).toHaveBeenCalledTimes(2))
  await openEdit()
  editable()
})

const named = (...paths: string[]): DiffFilesResult => ({
  kind: "files",
  files: { ...range(), files: paths.map((path) => ({ ...file, path })) },
})
const TREE = () => named("README.md", "src/a.go", "src/b.go") // tree order: a.go, b.go, README.md
const selectedPath = () => screen.getByTestId("diff-area").getAttribute("data-file")
const press = (key: string, target: Element = document.body) => fireEvent.keyDown(target, { key })

test("n and p walk the visible rows with wrap-around and the file goes to the URL", async () => {
  const router = render(OPEN, async () => TREE())
  await screen.findByTestId("diff-area")
  expect(selectedPath()).toBe("src/a.go")
  press("n")
  await waitFor(() => {
    expect(selectedPath()).toBe("src/b.go")
    expect(router.state.location.search).toContain("file=src%2Fb.go")
  })
  press("n")
  await waitFor(() => expect(selectedPath()).toBe("README.md"))
  press("n")
  await waitFor(() => expect(selectedPath()).toBe("src/a.go"))
  press("p")
  await waitFor(() => expect(selectedPath()).toBe("README.md"))
})

const many = (n: number) => named(...Array.from({ length: n }, (_, i) => `f${i}.go`))
const filterBox = () => screen.queryByTestId("file-filter")

test("no filter and no heading at 15 files, a filter at 16", async () => {
  render(OPEN, async () => many(15))
  await screen.findByTestId("diff-area")
  expect(filterBox()).toBeNull()
  expect(screen.queryByTestId("files-heading")).toBeNull()
  cleanup()
  render(OPEN, async () => many(16))
  await screen.findByTestId("diff-area")
  expect(filterBox()).not.toBeNull()
})

test("/ shows the filter focused without typing a slash, and it stays", async () => {
  render(OPEN, async () => TREE())
  await screen.findByTestId("diff-area")
  expect(filterBox()).toBeNull()
  press("/")
  const input = (await screen.findByTestId("file-filter")) as HTMLInputElement
  await waitFor(() => expect(document.activeElement).toBe(input))
  expect(input.value).toBe("")
  fireEvent.change(input, { target: { value: "src" } })
  expect(screen.getAllByTestId("tree-file")).toHaveLength(2)
  fireEvent.change(input, { target: { value: "a.go" } })
  expect(filterBox()).not.toBeNull()
})

test("/ in a focused text field shows nothing new, and a modifier does nothing", async () => {
  render(OPEN, async () => TREE())
  await screen.findByTestId("diff-area")
  const edit = await openEdit()
  press("/", edit)
  expect(filterBox()).toBeNull()
  fireEvent.keyDown(document.body, { key: "/", ctrlKey: true })
  fireEvent.keyDown(document.body, { key: "/", metaKey: true })
  fireEvent.keyDown(document.body, { key: "/", altKey: true })
  expect(filterBox()).toBeNull()
})

test("the selected row is the only aria-selected one", async () => {
  render(OPEN, async () => TREE())
  await screen.findByTestId("diff-area")
  const rows = screen.getAllByTestId("tree-file")
  expect(rows.map((r) => r.getAttribute("aria-selected"))).toEqual(["true", "false", "false"])
})

test("a filter narrows the walk and p/n do nothing while it has focus", async () => {
  render(OPEN, async () => TREE())
  await screen.findByTestId("diff-area")
  press("/")
  const input = await screen.findByTestId("file-filter")
  fireEvent.change(input, { target: { value: "src" } })
  expect(screen.getAllByTestId("tree-file")).toHaveLength(2)
  press("n", input)
  expect(selectedPath()).toBe("src/a.go")
  press("n")
  await waitFor(() => expect(selectedPath()).toBe("src/b.go"))
})

test("a filter that matches nothing selects no file and says so", async () => {
  render(OPEN, async () => TREE())
  await screen.findByTestId("diff-area")
  press("/")
  fireEvent.change(await screen.findByTestId("file-filter"), { target: { value: "zzzz" } })
  expect(selectedPath()).toBeNull()
  expect(screen.getByTestId("no-files").textContent).toBe("No files in this selection.")
  expect(screen.getByTestId("file-tree").textContent).toBe('No files match "zzzz".')
  expect(screen.queryByTestId("file-path")).toBeNull()
})

test("the selected file is in the URL and a fresh render from that URL picks it", async () => {
  render(`${OPEN}&file=src%2Fb.go`, async () => TREE())
  await screen.findByTestId("diff-area")
  expect(selectedPath()).toBe("src/b.go")
  cleanup()
  render(`${OPEN}&file=nope.go`, async () => TREE())
  await screen.findByTestId("diff-area")
  expect(selectedPath()).toBe("src/a.go")
})

test("on a phone the Files and Diff tabs show one pane each and picking a file opens Diff", async () => {
  render(OPEN, async () => TREE(), "phone")
  const files = await screen.findByRole("tab", { name: "Files 3" })
  expect(screen.getByRole("tab", { name: "Diff" })).toBeTruthy()
  expect(screen.queryByTestId("diff-area")).toBeNull()
  fireEvent.click(files)
  fireEvent.click(screen.getAllByTestId("tree-file")[1])
  expect(await screen.findByTestId("diff-area")).toBeTruthy()
  expect(screen.queryByTestId("file-tree")).toBeNull()
  expect(screen.getByRole("tab", { name: "Diff" }).getAttribute("aria-selected")).toBe("true")
  fireEvent.click(screen.getByRole("tab", { name: "Files 3" }))
  expect(screen.getByTestId("file-tree")).toBeTruthy()
})

test("desktop has no tabs", async () => {
  render(OPEN, async () => TREE())
  await screen.findByTestId("diff-area")
  expect(screen.queryByRole("tab")).toBeNull()
})

test("the Split/Unified switch is in the header", async () => {
  render(OPEN, async () => files(1))
  await screen.findByTestId("range-chip")
  const toggle = screen.getByTestId("view-toggle")
  expect(screen.getByTestId("diff-header").contains(toggle)).toBe(true)
  expect(screen.getByRole("button", { name: "Unified" }).getAttribute("aria-pressed")).toBe("true")
  fireEvent.click(screen.getByRole("button", { name: "Split" }))
  expect(screen.getByRole("button", { name: "Split" }).getAttribute("aria-pressed")).toBe("true")
})

const style = () => (pierre.last?.options as { diffStyle: string } | undefined)?.diffStyle

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

test("a plain file's path line shows the path only, with no status or counts", async () => {
  render(OPEN, async () => TREE())
  const line = await screen.findByTestId("file-path")
  expect(line.textContent).toBe("src/a.go")
  expect(screen.queryByTestId("file-status")).toBeNull()
})

test("a renamed file shows both paths in its path line", async () => {
  const renamed: DiffFilesResult = {
    kind: "files",
    files: { ...range(), files: [{ ...file, path: "new.go", old_path: "old.go", status: "R" }] },
  }
  render(OPEN, async () => renamed)
  const line = await screen.findByTestId("file-path")
  expect(line.textContent).toBe("old.go → new.go")
})

test("large, generated and binary files get a path line and no body or fetch", async () => {
  const fetchPatch = vi.fn<FetchDiffPatch>(async () => "x")
  const flagged: DiffFilesResult = {
    kind: "files",
    files: { ...range(), files: [{ ...file, path: "big.go", large: true }] },
  }
  render(OPEN, async () => flagged, undefined, fetchPatch)
  expect((await screen.findByTestId("file-path")).textContent).toBe("big.go")
  expect(screen.getByRole("button", { name: "Load diff" })).toBeTruthy()
  expect(screen.queryByTestId("pierre")).toBeNull()
  expect(fetchPatch).not.toHaveBeenCalled()
})

test("unified by default, and the switch changes the view and survives a file change", async () => {
  render(OPEN, async () => TREE())
  await screen.findByTestId("pierre")
  expect(style()).toBe("unified")
  fireEvent.click(screen.getByRole("button", { name: "Split" }))
  await waitFor(() => expect(style()).toBe("split"))
  press("n")
  await waitFor(() =>
    expect(screen.getByTestId("pierre").getAttribute("data-patch")).toBe("patch of src/b.go"),
  )
  expect(style()).toBe("split")
  cleanup()
  localStorage.clear()
  render(OPEN, async () => TREE(), "phone")
  fireEvent.click((await screen.findAllByTestId("tree-file"))[0])
  await screen.findByTestId("pierre")
  expect(style()).toBe("unified")
})

test("a picked view is remembered by the next render, in either direction", async () => {
  render(OPEN, async () => TREE())
  await screen.findByTestId("pierre")
  fireEvent.click(screen.getByRole("button", { name: "Split" }))
  await waitFor(() => expect(style()).toBe("split"))
  cleanup()
  pierre.last = undefined
  render(OPEN, async () => TREE())
  await screen.findByTestId("pierre")
  expect(style()).toBe("split")
  fireEvent.click(screen.getByRole("button", { name: "Unified" }))
  await waitFor(() => expect(style()).toBe("unified"))
  cleanup()
  pierre.last = undefined
  render(OPEN, async () => TREE())
  await screen.findByTestId("pierre")
  expect(style()).toBe("unified")
})

test("a junk stored view renders unified, and nothing is written on mount", async () => {
  localStorage.setItem("gx.diff.view", "sideways")
  const setItem = vi.spyOn(Storage.prototype, "setItem")
  render(OPEN, async () => TREE())
  await screen.findByTestId("pierre")
  expect(style()).toBe("unified")
  expect(setItem).not.toHaveBeenCalled()
})

test("an unreadable store renders unified and the switch still works", async () => {
  vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
    throw new Error("blocked")
  })
  vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
    throw new Error("blocked")
  })
  render(OPEN, async () => TREE())
  await screen.findByTestId("pierre")
  expect(style()).toBe("unified")
  fireEvent.click(screen.getByRole("button", { name: "Split" }))
  await waitFor(() => expect(style()).toBe("split"))
})

const NARROW = "/diff?project=gx&base=main&tip=dev&epic=gx:e-impl"
const commitsOf = async () => ({ commits: COMMITS, truncated: false })
const patchesOf = vi.fn<FetchDiffCommitPatches>(async (_p, _a, _b, path, shas) =>
  COMMITS.filter((c) => shas.includes(c.sha) && c.files.includes(path)).map((c) => ({
    sha: c.sha,
    subject: c.subject,
    patch: `${c.sha} of ${path}`,
  })),
)
const narrowedRender = (path: string, over: Parameters<typeof renderDiff>[1] = {}) =>
  render(path, filesForCommits, undefined, undefined, {
    diffCommits: commitsOf,
    diffCommitPatches: patchesOf,
    ...over,
  })

test("a narrowed selected file shows one patch per commit in order, each under its subject, with no expand rows", async () => {
  narrowedRender(`${NARROW}&commits=c2,c4&file=src%2Fa.go`)
  await waitFor(() => expect(screen.getAllByTestId("pierre")).toHaveLength(2))
  expect(screen.getAllByTestId("commit-patch-label").map((l) => l.textContent)).toEqual([
    "Add a",
    "Fix a",
  ])
  expect(screen.getAllByTestId("pierre").map((p) => p.getAttribute("data-patch"))).toEqual([
    "c2 of src/a.go",
    "c4 of src/a.go",
  ])
  expect(pierre.last?.options).not.toHaveProperty("loadDiffFiles")
  expect(patchesOf).toHaveBeenLastCalledWith("gx", "a", "b", "src/a.go", ["c2", "c4"])
})

test("a narrowing that matches no returned commit shows an empty tree and says why", async () => {
  const fetchFiles = vi.fn(filesForCommits)
  render(`${NARROW}&commits=zzz`, fetchFiles, undefined, undefined, { diffCommits: commitsOf })
  expect((await screen.findByTestId("narrow-notice")).textContent).toBe(
    "No commits of this ticket in this range",
  )
  expect(screen.queryAllByTestId("tree-file")).toHaveLength(0)
  expect(fetchFiles).not.toHaveBeenCalledWith("gx", "main", "dev", expect.anything())
  cleanup()
  render(`${NARROW}&commits=zzz`, fetchFiles, undefined, undefined, {
    diffCommits: async () => ({ commits: COMMITS, truncated: true }),
  })
  expect((await screen.findByTestId("narrow-notice")).textContent).toBe(
    "Showing the first 500 commits",
  )
})

test("the narrowing survives a render from the URL and the header counts commits", async () => {
  const fetchFiles = vi.fn(filesForCommits)
  render(`${NARROW}&commits=c3`, fetchFiles, undefined, undefined, { diffCommits: commitsOf })
  await waitFor(() => expect(screen.getAllByTestId("tree-file")).toHaveLength(1))
  expect(fetchFiles).toHaveBeenCalledWith("gx", "main", "dev", ["c3"])
  expect(screen.queryByTestId("narrow-chip")).toBeNull()
  fireEvent.click(screen.getByRole("button", { expanded: false, name: /commits$/ }))
  expect(screen.getByRole("button", { current: true }).textContent).toContain("Ticket 07")
  expect(screen.getByTestId("diff-counts").textContent).toMatch(/^1 files /)
})

test("a ticket key narrows to that ticket's commits and highlights its row", async () => {
  const fetchFiles = vi.fn(filesForCommits)
  render(`${NARROW}&ticket=gx:e-impl/03`, fetchFiles, undefined, undefined, {
    diffCommits: commitsOf,
  })
  await waitFor(() => expect(screen.getAllByTestId("tree-file")).toHaveLength(1))
  expect(fetchFiles).toHaveBeenCalledWith("gx", "main", "dev", ["c2", "c4"])
  fireEvent.click(screen.getByRole("button", { expanded: false, name: /commits$/ }))
  expect(screen.getByRole("button", { current: true }).textContent).toContain("Ticket 03")
})

const focus = () => act(() => void window.dispatchEvent(new Event("focus")))
const treePaths = () => screen.getAllByTestId("tree-file").map((n) => n.textContent)

test("focus asks again with the same refs; nothing is fetched without it or while a ref is empty", async () => {
  const fetchFiles = vi.fn(async () => files(1))
  const fetchCommits = vi.fn(async () => ({ commits: [], truncated: false }))
  render(OPEN, fetchFiles, undefined, undefined, { diffCommits: fetchCommits })
  await waitFor(() => expect(fetchFiles).toHaveBeenCalledTimes(1))
  await new Promise((r) => setTimeout(r, 30))
  expect(fetchFiles).toHaveBeenCalledTimes(1)
  focus()
  await waitFor(() => expect(fetchFiles).toHaveBeenCalledTimes(2))
  expect(fetchFiles).toHaveBeenLastCalledWith("gx", "main", "dev")
  expect(fetchCommits).toHaveBeenCalledTimes(2)
  cleanup()
  const none = vi.fn(async () => files(1))
  render("/diff?project=gx&base=main", none)
  focus()
  await new Promise((r) => setTimeout(r, 30))
  expect(none).not.toHaveBeenCalled()
})

test("a focus refetch keeps the tree node and the open commit list, with no loader", async () => {
  let paths = ["a.go", "b.go"]
  const fetchFiles = vi.fn(async () => named(...paths))
  render(OPEN, fetchFiles, undefined, undefined, { diffCommits: commitsOf })
  await screen.findByTestId("diff-area")
  fireEvent.click(screen.getByRole("button", { expanded: false, name: /commits$/ }))
  const tree = screen.getByTestId("file-tree")
  paths = ["a.go", "b.go", "c.go"]
  focus()
  expect(screen.queryByText("Loading diff")).toBeNull()
  await waitFor(() => expect(treePaths()).toHaveLength(3))
  expect(screen.getByTestId("file-tree")).toBe(tree)
  expect(screen.queryByText("Loading diff")).toBeNull()
  expect(screen.getByRole("button", { expanded: true, name: /commits$/ })).toBeTruthy()
})

test("a refetch keeps the selected file when listed and falls back to the first when gone", async () => {
  let paths = ["a.go", "b.go"]
  render(`${OPEN}&file=b.go`, async () => named(...paths))
  await screen.findByTestId("diff-area")
  expect(selectedPath()).toBe("b.go")
  focus()
  await waitFor(() => expect(selectedPath()).toBe("b.go"))
  paths = ["c.go", "d.go"]
  focus()
  await waitFor(() => expect(selectedPath()).toBe("c.go"))
})

test("a refetch sends the commits= narrowing again and keeps its row highlighted", async () => {
  const fetchFiles = vi.fn(filesForCommits)
  render(`${NARROW}&commits=c3`, fetchFiles, undefined, undefined, { diffCommits: commitsOf })
  await waitFor(() => expect(screen.getAllByTestId("tree-file")).toHaveLength(1))
  fireEvent.click(screen.getByRole("button", { expanded: false, name: /commits$/ }))
  const narrowedCalls = () => fetchFiles.mock.calls.filter((c) => c[3]?.join() === "c3").length
  expect(narrowedCalls()).toBe(1)
  focus()
  await waitFor(() => expect(narrowedCalls()).toBe(2))
  expect(screen.getByRole("button", { current: true }).textContent).toContain("Ticket 07")
})

test("a failed refetch keeps the old tree", async () => {
  const fetchFiles = vi.fn<FetchDiffFiles>().mockResolvedValueOnce(named("a.go", "b.go"))
  render(OPEN, fetchFiles)
  await screen.findByTestId("diff-area")
  fetchFiles.mockRejectedValueOnce(new Error("boom"))
  focus()
  await waitFor(() => expect(fetchFiles).toHaveBeenCalledTimes(2))
  await new Promise((r) => setTimeout(r, 30))
  expect(treePaths()).toHaveLength(2)
  expect(screen.queryByTestId("notice")).toBeNull()
})

test("an older refetch that answers last does not replace the newer list", async () => {
  const answers: Array<(r: DiffFilesResult) => void> = []
  const fetchFiles = vi.fn<FetchDiffFiles>().mockResolvedValueOnce(named("a.go"))
  render(OPEN, fetchFiles)
  await screen.findByTestId("diff-area")
  fetchFiles.mockImplementation(() => new Promise((r) => answers.push(r)))
  focus()
  focus()
  await waitFor(() => expect(answers).toHaveLength(2))
  await act(async () => answers[1](named("new.go")))
  await waitFor(() => expect(treePaths()).toEqual(["new.go+1"]))
  await act(async () => answers[0](named("old.go", "older.go")))
  expect(treePaths()).toEqual(["new.go+1"])
})

const WORKTREE = "/diff?project=p&worktree=ticket:gx:e-impl/03&base=x&tip=y"
const snap = (n: number): DiffWorktree => ({
  base_sha: "base1",
  base_label: "main@base1",
  head_sha: `head${n}`,
  snapshot_sha: `snap${n}`,
  uncommitted: true,
})

test("worktree mode: the chip names target and base, and every call gets the route's SHAs", async () => {
  const fetchWorktree = vi.fn<FetchDiffWorktree>(async () => snap(1))
  const fetchFiles = vi.fn<FetchDiffFiles>(async () => named("a.go"))
  const fetchCommits = vi.fn(async () => ({ commits: [], truncated: false }))
  const fetchPatch = vi.fn<FetchDiffPatch>(async () => "patch")
  render(WORKTREE, fetchFiles, undefined, fetchPatch, {
    diffWorktree: fetchWorktree,
    diffCommits: fetchCommits,
  })
  const chip = await screen.findByTestId("range-chip")
  expect(chip.textContent).toBe("Worktree · gx:e-impl/03 since main@base1")
  expect(screen.queryByRole("textbox")).toBeNull()
  expect(screen.queryByRole("button", { name: "Edit refs" })).toBeNull()
  await screen.findByTestId("diff-area")
  expect(fetchWorktree).toHaveBeenCalledWith("p", "ticket:gx:e-impl/03")
  expect(fetchFiles).toHaveBeenCalledWith("p", "base1", "snap1")
  expect(fetchCommits).toHaveBeenCalledWith("p", "base1", "head1")
  await waitFor(() => expect(fetchPatch).toHaveBeenCalled())
  expect(fetchPatch).toHaveBeenCalledWith("p", "a", "b", "a.go")
})

test("worktree mode: a no-worktree reason shows an empty state naming the target and fetches nothing", async () => {
  const fetchFiles = vi.fn<FetchDiffFiles>(async () => named("a.go"))
  const fetchCommits = vi.fn(async () => ({ commits: [], truncated: false }))
  render(WORKTREE, fetchFiles, undefined, undefined, {
    diffWorktree: async () => ({ reason: "no_worktree" }),
    diffCommits: fetchCommits,
  })
  const notice = await screen.findByTestId("notice")
  expect(notice.getAttribute("data-kind")).toBe("no-worktree")
  expect(notice.textContent).toContain("gx:e-impl/03")
  await new Promise((r) => setTimeout(r, 30))
  expect(fetchFiles).not.toHaveBeenCalled()
  expect(fetchCommits).not.toHaveBeenCalled()
})

test("worktree mode: focus calls the worktree route again and the files follow the new SHAs", async () => {
  let n = 1
  const fetchWorktree = vi.fn<FetchDiffWorktree>(async () => snap(n))
  const fetchFiles = vi.fn<FetchDiffFiles>(async (_p, _a, second) =>
    second === "snap1" ? named("a.go") : named("a.go", "b.go"),
  )
  render(WORKTREE, fetchFiles, undefined, undefined, { diffWorktree: fetchWorktree })
  await waitFor(() => expect(treePaths()).toHaveLength(1))
  n = 2
  focus()
  await waitFor(() => expect(treePaths()).toHaveLength(2))
  expect(fetchWorktree).toHaveBeenCalledTimes(2)
  expect(fetchFiles).toHaveBeenLastCalledWith("p", "base1", "snap2")
})
