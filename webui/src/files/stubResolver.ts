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

/**
 * A fetch for the real resolver client: candidates in `found` are found, the rest missing.
 * `bodies` collects the parsed request body of each resolve call, so a test can read the base.
 */
export function stubResolveFetch(found: string[]) {
  const bodies: { project?: string; session?: string; candidates: string[] }[] = []
  const fetchFn = async (url: RequestInfo | URL, init?: RequestInit) => {
    if (!String(url).endsWith("/v1/hosted-files/resolve")) return new Response("", { status: 404 })
    const body = JSON.parse(String(init?.body))
    bodies.push(body)
    const files: HostedResolved[] = body.candidates.map((c: string) =>
      found.includes(c)
        ? { candidate: c, verdict: HostedFound, path: `/abs/${c.replace(/^\//, "")}` }
        : { candidate: c, verdict: "missing" },
    )
    return new Response(JSON.stringify({ files }), { status: 200 })
  }
  return { fetchFn, bodies }
}
