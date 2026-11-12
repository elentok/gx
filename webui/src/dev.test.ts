import { renderHook } from "@testing-library/react"
import { createElement } from "react"
import { afterEach, describe, expect, it, vi } from "vitest"
import { ConnectionContext, type Connection } from "./connection"
import { postDevFrontend, postDevRestart, useDev } from "./dev"
import { WriteError } from "./useWrite"

describe("useDev", () => {
  const conn = (dev?: object) => {
    const state = { snapshot: { dev }, connected: true }
    return { subscribe: () => () => {}, getState: () => state } as unknown as Connection
  }
  const wrap =
    (c: Connection) =>
    ({ children }: { children: React.ReactNode }) =>
      createElement(ConnectionContext.Provider, { value: c }, children)

  it("returns the snapshot's dev block", () => {
    const dev = {
      state: "running",
      has_checkout: true,
      reason: "r",
      log_tail: ["a"],
      checkout: "/x",
    }
    expect(renderHook(() => useDev(), { wrapper: wrap(conn(dev)) }).result.current).toEqual(dev)
  })

  it("reads as off without a provider", () => {
    expect(renderHook(() => useDev()).result.current).toEqual({ state: "off", has_checkout: false })
  })

  it("reads as off when the block is missing", () => {
    expect(renderHook(() => useDev(), { wrapper: wrap(conn()) }).result.current.state).toBe("off")
  })
})

describe("postDevFrontend", () => {
  afterEach(() => vi.unstubAllGlobals())

  it("writes the key through the settings route", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response("{}", { status: 200 }))
    vi.stubGlobal("fetch", fetchMock)
    await postDevFrontend(true)
    expect(fetchMock.mock.calls[0][0]).toBe("/v1/settings")
    expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({
      changes: [{ key: "server.web.dev-frontend", value: true }],
    })
  })

  it("throws a WriteError when the write is refused", async () => {
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockResolvedValue(
          new Response('{"refused":true,"reason":"Dev is locked"}', { status: 200 }),
        ),
    )
    const err = await postDevFrontend(true).catch((e) => e)
    expect(err).toBeInstanceOf(WriteError)
    expect(err.message).toBe("Dev is locked")
  })

  it("rejects when the request fails", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("", { status: 500 })))
    await expect(postDevFrontend(false)).rejects.toThrow("Request failed (HTTP 500).")
  })
})

describe("postDevRestart", () => {
  afterEach(() => vi.unstubAllGlobals())

  it("posts to the restart route and throws a WriteError on a refusal", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(
        new Response('{"refused":true,"message":"Vite is running"}', { status: 200 }),
      )
    vi.stubGlobal("fetch", fetchMock)
    await expect(postDevRestart()).rejects.toThrow("Vite is running")
    expect(fetchMock.mock.calls[0][0]).toBe("/v1/dev/restart")
  })
})
