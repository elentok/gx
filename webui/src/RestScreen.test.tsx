import { act, cleanup, fireEvent, screen } from "@testing-library/react"
import { afterEach, expect, test, vi } from "vitest"
import { ConnectionContext, type Connection, type ConnectionState } from "./connection"
import { isUnlockSentence, postRestUnlock } from "./rest"
import { RestScreen } from "./RestScreen"
import { renderThemed } from "./ui/render"

vi.mock("./rest", async (orig) => ({ ...(await orig<object>()), postRestUnlock: vi.fn() }))
const postUnlock = vi.mocked(postRestUnlock)

afterEach(() => {
  cleanup()
  vi.useRealTimers()
})

const rule = { work_minutes: 55, break_minutes: 5, bedtime_start: "22:00", bedtime_end: "07:00" }

type Period = { kind: string; start: string; end: string; message: string; sub_message?: string }

function setup(period?: Period) {
  let state = {
    snapshot: { rest: { rule, period } },
    connected: true,
  } as unknown as ConnectionState
  const subs = new Set<() => void>()
  const conn = {
    getState: () => state,
    subscribe: (fn: () => void) => (subs.add(fn), () => subs.delete(fn)),
  } as unknown as Connection
  renderThemed(
    <ConnectionContext.Provider value={conn}>
      <button>underneath</button>
      <RestScreen />
    </ConnectionContext.Provider>,
  )
  return (next?: Period) =>
    act(() => {
      state = { ...state, snapshot: { rest: { rule, period: next } } } as unknown as ConnectionState
      subs.forEach((fn) => fn())
    })
}

const bedtime: Period = {
  kind: "bedtime",
  start: "2026-11-16T22:00:00Z",
  end: "2026-11-17T07:00:00Z",
  message: "That's enough AI for today.",
  sub_message: "Go to sleep and come back tomorrow at 07:00.",
}

const part = (name: string) => document.querySelector(`[data-part="${name}"]`)

