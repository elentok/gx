import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react"
import { afterEach, expect, test, vi } from "vitest"
import type { TriageRow, TriageView } from "../generated/server"
import { renderThemed as render } from "../ui/render"
import { WriteView, rowsWritten } from "./WriteView"

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
  screen.getByText("→ follow-ups-2026-11-08/01")
  screen.getByText("→ follow-ups-2026-11-08/02")
  screen.getByText("moved")
  screen.getByText("closed, obsolete")
  screen.getByText("pending")
  expect(rowsWritten(viewOf(rows))).toBe(3)
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
