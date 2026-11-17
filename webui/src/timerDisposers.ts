const disposers: Array<() => void> = []

/** A product module that keeps a timer at module level calls this once with a function that clears it. */
export function registerTimerDisposer(dispose: () => void): void {
  disposers.push(dispose)
}

/** test-setup.ts calls this after every test, so a module-level timer cannot outlive its test. */
export function runTimerDisposers(): void {
  for (const dispose of disposers) dispose()
}
