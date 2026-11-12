import { renderHook } from "@testing-library/react"
import { createElement } from "react"
import { afterEach, describe, expect, it, vi } from "vitest"
import { ConnectionContext, type Connection } from "./connection"
import { useLeaderKey } from "./leaderKey"

const conn = (snapshot: object) => {
  const state = { snapshot, connected: true }
  return { subscribe: () => () => {}, getState: () => state } as unknown as Connection
}

const leaderKey = (snapshot: object) =>
  renderHook(() => useLeaderKey(), {
    wrapper: ({ children }: { children: React.ReactNode }) =>
      createElement(ConnectionContext.Provider, { value: conn(snapshot) }, children),
  }).result.current

describe("useLeaderKey", () => {
  afterEach(() => vi.restoreAllMocks())
  const snapshot = { leader_key_mac: "cmd+e", leader_key: "ctrl+e" }

  it("picks the Mac setting on a Mac", () => {
    vi.spyOn(navigator, "platform", "get").mockReturnValue("MacIntel")
    expect(leaderKey(snapshot)).toBe("cmd+e")
  })

  it("picks the other setting elsewhere", () => {
    vi.spyOn(navigator, "platform", "get").mockReturnValue("Linux x86_64")
    expect(leaderKey(snapshot)).toBe("ctrl+e")
  })

  it("reads an empty or missing key as off", () => {
    vi.spyOn(navigator, "platform", "get").mockReturnValue("Linux x86_64")
    expect(leaderKey({})).toBe("")
    expect(leaderKey({ leader_key: "" })).toBe("")
  })
})
