import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import { signOut } from "./apiFetch"
import { dropQueue, enableQueue, queueSample } from "./webPerf"

const sample = { metric: "fcp", value: 10 } as const

let fetchMock: ReturnType<typeof vi.fn>

function setVisibility(state: "hidden" | "visible") {
  Object.defineProperty(document, "visibilityState", { value: state, configurable: true })
  document.dispatchEvent(new Event("visibilitychange"))
}

function sent(call = 0) {
  return JSON.parse(fetchMock.mock.calls[call][1].body as string)
}

beforeEach(() => {
  vi.useFakeTimers()
  fetchMock = vi.fn().mockResolvedValue(new Response("{}"))
  vi.stubGlobal("fetch", fetchMock)
  Object.defineProperty(navigator, "sendBeacon", { value: undefined, configurable: true })
  enableQueue()
})

afterEach(() => {
  dropQueue()
  setVisibility("visible")
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

describe("webPerf", () => {
  it("flushes at 20 samples", () => {
    for (let i = 0; i < 19; i++) queueSample(sample)
    expect(fetchMock).not.toHaveBeenCalled()
    queueSample(sample)
    expect(fetchMock).toHaveBeenCalledTimes(1)
    const body = sent()
    expect(body.samples).toHaveLength(20)
    expect(new Set(body.samples.map((s: { id: string }) => s.id)).size).toBe(20)
    expect(body.v).toBe(1)
  })

  it("flushes 10 s after the last sample", () => {
    queueSample(sample)
    vi.advanceTimersByTime(9_000)
    queueSample(sample)
    vi.advanceTimersByTime(9_000)
    expect(fetchMock).not.toHaveBeenCalled()
    vi.advanceTimersByTime(1_000)
    expect(sent().samples).toHaveLength(2)
  })

  it("sends a beacon with a JSON blob when the page is hidden", () => {
    const beacon = vi.fn().mockReturnValue(true)
    Object.defineProperty(navigator, "sendBeacon", { value: beacon, configurable: true })
    queueSample(sample)
    setVisibility("hidden")
    expect(beacon).toHaveBeenCalledTimes(1)
    expect(beacon.mock.calls[0][0]).toBe("/v1/web-performance")
    expect((beacon.mock.calls[0][1] as Blob).type).toBe("application/json")
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it.each([
    ["missing", undefined],
    ["false", () => false],
    [
      "throwing",
      () => {
        throw new Error("no")
      },
    ],
  ])("falls back to keepalive fetch when sendBeacon is %s", (_name, impl) => {
    Object.defineProperty(navigator, "sendBeacon", {
      value: impl && vi.fn(impl),
      configurable: true,
    })
    queueSample(sample)
    setVisibility("hidden")
    expect(fetchMock).toHaveBeenCalledTimes(1)
    const init = fetchMock.mock.calls[0][1]
    expect(init.method).toBe("POST")
    expect(init.keepalive).toBe(true)
    expect(init.headers).toEqual({ "Content-Type": "application/json" })
  })

  it.each([
    ["refused", () => Promise.resolve(new Response('{"refused":true}'))],
    ["non-2xx", () => Promise.resolve(new Response("no", { status: 401 }))],
    ["network failure", () => Promise.reject(new Error("offline"))],
    [
      "throw",
      () => {
        throw new Error("boom")
      },
    ],
  ])("drops the batch without retry on %s", async (_name, impl) => {
    fetchMock.mockImplementation(impl)
    const errors = vi.spyOn(console, "error").mockImplementation(() => {})
    queueSample(sample)
    vi.advanceTimersByTime(10_000)
    await vi.advanceTimersByTimeAsync(60_000)
    setVisibility("hidden")
    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(errors).not.toHaveBeenCalled()
  })

  it("signOut drops the queue and the idle timer without waiting", () => {
    queueSample(sample)
    signOut()
    vi.advanceTimersByTime(60_000)
    setVisibility("hidden")
    expect(fetchMock).not.toHaveBeenCalled()
  })

  it("discards samples queued while the gate switch is off", async () => {
    vi.resetModules()
    const fresh = await import("./webPerf")
    fresh.queueSample(sample)
    vi.advanceTimersByTime(60_000)
    setVisibility("hidden")
    expect(fetchMock).not.toHaveBeenCalled()
    fresh.enableQueue()
    fresh.queueSample(sample)
    vi.advanceTimersByTime(10_000)
    expect(sent().samples).toHaveLength(1)
  })

  it("keeps one session per load and carries build and version", () => {
    queueSample(sample)
    vi.advanceTimersByTime(10_000)
    queueSample(sample)
    vi.advanceTimersByTime(10_000)
    expect(sent(0).session).toMatch(/^[0-9a-f]{32}$/)
    expect(sent(1).session).toBe(sent(0).session)
    expect(sent().web_version).toBe(__WEB_VERSION__)
  })

  it("reads build_id from the entry script and sends env dev without a hash", () => {
    queueSample(sample)
    vi.advanceTimersByTime(10_000)
    expect(sent().build_id).toBe("dev")
    expect(sent().samples[0].env).toBe("dev")

    const script = document.createElement("script")
    script.type = "module"
    script.src = "/assets/index-AbC123_-x.js"
    document.head.append(script)
    queueSample(sample)
    vi.advanceTimersByTime(10_000)
    script.remove()
    expect(sent(1).build_id).toBe("AbC123_-x")
    expect(sent(1).samples[0].env).toBeUndefined()
  })

  it("never writes to storage", () => {
    queueSample(sample)
    vi.advanceTimersByTime(10_000)
    expect(localStorage.length + sessionStorage.length).toBe(0)
  })
})
