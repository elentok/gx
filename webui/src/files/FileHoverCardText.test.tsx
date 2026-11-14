import { act } from "@testing-library/react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import {
  type HostedResolved,
  HostedFound,
  HostedKindCode,
  HostedKindMarkdown,
} from "../generated/ops"
import { renderThemed } from "../ui/render"
import { FileHoverCardText } from "./FileHoverCardText"
import { FileLinkProvider, useFileLinks } from "./FileLinkContext"
import { previewCache } from "./previewCache"

const settle = () => act(async () => {})

function file(kind: HostedResolved["kind"], modified = "t1"): HostedResolved {
  return { candidate: "x", verdict: HostedFound, path: "/abs/a.x", size: 10, kind, modified }
}

function Missing() {
  return <span data-testid="missing">{String(useFileLinks()?.isMissing("/abs/a.x"))}</span>
}

function fakeFetch(status: number, body = "") {
  return vi
    .spyOn(globalThis, "fetch")
    .mockImplementation(async () => new Response(body, { status }))
}

async function show(f: HostedResolved, line?: number) {
  const view = renderThemed(
    <FileLinkProvider>
      <FileHoverCardText path="/abs/a.x" file={f} line={line} />
      <Missing />
    </FileLinkProvider>,
  )
  await settle()
  return view
}

describe("FileHoverCardText", () => {
  const scroll = vi.fn()
  beforeEach(() => {
    previewCache.clear()
    scroll.mockClear()
    Element.prototype.scrollIntoView = scroll
  })
  afterEach(() => vi.restoreAllMocks())

  it("renders Markdown with plain links, alt text for relative images and a plain mermaid fence", async () => {
    fakeFetch(
      200,
      "# Title\n\n- one\n- two\n\n[rel](./b.md) [frag](#x)\n\n![the alt](pic.png)\n\n```mermaid\ngraph TD\n```\n",
    )
    const { container } = await show(file(HostedKindMarkdown))
    expect(container.querySelector("h1")?.textContent).toBe("Title")
    expect(container.querySelectorAll("li")).toHaveLength(2)
    expect(container.querySelector("a")).toBeNull()
    expect(container.querySelector("img")).toBeNull()
    expect(container.textContent).toContain("the alt")
    expect(container.querySelector("pre")?.textContent).toContain("graph TD")
  })

  it("shows numbered lines for code, marks the cited line and scrolls to it", async () => {
    const text = Array.from({ length: 60 }, (_, i) => `line ${i + 1}`).join("\n")
    fakeFetch(200, text)
    const { container } = await show(file(HostedKindCode), 42)
    expect(container.querySelectorAll("[data-line]")).toHaveLength(60)
    const marked = container.querySelector('[data-line="42"]')
    expect(marked?.getAttribute("aria-current")).toBe("true")
    expect(scroll).toHaveBeenCalled()
    expect(scroll.mock.instances[0]).toBe(marked)
  })

  it("fetches once for two views of the same path and mtime", async () => {
    const spy = fakeFetch(200, "hello")
    const first = await show(file(HostedKindCode))
    first.unmount()
    await show(file(HostedKindCode))
    expect(spy).toHaveBeenCalledTimes(1)
    const again = await show(file(HostedKindCode, "t2"))
    expect(spy).toHaveBeenCalledTimes(2)
    again.unmount()
  })

  it("marks the file missing on a 404", async () => {
    fakeFetch(404)
    const { getByTestId } = await show(file(HostedKindCode))
    expect(getByTestId("missing").textContent).toBe("true")
  })
})
