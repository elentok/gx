// @vitest-environment node
import { readFileSync } from "node:fs"
import { describe, expect, it } from "vitest"
import { CACHE_PREFIX } from "./workerCore"

const source = readFileSync(new URL("./devWorker.js", import.meta.url), "utf8")

type Handler = (event: { waitUntil: (p: Promise<unknown>) => void }) => void

async function activate(cacheNames: string[]) {
  const steps: string[] = []
  const handlers = new Map<string, Handler>()
  const fakeSelf = {
    addEventListener: (type: string, h: Handler) => handlers.set(type, h),
    skipWaiting: () => steps.push("skipWaiting"),
    caches: {
      keys: async () => cacheNames,
      delete: async (n: string) => void steps.push(`delete ${n}`),
    },
    registration: { unregister: async () => void steps.push("unregister") },
    clients: {
      matchAll: async () => [
        {
          url: "https://gx.test/a",
          navigate: async (u: string) => void steps.push(`navigate ${u}`),
        },
        {
          url: "https://gx.test/b",
          navigate: async (u: string) => void steps.push(`navigate ${u}`),
        },
      ],
    },
  }
  new Function("self", source)(fakeSelf)
  handlers.get("install")?.({ waitUntil: () => {} })
  let done: Promise<unknown> = Promise.resolve()
  handlers.get("activate")?.({ waitUntil: (p) => (done = p) })
  await done
  return steps
}

describe("dev self-removing worker", () => {
  it("deletes only gx-shell- caches, then unregisters, then navigates the windows", async () => {
    const steps = await activate([`${CACHE_PREFIX}1`, "other-cache", `${CACHE_PREFIX}2`])
    expect(steps[0]).toBe("skipWaiting")
    expect(steps.filter((s) => s.startsWith("delete")).sort()).toEqual([
      `delete ${CACHE_PREFIX}1`,
      `delete ${CACHE_PREFIX}2`,
    ])
    expect(steps.slice(3)).toEqual([
      "unregister",
      "navigate https://gx.test/a",
      "navigate https://gx.test/b",
    ])
  })

  it("uses the prefix the page shares", () => {
    expect(source).toContain(`"${CACHE_PREFIX}"`)
  })
})
