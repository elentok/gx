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

function at(h: number, m: number): string {
  return new Date(2026, 0, 5, h, m).toISOString()
}

function setup(warning?: RestWarning) {
  let state = {
    snapshot: { rest: { counting: true, warning } },
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
        snapshot: { rest: { counting: true, warning: next } },
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
  setup({ kind: "bedtime", start: at(22, 0) })
  expect(await screen.findByText("Bedtime starts at 22:00 (in 10 minutes)")).toBeTruthy()
  expect(screen.getByText("Wrap up what you are doing.")).toBeTruthy()
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
