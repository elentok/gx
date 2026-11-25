import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react"
import { afterEach, expect, test, vi } from "vitest"
import type { DiffRange } from "../generated/server"
import type { Device } from "../useWide"
import type { DiffFilesResult, FetchDiffCommitPatches, FetchDiffFiles, FetchDiffPatch } from "./api"
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
  expect(screen.queryByTestId("file-card-header")).toBeNull()
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
  expect(screen.getByTestId("narrow-chip").textContent).toContain("Showing: Ticket 07")
  expect(screen.getByTestId("diff-counts").textContent).toMatch(/^1 files /)
})

test("a ticket key narrows to that ticket's commits and the chip names the ticket", async () => {
  const fetchFiles = vi.fn(filesForCommits)
  render(`${NARROW}&ticket=gx:e-impl/03`, fetchFiles, undefined, undefined, {
    diffCommits: commitsOf,
  })
  await waitFor(() => expect(screen.getAllByTestId("tree-file")).toHaveLength(1))
  expect(fetchFiles).toHaveBeenCalledWith("gx", "main", "dev", ["c2", "c4"])
  expect(screen.getByTestId("narrow-chip").textContent).toContain("Showing: Ticket 03")
})
