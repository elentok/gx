import { act, cleanup, fireEvent, screen, waitFor } from "@testing-library/react"
import { createMemoryRouter, RouterProvider } from "react-router"
import { afterEach, beforeEach, expect, test, vi } from "vitest"
import {
  HostedFound,
  HostedKindBinary,
  HostedKindCode,
  HostedKindImage,
  HostedKindMarkdown,
  HostedKindPDF,
  HostedKindSVG,
  HostedKindText,
  type HostedResolved,
} from "../generated/ops"
import { ConnectionContext, createConnection } from "../connection"
import { appRoutes } from "../routes"
import { renderThemed } from "../ui/render"
import type { FetchFileResolve } from "./FileViewerPage"
import type { FetchFileText } from "./rawText"

vi.mock("../webPerf", () => ({ queueSample: vi.fn(), enableQueue: vi.fn(), dropQueue: vi.fn() }))

afterEach(cleanup)
beforeEach(() => sessionStorage.clear())

const found = (path: string): HostedResolved => ({
  candidate: path,
  verdict: HostedFound,
  path,
  size: 3174,
  kind: HostedKindMarkdown,
  modified: new Date(Date.now() - 2 * 60_000).toISOString(),
})

// Never started, so it touches no network.
const idleConnection = createConnection({ fetchSnapshot: () => new Promise(() => {}) })

function renderAt(
  route: string,
  fileResolve: FetchFileResolve,
  fileText: FetchFileText = async () => "# Hello",
) {
  const router = createMemoryRouter(appRoutes({ fileResolve, fileText }), {
    initialEntries: [route],
  })
  renderThemed(
    <ConnectionContext.Provider value={idleConnection}>
      <RouterProvider router={router} />
    </ConnectionContext.Provider>,
  )
  return router
}

test("a found file shows path, meta, buttons and the neutral note", async () => {
  const resolve = vi.fn(async (p: string) => found(p))
  renderAt("/files/Users/me/my%20notes.md", resolve)
  expect(await screen.findByText("/Users/me/my notes.md")).toBeTruthy()
  expect(resolve).toHaveBeenCalledWith("/Users/me/my notes.md")
  expect(screen.getByText("3.1 KB · 2m ago")).toBeTruthy()
  expect(await screen.findByRole("heading", { name: "Hello" })).toBeTruthy()
  const raw = screen.getByRole("link", { name: "Raw" })
  expect(raw.getAttribute("href")).toBe("/v1/files/Users/me/my%20notes.md")
  expect(raw.getAttribute("target")).toBe("_blank")
  expect(raw.getAttribute("rel")).toBe("noopener")
  const download = screen.getByRole("link", { name: "Download" })
  expect(download.getAttribute("href")).toBe("/v1/files/Users/me/my%20notes.md?download=1")
  expect(download.hasAttribute("download")).toBe(true)
  expect(screen.getByRole("button", { name: "Copy link" })).toBeTruthy()
})

test("a literal percent is not decoded twice", async () => {
  const resolve = vi.fn(async (p: string) => found(p))
  renderAt("/files/Users/me/100%25.md", resolve)
  expect(await screen.findByText("/Users/me/100%.md")).toBeTruthy()
  expect(resolve).toHaveBeenCalledWith("/Users/me/100%.md")
  expect(screen.getByRole("link", { name: "Raw" }).getAttribute("href")).toBe(
    "/v1/files/Users/me/100%25.md",
  )
})

test("Copy link writes the stable viewer link", async () => {
  const writeText = vi.fn(async () => {})
  Object.defineProperty(navigator, "clipboard", { value: { writeText }, configurable: true })
  renderAt("/files/Users/me/my%20notes.md", async (p) => found(p))
  fireEvent.click(await screen.findByRole("button", { name: "Copy link" }))
  await waitFor(() =>
    expect(writeText).toHaveBeenCalledWith(
      `${window.location.origin}/#/files/Users/me/my%20notes.md`,
    ),
  )
  expect(await screen.findByText("Link copied")).toBeTruthy()
})