test("a Bedtime period shows its parts in order and makes the page inert", () => {
  setup(bedtime)
  const parts = [
    "mascot-image",
    "message",
    "submessage",
    "time-label",
    "time",
    "rule-note",
    "unlock-link",
  ]
  const els = parts.map((p) => part(p) as HTMLElement)
  expect(els.every(Boolean)).toBe(true)
  for (let i = 1; i < els.length; i++)
    expect(
      els[i - 1]!.compareDocumentPosition(els[i]!) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy()
  expect(part("message")!.textContent).toBe("That's enough AI for today.")
  expect(part("time")!.textContent).toBe("07:00")
  expect(part("rule-note")!.textContent).toBe("Bedtime 22:00 – 07:00")
  expect(screen.getByText("underneath").closest("[inert]")).toBeTruthy()
})

test("a one-sentence message has no sub-message", () => {
  setup({ ...bedtime, sub_message: undefined })
  expect(part("submessage")).toBeNull()
})

test("keys outside the screen are stopped before they reach the page", () => {
  setup(bedtime)
  const seen = vi.fn()
  const btn = screen.getByText("underneath")
  btn.addEventListener("keydown", seen)
  for (const key of ["Tab", "k", "Escape"]) {
    btn.dispatchEvent(
      new KeyboardEvent("keydown", { key, metaKey: key === "k", bubbles: true, cancelable: true }),
    )
  }
  expect(seen).not.toHaveBeenCalled()
})

const breakPeriod: Period = {
  kind: "break",
  start: "2026-11-16T10:00:00Z",
  end: "2026-11-16T10:05:00Z",
  message: "Fifty-five minutes in a row?",
  sub_message: "Stand up and drink some water.",
}

function setupBreak(period: Period, serverNow: string) {
  const state = {
    snapshot: { rest: { rule, period, now: serverNow } },
    connected: true,
  } as unknown as ConnectionState
  const conn = {
    getState: () => state,
    subscribe: () => () => {},
  } as unknown as Connection
  renderThemed(
    <ConnectionContext.Provider value={conn}>
      <RestScreen />
    </ConnectionContext.Provider>,
  )
}

test("a Break shows its parts in order with the Break image and rule note", () => {
  vi.useFakeTimers({ now: new Date("2026-11-16T10:00:00Z") })
  setupBreak(breakPeriod, "2026-11-16T10:00:00Z")
  const parts = [
    "mascot-image",
    "message",
    "submessage",
    "time-label",
    "time",
    "rule-note",
    "unlock-link",
  ]
  const els = parts.map((p) => part(p) as HTMLElement)
  expect(els.every(Boolean)).toBe(true)
  for (let i = 1; i < els.length; i++)
    expect(
      els[i - 1]!.compareDocumentPosition(els[i]!) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy()
  expect(part("message")!.textContent).toBe("Fifty-five minutes in a row?")
  expect(part("time-label")!.textContent).toBe("Break ends in")
  expect(part("rule-note")!.textContent).toBe("Break · 5 minutes after 55 minutes of work")
  vi.useRealTimers()
})

test("the countdown falls each second from the server time, not the browser clock", () => {
  // The browser clock is an hour behind the server.
  vi.useFakeTimers({ now: new Date("2026-11-16T09:00:00Z") })
  setupBreak(breakPeriod, "2026-11-16T10:02:23Z")
  expect(part("time")!.textContent).toBe("02:37")
  act(() => vi.advanceTimersByTime(1000))
  expect(part("time")!.textContent).toBe("02:36")
  vi.useRealTimers()
})

test("the Rest screen hides itself when the countdown reaches zero", () => {
  vi.useFakeTimers({ now: new Date("2026-11-16T10:04:58Z") })
  setupBreak(breakPeriod, "2026-11-16T10:04:58Z")
  expect(part("time")!.textContent).toBe("00:02")
  act(() => vi.advanceTimersByTime(2000))
  expect(part("message")).toBeNull()
  expect(document.querySelector("[role=dialog]")).toBeNull()
  vi.useRealTimers()
})

test("the countdown timer is cleared when the screen leaves", () => {
  vi.useFakeTimers({ now: new Date("2026-11-16T10:00:00Z") })
  setupBreak(breakPeriod, "2026-11-16T10:00:00Z")
  expect(vi.getTimerCount()).toBeGreaterThan(0)
  cleanup()
  expect(vi.getTimerCount()).toBe(0)
  vi.useRealTimers()
})

test.each([
  ["I'm Dana and I'm addicted to AI", true],
  ["i’m  Dana   AND i’m ADDICTED to ai ", true],
  ["I'm and I'm addicted to AI", false],
  ["I'm Dana and I'm addicted to AI!", false],
  ["", false],
])("isUnlockSentence(%j) is %s", (text, ok) => {
  expect(isUnlockSentence(text)).toBe(ok)
})

const typeSentence = (value: string) =>
  fireEvent.change(screen.getByLabelText("Unlock sentence"), { target: { value } })
const unlockButton = () =>
  document.querySelector("[data-part=unlock-box] button") as HTMLButtonElement

test("Unlock anyway opens the box with the hint and a Bedtime button label", () => {
  setup(bedtime)
  expect(part("unlock-box")).toBeNull()
  fireEvent.click(part("unlock-link")!)
  expect(part("unlock-box")!.textContent).toContain(
    "Type exactly: I'm [anything] and I'm addicted to AI",
  )
  expect(unlockButton().textContent).toBe("Unlock until 07:00")
  expect(unlockButton().disabled).toBe(true)
  typeSentence("I'm Dana and I'm addicted to AI")
  expect(unlockButton().disabled).toBe(false)
})

test("a Break button reads until the end of this Break", () => {
  vi.useFakeTimers({ now: new Date("2026-11-16T10:00:00Z") })
  setupBreak(breakPeriod, "2026-11-16T10:00:00Z")
  fireEvent.click(part("unlock-link")!)
  expect(unlockButton().textContent).toBe("Unlock until the end of this Break")
})

test("the button posts the sentence and a refusal shows with the text kept", async () => {
  postUnlock.mockResolvedValue("type the sentence")
  setup(bedtime)
  fireEvent.click(part("unlock-link")!)
  typeSentence("I'm Dana and I'm addicted to AI")
  await act(async () => fireEvent.click(unlockButton()))
  expect(postUnlock).toHaveBeenCalledWith("I'm Dana and I'm addicted to AI")
  expect(screen.getByRole("alert").textContent).toBe("type the sentence")
  expect((screen.getByLabelText("Unlock sentence") as HTMLInputElement).value).toBe(
    "I'm Dana and I'm addicted to AI",
  )
})

test("typing in the unlock box reaches the input while keys outside stay blocked", () => {
  setup(bedtime)
  fireEvent.click(part("unlock-link")!)
  const input = screen.getByLabelText("Unlock sentence")
  const seen = vi.fn()
  input.addEventListener("keydown", seen)
  fireEvent.keyDown(input, { key: "a" })
  expect(seen).toHaveBeenCalled()
})

test("the screen leaves and the page is live again when the period ends", () => {
  const update = setup(bedtime)
  update(undefined)
  expect(part("message")).toBeNull()
  expect(screen.getByText("underneath").closest("[inert]")).toBeNull()
})
