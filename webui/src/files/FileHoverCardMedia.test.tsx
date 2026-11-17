import { act, fireEvent } from "@testing-library/react"
import { useState } from "react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import {
  type HostedResolved,
  HostedFound,
  HostedKindImage,
  HostedKindMarkdown,
  HostedKindMermaid,
  HostedKindSVG,
} from "../generated/ops"
import { renderThemed } from "../ui/render"
import type { PixelSize } from "./FileHoverCardMedia"
import { FileLinkProvider, useFileLinks } from "./FileLinkContext"
import { FilePreviewBody, FilePreviewHeader } from "./FilePreviewBody"
import { previewCache } from "./previewCache"

vi.mock("./renderMermaid", () => ({ renderMermaid: vi.fn() }))
import { renderMermaid } from "./renderMermaid"

const settle = () => act(async () => {})

function file(kind: HostedResolved["kind"], size = 2048): HostedResolved {
  return { candidate: "x", verdict: HostedFound, path: "/abs/a b.png", size, kind, modified: "t1" }
}

function Card({ f }: { f: HostedResolved }) {
  const [pixels, setPixels] = useState<PixelSize>()
  const links = useFileLinks()
  return (
    <>
      <FilePreviewHeader path="/abs/a b.png" file={f} pixels={pixels} />
      <FilePreviewBody
        path="/abs/a b.png"
        file={f}
        missing={links?.isMissing("/abs/a b.png") ?? false}
        onPixelSize={setPixels}
      />
      <span data-testid="missing">{String(links?.isMissing("/abs/a b.png"))}</span>
    </>
  )
}

async function show(f: HostedResolved) {
  const view = renderThemed(
    <FileLinkProvider>
      <Card f={f} />
    </FileLinkProvider>,
  )
  await settle()
  return view
}

function fakeFetch(status: number, body = "") {
  return vi
    .spyOn(globalThis, "fetch")
    .mockImplementation(async () => new Response(body, { status }))
}

describe("FileHoverCardMedia", () => {
  beforeEach(() => {
    previewCache.clear()
    vi.mocked(renderMermaid).mockReset()
  })
  afterEach(() => vi.restoreAllMocks())

  it("renders a Mermaid file with the shared renderer", async () => {
    vi.mocked(renderMermaid).mockResolvedValue('<svg data-testid="d"></svg>')
    const spy = fakeFetch(200, "graph TD\nA-->B")
    const { container } = await show(file(HostedKindMermaid))
    expect(renderMermaid).toHaveBeenCalledWith("graph TD\nA-->B")
    expect(container.querySelector('[data-diagram="done"] svg')).not.toBeNull()
    // A second hover uses the cache.
    spy.mockClear()
    await show(file(HostedKindMermaid))
    expect(spy).not.toHaveBeenCalled()
  })

  it("does not call the renderer for Markdown", async () => {
    fakeFetch(200, "# hi")
    await show(file(HostedKindMarkdown))
    expect(renderMermaid).not.toHaveBeenCalled()
  })

  it("shows the parse error", async () => {
    vi.mocked(renderMermaid).mockRejectedValue(new Error("Parse error on line 2"))
    fakeFetch(200, "graph TD\n???")
    const { container } = await show(file(HostedKindMermaid))
    expect(container.querySelector('[role="alert"]')?.textContent).toBe("Parse error on line 2")
  })

  it("shows a thumbnail from the raw route and adds WxH after load", async () => {
    const { container } = await show(file(HostedKindImage, 212 * 1024))
    const img = container.querySelector("img") as HTMLImageElement
    expect(img.getAttribute("src")).toBe("/v1/files/abs/a%20b.png")
    expect(container.textContent).toContain("212 KB")
    expect(container.textContent).not.toContain("×")
    Object.defineProperty(img, "naturalWidth", { value: 1280 })
    Object.defineProperty(img, "naturalHeight", { value: 800 })
    fireEvent.load(img)
    expect(container.textContent).toContain("212 KB · 1280×800")
  })

  it("shows an SVG through <img>, never inline", async () => {
    const { container } = await show(file(HostedKindSVG))
    expect(container.querySelector("img")).not.toBeNull()
    expect(container.querySelector("svg")).toBeNull()
  })

  it("marks the link missing when the image fails and the raw URL is a 404", async () => {
    fakeFetch(404)
    const { container, getByTestId } = await show(file(HostedKindImage))
    fireEvent.error(container.querySelector("img") as HTMLImageElement)
    await settle()
    expect(getByTestId("missing").textContent).toBe("true")
  })

  it.each([200, 500])(
    "shows No preview and keeps the link when the image fails with %i",
    async (status) => {
      fakeFetch(status)
      const { container, getByTestId } = await show(file(HostedKindImage))
      fireEvent.error(container.querySelector("img") as HTMLImageElement)
      await settle()
      expect(container.textContent).toContain("No preview")
      expect(getByTestId("missing").textContent).toBe("false")
    },
  )

  it("marks the link missing on a 404 for Mermaid", async () => {
    fakeFetch(404)
    const { getByTestId } = await show(file(HostedKindMermaid))
    expect(getByTestId("missing").textContent).toBe("true")
  })
})
