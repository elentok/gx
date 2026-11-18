import { cleanup, render } from "@testing-library/react"
import { useEffect } from "react"
import { describe, expect, it, vi } from "vitest"
import {
  describePending,
  installTimerTracking,
  madeByLibrary,
  type TimerTarget,
} from "./pendingTimers"
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

function Timed({ target }: { target: TimerTarget }) {
  useEffect(() => {
    const id = target.setTimeout(() => {}, 1000)
    return () => target.clearTimeout(id)
  }, [target])
  return null
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

  it("does not wait for a short product timeout, so it is still reported", async () => {
    const real = globalThis as unknown as TimerTarget
    const target = {
      setTimeout: real.setTimeout.bind(globalThis),
      setInterval: real.setInterval.bind(globalThis),
      clearTimeout: real.clearTimeout.bind(globalThis),
      clearInterval: real.clearInterval.bind(globalThis),
    } as TimerTarget
    const tracker = installTimerTracking(target)
    const id = target.setTimeout(() => {}, 100)
    try {
      await tracker.settle()
      expect(tracker.pending()).toHaveLength(1)
    } finally {
      target.clearTimeout(id)
      tracker.uninstall()
    }
  })

  it("treats only a timer made from node_modules as a library timer", () => {
    const frame = (path: string) => `Error: x\n    at wrap (a.ts:1:1)\n    at f (${path}:2:3)`
    expect(madeByLibrary(frame("/repo/node_modules/@codemirror/view/dist/index.js"))).toBe(true)
    expect(madeByLibrary(frame("/repo/node_modules/.pnpm/x@1/node_modules/jsdom/lib/a.js"))).toBe(
      true,
    )
    expect(madeByLibrary(frame("/repo/webui/src/design/DraftPage.tsx"))).toBe(false)
  })

  it("runs the pending check after Testing Library cleanup", () => {
    const { target } = standIn()
    const tracker = installTimerTracking(target)
    render(<Timed target={target} />)
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
