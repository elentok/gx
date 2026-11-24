import { act, cleanup, fireEvent, screen } from "@testing-library/react"
import { afterEach, expect, test, vi } from "vitest"
import { ConnectionContext, type Connection, type EventListener } from "./connection"
import { NotificationToast } from "./NotificationToast"
import { setTapRouter } from "./pwa/notificationTap"
import { renderThemed } from "./ui/render"
import { toaster } from "./ui/toaster"

afterEach(() => {
  cleanup()
  toaster.remove()
})

function setup() {
  const listeners = new Set<EventListener>()
  const conn = {
    onEvent: (l: EventListener) => {
      listeners.add(l)
      return () => listeners.delete(l)
    },
  } as unknown as Connection
  renderThemed(
    <ConnectionContext.Provider value={conn}>
      <NotificationToast />
    </ConnectionContext.Provider>,
  )
  return (data: Record<string, unknown>, type = "notification-toast") =>
    act(() => {
      for (const l of listeners) l(undefined, type, data)
    })
}

test("shows the title and the body", async () => {
  const send = setup()
  send({ title: "Ticket paused", body: "Needs a look", path: "/t/p:e/01" })
  expect(await screen.findByText("Ticket paused")).toBeTruthy()
  expect(screen.getByText("Needs a look")).toBeTruthy()
})

test("Open navigates to a valid path", async () => {
  const navigate = vi.fn()
  const unset = setTapRouter({ navigate })
  const send = setup()
  send({ title: "Ticket paused", body: "b", path: "/t/p:e/01" })
  fireEvent.click(await screen.findByText("Open"))
  expect(navigate).toHaveBeenCalledWith("/t/p:e/01")
  unset()
})

test.each(["//x", "https://x", "", undefined])("path %j gives no Open action", async (path) => {
  const send = setup()
  send({ title: "Ticket paused", body: "b", path })
  expect(await screen.findByText("Ticket paused")).toBeTruthy()
  expect(screen.queryByText("Open")).toBeNull()
})

test("an event with no title shows nothing", () => {
  const send = setup()
  send({ body: "b", path: "/" })
  expect(toaster.getVisibleToasts()).toHaveLength(0)
})

test("another event type shows nothing", () => {
  const send = setup()
  send({ title: "t" }, "ticket")
  expect(toaster.getVisibleToasts()).toHaveLength(0)
})
