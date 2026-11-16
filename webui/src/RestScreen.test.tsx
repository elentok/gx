import { act, cleanup, screen } from "@testing-library/react"
import { afterEach, expect, test, vi } from "vitest"
import { ConnectionContext, type Connection, type ConnectionState } from "./connection"
import { RestScreen } from "./RestScreen"
import { renderThemed } from "./ui/render"

afterEach(cleanup)

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

test("the screen leaves and the page is live again when the period ends", () => {
  const update = setup(bedtime)
  update(undefined)
  expect(part("message")).toBeNull()
  expect(screen.getByText("underneath").closest("[inert]")).toBeNull()
})
