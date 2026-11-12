import { act, cleanup, screen } from "@testing-library/react"
import { afterEach, expect, test } from "vitest"
import { ConnectionContext, type Connection, type ConnectionState } from "../connection"
import type { SessionInfo } from "../generated/server"
import { renderThemed } from "../ui/render"
import { WideContext } from "../useWide"
import { FooterLine } from "./FooterLine"

afterEach(cleanup)

const session = (over: Partial<SessionInfo> = {}): SessionInfo =>
  ({
    id: "s-a",
    harness: "claude",
    model: "claude-opus-4-7",
    effort: "high",
    status: { state: "idle" },
    ...over,
  }) as SessionInfo

const render = (s: SessionInfo, o: { wide?: boolean; ceiling?: number } = {}) => {
  const state = {
    snapshot: { context_ceiling: "ceiling" in o ? o.ceiling : 200_000 },
    connected: true,
  } as unknown as ConnectionState
  const conn = { getState: () => state, subscribe: () => () => {} } as unknown as Connection
  return renderThemed(
    <ConnectionContext.Provider value={conn}>
      <WideContext.Provider value={o.wide ?? true}>
        <FooterLine session={s} />
      </WideContext.Provider>
    </ConnectionContext.Provider>,
  )
}

const badge = () => screen.queryByRole("button", { name: "Context" })
const fill = () => Number(screen.getByTestId("context-ring").getAttribute("data-fill"))

test("shows the model and effort, with no badge before the first reply", () => {
  render(session())
  expect(screen.getByText("Claude Opus 4.7 · high")).toBeTruthy()
  expect(badge()).toBeNull()
})

test("a phone drops the effort", () => {
  render(session(), { wide: false })
  expect(screen.getByText("Claude Opus 4.7")).toBeTruthy()
})

test("the badge shows whole thousands and a tooltip against the ceiling on focus", async () => {
  render(session({ context_tokens: 70_400, context_level: "green" }))
  expect(badge()!.textContent).toBe("70K")
  await act(async () => badge()!.focus())
  expect(await screen.findByText("70K of 200K smart zone")).toBeTruthy()
})

test("each level maps to its own colour", () => {
  const tone = (level: "green" | "yellow" | "orange" | "red") => {
    render(session({ context_tokens: 1000, context_level: level }))
    const t = document.querySelector("[data-tone]")!.getAttribute("data-tone")
    cleanup()
    return t
  }
  const tones = (["green", "yellow", "orange", "red"] as const).map(tone)
  expect(tones).toEqual(["done", "waiting", "paused", "deadlocked"])
})

test("the ring fills with the count and is full at or above the ceiling", () => {
  render(session({ context_tokens: 50_000, context_level: "green" }))
  const quarter = fill()
  cleanup()
  render(session({ context_tokens: 100_000, context_level: "green" }))
  expect(fill()).toBeGreaterThan(quarter)
  cleanup()
  render(session({ context_tokens: 250_000, context_level: "red" }))
  expect(fill()).toBe(1)
})

test("no ceiling, no badge", () => {
  render(session({ context_tokens: 70_000, context_level: "green" }), { ceiling: undefined })
  expect(badge()).toBeNull()
})
