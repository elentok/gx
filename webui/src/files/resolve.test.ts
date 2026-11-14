import { describe, expect, it, vi } from "vitest"
import type { HostedResolveRequest } from "../generated/ops"
import { createResolver } from "./resolve"

function reply(req: HostedResolveRequest, verdict = "found") {
  const files = req.candidates.map((c) => ({
    candidate: c,
    verdict,
    ...(verdict === "found" ? { path: `/abs/${c}`, size: 1, kind: "text" } : {}),
  }))
  return new Response(JSON.stringify({ files }), { status: 200 })
}

const sent = (fn: ReturnType<typeof vi.fn>, n: number): HostedResolveRequest =>
  JSON.parse(fn.mock.calls[n][1].body)

const names = (n: number, prefix = "f") => Array.from({ length: n }, (_, i) => `${prefix}${i}.go`)

describe("createResolver", () => {
  it("drops duplicates and splits 250 distinct candidates into calls of at most 200", async () => {
    const fn = vi.fn(async (_u: unknown, init: RequestInit) => reply(JSON.parse(String(init.body))))
    const r = createResolver({ session: "s1" }, fn as unknown as typeof fetch)
    const list = names(250)
    const got = await r.resolve([...list, ...list])
    expect(fn).toHaveBeenCalledTimes(2)
    expect(sent(fn, 0).candidates).toHaveLength(200)
    expect(sent(fn, 1).candidates).toHaveLength(50)
    expect(sent(fn, 0).session).toBe("s1")
    expect(got.size).toBe(250)
    expect(got.get("f3.go")?.path).toBe("/abs/f3.go")
  })

  it("keeps at most 2 calls in flight", async () => {
    const release: (() => void)[] = []
    let live = 0
    let peak = 0
    const fn = vi.fn(
      (_u: unknown, init: RequestInit) =>
        new Promise<Response>((done) => {
          live++
          peak = Math.max(peak, live)
          release.push(() => {
            live--
            done(reply(JSON.parse(String(init.body))))
          })
        }),
    )
    const r = createResolver({}, fn as unknown as typeof fetch)
    const all = r.resolve(names(450))
    await vi.waitFor(() => expect(fn).toHaveBeenCalledTimes(2))
    await Promise.resolve()
    expect(fn).toHaveBeenCalledTimes(2)
    release[0]()
    await vi.waitFor(() => expect(fn).toHaveBeenCalledTimes(3))
    release[1]()
    release[2]()
    await all
    expect(peak).toBe(2)
  })

  it("does not send a found candidate again, and asks a missing one again after the recheck signal", async () => {
    const verdicts: Record<string, string> = { "a.go": "found", "b.go": "missing" }
    const fn = vi.fn(async (_u: unknown, init: RequestInit) => {
      const req: HostedResolveRequest = JSON.parse(String(init.body))
      return new Response(
        JSON.stringify({
          files: req.candidates.map((c) => ({ candidate: c, verdict: verdicts[c], path: `/${c}` })),
        }),
      )
    })
    const r = createResolver({}, fn as unknown as typeof fetch)
    await r.resolve(["a.go", "b.go"])
    await r.resolve(["a.go", "b.go"])
    expect(fn).toHaveBeenCalledTimes(1)
    verdicts["b.go"] = "found"
    const got = await r.resolve(["a.go", "b.go"], { recheckMissing: true })
    expect(fn).toHaveBeenCalledTimes(2)
    expect(sent(fn, 1).candidates).toEqual(["b.go"])
    expect(got.get("b.go")?.verdict).toBe("found")
  })

  it("leaves candidates unresolved when a call fails, is refused or throws", async () => {
    const responses: (() => Promise<Response>)[] = [
      async () => new Response("boom", { status: 500 }),
      async () => new Response(JSON.stringify({ refused: true, reason: "too-many-candidates" })),
      async () => {
        throw new Error("offline")
      },
    ]
    const fn = vi.fn(async () => responses.shift()?.() as Promise<Response>)
    const r = createResolver({}, fn as unknown as typeof fetch)
    for (const name of ["a.go", "b.go", "c.go"]) {
      expect((await r.resolve([name])).size).toBe(0)
    }
    expect(fn).toHaveBeenCalledTimes(3)
  })
})
