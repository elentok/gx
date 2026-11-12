import { renderHook, screen } from "@testing-library/react"
import { createElement } from "react"
import { afterEach, describe, expect, it, vi } from "vitest"
import { ConnectionContext, type Connection } from "./connection"
import { postDevFrontend, postDevRestart, useDev } from "./dev"
import { renderThemed } from "./ui/render"

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
    postDevFrontend(true)
    await vi.waitFor(() => expect(fetchMock).toHaveBeenCalled())
    expect(fetchMock.mock.calls[0][0]).toBe("/v1/settings")
    expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({
      changes: [{ key: "server.web.dev-frontend", value: true }],
    })
  })

  it("shows an error toast when the write is refused", async () => {
    vi.stubGlobal(
      "fetch",
      vi
        .fn()
        .mockResolvedValue(
          new Response('{"refused":true,"reason":"Dev is locked"}', { status: 200 }),
        ),
    )
    renderThemed(createElement("div"))
    postDevFrontend(true)
    expect(await screen.findByText("Dev is locked")).toBeTruthy()
  })

  it("shows an error toast when the request fails", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("", { status: 500 })))
    renderThemed(createElement("div"))
    postDevFrontend(false)
    expect(await screen.findByText("Request failed (HTTP 500).")).toBeTruthy()
  })
})

describe("postDevRestart", () => {
  afterEach(() => vi.unstubAllGlobals())

  it("posts to the restart route and toasts a refusal", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValue(
        new Response('{"refused":true,"message":"Vite is running"}', { status: 200 }),
      )
    vi.stubGlobal("fetch", fetchMock)
    renderThemed(createElement("div"))
    postDevRestart()
    expect(await screen.findByText("Vite is running")).toBeTruthy()
    expect(fetchMock.mock.calls[0][0]).toBe("/v1/dev/restart")
  })
})
