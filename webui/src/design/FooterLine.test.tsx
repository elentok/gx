import { act, cleanup, screen } from "@testing-library/react"
import { afterEach, expect, test } from "vitest"
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

const render = (
  s: SessionInfo,
  o: { wide?: boolean; ceiling?: number; modelOnly?: boolean } = {},
) =>
  renderThemed(
    <WideContext.Provider value={o.wide ?? true}>
      <FooterLine
        session={s}
        ceiling={"ceiling" in o ? o.ceiling : 200_000}
        modelOnly={o.modelOnly}
      />
    </WideContext.Provider>,
  )

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

test("each level maps to its own colour, and unknown ones are muted", () => {
  const tone = (level: string) => {
    render(session({ context_tokens: 1000, context_level: level }))
    const t = document.querySelector("[data-tone]")!.getAttribute("data-tone")
    cleanup()
    return t
  }
  const tones = ["green", "yellow", "orange", "red"].map(tone)
  expect(tones).toEqual(["done", "waiting", "paused", "deadlocked"])
  expect(tone("mauve")).toBe("muted")
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

test("model only: no badge even with a count", () => {
  render(session({ context_tokens: 70_000, context_level: "green" }), { modelOnly: true })
  expect(badge()).toBeNull()
  expect(screen.getByText("Claude Opus 4.7 · high")).toBeTruthy()
})

test("no ceiling, no badge", () => {
  render(session({ context_tokens: 70_000, context_level: "green" }), { ceiling: undefined })
  expect(badge()).toBeNull()
})
