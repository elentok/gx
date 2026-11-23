import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import { fetchLogins } from "../settings/logins"
import { getDevice, refreshPushDevice, resetPushDeviceForTest } from "./deviceState"
import { runOpenChecks } from "./openChecks"
import { fetchPushState, removePush, renewPush } from "./pushApi"

vi.mock("./pushApi", () => ({
  fetchPushState: vi.fn(),
  removePush: vi.fn(),
  renewPush: vi.fn(),
}))
vi.mock("../settings/logins", () => ({ fetchLogins: vi.fn() }))

const SERVER_KEY = "AQID" // bytes 1, 2, 3
const KEY_STORE = "gx.push.keyChanged"

const unsubscribe = vi.fn()

function stubBrowser(sub: { key: number[]; endpoint: string } | null) {
  vi.stubGlobal("Notification", { permission: "granted" })
  vi.stubGlobal("PushManager", class {})
  const subscription = sub && {
    endpoint: sub.endpoint,
    options: { applicationServerKey: new Uint8Array(sub.key).buffer },
    toJSON: () => ({ endpoint: sub.endpoint, keys: { p256dh: "P", auth: "A" } }),
    unsubscribe,
  }
  Object.defineProperty(navigator, "serviceWorker", {
    value: {
      ready: Promise.resolve({ pushManager: { getSubscription: async () => subscription } }),
    },
    configurable: true,
  })
}

function stubServer(opts: { subscribed: boolean; endpoint?: string }) {
  vi.mocked(fetchPushState).mockResolvedValue({
    public_key: SERVER_KEY,
    contact_set: true,
    subscribed: opts.subscribed,
    endpoint: opts.endpoint ?? "",
  })
  vi.mocked(fetchLogins).mockResolvedValue([])
}

describe("runOpenChecks", () => {
  beforeEach(() => {
    resetPushDeviceForTest()
    localStorage.clear()
    vi.mocked(removePush).mockResolvedValue({})
    vi.mocked(renewPush).mockResolvedValue({})
  })
  afterEach(() => {
    vi.clearAllMocks()
    vi.unstubAllGlobals()
    Reflect.deleteProperty(navigator, "serviceWorker")
    vi.restoreAllMocks()
  })

  it("1: no browser subscription and a server record calls the remove route, and the switch reads Off", async () => {
    stubBrowser(null)
    stubServer({ subscribed: true })
    await runOpenChecks()
    expect(removePush).toHaveBeenCalledTimes(1)
    stubServer({ subscribed: false })
    await refreshPushDevice()
    expect(getDevice().on).toBe(false)
  })

  it("2: a mismatching key unsubscribes, removes the record and keeps the reason across reopens", async () => {
    stubBrowser({ key: [9, 9, 9], endpoint: "https://push.example/a" })
    stubServer({ subscribed: true, endpoint: "https://push.example/a" })
    await runOpenChecks()
    expect(unsubscribe).toHaveBeenCalledTimes(1)
    expect(removePush).toHaveBeenCalledTimes(1)
    expect(renewPush).not.toHaveBeenCalled()
    stubBrowser(null)
    stubServer({ subscribed: false })
    await refreshPushDevice()
    expect(getDevice()).toMatchObject({ on: false, keyChanged: true })
  })

  it("2: a mismatching key with no server record does not call the remove route", async () => {
    stubBrowser({ key: [9, 9, 9], endpoint: "https://push.example/a" })
    stubServer({ subscribed: false })
    await runOpenChecks()
    expect(unsubscribe).toHaveBeenCalledTimes(1)
    expect(removePush).not.toHaveBeenCalled()
    expect(localStorage.getItem(KEY_STORE)).toBe("1")
  })

  it("3: a differing endpoint calls the renewal route with the old endpoint", async () => {
    stubBrowser({ key: [1, 2, 3], endpoint: "https://push.example/new" })
    stubServer({ subscribed: true, endpoint: "https://push.example/old" })
    await runOpenChecks()
    expect(renewPush).toHaveBeenCalledWith({
      endpoint: "https://push.example/new",
      p256dh: "P",
      auth: "A",
      vapid_key: SERVER_KEY,
      old_endpoint: "https://push.example/old",
    })
    expect(removePush).not.toHaveBeenCalled()
  })

  it("4: a browser subscription with no record calls no route", async () => {
    stubBrowser({ key: [1, 2, 3], endpoint: "https://push.example/a" })
    stubServer({ subscribed: false })
    await runOpenChecks()
    expect(removePush).not.toHaveBeenCalled()
    expect(renewPush).not.toHaveBeenCalled()
    expect(unsubscribe).not.toHaveBeenCalled()
  })

  it("does nothing on a matching subscription and record", async () => {
    stubBrowser({ key: [1, 2, 3], endpoint: "https://push.example/a" })
    stubServer({ subscribed: true, endpoint: "https://push.example/a" })
    await runOpenChecks()
    expect(removePush).not.toHaveBeenCalled()
    expect(renewPush).not.toHaveBeenCalled()
  })

  it("works when localStorage throws", async () => {
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("denied")
    })
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("denied")
    })
    stubBrowser({ key: [9, 9, 9], endpoint: "https://push.example/a" })
    stubServer({ subscribed: true, endpoint: "https://push.example/a" })
    await expect(runOpenChecks()).resolves.toBeUndefined()
    expect(unsubscribe).toHaveBeenCalledTimes(1)
    stubBrowser(null)
    stubServer({ subscribed: false })
    await refreshPushDevice()
    expect(getDevice().keyChanged).toBe(false)
  })
})
