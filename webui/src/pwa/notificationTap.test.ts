import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import { listenForNotificationTaps, setTapRouter } from "./notificationTap"
import { TAP_MESSAGE } from "./workerCore"

function fakeWorkerContainer() {
  const target = new EventTarget()
  Object.defineProperty(navigator, "serviceWorker", { value: target, configurable: true })
  return {
    tap(path: unknown, type: string = TAP_MESSAGE) {
      const port = { postMessage: vi.fn() }
      target.dispatchEvent(
        Object.assign(new Event("message"), { data: { type, path }, ports: [port] }),
      )
      return port
    },
  }
}

describe("notificationTap", () => {
  let worker: ReturnType<typeof fakeWorkerContainer>
  beforeEach(() => {
    window.location.hash = ""
    worker = fakeWorkerContainer()
    listenForNotificationTaps()
  })
  afterEach(() => {
    setTapRouter(null)
    Reflect.deleteProperty(navigator, "serviceWorker")
  })

  it("navigates the registered router and answers on the port", () => {
    const router = { navigate: vi.fn() }
    setTapRouter(router)
    const port = worker.tap("/s/1")
    expect(router.navigate).toHaveBeenCalledWith("/s/1")
    expect(port.postMessage).toHaveBeenCalledOnce()
    expect(window.location.hash).toBe("")
  })

  it("sets the hash when no router is mounted", () => {
    const port = worker.tap("/s/1")
    expect(window.location.hash).toBe("#/s/1")
    expect(port.postMessage).toHaveBeenCalledOnce()
  })

  it("stops using a router once it is unregistered", () => {
    const router = { navigate: vi.fn() }
    setTapRouter(router)()
    worker.tap("/s/2")
    expect(router.navigate).not.toHaveBeenCalled()
    expect(window.location.hash).toBe("#/s/2")
  })

  it.each(["//evil.example", "https://evil.example", "/\\evil", "s/1", "", 42, undefined])(
    "ignores the unsafe path %j",
    (path) => {
      const router = { navigate: vi.fn() }
      setTapRouter(router)
      const port = worker.tap(path)
      expect(router.navigate).not.toHaveBeenCalled()
      expect(window.location.hash).toBe("")
      expect(port.postMessage).not.toHaveBeenCalled()
    },
  )

  it("ignores an unrelated message", () => {
    const router = { navigate: vi.fn() }
    setTapRouter(router)
    const port = worker.tap("/s/1", "other")
    expect(router.navigate).not.toHaveBeenCalled()
    expect(port.postMessage).not.toHaveBeenCalled()
  })
})
