import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react"
import { afterEach, expect, test, vi } from "vitest"
import {
  DeliveryMaybeSent,
  DeliverySendFailed,
  type SessionInfo,
  type SessionRequest,
  type SessionResult,
} from "../generated/server"
import { renderThemed } from "../ui/render"
import { DeliveryCard, isDeliveryCard } from "./DeliveryCard"
import type { FetchScreen } from "./screen"

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})

const TEXT = "  /model opus  "

const session = (kind?: string): SessionInfo =>
  ({
    id: "s-a",
    status: { state: "idle" },
    unread: {},
    actionable: { gx_raised: true, reason: "the card reason", kind, copy_text: TEXT },
  }) as SessionInfo

function open(kind: string, dismiss?: (b: SessionRequest) => Promise<SessionResult>) {
  const post = vi.fn(dismiss ?? (async () => ({})))
  const onDone = vi.fn()
  const fetchScreen = vi.fn<FetchScreen>(async () => ({
    kind: "ok",
    detail: { rows: ["SCREEN BODY"] },
  }))
  renderThemed(
    <DeliveryCard
      session={session(kind)}
      dismiss={post}
      onDone={onDone}
      fetchScreen={fetchScreen}
    />,
  )
  return { post, onDone, fetchScreen }
}

test.each([
  [DeliveryMaybeSent, "May not have been sent"],
  [DeliverySendFailed, "Send failed"],
])("the %s card shows its title, reason, text and two buttons", (kind, title) => {
  open(kind)
  const card = screen.getByRole("region", { name: title })
  expect(card.textContent).toContain("the card reason")
  expect(card.textContent).toContain(TEXT)
  const names = screen.getAllByRole("button").map((b) => b.textContent)
  expect(names).toEqual(["Copy", "Show screen", "Dismiss"])
})

test("isDeliveryCard is true for the two kinds with a reason only", () => {
  expect(isDeliveryCard(session(DeliveryMaybeSent))).toBe(true)
  expect(isDeliveryCard(session(DeliverySendFailed))).toBe(true)
  expect(isDeliveryCard(session("queue"))).toBe(false)
  expect(isDeliveryCard(session())).toBe(false)
})

test("Copy writes the text as sent to the clipboard", async () => {
  const writeText = vi.fn(async () => {})
  vi.stubGlobal("navigator", { clipboard: { writeText } })
  open(DeliveryMaybeSent)
  fireEvent.click(screen.getByRole("button", { name: "Copy" }))
  await waitFor(() => expect(writeText).toHaveBeenCalledWith(TEXT))
})

test("Show screen opens the screen modal", async () => {
  const { fetchScreen } = open(DeliverySendFailed)
  fireEvent.click(screen.getByRole("button", { name: "Show screen" }))
  await waitFor(() => expect(fetchScreen).toHaveBeenCalledWith("s-a"))
})

test("Dismiss posts the id and then calls onDone", async () => {
  const { post, onDone } = open(DeliveryMaybeSent)
  fireEvent.click(screen.getByRole("button", { name: "Dismiss" }))
  await waitFor(() => expect(onDone).toHaveBeenCalled())
  expect(post).toHaveBeenCalledWith({ id: "s-a" })
})

test("a refused Dismiss shows its message in an alert", async () => {
  const { onDone } = open(DeliveryMaybeSent, async () => ({
    refused: true,
    reason: "not-delivery-card",
    message: "No card to dismiss.",
  }))
  fireEvent.click(screen.getByRole("button", { name: "Dismiss" }))
  expect((await screen.findByRole("alert")).textContent).toContain("No card to dismiss.")
  expect(onDone).not.toHaveBeenCalled()
})
