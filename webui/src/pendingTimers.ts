// Records every real timer a test creates, so test-setup can fail a test that leaves one pending.
// A timer left pending can fire after the test ends and fail the run with every test green.
// vi.useFakeTimers() replaces the globals with the fake clock's own timers, which are not recorded;
// useRealTimers() puts back what it found, which is the wrapper installed here.

type Id = unknown
type TimerFn = (handler: TimerHandler, ...rest: unknown[]) => Id
type ClearFn = (id?: Id) => void

export type TimerTarget = {
  setTimeout: TimerFn
  setInterval: TimerFn
  clearTimeout: ClearFn
  clearInterval: ClearFn
}

export type PendingTimer = { kind: "setTimeout" | "setInterval"; stack: string; delay: number }

const SETTLE_MAX_MS = 500

export type TimerTracker = {
  /** Every recorded timer that has neither fired (a timeout) nor been cleared. */
  pending(): PendingTimer[]
  /** Waits for pending short timeouts to fire on their own, at most SETTLE_MAX_MS. */
  settle(): Promise<void>
  /** Forgets every record without touching the timers. */
  reset(): void
  /** Puts the original functions back. */
  uninstall(): void
}

// jsdom's own storage code schedules a 0 ms timer for each storage event. It is the environment's
// timer, not the product's or the test's, and it fires on the next tick. Only a timer that jsdom
// itself creates (the direct caller, frame 2 of the stack) is skipped.
function madeByJsdom(stack: string): boolean {
  return /node_modules[\\/](?:\.pnpm[\\/][^\\/]+[\\/]node_modules[\\/])?jsdom[\\/]/.test(
    stack.split("\n")[2] ?? "",
  )
}

/** Wraps the timer functions on each target; a target listed twice is wrapped once. */
export function installTimerTracking(...targets: TimerTarget[]): TimerTracker {
  // Node timers are objects and jsdom's are numbers, so each target keeps its own id space.
  const recordSets: Array<Map<Id, PendingTimer>> = []
  const originalSetTimeouts: Array<(fn: () => void, ms: number) => unknown> = []
  const restores: Array<() => void> = []

  for (const target of new Set(targets)) {
    const originals = {
      setTimeout: target.setTimeout,
      setInterval: target.setInterval,
      clearTimeout: target.clearTimeout,
      clearInterval: target.clearInterval,
    }
    const records = new Map<Id, PendingTimer>()
    originalSetTimeouts.push((fn, ms) => originals.setTimeout(fn, ms))
    const wrapSet = (kind: PendingTimer["kind"]): TimerFn =>
      function (this: unknown, handler, ...rest) {
        const stack = new Error(`${kind} created here`).stack ?? ""
        let id: Id
        const wrapped: TimerHandler =
          typeof handler === "function" && kind === "setTimeout"
            ? (...args: unknown[]) => {
                records.delete(id)
                return (handler as (...a: unknown[]) => unknown)(...args)
              }
            : handler
        id = originals[kind].call(this, wrapped, ...rest)
        if (!madeByJsdom(stack)) {
          records.set(id, { kind, stack, delay: typeof rest[0] === "number" ? rest[0] : 0 })
        }
        return id
      }
    const wrapClear = (name: "clearTimeout" | "clearInterval"): ClearFn =>
      function (this: unknown, id) {
        records.delete(id)
        return originals[name].call(this, id)
      }
    target.setTimeout = wrapSet("setTimeout")
    target.setInterval = wrapSet("setInterval")
    target.clearTimeout = wrapClear("clearTimeout")
    target.clearInterval = wrapClear("clearInterval")
    recordSets.push(records)
    restores.push(() => Object.assign(target, originals))
  }

  const pending = () => recordSets.flatMap((records) => [...records.values()])
  return {
    pending,
    // Libraries (CodeMirror focus and blur, the vim dialog, Chakra's menu typeahead) keep a timer of a
    // few hundred ms that they never clear on destroy. Waiting for those to fire lets them end on their
    // own; a product timer of a second or more is not waited for and is reported.
    async settle() {
      const sleep = originalSetTimeouts[0]
      const deadline = Date.now() + SETTLE_MAX_MS
      while (
        Date.now() < deadline &&
        pending().some((t) => t.kind === "setTimeout" && t.delay <= SETTLE_MAX_MS)
      ) {
        await new Promise<void>((resolve) => sleep(resolve, 10))
      }
    },
    reset: () => recordSets.forEach((records) => records.clear()),
    uninstall: () => restores.forEach((restore) => restore()),
  }
}

/** The failure message: one block per pending timer with its creation stack. */
export function describePending(timers: PendingTimer[]): string {
  const blocks = timers.map((t) => t.stack.replace(/^Error: /, ""))
  return `${timers.length} timer(s) still pending after the test; clear each on unmount or register a disposer:\n${blocks.join("\n")}`
}
