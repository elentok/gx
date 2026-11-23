import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react"
import { afterEach, expect, test, vi } from "vitest"
import {
  DeliveryMaybeSent,
  DeliverySendFailed,
  DeliveryUnknownCommand,
  type SessionInfo,
  type SessionRequest,
  type SessionResult,
} from "../generated/server"
import { renderThemed } from "../ui/render"
import { DeliveryCard, isDeliveryCard, tailCard } from "./DeliveryCard"
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

const unknown = (state: string) => {
  const s = session(DeliveryUnknownCommand)
  s.status = { state } as SessionInfo["status"]
  s.run_started_at = "t1"
  return s
}

function openUnknown(
  state = "idle",
  over: Partial<Record<"resend" | "stop" | "resume", Post>> = {},
) {
  const calls: string[] = []
  const fake = (name: "resend" | "stop" | "resume") =>
    vi.fn<Post>(async (b) => {
      calls.push(name)
      return (over[name] ?? (async () => ({})))(b)
    })
  const resend = fake("resend")
  const stop = fake("stop")
  const resume = fake("resume")
  renderThemed(
    <DeliveryCard session={unknown(state)} resend={resend} stop={stop} resume={resume} />,
  )
  return { resend, stop, resume, calls }
}

type Post = (b: SessionRequest) => Promise<SessionResult>

test("the unknown-command card lists Resend, Restart agent, Show screen, Dismiss in order", () => {
  openUnknown()
  const card = screen.getByRole("region", { name: "Send failed" })
  expect(card.textContent).toContain("the card reason")
  const names = screen.getAllByRole("button").map((b) => b.textContent)
  expect(names).toEqual(["Copy", "Resend", "Restart agent", "Show screen", "Dismiss"])
})

test("isDeliveryCard is true for the unknown-command kind", () => {
  expect(isDeliveryCard(session(DeliveryUnknownCommand))).toBe(true)
})

test.each([DeliveryMaybeSent, DeliverySendFailed])(
  "the %s card has no Resend or Restart agent",
  (kind) => {
    open(kind)
    expect(screen.queryByRole("button", { name: "Resend" })).toBeNull()
    expect(screen.queryByRole("button", { name: "Restart agent" })).toBeNull()
  },
)

test("Resend posts the id", async () => {
  const { resend } = openUnknown("idle")
  fireEvent.click(screen.getByRole("button", { name: "Resend" }))
  await waitFor(() => expect(resend).toHaveBeenCalledWith({ id: "s-a" }))
})

test.each([
  ["idle", false],
  ["exited", true],
  ["starting", true],
  ["working", true],
])("Resend for a %s session is disabled: %s", (state, disabled) => {
  openUnknown(state)
  expect((screen.getByRole("button", { name: "Resend" }) as HTMLButtonElement).disabled).toBe(
    disabled,
  )
})

test("while Resend is pending only Resend is disabled", async () => {
  const { resend } = openUnknown("idle", { resend: () => new Promise(() => {}) })
  fireEvent.click(screen.getByRole("button", { name: "Resend" }))
  await waitFor(() => expect(resend).toHaveBeenCalled())
  const disabled = screen
    .getAllByRole("button")
    .filter((b) => (b as HTMLButtonElement).disabled)
    .map((b) => b.textContent)
  expect(disabled).toEqual(["Resend"])
})

test("tailCard picks blocking, delivery, stopped or null", () => {
  const of = (actionable: object) => ({ ...session(), actionable }) as SessionInfo
  expect(tailCard(of({ gx_raised: true, reason: "r", blocking: true }))).toBe("blocking")
  expect(tailCard(session(DeliverySendFailed))).toBe("delivery")
  expect(tailCard(of({ gx_raised: true, reason: "r", kind: "toString" }))).toBe("stopped")
  expect(tailCard(of({}))).toBeNull()
})

test("a refused Resend shows its message in an alert", async () => {
  openUnknown("idle", { resend: async () => ({ refused: true, message: "Session is exited." }) })
  fireEvent.click(screen.getByRole("button", { name: "Resend" }))
  expect((await screen.findByRole("alert")).textContent).toContain("Session is exited.")
})

test("Restart agent calls nothing before the confirm, and Keep calls nothing", () => {
  const { stop, resume } = openUnknown()
  fireEvent.click(screen.getByRole("button", { name: "Restart agent" }))
  expect(screen.getByRole("button", { name: "Keep" })).toBeTruthy()
  fireEvent.click(screen.getByRole("button", { name: "Keep" }))
  expect(stop).not.toHaveBeenCalled()
  expect(resume).not.toHaveBeenCalled()
  expect(screen.getByRole("button", { name: "Restart agent" })).toBeTruthy()
})

test("Restart stops with the row token, then resumes", async () => {
  const { stop, resume, calls } = openUnknown()
  fireEvent.click(screen.getByRole("button", { name: "Restart agent" }))
  fireEvent.click(screen.getByRole("button", { name: "Restart" }))
  await waitFor(() => expect(resume).toHaveBeenCalledWith({ id: "s-a" }))
  expect(stop).toHaveBeenCalledWith({ id: "s-a", state: "s-a:t1:live" })
  expect(calls).toEqual(["stop", "resume"])
})

test("a refused stop skips the resume and shows the message", async () => {
  const { resume } = openUnknown("idle", {
    stop: async () => ({ refused: true, message: "Cannot stop." }),
  })
  fireEvent.click(screen.getByRole("button", { name: "Restart agent" }))
  fireEvent.click(screen.getByRole("button", { name: "Restart" }))
  expect((await screen.findByRole("alert")).textContent).toContain("Cannot stop.")
  expect(resume).not.toHaveBeenCalled()
})

test("the card still renders for an Exited session", () => {
  openUnknown("exited")
  expect(screen.getByRole("region", { name: "Send failed" })).toBeTruthy()
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
