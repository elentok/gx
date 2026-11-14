import { expect, test } from "vitest"
import type { DiffFile } from "../generated/server"
import { buildTree } from "./fileTree"

const f = (path: string, over: Partial<DiffFile> = {}): DiffFile => ({
  path,
  status: "M",
  added: 1,
  deleted: 0,
  ...over,
})
const shape = (files: DiffFile[], collapsed: string[] = []) =>
  buildTree(files, new Set(collapsed)).map((r) =>
    r.kind === "dir"
      ? `${"  ".repeat(r.depth)}${r.name}/ (${r.count})`
      : `${"  ".repeat(r.depth)}${r.name}`,
  )

test("directories come first, each group sorted by name", () => {
  expect(shape([f("z.go"), f("b/x.go"), f("a.go"), f("a/y.go"), f("a/b.go")])).toEqual([
    "a/ (2)",
    "  b.go",
    "  y.go",
    "b/ (1)",
    "  x.go",
    "a.go",
    "z.go",
  ])
})

test("a chain of single-directory parents folds into one row", () => {
  expect(shape([f("a/b/c/x.go"), f("a/b/c/y.go")])).toEqual(["a/b/c/ (2)", "  x.go", "  y.go"])
})

test("a directory that also branches does not fold", () => {
  expect(shape([f("a/b/x.go"), f("a/c/y.go")])).toEqual([
    "a/ (2)",
    "  b/ (1)",
    "    x.go",
    "  c/ (1)",
    "    y.go",
  ])
})

test("a collapsed directory hides its rows and keeps its count", () => {
  expect(shape([f("a/b/x.go"), f("a/c/y.go")], ["a"])).toEqual(["a/ (2)"])
})

test("a renamed file shows its new path and keeps the status and source", () => {
  const rows = buildTree([f("n/new.go", { status: "R", old_path: "o/old.go" })])
  const file = rows.find((r) => r.kind === "file")
  expect(file).toMatchObject({ name: "new.go", file: { status: "R", old_path: "o/old.go" } })
})
