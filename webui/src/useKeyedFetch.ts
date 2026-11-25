import { useEffect, useState } from "react"
import { useLatest } from "./hooks"

/** The settled result of a keyed fetch: its value, or a failure. */
export type Keyed<T> = { value: T } | { failed: true }

/**
 * Runs `fetcher` once per `key` and returns its result only while it belongs to the current key, so a
 * stale answer is never shown. Undefined means pending (or `key` is null, which turns the fetch off).
 * The fetcher is not a dependency: put everything it reads into the key.
 * Bumping `refresh` asks again for the same key: the last answer stays until a newer one replaces it,
 * a failed ask keeps a good answer, and an answer to an older ask is dropped.
 */
export function useKeyedFetch<T>(
  key: string | null,
  fetcher: () => Promise<T>,
  refresh = 0,
): Keyed<T> | undefined {
  const latest = useLatest(fetcher)
  const [settled, setSettled] = useState<{ key: string; result: Keyed<T> }>()
  // biome-ignore lint/correctness/useExhaustiveDependencies: `refresh` only re-runs the effect
  useEffect(() => {
    if (key === null) return
    let current = true
    latest.current().then(
      (value) => current && setSettled({ key, result: { value } }),
      () =>
        current &&
        setSettled((prev) =>
          prev?.key === key && "value" in prev.result ? prev : { key, result: { failed: true } },
        ),
    )
    return () => {
      current = false
    }
  }, [key, refresh])
  return key !== null && settled?.key === key ? settled.result : undefined
}