test("any missing reply shows the same not-found page with only Copy link", async () => {
  const html: string[] = []
  for (const verdict of ["missing", "denied", "outside"]) {
    renderAt("/files/home/me/.env", async (p) => ({ candidate: p, verdict }))
    await screen.findByText("File not found")
    expect(
      screen.getByText("It may have been deleted, moved or outside the known folders."),
    ).toBeTruthy()
    expect(screen.queryByRole("link", { name: "Raw" })).toBeNull()
    expect(screen.queryByRole("link", { name: "Download" })).toBeNull()
    expect(screen.getByRole("button", { name: "Copy link" })).toBeTruthy()
    html.push(screen.getByTestId("viewer-bar").parentElement?.innerHTML ?? "")
    cleanup()
  }
  expect(new Set(html).size).toBe(1)
})

test("a failed resolve shows an error with Retry, not not-found", async () => {
  let calls = 0
  renderAt("/files/tmp/a.md", async (p) => {
    if (++calls === 1) throw new Error("boom")
    return found(p)
  })
  expect(await screen.findByText("Could not load file")).toBeTruthy()
  expect(screen.queryByText("File not found")).toBeNull()
  fireEvent.click(screen.getByRole("button", { name: "Retry" }))
  expect(await screen.findByRole("heading", { name: "Hello" })).toBeTruthy()
})

test("a Markdown file opens rendered; Source toggles raw text with no refetch or URL change", async () => {
  const fetchText = vi.fn(async () => "# Hello\n\nsome *words*")
  const resolve = vi.fn(async (p: string) => found(p))
  const router = renderAt("/files/Users/me/a.md", resolve, fetchText)
  expect(await screen.findByRole("heading", { name: "Hello" })).toBeTruthy()
  const before = router.state.location.pathname + router.state.location.search
  fireEvent.click(screen.getByRole("button", { name: "Source" }))
  expect(screen.queryByRole("heading", { name: "Hello" })).toBeNull()
  expect(document.body.textContent).toContain("some *words*")
  fireEvent.click(screen.getByRole("button", { name: "Source" }))
  expect(await screen.findByRole("heading", { name: "Hello" })).toBeTruthy()
  expect(fetchText).toHaveBeenCalledTimes(1)
  expect(resolve).toHaveBeenCalledTimes(1)
  expect(router.state.location.pathname + router.state.location.search).toBe(before)
})

const withKind = (p: string, kind: HostedResolved["kind"], size = 1000): HostedResolved => ({
  ...found(p),
  kind,
  size,
})

// The page header has its own logo <img>, so pick the viewer's by its raw-route src.
const rawImg = 'img[src^="/v1/files/"]'
const findViewerImg = () =>
  waitFor(() => {
    const img = document.querySelector<HTMLElement>(rawImg)
    if (!img) throw new Error("viewer image not rendered")
    return img
  })

function expectNoSource() {
  expect(screen.getByRole("link", { name: "Raw" })).toBeTruthy()
  expect(screen.getByRole("link", { name: "Download" })).toBeTruthy()
  expect(screen.getByRole("button", { name: "Copy link" })).toBeTruthy()
  expect(screen.queryByText("Source")).toBeNull()
}

test("an image is an <img> from the raw route and the bar gets its pixel size", async () => {
  renderAt("/files/tmp/shot-1.png", async (p) => withKind(p, HostedKindImage, 212 * 1024))
  const img = await findViewerImg()
  expect(img.getAttribute("src")).toBe("/v1/files/tmp/shot-1.png")
  Object.defineProperty(img, "naturalWidth", { value: 1280 })
  Object.defineProperty(img, "naturalHeight", { value: 800 })
  fireEvent.load(img)
  expect(await screen.findByText(/212 KB · .* · 1280×800/)).toBeTruthy()
})

test("an image that fails to load shows the not-found page", async () => {
  renderAt("/files/tmp/shot-1.png", async (p) => withKind(p, HostedKindImage))
  fireEvent.error(await findViewerImg())
  expect(await screen.findByText("File not found")).toBeTruthy()
  expect(screen.queryByRole("link", { name: "Raw" })).toBeNull()
})

test("an SVG is shown through <img> only", async () => {
  renderAt("/files/tmp/logo.svg", async (p) => withKind(p, HostedKindSVG))
  const img = await findViewerImg()
  expect(img.tagName).toBe("IMG")
  expect(img.getAttribute("src")).toBe("/v1/files/tmp/logo.svg")
  expect(document.querySelector("iframe, object, embed")).toBeNull()
  expect(img.parentElement?.querySelector("svg")).toBeNull()
})

