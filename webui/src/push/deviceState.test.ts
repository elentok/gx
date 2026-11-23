import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import { fetchLogins } from "../settings/logins"
import { getDevice, refreshPushDevice, resetPushDeviceForTest } from "./deviceState"
import { fetchPushState } from "./pushApi"

vi.mock("./pushApi", () => ({ fetchPushState: vi.fn() }))
vi.mock("../settings/logins", () => ({ fetchLogins: vi.fn() }))

const SERVER_KEY = "AQID" // bytes 1, 2, 3

function stubBrowser(opts: { permission: NotificationPermission; subKey?: number[] }) {
  vi.stubGlobal("Notification", { permission: opts.permission })
  vi.stubGlobal("PushManager", class {})
  const sub = opts.subKey && {
    options: { applicationServerKey: new Uint8Array(opts.subKey).buffer },
  }
  vi.stubGlobal("navigator", {
    serviceWorker: {
      ready: Promise.resolve({ pushManager: { getSubscription: async () => sub ?? null } }),
    },
  })
}

function stubServer(subscribed: boolean) {
  vi.mocked(fetchPushState).mockResolvedValue({
    public_key: SERVER_KEY,
    contact_set: true,
    subscribed,
    endpoint: "",
  })
  vi.mocked(fetchLogins).mockResolvedValue([
    { id: "a", name: "Other", last_used: "", last_ip: "", this_device: false },
    { id: "b", name: "David's iPhone", last_used: "", last_ip: "", this_device: true },
  ])
}

describe("push device state", () => {
  beforeEach(() => resetPushDeviceForTest())
  afterEach(() => vi.unstubAllGlobals())

  it("is on only with permission, a browser subscription, a server record and equal keys", async () => {
    const cases: [string, NotificationPermission, number[] | undefined, boolean, boolean][] = [
      ["all four", "granted", [1, 2, 3], true, true],
      ["permission not granted", "denied", [1, 2, 3], true, false],
      ["no browser subscription", "granted", undefined, true, false],
      ["no server record", "granted", [1, 2, 3], false, false],
      ["keys differ", "granted", [9, 9, 9], true, false],
    ]
    for (const [name, permission, subKey, record, want] of cases) {
      stubBrowser({ permission, subKey })
      stubServer(record)
      await refreshPushDevice()
      expect(getDevice().on, name).toBe(want)
    }
  })

  it("reads the login name of this device and the parts", async () => {
    stubBrowser({ permission: "granted", subKey: [9, 9, 9] })
    stubServer(true)
    await refreshPushDevice()
    expect(getDevice()).toMatchObject({
      supported: true,
      permission: "granted",
      browserSub: true,
      serverRecord: true,
      keyMatches: false,
      loginName: "David's iPhone",
    })
  })

  it("is unsupported without Notification", async () => {
    vi.stubGlobal("Notification", undefined)
    await refreshPushDevice()
    expect(getDevice()).toMatchObject({ supported: false, on: false })
  })
})
