import { useEffect, useRef } from "react"

/** A ref that always holds the value of the latest render, for callbacks and effects that must not re-subscribe when it changes. */
export function useLatest<T>(value: T) {
  const ref = useRef(value)
  ref.current = value
  return ref
}

// A tuple counts as changed when one of its elements does, so several triggers can share one hook.
const same = (a: unknown, b: unknown) =>
  Array.isArray(a) && Array.isArray(b)
    ? a.length === b.length && a.every((x, i) => Object.is(x, b[i]))
    : Object.is(a, b)

/**
 * Runs `fn` after a render in which `value` changed, and not on mount unless `immediate` is set.
 * Pass a tuple to watch several triggers. `fn` is read at run time, so it may close over anything
 * without being a dependency; the cleanup it returns runs before the next change and on unmount.
 */
export function useOnChange<T>(
  value: T,
  fn: (value: T) => undefined | (() => void),
  { immediate = false }: { immediate?: boolean } = {},
) {
  const seen = useRef({ value, changes: 0 })
  if (!same(seen.current.value, value)) seen.current = { value, changes: seen.current.changes + 1 }
  const { changes } = seen.current
  const latestValue = useLatest(value)
  const latestFn = useLatest(fn)
  useEffect(() => {
    // `changes` is the trigger; it is a counter so the effect re-runs once per change.
    if (changes === 0 && !immediate) return
    return latestFn.current(latestValue.current)
  }, [changes, immediate])
}
