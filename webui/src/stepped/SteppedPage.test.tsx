import { cleanup, fireEvent, screen } from "@testing-library/react"
import { afterEach, expect, test, vi } from "vitest"
import { ConnectionContext, type Connection, type ConnectionState } from "../connection"
import type { MapStep, SessionInfo } from "../generated/server"
import { renderThemed } from "../ui/render"
import { triageModel } from "./model"
import { SteppedPage, type SteppedPageProps } from "./SteppedPage"

afterEach(cleanup)

const STEPS: MapStep[] = [
  { step: "scan", label: "Scan", state: "done" },
  { step: "review", label: "Review", state: "current" },
  { step: "approve", label: "Approve", state: "todo" },
  { step: "apply", label: "Apply", state: "todo" },
  { step: "done", label: "Done", state: "todo" },
]

// One stable state object: a store that returns a fresh one per read loops useSyncExternalStore.
const state = {
  snapshot: { sessions: [] },
  connected: true,
  loadedOnce: true,
  tick: 0,
} as unknown as ConnectionState
const conn = {
  getState: () => state,
  subscribe: () => () => {},
  onEvent: () => () => {},
  refresh: () => {},
} as unknown as Connection

function render(over: Partial<SteppedPageProps> = {}) {
  renderThemed(
    <ConnectionContext.Provider value={conn}>
      <SteppedPage
        address="gx"
        title="gx"
        icon={null}
        wide
        model={triageModel({ key: "gx", steps: STEPS })}
        live={[]}
        actions={{}}
        startLabel="Start triage session"
        working={() => ({})}
        {...over}
      />
    </ConnectionContext.Provider>,
  )
}

test("a Triage model renders its title, five steps, the Start button, and a menu without Delete, Hold or Move", async () => {
  const open = vi.fn(async () => ({}))
  render({ actions: { open } })
  expect(screen.getByText("gx")).toBeTruthy()
  const progress = screen.getByRole("region", { name: "Progress" })
  for (const s of STEPS) expect(progress.textContent).toContain(s.label)
  fireEvent.click(screen.getByRole("button", { name: "Start triage session" }))
  expect(open).toHaveBeenCalledWith({ epic: "gx" })
  fireEvent.click(await screen.findByRole("button", { name: "More actions" }))
  const items = (await screen.findAllByRole("menuitem")).map((i) => i.textContent ?? "")
  expect(items).toHaveLength(1)
  expect(items[0]).toContain("Copy triage ID")
})

test("a live Triage session's header holds no model", async () => {
  const live = {
    id: "t-1",
    project: "gx",
    dir: "/w",
    harness: "claude",
    model: "claude-opus-5",
    effort: "high",
    status: { state: "idle" },
  } as unknown as SessionInfo
  render({ live: [live], actions: { fetchChat: async () => ({}) as never } })
  await screen.findByRole("heading", { name: "gx" })
  expect(screen.getByTestId("page-header-meta").textContent).not.toContain("Opus")
  expect(screen.getByTestId("page-header-subtitle").textContent).not.toContain("Opus")
})

test("the Design slots render only when given", () => {
  render()
  expect(screen.queryByText("Design epic ↗")).toBeNull()
  expect(screen.queryByText(" · On hold")).toBeNull()
  cleanup()
  render({
    metaLink: <a href="#x">Design epic ↗</a>,
    subtitleSuffix: " · On hold",
    chatTop: () => <p>chat top</p>,
  })
  expect(screen.getByText("Design epic ↗")).toBeTruthy()
  expect(screen.getByText("chat top")).toBeTruthy()
  expect(screen.getByTestId("page-header-subtitle").textContent).toContain("On hold")
})
