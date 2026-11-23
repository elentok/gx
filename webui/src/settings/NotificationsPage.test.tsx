import { fireEvent, waitFor } from "@testing-library/react"
import { MemoryRouter, Route, Routes } from "react-router"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import { ConnectionContext, type Connection, type ConnectionState } from "../connection"
import { refreshPushDevice, resetPushDeviceForTest } from "../push/deviceState"
import { fetchPushState, removePush, subscribePush, testPush } from "../push/pushApi"
import { renderThemed } from "../ui/render"
import { NotificationsPage } from "./NotificationsPage"
import { fetchLogins } from "./logins"

vi.mock("../push/pushApi", () => ({
  fetchPushState: vi.fn(),
  subscribePush: vi.fn(),
  removePush: vi.fn(),
  testPush: vi.fn(),
}))
vi.mock("./logins", () => ({ fetchLogins: vi.fn() }))

const connState = {
  connected: true,
  loadedOnce: true,
  liveSnapshot: true,
  tick: 0,
  streams: 0,
} as ConnectionState
const conn = {
  getState: () => connState,
  subscribe: () => () => {},
  onEvent: () => () => {},
} as unknown as Connection

const renderPage = () =>
  renderThemed(
    <ConnectionContext.Provider value={conn}>
      <MemoryRouter initialEntries={["/settings/notifications"]}>
        <Routes>
          <Route path="/settings/notifications" element={<NotificationsPage />} />
        </Routes>
      </MemoryRouter>
    </ConnectionContext.Provider>,
  )

const SERVER_KEY = "AQID"
const NAME = "David's iPhone"

let permission: NotificationPermission
let held: { options: { applicationServerKey: ArrayBuffer }; toJSON: () => unknown } | null
const requestPermission = vi.fn()
const subscribe = vi.fn()
const clearAppBadge = vi.fn()
let serverRecord: boolean

const subscription = {
  options: { applicationServerKey: new Uint8Array([1, 2, 3]).buffer },
  toJSON: () => ({ endpoint: "https://push.example/x", keys: { p256dh: "P", auth: "A" } }),
}

beforeEach(() => {
  permission = "default"
  held = null
  serverRecord = false
  requestPermission.mockImplementation(async () => {
    permission = "granted"
    return "granted"
  })
  subscribe.mockImplementation(async () => {
    held = subscription
    return subscription
  })
  vi.stubGlobal("Notification", {
    get permission() {
      return permission
    },
    requestPermission,
  })
  vi.stubGlobal("PushManager", class {})
  // Added to the real navigator: replacing it breaks the device detection the page chrome reads.
  Object.defineProperties(navigator, {
    clearAppBadge: { value: clearAppBadge, configurable: true },
    serviceWorker: {
      value: {
        ready: Promise.resolve({ pushManager: { getSubscription: async () => held, subscribe } }),
      },
      configurable: true,
    },
  })
  vi.mocked(fetchPushState).mockImplementation(async () => ({
    public_key: SERVER_KEY,
    contact_set: true,
    subscribed: serverRecord,
    endpoint: "",
  }))
  vi.mocked(fetchLogins).mockResolvedValue([
    { id: "b", name: NAME, last_used: "", last_ip: "", this_device: true },
  ])
  vi.mocked(subscribePush).mockImplementation(async () => {
    serverRecord = true
    return {}
  })
  vi.mocked(removePush).mockImplementation(async () => {
    serverRecord = false
    return {}
  })
  vi.mocked(testPush).mockResolvedValue({ sent: true })
  resetPushDeviceForTest()
})

afterEach(() => {
  vi.clearAllMocks()
  vi.unstubAllGlobals()
  Reflect.deleteProperty(navigator, "clearAppBadge")
  Reflect.deleteProperty(navigator, "serviceWorker")
})

describe("NotificationsPage", () => {
  it("never asks for permission on load, and asks exactly once on the switch tap", async () => {
    const { findByRole } = renderPage()
    const sw = await findByRole("switch", { name: "Notifications on this device" })
    await waitFor(() => expect(fetchPushState).toHaveBeenCalled())
    expect(requestPermission).not.toHaveBeenCalled()
    expect(sw.getAttribute("aria-checked")).toBe("false")
    fireEvent.click(sw)
    await waitFor(() => expect(sw.getAttribute("aria-checked")).toBe("true"))
    expect(requestPermission).toHaveBeenCalledTimes(1)
  })

  it("posts the subscription with the server key, then reads On for the login name", async () => {
    const { findByRole, findByText } = renderPage()
    fireEvent.click(await findByRole("switch"))
    await findByText(`On for “${NAME}”.`)
    expect(subscribe).toHaveBeenCalledWith({
      userVisibleOnly: true,
      applicationServerKey: new Uint8Array([1, 2, 3]),
    })
    expect(subscribePush).toHaveBeenCalledWith({
      endpoint: "https://push.example/x",
      p256dh: "P",
      auth: "A",
      vapid_key: SERVER_KEY,
    })
  })

  it("reuses a browser subscription that already has the server key", async () => {
    held = subscription
    const { findByRole, findByText } = renderPage()
    fireEvent.click(await findByRole("switch"))
    await findByText(`On for “${NAME}”.`)
    expect(subscribe).not.toHaveBeenCalled()
  })

  it("stays Off and shows the reason when the server refuses", async () => {
    vi.mocked(subscribePush).mockResolvedValue({
      refused: true,
      reason: "push-no-contact",
      message: "Set a contact email.",
    })
    const { findByRole, findByText } = renderPage()
    const sw = await findByRole("switch")
    fireEvent.click(sw)
    await findByText("Set a contact email.")
    expect(sw.getAttribute("aria-checked")).toBe("false")
  })

  it("stays Off when the prompt is denied", async () => {
    requestPermission.mockImplementation(async () => {
      permission = "denied"
      return "denied"
    })
    const { findByRole } = renderPage()
    const sw = await findByRole("switch")
    fireEvent.click(sw)
    await waitFor(() => expect(requestPermission).toHaveBeenCalled())
    expect(subscribePush).not.toHaveBeenCalled()
    expect(sw.getAttribute("aria-checked")).toBe("false")
  })

  it("turns Off through the remove route and clears the badge, keeping the browser subscription", async () => {
    permission = "granted"
    held = subscription
    serverRecord = true
    await refreshPushDevice()
    const { findByRole, findByText } = renderPage()
    const sw = await findByRole("switch")
    await waitFor(() => expect(sw.getAttribute("aria-checked")).toBe("true"))
    fireEvent.click(sw)
    await findByText("Off. Turning it on asks this browser for permission.")
    expect(removePush).toHaveBeenCalledTimes(1)
    expect(clearAppBadge).toHaveBeenCalledTimes(1)
    expect(requestPermission).not.toHaveBeenCalled()
  })

  it("disables Send test push while Off and sends one while On", async () => {
    const { findByRole, findByText, getByRole } = renderPage()
    const sw = await findByRole("switch")
    const test = getByRole("button", { name: /Send test push/ })
    expect(test.hasAttribute("disabled")).toBe(true)
    await findByText("Turn notifications on first.")
    fireEvent.click(sw)
    await findByText("Sends one push to this device only.")
    expect(test.hasAttribute("disabled")).toBe(false)
    fireEvent.click(test)
    await findByText(
      "Sent. It should arrive in a few seconds. If nothing shows, check the Focus mode and iOS notification settings.",
    )
    expect(testPush).toHaveBeenCalledTimes(1)
  })
})
