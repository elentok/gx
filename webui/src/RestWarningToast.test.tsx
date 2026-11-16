import { act, cleanup, screen, waitFor } from "@testing-library/react"
import { afterEach, expect, test } from "vitest"
import { ConnectionContext, type Connection, type ConnectionState } from "./connection"
import type { RestWarning } from "./generated/server"
import { RestWarningToast } from "./RestWarningToast"
import { renderThemed } from "./ui/render"
import { toaster } from "./ui/toaster"

afterEach(() => {
  cleanup()
  toaster.remove()
})

// UTC instants, so the expected text never depends on the zone of the test process.
function at(h: number, m: number): string {
  return new Date(Date.UTC(2026, 0, 5, h, m)).toISOString()
}

type Zone = { zone?: string; zone_offset_seconds: number }
const utc: Zone = { zone: "UTC", zone_offset_seconds: 0 }

// `now` is `lead` minutes before the warning start, as the server sends it.
function snap(warning: RestWarning | undefined, lead: number, zone: Zone) {
  const now = warning ? new Date(Date.parse(warning.start) - lead * 60000).toISOString() : at(0, 0)
  return { rest: { counting: true, warning, now, ...zone } }
}

function setup(warning?: RestWarning, lead = 2, zone: Zone = utc) {
  let state = {
    snapshot: snap(warning, lead, zone),
    connected: true,
  } as unknown as ConnectionState
  const subs = new Set<() => void>()
  const conn = {
    getState: () => state,
    subscribe: (fn: () => void) => (subs.add(fn), () => subs.delete(fn)),
  } as unknown as Connection
  renderThemed(
    <ConnectionContext.Provider value={conn}>
      <RestWarningToast />
    </ConnectionContext.Provider>,
  )
  return (next?: RestWarning) =>
    act(() => {
      state = {
        ...state,
        snapshot: snap(next, lead, zone),
      } as unknown as ConnectionState
      subs.forEach((fn) => fn())
    })
}

const brk = (h: number, m: number): RestWarning => ({
  kind: "break",
  start: at(h, m),
  break_minutes: 5,
})

test("break warning shows start and length from the snapshot", async () => {
  setup(brk(14, 55))
  expect(await screen.findByText("Break in 2 minutes")).toBeTruthy()
  expect(screen.getByText("Your Break starts at 14:55 and lasts 5 minutes.")).toBeTruthy()
  expect(toaster.getVisibleToasts()[0]?.duration).toBe(Infinity)
})

test("bedtime warning text", async () => {
  setup({ kind: "bedtime", start: at(22, 0) }, 10)
  expect(await screen.findByText("Bedtime starts at 22:00 (in 10 minutes)")).toBeTruthy()
  expect(screen.getByText("Wrap up what you are doing.")).toBeTruthy()
})

test("clock time prints in the snapshot zone, not the browser zone", async () => {
  setup(brk(14, 55), 2, { zone: "Asia/Tokyo", zone_offset_seconds: 9 * 3600 })
  expect(await screen.findByText("Your Break starts at 23:55 and lasts 5 minutes.")).toBeTruthy()
})

test("clock time uses the offset when the snapshot has no zone name", async () => {
  setup({ kind: "bedtime", start: at(22, 0) }, 10, { zone_offset_seconds: -5 * 3600 })
  expect(await screen.findByText("Bedtime starts at 17:00 (in 10 minutes)")).toBeTruthy()
})

test("lead minutes follow start minus now", async () => {
  setup(brk(14, 55), 3)
  expect(await screen.findByText("Break in 3 minutes")).toBeTruthy()
})

test("moved start updates the toast in place", async () => {
  const set = setup(brk(14, 55))
  await screen.findByText("Break in 2 minutes")
  set(brk(14, 56))
  expect(await screen.findByText("Your Break starts at 14:56 and lasts 5 minutes.")).toBeTruthy()
  expect(toaster.getVisibleToasts()).toHaveLength(1)
})

test("warning leaving the snapshot removes the toast; none without a warning", async () => {
  const set = setup(brk(14, 55))
  await screen.findByText("Break in 2 minutes")
  set(undefined)
  await waitFor(() => expect(toaster.getVisibleToasts()).toHaveLength(0))
})

test("no warning, no toast", () => {
  setup()
  expect(toaster.getVisibleToasts()).toHaveLength(0)
})

test("a closed toast stays closed until a new warning", async () => {
  const set = setup(brk(14, 55))
  await screen.findByText("Break in 2 minutes")
  act(() => toaster.dismiss(toaster.getVisibleToasts()[0]!.id))
  await waitFor(() => expect(screen.queryByText("Break in 2 minutes")).toBeNull())

  set(brk(14, 56))
  expect(screen.queryByText("Break in 2 minutes")).toBeNull()

  set(undefined)
  set(brk(15, 30))
  expect(await screen.findByText("Break in 2 minutes")).toBeTruthy()
})
