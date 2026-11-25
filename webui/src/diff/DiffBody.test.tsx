import { render } from "@testing-library/react"
import { expect, test, vi } from "vitest"
import { DiffBody, UNSAFE_CSS, lineAnchors } from "./DiffBody"

const props = vi.hoisted(() => ({ last: undefined as Record<string, unknown> | undefined }))
// Registration runs at import time, so only a module mock can see the call.
const registerTheme = vi.hoisted(() => vi.fn())
vi.mock("@pierre/diffs", () => ({ registerCustomCSSVariableTheme: registerTheme }))
vi.mock("@pierre/diffs/react", () => ({
  PatchDiff: (p: Record<string, unknown>) => {
    props.last = p
    return <div data-testid="pierre" />
  },
  Virtualizer: ({ children }: { children: unknown }) => <>{children}</>,
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

test("maps the theme and accent tokens in one :host block at 12.5px and 19px", () => {
  render(<DiffBody patch={PATCH} view="split" path="a.go" />)
  const options = props.last?.options as { unsafeCSS: string; theme: string }
  expect(options.unsafeCSS).toBe(UNSAFE_CSS)
  expect(options.theme).toBe("gx")
  expect(UNSAFE_CSS.match(/:host \{/g)).toHaveLength(1)
  const decl = (name: string) => new RegExp(`${name}: ([^;]+);`).exec(UNSAFE_CSS)?.[1]
  const c = (t: string) => `var(--chakra-colors-${t})`
  expect(decl("--diffs-font-family")).toBe("var(--chakra-fonts-mono)")
  expect(decl("--diffs-font-size")).toBe("12.5px")
  expect(decl("--diffs-line-height")).toBe("19px")
  expect(UNSAFE_CSS).toContain("pre, code { font-size: 12.5px; line-height: 19px; }")
  const want: Record<string, string> = {
    foreground: "text-muted",
    background: "surface",
    bg: "surface",
    "token-comment": "text-faint",
    "token-parameter": "text-muted",
    "token-punctuation": "text-dim",
    "token-keyword": "kind-design-fg",
    "token-string": "kind-implementation-fg",
    "token-string-expression": "kind-implementation-fg",
    "token-function": "tone-running-fg",
    "token-constant": "tone-waiting-fg",
    "bg-context-gutter-override": "surface",
    "addition-color-override": "tone-done-fg",
    "deletion-color-override": "tone-deadlocked-fg",
  }
  for (const [name, tok] of Object.entries(want)) expect(decl(`--diffs-${name}`)).toBe(c(tok))
  expect(decl("--diffs-bg-addition-emphasis-override")).toBe(
    "color-mix(in srgb, var(--chakra-colors-tone-done-fg) 30%, transparent)",
  )
  expect(decl("--diffs-bg-deletion-emphasis-override")).toBe(
    "color-mix(in srgb, var(--chakra-colors-tone-deadlocked-fg) 30%, transparent)",
  )
  expect(UNSAFE_CSS).not.toContain("!important")
  expect(UNSAFE_CSS).not.toContain("diffs-host")
})

test("registers the theme once with the token defaults", () => {
  expect(registerTheme).toHaveBeenCalledTimes(1)
  expect(registerTheme).toHaveBeenCalledWith(
    "gx",
    expect.objectContaining({
      foreground: "var(--chakra-colors-text-muted)",
      "token-keyword": "var(--chakra-colors-kind-design-fg)",
    }),
  )
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
