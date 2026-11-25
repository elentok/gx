import { cleanup, fireEvent, screen } from "@testing-library/react"
import { useState } from "react"
import { afterEach, expect, test } from "vitest"
import type { DiffFile } from "../generated/server"
import { renderThemed } from "../ui/render"
import { FileFilter, FileTree } from "./FileTreePanel"

afterEach(cleanup)

const f = (path: string, over: Partial<DiffFile> = {}): DiffFile => ({
  path,
  status: "M",
  added: 3,
  deleted: 1,
  ...over,
})
const FILES = [f("README.md"), f("src/a.go", { status: "A" }), f("src/b.go", { status: "D" })]

function Harness({ files = FILES, selected }: { files?: DiffFile[]; selected?: string }) {
  const [filter, setFilter] = useState("")
  return (
    <>
      <FileFilter value={filter} onChange={setFilter} />
      <FileTree files={files} filter={filter} selected={selected} onSelect={() => {}} />
    </>
  )
}
const paths = () => screen.queryAllByTestId("tree-file").map((e) => e.getAttribute("data-path"))
const type = (v: string) =>
  fireEvent.change(screen.getByTestId("file-filter"), { target: { value: v } })
const row = (path: string) =>
  screen.getAllByTestId("tree-file").find((r) => r.getAttribute("data-path") === path)!

test("a directory is a plain label: no chevron, count or button, and no FILES heading", () => {
  renderThemed(<Harness />)
  const dir = screen.getByTestId("tree-dir")
  expect(dir.textContent).toBe("src")
  expect(dir.tagName).not.toBe("BUTTON")
  expect(screen.queryByTestId("dir-count")).toBeNull()
  expect(screen.queryByTestId("files-heading")).toBeNull()
  fireEvent.click(dir)
  expect(paths()).toEqual(["src/a.go", "src/b.go", "README.md"])
})

test("an added file has the done dot; modified, deleted and renamed have the waiting dot", () => {
  renderThemed(
    <Harness
      files={[
        f("a.go", { status: "A" }),
        f("m.go", { status: "M" }),
        f("d.go", { status: "D" }),
        f("r.go", { status: "R" }),
      ]}
    />,
  )
  const tone = (p: string) =>
    row(p).querySelector("[data-testid=file-dot]")?.getAttribute("data-tone")
  expect([tone("a.go"), tone("m.go"), tone("d.go"), tone("r.go")]).toEqual([
    "done",
    "waiting",
    "waiting",
    "waiting",
  ])
})

test("a row shows +n and −n, hides a zero side, and has no letter or change bar", () => {
  renderThemed(
    <Harness files={[f("both.go"), f("add.go", { deleted: 0 }), f("del.go", { added: 0 })]} />,
  )
  expect(row("both.go").querySelector("[data-testid=file-counts]")?.textContent).toBe("+3 −1")
  expect(row("add.go").querySelector("[data-testid=file-counts]")?.textContent).toBe("+3")
  expect(row("del.go").querySelector("[data-testid=file-counts]")?.textContent).toBe("−1")
  expect(screen.queryByTestId("file-status")).toBeNull()
  expect(screen.queryByTestId("change-bar")).toBeNull()
})

test("the name is in the Saira text font, not mono", () => {
  renderThemed(<Harness />)
  const names = screen.getAllByTestId("tree-file")
  for (const r of names) expect(getComputedStyle(r).fontFamily).not.toMatch(/mono/i)
})

test("only the selected row is aria-selected", () => {
  renderThemed(<Harness selected="src/b.go" />)
  expect(screen.getAllByTestId("tree-file").map((r) => r.getAttribute("aria-selected"))).toEqual([
    "false",
    "true",
    "false",
  ])
})

test("the filter is case-insensitive and rebuilds the tree from the matches", () => {
  renderThemed(<Harness />)
  type("readme")
  expect(paths()).toEqual(["README.md"])
  expect(screen.queryByTestId("tree-dir")).toBeNull()
})

test("a filter with no match says No files match", () => {
  renderThemed(<Harness />)
  type("zzz")
  expect(screen.getByText('No files match "zzz".')).toBeTruthy()
  expect(paths()).toEqual([])
})
