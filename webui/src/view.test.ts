import { describe, expect, it } from "vitest"
import * as wire from "./generated/server"
import { epicBadge } from "./view"

describe("epicBadge", () => {
  it("has a label for every wire epic state", () => {
    const states = Object.entries(wire)
      .filter(([name, value]) => name.startsWith("EpicState") && typeof value === "string")
      .map(([, value]) => value as string)
    expect(states.length).toBeGreaterThan(0)
    for (const state of states) {
      expect(epicBadge(state as wire.EpicState), state).toBeTruthy()
    }
  })

  it("labels each state", () => {
    expect(epicBadge("waiting-on-epic")).toBe("Waiting")
    expect(epicBadge("paused")).toBe("Paused")
    expect(epicBadge("running")).toBe("Running")
    expect(epicBadge("waiting")).toBe("Waiting for a slot")
    expect(epicBadge("awaiting-merge")).toBe("Awaiting merge")
    expect(epicBadge("finished")).toBe("Merged")
  })

  it("labels Waiting by queue mode and leaves Running unchanged", () => {
    expect(epicBadge("waiting", "paused")).toBe("Waiting: queue paused")
    expect(epicBadge("waiting", "running")).toBe("Waiting for a slot")
    expect(epicBadge("running", "paused")).toBe("Running")
  })
})
