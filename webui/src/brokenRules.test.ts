import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import { reportBrokenRule } from "./brokenRules"

const fetchMock = vi.fn(async (_url: string, _init: RequestInit) => new Response("{}"))
const consoleError = vi.fn()

beforeEach(() => {
  vi.stubGlobal("fetch", fetchMock)
  vi.spyOn(console, "error").mockImplementation(consoleError)
})
afterEach(() => {
  fetchMock.mockClear()
  consoleError.mockClear()
  vi.restoreAllMocks()
  vi.unstubAllGlobals()
  window.location.hash = ""
})

const bodies = () => fetchMock.mock.calls.map(([, init]) => JSON.parse(init.body as string))

describe("reportBrokenRule", () => {
  it("posts and logs each rule and value pair once", () => {
    reportBrokenRule("some-rule", "it broke", "once")
    reportBrokenRule("some-rule", "it broke again", "once")
    expect(fetchMock).toHaveBeenCalledTimes(1)
    const [url, init] = fetchMock.mock.calls[0]
    expect(url).toBe("/v1/broken-rules")
    expect(init.method).toBe("POST")
    expect(init.headers).toEqual({ "Content-Type": "application/json" })
    expect(bodies()[0]).toEqual({
      rule: "some-rule",
      message: "it broke",
      value: "once",
      url: window.location.origin + window.location.pathname,
      user_agent: navigator.userAgent,
    })
    expect(consoleError).toHaveBeenCalledTimes(1)
    expect(consoleError.mock.calls[0].join(" ")).toContain("some-rule: it broke")
  })

  it("posts again for another value or another rule", () => {
    reportBrokenRule("some-rule", "it broke", "first")
    reportBrokenRule("some-rule", "it broke", "second")
    reportBrokenRule("other-rule", "it broke", "first")
    expect(bodies().map((b) => [b.rule, b.value])).toEqual([
      ["some-rule", "first"],
      ["some-rule", "second"],
      ["other-rule", "first"],
    ])
    expect(consoleError).toHaveBeenCalledTimes(3)
  })

  it.each([
    ["a failed send", () => Promise.reject(new Error("offline"))],
    ["a 401 answer", async () => new Response("", { status: 401 })],
  ])("ignores %s and stays on the page", async (title, answer) => {
    window.location.hash = "#/here"
    const before = window.location.href
    fetchMock.mockImplementationOnce(answer)
    reportBrokenRule("some-rule", "it broke", title)
    // A macrotask lets every handler on the answer run before the checks.
    await new Promise((resolve) => setTimeout(resolve, 0))
    expect(window.location.href).toBe(before)
    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(consoleError).toHaveBeenCalledTimes(1)
  })

  it("drops the hash query from the posted page URL", () => {
    window.location.hash = "#/setup?code=K7QM-3XPD"
    reportBrokenRule("some-rule", "it broke", "setup")
    const { url } = bodies()[0]
    expect(url).toContain("#/setup")
    expect(url).not.toContain("K7QM")
    expect(url).not.toContain("?")
  })
})
