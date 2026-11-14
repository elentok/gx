import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react"
import { createMemoryRouter, RouterProvider } from "react-router"
import { afterEach, expect, test, vi } from "vitest"
import { ConnectionContext, type Connection, type ConnectionState } from "../connection"
import type { DiffFiles } from "../generated/server"
import { appRoutes } from "../routes"
import { renderThemed } from "../ui/render"
import type { DiffFilesResult, FetchDiffFiles } from "./api"

afterEach(cleanup)

const range = (over: Partial<DiffFiles["DiffRange"]> = {}): DiffFiles["DiffRange"] => ({
  first_sha: "a",
  second_sha: "b",
  merge_base: "a",
  ...over,
})
const file = { path: "a.go", status: "M", added: 1, deleted: 0 }
const files = (n: number, over: Partial<DiffFiles["DiffRange"]> = {}): DiffFilesResult => ({
  kind: "files",
  files: { DiffRange: range(over), files: Array.from({ length: n }, () => file) },
})

function render(path: string, fetchFiles: FetchDiffFiles) {
  const state = {
    snapshot: { sessions: [], pending: [] },
    connected: true,
    loadedOnce: true,
    tick: 0,
  }
  const conn = {
    getState: () => state as unknown as ConnectionState,
    subscribe: () => () => {},
    onEvent: () => () => {},
    refresh: () => {},
  } as unknown as Connection
  const router = createMemoryRouter(appRoutes({ diffFiles: fetchFiles }), {
    initialEntries: [path],
  })
  renderThemed(
    <ConnectionContext.Provider value={conn}>
      <RouterProvider router={router} />
    </ConnectionContext.Provider>,
  )
  return router
}

const base = () => screen.findByRole("textbox", { name: "Base ref" }) as Promise<HTMLInputElement>
const tip = () => screen.getByRole("textbox", { name: "Tip ref" }) as HTMLInputElement
const OPEN = "/diff?project=gx&base=main&tip=dev"

test("the fields fill from the URL with ... between them", async () => {
  render(
    OPEN,
    vi.fn(async () => files(2)),
  )
  expect((await base()).value).toBe("main")
  expect(tip().value).toBe("dev")
  expect(screen.getByTestId("ref-bar").textContent).toContain("...")
  expect(await screen.findByText("2 files")).toBeTruthy()
})

test("Reload writes the URL and asks again with the typed refs; no polling", async () => {
  const fetchFiles = vi.fn(async () => files(1))
  const router = render(OPEN, fetchFiles)
  const input = await base()
  await waitFor(() => expect(fetchFiles).toHaveBeenCalledTimes(1))
  fireEvent.change(input, { target: { value: "v1" } })
  expect(fetchFiles).toHaveBeenCalledTimes(1)
  fireEvent.click(screen.getByRole("button", { name: "Reload" }))
  await waitFor(() => expect(fetchFiles).toHaveBeenCalledTimes(2))
  expect(fetchFiles).toHaveBeenLastCalledWith("gx", "v1", "dev")
  expect(router.state.location.search).toContain("base=v1")
  await new Promise((r) => setTimeout(r, 50))
  expect(fetchFiles).toHaveBeenCalledTimes(2)
})

test("Enter reloads like the button, and Reload with unchanged refs asks again", async () => {
  const fetchFiles = vi.fn(async () => files(1))
  render(OPEN, fetchFiles)
  const input = await base()
  await waitFor(() => expect(fetchFiles).toHaveBeenCalledTimes(1))
  fireEvent.change(tip(), { target: { value: "v2" } })
  fireEvent.keyDown(tip(), { key: "Enter" })
  await waitFor(() => expect(fetchFiles).toHaveBeenCalledTimes(2))
  expect(fetchFiles).toHaveBeenLastCalledWith("gx", "main", "v2")
  fireEvent.click(screen.getByRole("button", { name: "Reload" }))
  await waitFor(() => expect(fetchFiles).toHaveBeenCalledTimes(3))
  expect(input.value).toBe("main")
})

test("nothing is fetched while a ref is empty", async () => {
  const fetchFiles = vi.fn(async () => files(1))
  render("/diff?project=gx&base=main", fetchFiles)
  await base()
  await new Promise((r) => setTimeout(r, 30))
  expect(fetchFiles).not.toHaveBeenCalled()
})

const notice = () => screen.findByTestId("notice")
const editable = () => {
  for (const name of ["Base ref", "Tip ref"]) {
    const input = screen.getByRole("textbox", { name }) as HTMLInputElement
    fireEvent.change(input, { target: { value: "zzz" } })
    expect(input.value).toBe("zzz")
  }
}

test("an unknown ref names the field, marks it invalid and stays editable", async () => {
  render(OPEN, async () => ({ kind: "unknown-ref", side: "second" }))
  const n = await notice()
  expect(n.textContent).toContain('Unknown ref "dev"')
  expect(tip().getAttribute("aria-invalid")).toBe("true")
  expect((await base()).getAttribute("aria-invalid")).toBeNull()
  editable()
})

test("an empty range says No changes", async () => {
  render("/diff?project=gx&base=main&tip=main", async () => files(0))
  const n = await notice()
  expect(n.textContent).toContain("No changes")
  expect(n.textContent).toContain("Nothing differs between main and main.")
  editable()
})

test("no common history is a neutral notice like No changes", async () => {
  render(OPEN, async () => files(0, { no_common_history: true }))
  const n = await notice()
  expect(n.textContent).toContain("No common history")
  expect(n.querySelector("[data-tone]")?.getAttribute("data-tone")).toBe("muted")
  editable()
})

test("an epic with empty refs shows No diff recorded and fetches nothing", async () => {
  const fetchFiles = vi.fn(async () => files(1))
  render("/diff?project=gx&epic=gx:e", fetchFiles)
  const n = await notice()
  expect(n.textContent).toContain("No diff recorded")
  expect(n.textContent).toContain(
    "gx has no exact range for this epic. Type two refs above to compare them.",
  )
  expect(n.querySelector("[data-tone]")?.getAttribute("data-tone")).toBe("waiting")
  expect(fetchFiles).not.toHaveBeenCalled()
  await base()
  editable()
})

test("unavailable commits show an amber notice in the same area", async () => {
  render(OPEN, async () => files(0, { commits_unavailable: true }))
  const n = await notice()
  expect(n.textContent).toContain("Commits not available in this clone")
  expect(n.querySelector("[data-tone]")?.getAttribute("data-tone")).toBe("waiting")
  editable()
})

test("the Split/Unified switch is in the ref bar", async () => {
  render(OPEN, async () => files(1))
  await base()
  const toggle = screen.getByTestId("view-toggle")
  expect(screen.getByTestId("ref-bar").contains(toggle)).toBe(true)
  expect(screen.getByRole("button", { name: "Split" }).getAttribute("aria-pressed")).toBe("true")
  fireEvent.click(screen.getByRole("button", { name: "Unified" }))
  expect(screen.getByRole("button", { name: "Unified" }).getAttribute("aria-pressed")).toBe("true")
})
