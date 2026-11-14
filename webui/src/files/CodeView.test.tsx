import { cleanup, screen } from "@testing-library/react"
import { afterEach, beforeEach, expect, test, vi } from "vitest"
import { renderThemed } from "../ui/render"
import { CodeView } from "./CodeView"

const scrollIntoView = vi.fn()
beforeEach(() => {
  scrollIntoView.mockClear()
  Element.prototype.scrollIntoView = scrollIntoView
})
afterEach(cleanup)

const source = "package main\n\nfunc main() {\n}\n"

const rows = () => Array.from(document.querySelectorAll("[data-line]"))

test("N lines make N numbered rows and the text is only the code", () => {
  renderThemed(<CodeView text={source} />)
  expect(rows().map((r) => r.getAttribute("data-line"))).toEqual(["1", "2", "3", "4"])
  expect(screen.getByTestId("code-view").textContent).toBe("package mainfunc main() {}")
})

test("the cited line is marked and scrolled into view", () => {
  renderThemed(<CodeView text={source} line={3} />)
  const marked = rows().filter((r) => r.getAttribute("aria-current") === "true")
  expect(marked.map((r) => r.getAttribute("data-line"))).toEqual(["3"])
  expect(scrollIntoView).toHaveBeenCalledTimes(1)
})

test.each([99, 0, -1, 1.5, NaN])("line %s marks and scrolls nothing", (line) => {
  renderThemed(<CodeView text={source} line={line} />)
  expect(document.querySelector("[aria-current]")).toBeNull()
  expect(scrollIntoView).not.toHaveBeenCalled()
})

test("highlighted output appears after the lazy import resolves", async () => {
  renderThemed(<CodeView text={source} language="go" />)
  expect(document.querySelector(".hljs-keyword")).toBeNull()
  await vi.waitFor(() => expect(document.querySelector(".hljs-keyword")).not.toBeNull())
  expect(rows()).toHaveLength(4)
  expect(screen.getByTestId("code-view").textContent).toBe("package mainfunc main() {}")
})

test("a language the highlighter lacks stays plain", async () => {
  renderThemed(<CodeView text={source} language="nope" />)
  await new Promise((r) => setTimeout(r, 50))
  expect(document.querySelector(".hljs-keyword")).toBeNull()
  expect(rows()).toHaveLength(4)
})
