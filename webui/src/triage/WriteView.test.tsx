import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react"
import { afterEach, expect, test, vi } from "vitest"
import type { TriageRow, TriageView } from "../generated/server"
import { renderThemed as render } from "../ui/render"
import { WriteView, rowsWritten, writtenLine } from "./WriteView"

afterEach(cleanup)

const rowOf = (n: number, over: Partial<TriageRow> = {}): TriageRow => ({
  address: `gx:follow-ups/${n}`,
  title: `row ${n}`,
  action: "ready",
  ...over,
})

const viewOf = (rows: TriageRow[]): TriageView =>
  ({
    project: "gx",
    state: "live",
    flow: "triage",
    steps: [],
    ring_percent: 80,
    revision: 7,
    rows,
    open_questions: 0,
    action_counts: {},
    changed_rows: 0,
    destinations: [],
    epic_actions: ["ready", "map"],
    can_abandon: false,
    history: [],
  }) as TriageView

const refusal = "close: ticket is claimed, not draft or open"

test("rows show state, new address and what was done", () => {
  const rows = [
    rowOf(1, {
      write_state: "done",
      new_addresses: ["follow-ups-2026-11-08/01", "follow-ups-2026-11-08/02"],
    }),
    rowOf(2, { write_state: "done", action: "close", destination: "obsolete" }),
    rowOf(3, { write_state: "pending" }),
    rowOf(4, { write_state: "gone" }),
  ]
  render(<WriteView project="gx" view={viewOf(rows)} refetch={() => {}} />)
  expect(screen.queryByText("moved")).toBeNull()
  fireEvent.click(screen.getByRole("button", { name: "3 more rows written" }))
  screen.getByText("→ follow-ups-2026-11-08/01")
  screen.getByText("→ follow-ups-2026-11-08/02")
  screen.getByText("moved")
  screen.getByText("closed, obsolete")
  screen.getByText("pending")
  expect(rowsWritten(viewOf(rows))).toBe(3)
  screen.getByText("Write · 3 of 4 rows")
  screen.getByText(/code-review stub are written last/)
})

test("a failed row shows the refusal verbatim, and buttons follow can_keep and can_retry", () => {
  const rows = [
    rowOf(1, { write_state: "failed", refusal, can_keep: true, can_retry: true }),
    rowOf(2, { write_state: "failed", refusal: "epic add failed", can_retry: true }),
    rowOf(3, { write_state: "running" }),
  ]
  render(<WriteView project="gx" view={viewOf(rows)} refetch={() => {}} />)
  screen.getByText(refusal)
  screen.getByText("epic add failed")
  expect(screen.getAllByRole("button", { name: "Retry" })).toHaveLength(2)
  expect(screen.getAllByRole("button", { name: "Keep for later" })).toHaveLength(1)
})

test("Retry and Keep for later post once with the revision", async () => {
  const retry = vi.fn().mockResolvedValue({})
  const keep = vi.fn().mockResolvedValue({})
  const refetch = vi.fn()
  const rows = [rowOf(1, { write_state: "failed", refusal, can_keep: true, can_retry: true })]
  render(<WriteView project="gx" view={viewOf(rows)} refetch={refetch} retry={retry} keep={keep} />)
  fireEvent.click(screen.getByRole("button", { name: "Retry" }))
  await waitFor(() => expect(retry).toHaveBeenCalledTimes(1))
  expect(retry).toHaveBeenCalledWith({ project: "gx", revision: 7, address: "gx:follow-ups/1" })
  await waitFor(() => expect(refetch).toHaveBeenCalled())
  fireEvent.click(screen.getByRole("button", { name: "Keep for later" }))
  await waitFor(() => expect(keep).toHaveBeenCalledTimes(1))
  expect(keep).toHaveBeenCalledWith({ project: "gx", revision: 7, address: "gx:follow-ups/1" })
  expect(retry).toHaveBeenCalledTimes(1)
})

test("failed rows come first and written rows fold", () => {
  const rows = [rowOf(1, { write_state: "done" }), rowOf(2, { write_state: "failed", refusal })]
  render(<WriteView project="gx" view={viewOf(rows)} refetch={() => {}} />)
  const failed = screen.getByText("gx:follow-ups/2")
  expect(screen.queryByText("gx:follow-ups/1")).toBeNull()
  const toggle = screen.getByRole("button", { name: "1 more rows written" })
  expect(failed.compareDocumentPosition(toggle) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  fireEvent.click(toggle)
  screen.getByText("gx:follow-ups/1")
})

test("a failed move unit posts Retry and Keep for later once with the revision and the old epic", async () => {
  const retry = vi.fn().mockResolvedValue({})
  const keep = vi.fn().mockResolvedValue({})
  const view = viewOf([])
  view.summary = {
    actions: {},
    areas: {},
    waiting: [],
    moves: [
      {
        old_epic: "b",
        area: "web",
        state: "failed",
        last_error: "boom",
        can_retry: true,
        can_keep: true,
      },
      { old_epic: "d", area: "ops", state: "pending" },
    ],
  }
  render(<WriteView project="gx" view={view} refetch={() => {}} retry={retry} keep={keep} />)
  expect(screen.getAllByRole("button", { name: "Retry" })).toHaveLength(1)
  fireEvent.click(screen.getByRole("button", { name: "Retry" }))
  await waitFor(() => expect(retry).toHaveBeenCalledTimes(1))
  expect(retry).toHaveBeenCalledWith({ project: "gx", revision: 7, address: "b" })
  fireEvent.click(screen.getByRole("button", { name: "Keep for later" }))
  await waitFor(() => expect(keep).toHaveBeenCalledTimes(1))
  expect(keep).toHaveBeenCalledWith({ project: "gx", revision: 7, address: "b" })
})

test("move units are rows with state and error, and count in x of y", () => {
  const view = viewOf([rowOf(1, { write_state: "done" })])
  view.summary = {
    actions: {},
    areas: {},
    waiting: [],
    moves: [
      { old_epic: "a", area: "web", state: "done" },
      { old_epic: "b", area: "web", state: "failed", last_error: "design session is live" },
      { old_epic: "c", area: "ops", state: "kept" },
      { old_epic: "d", area: "ops", state: "pending" },
    ],
  }
  render(<WriteView project="gx" view={view} refetch={() => {}} />)
  screen.getByText("Write · 3 of 5 rows")
  expect(writtenLine(view)).toBe("3 of 5 rows written")
  screen.getByText("move b -> web")
  screen.getByText("design session is live")
  screen.getByText("move c -> ops")
  screen.getByText("kept for later")
  screen.getByText("move d -> ops")
  expect(screen.queryByText("move a -> web")).toBeNull()
  screen.getByRole("button", { name: "2 more rows written" })
})
