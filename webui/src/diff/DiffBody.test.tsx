import { render } from "@testing-library/react"
import { expect, test, vi } from "vitest"
import { DiffBody, UNSAFE_CSS, lineAnchors } from "./DiffBody"

const props = vi.hoisted(() => ({ last: undefined as Record<string, unknown> | undefined }))
vi.mock("@pierre/diffs/react", () => ({
  PatchDiff: (p: Record<string, unknown>) => {
    props.last = p
    return <div data-testid="pierre" />
  },
}))

const PATCH = `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -3,3 +3,3 @@
 keep
-old
+new
 keep
`

test("passes the patch and the view to Pierre", () => {
  const { rerender } = render(<DiffBody patch={PATCH} view="split" path="a.go" />)
  expect(props.last).toMatchObject({ patch: PATCH, options: { diffStyle: "split" } })
  rerender(<DiffBody patch={PATCH} view="unified" path="a.go" />)
  expect(props.last).toMatchObject({ options: { diffStyle: "unified" } })
})

test("maps the surface and mono tokens at 12px and 18px", () => {
  render(<DiffBody patch={PATCH} view="split" path="a.go" />)
  const options = props.last?.options as { unsafeCSS: string }
  expect(options.unsafeCSS).toBe(UNSAFE_CSS)
  expect(UNSAFE_CSS).toContain("--chakra-colors-surface")
  expect(UNSAFE_CSS).toContain("--chakra-fonts-mono")
  expect(UNSAFE_CSS).toMatch(/pre[^{]*\{[^}]*font-size: 12px; line-height: 18px/)
  expect(UNSAFE_CSS).toMatch(/code \{ font-size: 12px; line-height: 18px/)
})

test("anchors the added line on the additions side and the deleted line on the deletions side", () => {
  render(<DiffBody patch={PATCH} view="split" path="a.go" />)
  expect(props.last?.lineAnnotations).toEqual([
    { side: "deletions", lineNumber: 4, metadata: { path: "a.go" } },
    { side: "additions", lineNumber: 4, metadata: { path: "a.go" } },
  ])
})

test("numbers lines across hunks and ignores the no-newline marker", () => {
  const patch = `@@ -1,2 +1,3 @@
 a
+b
 c
@@ -10 +11,0 @@
-z
\\ No newline at end of file
`
  expect(lineAnchors(patch, "f")).toEqual([
    { side: "additions", lineNumber: 2, metadata: { path: "f" } },
    { side: "deletions", lineNumber: 10, metadata: { path: "f" } },
  ])
})
