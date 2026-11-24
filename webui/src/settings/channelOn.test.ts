import { describe, expect, it } from "vitest"
import type { SettingsEntry } from "../generated/server"
import { channelOn } from "./channelOn"

const sw = (over: Partial<SettingsEntry>) =>
  ({
    key: "notification-channels.push.enabled",
    set: false,
    default: true,
    ...over,
  }) as SettingsEntry
const channel = { switch_key: "notification-channels.push.enabled" }

describe("channelOn", () => {
  it("reads a missing switch entry as on", () => {
    expect(channelOn([], channel)).toBe(true)
  })
  it("reads the written value over the default", () => {
    expect(channelOn([sw({ set: true, value: false })], channel)).toBe(false)
    expect(channelOn([sw({ default: false })], channel)).toBe(false)
    expect(channelOn([sw({ set: true, value: true, default: false })], channel)).toBe(true)
  })
})