test("a 3 MB code file and a 3 MB image are too large", async () => {
  for (const kind of [HostedKindCode, HostedKindImage] as const) {
    renderAt("/files/tmp/big.log", async (p) => withKind(p, kind, 3 * 1024 * 1024))
    expect(await screen.findByText("Too large to show")).toBeTruthy()
    expect(screen.getByText("Files over 2 MB show no preview. Use Raw or Download.")).toBeTruthy()
    expect(document.querySelector(rawImg)).toBeNull()
    expectNoSource()
    cleanup()
  }
})

test("binary and PDF show their notes", async () => {
  renderAt("/files/tmp/blob.bin", async (p) => withKind(p, HostedKindBinary))
  expect(
    await screen.findByText("No preview for this file type. Use Raw or Download."),
  ).toBeTruthy()
  expectNoSource()
  cleanup()
  renderAt("/files/tmp/doc.pdf", async (p) => withKind(p, HostedKindPDF))
  expect(
    await screen.findByText("PDFs open in the browser's PDF viewer. Use Raw or Download."),
  ).toBeTruthy()
  expectNoSource()
  expect(document.querySelector("iframe, object, embed")).toBeNull()
  expect(screen.getByRole("link", { name: "Download" }).getAttribute("href")).toBe(
    "/v1/files/tmp/doc.pdf?download=1",
  )
})

const lines50 = Array.from({ length: 50 }, (_, i) => `// line ${i + 1}`).join("\n")

test.each([HostedKindCode, HostedKindText] as const)(
  "a %s file shows numbered lines, marks ?line=42 and has no Source toggle",
  async (kind) => {
    Element.prototype.scrollIntoView = vi.fn()
    const name = kind === HostedKindCode ? "handler.go" : "notes.txt"
    renderAt(
      `/files/Users/me/${name}?line=42`,
      async (p) => ({ ...found(p), kind }),
      async () => lines50,
    )
    await waitFor(() => expect(document.querySelectorAll("[data-line]")).toHaveLength(50))
    const marked = document.querySelectorAll("[aria-current='true']")
    expect(Array.from(marked).map((r) => r.getAttribute("data-line"))).toEqual(["42"])
    expect(screen.getByRole("link", { name: "Raw" })).toBeTruthy()
    expect(screen.getByRole("link", { name: "Download" })).toBeTruthy()
    expect(screen.getByRole("button", { name: "Copy link" })).toBeTruthy()
    expect(screen.queryByRole("button", { name: "Source" })).toBeNull()
    expect(sessionStorage.length).toBe(1) // the resolve result only, not the file text
    expect(JSON.stringify({ ...sessionStorage })).not.toContain("line 42")
  },
)

test("a bad line marks nothing on a code file", async () => {
  renderAt(
    "/files/Users/me/handler.go?line=abc",
    async (p) => ({ ...found(p), kind: HostedKindCode }),
    async () => lines50,
  )
  await waitFor(() => expect(document.querySelectorAll("[data-line]")).toHaveLength(50))
  expect(document.querySelector("[aria-current]")).toBeNull()
})

test("a 404 on the raw fetch shows the not-found page", async () => {
  renderAt(
    "/files/Users/me/handler.go",
    async (p) => ({ ...found(p), kind: HostedKindCode }),
    async () => undefined,
  )
  expect(await screen.findByText("File not found")).toBeTruthy()
})

test("a pending resolve shows the spinner", async () => {
  renderAt("/files/tmp/a.md", () => new Promise(() => {}))
  expect(await screen.findByRole("status", { name: "Loading file" })).toBeTruthy()
})

test("changing only the heading scrolls without refetching the file", async () => {
  const scroll = vi.fn()
  Element.prototype.scrollIntoView = scroll
  const resolve = vi.fn(async (p: string) => found(p))
  const text = vi.fn(async () => "# Top\n\n## Install\n")
  const router = renderAt("/files/Users/me/a.md", resolve, text)
  await screen.findByRole("heading", { name: "Install" })
  expect(scroll).not.toHaveBeenCalled()
  await act(() => router.navigate("/files/Users/me/a.md?heading=install"))
  await waitFor(() => expect(scroll).toHaveBeenCalledTimes(1))
  expect((scroll.mock.contexts[0] as HTMLElement).textContent).toBe("Install")
  expect(resolve).toHaveBeenCalledTimes(1)
  expect(text).toHaveBeenCalledTimes(1)
})
