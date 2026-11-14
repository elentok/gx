import { HostedFound, type HostedResolved } from "../generated/ops"
import type { FileResolver } from "./resolve"

/** A test resolver: candidates in `found` resolve to `/abs/<candidate>`, the rest are missing. */
export function stubResolver(found: string[]) {
  const calls: { candidates: string[]; recheckMissing?: boolean }[] = []
  const resolver: FileResolver = {
    async resolve(candidates, options) {
      calls.push({ candidates, recheckMissing: options?.recheckMissing })
      const out = new Map<string, HostedResolved>()
      for (const c of candidates) {
        out.set(
          c,
          found.includes(c)
            ? { candidate: c, verdict: HostedFound, path: `/abs/${c.replace(/^\//, "")}` }
            : { candidate: c, verdict: "missing" },
        )
      }
      return out
    },
  }
  return { resolver, calls }
}
