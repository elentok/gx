import { afterEach, describe, expect, it, vi } from "vitest"
import { leaderKeyOf } from "./leaderKey"

describe("leaderKeyOf", () => {
  afterEach(() => vi.restoreAllMocks())
  const snapshot = { leader_key_mac: "cmd+e", leader_key: "ctrl+e" }

  it("picks the Mac setting on a Mac", () => {
    vi.spyOn(navigator, "platform", "get").mockReturnValue("MacIntel")
    expect(leaderKeyOf(snapshot)).toBe("cmd+e")
  })

  it("picks the other setting elsewhere", () => {
    vi.spyOn(navigator, "platform", "get").mockReturnValue("Linux x86_64")
    expect(leaderKeyOf(snapshot)).toBe("ctrl+e")
  })

  it("reads an empty or missing key as off", () => {
    vi.spyOn(navigator, "platform", "get").mockReturnValue("Linux x86_64")
    expect(leaderKeyOf({})).toBe("")
    expect(leaderKeyOf({ leader_key: "" })).toBe("")
    expect(leaderKeyOf(null)).toBe("")
  })
})
