import { renderHook } from "@testing-library/react"
import { expect, test, vi } from "vitest"
import { ReasonDraftGone, ReasonDraftRevisionConflict } from "../generated/ops"
import { openHistory } from "./historyOpener"
import { useHistoryOpener, usePickHistory } from "./usePickHistory"

type Result = {
  refused?: boolean
  reason?: string
  draft?: { revision: number }
  notices: string[]
}

function setup(phase = "editing") {
  const handlers = { apply: vi.fn(), onConflict: vi.fn(), onGone: vi.fn() }
  const { result } = renderHook(() =>
    usePickHistory<{ revision: number }, Result>({
      timer: { current: undefined },
      chain: { current: Promise.resolve() },
      phase: { current: phase },
      revision: () => 7,
      ...handlers,
    }),
  )
  return { pick: result.current, ...handlers }
}

test("a success goes to apply with the revision the next save would send", async () => {
  const h = setup()
  const call = vi.fn(async () => ({ draft: { revision: 8 }, notices: [] }))
  await h.pick(call)
  expect(call).toHaveBeenCalledWith(7)
  expect(h.apply).toHaveBeenCalledWith({ revision: 8 })
  expect(h.onConflict).not.toHaveBeenCalled()
  expect(h.onGone).not.toHaveBeenCalled()
})

test("a revision conflict goes to onConflict and applies nothing", async () => {
  const h = setup()
  await h.pick(async () => ({ refused: true, reason: ReasonDraftRevisionConflict, notices: [] }))
  expect(h.onConflict).toHaveBeenCalledTimes(1)
  expect(h.apply).not.toHaveBeenCalled()
  expect(h.onGone).not.toHaveBeenCalled()
})

test("a gone draft goes to onGone and applies nothing", async () => {
  const h = setup()
  await h.pick(async () => ({ refused: true, reason: ReasonDraftGone, notices: [] }))
  expect(h.onGone).toHaveBeenCalledTimes(1)
  expect(h.apply).not.toHaveBeenCalled()
  expect(h.onConflict).not.toHaveBeenCalled()
})

test("another refusal changes nothing", async () => {
  const h = setup()
  const res = await h.pick(async () => ({ refused: true, reason: "other", notices: ["no"] }))
  expect(res?.notices).toEqual(["no"])
  expect([h.apply, h.onConflict, h.onGone].every((f) => f.mock.calls.length === 0)).toBe(true)
})

test("a page that left editing makes no call", async () => {
  const h = setup("gone")
  const call = vi.fn()
  expect(await h.pick(call)).toBeUndefined()
  expect(call).not.toHaveBeenCalled()
})

test("useHistoryOpener registers while mounted and only when enabled", () => {
  const open = vi.fn()
  const off = renderHook(() => useHistoryOpener(open))
  expect(openHistory()).toBe(true)
  expect(open).toHaveBeenCalledTimes(1)
  off.unmount()
  expect(openHistory()).toBe(false)
  renderHook(() => useHistoryOpener(open, false)).unmount()
  expect(openHistory()).toBe(false)
})
