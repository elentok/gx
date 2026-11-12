import { cleanup, screen } from "@testing-library/react"
import { afterEach, expect, test } from "vitest"
import type { ScheduleRun } from "../generated/server"
import { renderThemed } from "../ui/render"
import { LastRuns } from "./LastRuns"

afterEach(cleanup)

const run = (over: Partial<ScheduleRun>): ScheduleRun => ({
  scheduled_for: "2026-11-10T07:00:00Z",
  created_at: "",
  address: "gx:oneoff/1",
  status: "finished",
  result: "done it",
  reason: "",
  reason_text: "",
  attention_address: "",
  ...over,
})

test("every status renders its word", () => {
  const words: Record<string, string> = {
    finished: "Finished",
    "ran-late": "Ran late",
    skipped: "Skipped",
    "needs-answer": "Needs answer",
    running: "Running",
    queued: "Queued",
    cancelled: "Cancelled",
  }
  renderThemed(<LastRuns runs={Object.keys(words).map((status) => run({ status }))} />)
  for (const w of Object.values(words)) expect(screen.getByText(w)).toBeTruthy()
  expect(screen.getByText("Last 10 runs.")).toBeTruthy()
})

test("an empty scheduled time reads Run now and a One-off links its ticket", () => {
  renderThemed(<LastRuns runs={[run({ scheduled_for: "" })]} />)
  expect(screen.getByText("Run now")).toBeTruthy()
  expect(screen.getByRole("link", { name: "gx:oneoff/1" }).getAttribute("href")).toBe(
    "#/t/gx:oneoff/1",
  )
})

test("a skip has no ticket link", () => {
  renderThemed(
    <LastRuns
      runs={[
        run({
          status: "skipped",
          address: "",
          reason: "late",
          reason_text: "Past the 2 h Late limit",
        }),
      ]}
    />,
  )
  expect(screen.getByText("Past the 2 h Late limit")).toBeTruthy()
  expect(screen.queryByRole("link")).toBeNull()
})

test("a needs-attention skip links the blocking ticket", () => {
  renderThemed(
    <LastRuns
      runs={[
        run({
          status: "skipped",
          address: "",
          reason: "previous-run-needs-attention",
          reason_text: "previous run needs attention",
          attention_address: "gx:epic/3",
        }),
      ]}
    />,
  )
  expect(screen.getByRole("link", { name: "gx:epic/3" }).getAttribute("href")).toBe("#/t/gx:epic/3")
})
