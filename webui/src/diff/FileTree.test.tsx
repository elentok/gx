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

function Harness() {
  const [filter, setFilter] = useState("")
  return (
    <>
      <FileFilter value={filter} onChange={setFilter} />
      <FileTree files={FILES} filter={filter} selected={undefined} onSelect={() => {}} />
    </>
  )
}
const paths = () => screen.queryAllByTestId("tree-file").map((e) => e.getAttribute("data-path"))
const type = (v: string) =>
  fireEvent.change(screen.getByTestId("file-filter"), { target: { value: v } })

test("collapsing a directory hides its rows", () => {
  renderThemed(<Harness />)
  expect(paths()).toEqual(["src/a.go", "src/b.go", "README.md"])
  fireEvent.click(screen.getByTestId("tree-dir"))
  expect(paths()).toEqual(["README.md"])
  expect(screen.getByTestId("dir-count").textContent).toBe("2")
  fireEvent.click(screen.getByTestId("tree-dir"))
  expect(paths()).toHaveLength(3)
})

test("a file row shows its status letter and a change bar", () => {
  renderThemed(<Harness />)
  const rows = screen.getAllByTestId("tree-file")
  expect(rows.map((r) => r.querySelector("[data-testid=file-status]")?.textContent)).toEqual([
    "A",
    "D",
    "M",
  ])
  expect(rows[0].querySelector("[data-testid=change-bar]")).not.toBeNull()
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
  expect(screen.getByText("No files match")).toBeTruthy()
  expect(paths()).toEqual([])
})
