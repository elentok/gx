import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react"
import type { ReactNode } from "react"
import { afterEach, beforeEach, expect, test, vi } from "vitest"
import type { DiffFile } from "../generated/server"
import { renderThemed } from "../ui/render"
import type { BlobSide, DiffBlobsBody, FetchDiffBlob, FetchDiffPatch } from "./api"
import { DiffFileCard } from "./DiffFileCard"

const pierre = vi.hoisted(() => ({ last: undefined as Record<string, unknown> | undefined }))
vi.mock("@pierre/diffs/react", () => ({
  PatchDiff: (p: Record<string, unknown>) => {
    pierre.last = p
    return <div data-testid="pierre" />
  },
  Virtualizer: (p: { children: ReactNode; style: { height: string } }) => (
    <div data-testid="virtualizer" style={{ height: p.style.height }}>
      {p.children}
    </div>
  ),
}))

const range = { project: "gx", firstSha: "a".repeat(40), secondSha: "b".repeat(40) }
const base: DiffFile = { path: "a.go", status: "M", added: 1, deleted: 0 }

const text = (t: string): BlobSide => ({ kind: "text", text: t })
const blobs = (over: Partial<DiffBlobsBody> = {}): DiffBlobsBody => ({
  merge_base: range.firstSha,
  tip: range.secondSha,
  old: text("old"),
  new: text("new"),
  ...over,
})

type Props = { fetchPatch?: FetchDiffPatch; fetchBlob?: FetchDiffBlob; narrowed?: boolean }
function show(file: DiffFile, props: Props = {}) {
  const fetchPatch = props.fetchPatch ?? vi.fn(async () => "PATCH")
  const fetchBlob = props.fetchBlob ?? vi.fn(async () => blobs())
  renderThemed(
    <DiffFileCard
      key={file.path}
      file={file}
      range={range}
      view="split"
      fetchPatch={fetchPatch}
      fetchBlob={fetchBlob}
      narrowed={props.narrowed}
    />,
  )
  return { fetchPatch, fetchBlob }
}

type Options = { loadDiffFiles?: (m: unknown) => Promise<unknown>; hunkSeparators?: string }
const options = () => pierre.last?.options as Options

beforeEach(() => {
  pierre.last = undefined
  URL.createObjectURL = vi.fn(() => "blob:fake")
  URL.revokeObjectURL = vi.fn()
})
afterEach(cleanup)

test("a large file is collapsed and fetches nothing until Load diff", async () => {
  const { fetchPatch } = show({ ...base, large: true, added: 4000, deleted: 1000 })
  expect(screen.getByText(/Large diff, collapsed \(5000 lines\)/)).toBeTruthy()
  expect(fetchPatch).not.toHaveBeenCalled()
  fireEvent.click(screen.getByRole("button", { name: "Load diff" }))
  await screen.findByTestId("pierre")
  expect(fetchPatch).toHaveBeenCalledTimes(1)
  expect(screen.getByTestId("virtualizer").style.height).toBe("70vh")
})

test("a loaded generated file that is not large renders without Virtualizer", async () => {
  const { fetchPatch } = show({ ...base, generated: true })
  expect(screen.getByText(/Generated file, collapsed/)).toBeTruthy()
  expect(fetchPatch).not.toHaveBeenCalled()
  fireEvent.click(screen.getByRole("button", { name: "Load diff" }))
  await screen.findByTestId("pierre")
  expect(screen.queryByTestId("virtualizer")).toBeNull()
})

test("a new card for another file starts collapsed again", async () => {
  show({ ...base, large: true })
  fireEvent.click(screen.getByRole("button", { name: "Load diff" }))
  await screen.findByTestId("pierre")
  cleanup()
  show({ ...base, path: "b.go", large: true })
  expect(screen.getByRole("button", { name: "Load diff" })).toBeTruthy()
})

test("loadDiffFiles reads both blobs at the resolved SHAs", async () => {
  const { fetchBlob } = show(base)
  await screen.findByTestId("pierre")
  const loaded = await options().loadDiffFiles?.({ name: "a.go", type: "change" })
  expect(fetchBlob).toHaveBeenCalledWith("gx", range.firstSha, range.secondSha, "a.go")
  expect(loaded).toEqual({
    oldFile: { name: "a.go", contents: "old" },
    newFile: { name: "a.go", contents: "new" },
  })
})

test("a renamed file loads the old file under its old name, and a pure rename has none", async () => {
  show({ ...base, path: "new.go", old_path: "old.go", status: "R" })
  await screen.findByTestId("pierre")
  const changed = await options().loadDiffFiles?.({
    name: "new.go",
    prevName: "old.go",
    type: "rename-changed",
  })
  expect(changed).toMatchObject({ oldFile: { name: "old.go", contents: "old" } })
  const pure = await options().loadDiffFiles?.({
    name: "new.go",
    prevName: "old.go",
    type: "rename-pure",
  })
  expect(pure).toEqual({ oldFile: null, newFile: { name: "new.go", contents: "new" } })
})

test("a rejected blob call rejects out of loadDiffFiles", async () => {
  show(base, { fetchBlob: vi.fn().mockRejectedValue(new Error("boom")) })
  await screen.findByTestId("pierre")
  await expect(options().loadDiffFiles?.({ name: "a.go", type: "change" })).rejects.toThrow("boom")
})

test("a narrowed view offers no expansion", async () => {
  show(base, { narrowed: true })
  await screen.findByTestId("pierre")
  expect(options().loadDiffFiles).toBeUndefined()
  expect(options().hunkSeparators).toBe("simple")
})

const png = { ...base, path: "a.png", binary: true, status: "A" }
const image = (size: number): BlobSide => ({ kind: "image", size, image_base64: "AAEC" })

test("an added png shows none before and the size after", async () => {
  const { fetchPatch } = show(png, {
    fetchBlob: async () => blobs({ old: { kind: "absent" }, new: image(2150) }),
  })
  await screen.findByText("after: 2.1 KB")
  expect(screen.getByText("before: none")).toBeTruthy()
  await waitFor(() => expect(screen.getAllByRole("img")).toHaveLength(1))
  expect(fetchPatch).not.toHaveBeenCalled()
})

test("a changed png shows both sizes and revokes its URLs on unmount", async () => {
  show(
    { ...png, status: "M" },
    { fetchBlob: async () => blobs({ old: image(1024), new: image(2048) }) },
  )
  await screen.findByText("after: 2.0 KB")
  expect(screen.getByText("before: 1.0 KB")).toBeTruthy()
  await waitFor(() => expect(screen.getAllByRole("img")).toHaveLength(2))
  cleanup()
  expect(URL.revokeObjectURL).toHaveBeenCalledWith("blob:fake")
})

test.each([
  ["an svg", { ...base, path: "a.svg", status: "A" }, { kind: "text", size: 3072 } as BlobSide],
  [
    "another binary",
    { ...base, path: "a.bin", binary: true },
    { kind: "binary", size: 3072 } as BlobSide,
  ],
  ["a png over 2 MB", png, { kind: "too_large", size: 3072 } as BlobSide],
])("%s shows Binary file with no image and no patch call", async (_n, file, side) => {
  const { fetchPatch } = show(file, {
    fetchBlob: async () => blobs({ old: { kind: "absent" }, new: side }),
  })
  await screen.findByText("Binary file, 3.0 KB")
  expect(screen.queryByRole("img")).toBeNull()
  expect(fetchPatch).not.toHaveBeenCalled()
})
