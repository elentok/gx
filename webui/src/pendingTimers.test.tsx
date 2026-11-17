import { cleanup, render } from "@testing-library/react"
import { useEffect } from "react"
import { describe, expect, it, vi } from "vitest"
import { describePending, installTimerTracking, type TimerTarget } from "./pendingTimers"
import { registerTimerDisposer, runTimerDisposers } from "./timerDisposers"

// A stand-in target with its own instant "timers", so nothing real is ever scheduled.
function standIn() {
  let next = 1
  const fire = new Map<number, () => void>()
  const target = {
    setTimeout: (h: TimerHandler) => {
      fire.set(next, h as () => void)
      return next++
    },
    setInterval: (h: TimerHandler) => {
      fire.set(next, h as () => void)
      return next++
    },
    clearTimeout: (id?: unknown) => void fire.delete(id as number),
    clearInterval: (id?: unknown) => void fire.delete(id as number),
  } as TimerTarget
  return { target, fire }
}

describe("pending timer check", () => {
  it("reports a pending timeout with its creation stack", () => {
    const { target } = standIn()
    const tracker = installTimerTracking(target)
    target.setTimeout(() => {}, 1000)
    const pending = tracker.pending()
    expect(pending).toHaveLength(1)
    expect(pending[0].stack).toContain("pendingTimers.test.tsx")
    expect(describePending(pending)).toContain("pendingTimers.test.tsx")
  })

  it("does not report a cleared timeout, a fired timeout or a cleared interval", () => {
    const { target, fire } = standIn()
    const tracker = installTimerTracking(target)
    const cleared = target.setTimeout(() => {}, 1)
    target.clearTimeout(cleared)
    const firing = target.setTimeout(() => {}, 1)
    fire.get(firing as number)!()
    const interval = target.setInterval(() => {}, 1)
    target.clearInterval(interval)
    expect(tracker.pending()).toEqual([])
  })

  it("reports a pending interval, even after it has fired", () => {
    const { target, fire } = standIn()
    const tracker = installTimerTracking(target)
    const id = target.setInterval(() => {}, 1)
    fire.get(id as number)!()
    expect(tracker.pending().map((t) => t.kind)).toEqual(["setInterval"])
  })

  it("reports a timer made through a second target such as window", () => {
    const a = standIn()
    const b = standIn()
    const tracker = installTimerTracking(a.target, b.target)
    b.target.setTimeout(() => {}, 1)
    expect(tracker.pending()).toHaveLength(1)
  })

  it("keeps the real timer firing and survives fake timers", () => {
    const tracker = installTimerTracking(globalThis as unknown as TimerTarget)
    try {
      vi.useFakeTimers()
      const fn = vi.fn()
      setTimeout(fn, 10)
      vi.advanceTimersByTime(10)
      expect(fn).toHaveBeenCalledTimes(1)
      vi.useRealTimers()
      const id = setTimeout(() => {}, 100_000)
      expect(tracker.pending()).toHaveLength(1)
      clearTimeout(id)
      expect(tracker.pending()).toHaveLength(0)
    } finally {
      vi.useRealTimers()
      tracker.uninstall()
    }
  })

  it("runs the pending check after Testing Library cleanup", () => {
    const { target } = standIn()
    const tracker = installTimerTracking(target)
    function Timed() {
      useEffect(() => {
        const id = target.setTimeout(() => {}, 1000)
        return () => target.clearTimeout(id)
      }, [])
      return null
    }
    render(<Timed />)
    expect(tracker.pending()).toHaveLength(1)
    cleanup()
    expect(tracker.pending()).toHaveLength(0)
  })
})

describe("timer disposers", () => {
  it("runs every registered disposer", () => {
    const dispose = vi.fn()
    registerTimerDisposer(dispose)
    runTimerDisposers()
    expect(dispose).toHaveBeenCalledTimes(1)
  })
})
