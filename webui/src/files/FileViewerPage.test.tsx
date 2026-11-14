import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react"
import { createMemoryRouter, RouterProvider } from "react-router"
import { afterEach, beforeEach, expect, test, vi } from "vitest"
import { HostedFound, HostedKindMarkdown, type HostedResolved } from "../generated/ops"
import { ConnectionContext, createConnection } from "../connection"
import { appRoutes } from "../routes"
import { renderThemed } from "../ui/render"
import type { FetchFileResolve } from "./FileViewerPage"

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

function renderAt(route: string, fileResolve: FetchFileResolve) {
  const router = createMemoryRouter(appRoutes({ fileResolve }), { initialEntries: [route] })
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
  expect(screen.getByText("No preview yet")).toBeTruthy()
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
  expect(await screen.findByText("No preview yet")).toBeTruthy()
})

test("a pending resolve shows the spinner", async () => {
  renderAt("/files/tmp/a.md", () => new Promise(() => {}))
  expect(await screen.findByRole("status", { name: "Loading file" })).toBeTruthy()
})
