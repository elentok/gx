import { apiFetch } from "../apiFetch"
import {
  type HostedResolved,
  type HostedResolveResult,
  MaxHostedCandidates,
} from "../generated/ops"

const MAX_IN_FLIGHT = 2

/** At most one of these is set; with none only absolute and `~/` candidates resolve. */
export type ResolveBase = { session?: string; project?: string; dir?: string }

export type ResolveOptions = {
  /** The session has a new message: these candidates, if cached as missing, are asked again. */
  recheckMissing?: ReadonlySet<string>
  /** Ask the server again even for candidates cached as found. */
  fresh?: boolean
}

export type FileResolver = {
  /** The verdict per candidate. A candidate whose call failed is absent: it stays plain text. */
  resolve(candidates: string[], options?: ResolveOptions): Promise<Map<string, HostedResolved>>
}

/** One resolver per page: it caches `found` results for its own life. */
export function createResolver(
  base: ResolveBase = {},
  fetchFn: typeof fetch = apiFetch,
): FileResolver {
  const found = new Map<string, HostedResolved>()
  const missing = new Map<string, HostedResolved>()
  let inFlight = 0
  const waiting: (() => void)[] = []

  async function slot<T>(run: () => Promise<T>): Promise<T> {
    if (inFlight >= MAX_IN_FLIGHT) await new Promise<void>((go) => waiting.push(go))
    inFlight++
    try {
      return await run()
    } finally {
      inFlight--
      waiting.shift()?.()
    }
  }

  async function call(chunk: string[]) {
    try {
      const res = await fetchFn("/v1/hosted-files/resolve", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ ...base, candidates: chunk }),
      })
      if (!res.ok) return
      const body: HostedResolveResult = await res.json()
      if (body.refused) return
      for (const r of body.files ?? []) {
        if (r.verdict === "found") found.set(r.candidate, r)
        else missing.set(r.candidate, r)
      }
    } catch {
      // The candidates stay unresolved, so the page shows plain text.
    }
  }

  return {
    async resolve(candidates, options = {}) {
      for (const c of candidates) {
        if (options.fresh || options.recheckMissing?.has(c)) missing.delete(c)
      }
      if (options.fresh) for (const c of candidates) found.delete(c)
      const todo = [...new Set(candidates)].filter((c) => !found.has(c) && !missing.has(c))
      const calls: Promise<void>[] = []
      for (let i = 0; i < todo.length; i += MaxHostedCandidates) {
        const chunk = todo.slice(i, i + MaxHostedCandidates)
        calls.push(slot(() => call(chunk)))
      }
      await Promise.all(calls)
      const out = new Map<string, HostedResolved>()
      for (const c of candidates) {
        const r = found.get(c) ?? missing.get(c)
        if (r) out.set(c, r)
      }
      return out
    },
  }
}
