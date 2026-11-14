import { cleanup, fireEvent, screen, within } from "@testing-library/react"
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

const render = (s: SessionInfo, o: { wide?: boolean; ceiling?: number } = {}) => {
  const ceiling = "ceiling" in o ? o.ceiling : 200_000
  return renderThemed(
    <WideContext.Provider value={o.wide ?? true}>
      <FooterLine session={{ ...s, context_ceiling: ceiling }} />
    </WideContext.Provider>,
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

test("the badge shows whole thousands and opens context details", async () => {
  render(session({ context_tokens: 70_400, context_level: "green" }))
  expect(badge()!.textContent).toBe("70K")
  fireEvent.click(badge()!)
  const details = await screen.findByRole("dialog", { name: "Context usage" })
  expect(details.textContent).toContain("Latest request input35%")
  expect(details.textContent).toContain("70Kof 200K smart zone")
  expect(details.textContent).not.toContain("headroom")
  expect(details.textContent).not.toContain("over smart zone")
  expect(details.textContent).not.toContain("Used")
})

test("the popover reports usage above the smart zone with zero remaining", async () => {
  render(session({ context_tokens: 185_375, context_level: "red" }), { ceiling: 130_000 })
  fireEvent.click(badge()!)
  const details = await screen.findByRole("dialog", { name: "Context usage" })
  expect(details.textContent).toContain("143%")
  expect(details.textContent).toContain("185Kof 130K smart zone")
  expect(details.textContent).toContain("Remaining to smart zone0")
  expect(details.textContent).not.toContain("headroom")
  expect(details.textContent).not.toContain("over smart zone")
})

test("the popover splits Claude context and shows cumulative usage", async () => {
  render(
    session({
      context_tokens: 70_000,
      context_level: "green",
      usage: {
        input_tokens: 5_000,
        cache_read_tokens: 60_000,
        cache_creation_tokens: 5_000,
        total_tokens: 1_840_000,
        estimated_cost: 4.72,
        compactions: 2,
      },
    }),
  )
  fireEvent.click(badge()!)
  const details = within(await screen.findByRole("dialog", { name: "Context usage" }))
  expect(details.getByText("Reused cache")).toBeTruthy()
  expect(details.getByText("60K")).toBeTruthy()
  expect(details.getByText("New input")).toBeTruthy()
  expect(details.getByText("Newly cached")).toBeTruthy()
  expect(details.getByText("1.8M")).toBeTruthy()
  expect(details.getByText("total tokens")).toBeTruthy()
  expect(details.getByText("≈$4.72")).toBeTruthy()
  expect(details.getByText("2 compactions")).toBeTruthy()
})

test("Codex total tokens render without an estimated cost", async () => {
  render(
    session({ context_tokens: 70_000, context_level: "green", usage: { total_tokens: 300_000 } }),
  )
  fireEvent.click(badge()!)
  const details = within(await screen.findByRole("dialog", { name: "Context usage" }))
  expect(details.getByText("300K")).toBeTruthy()
  expect(details.getByText("total tokens")).toBeTruthy()
  expect(details.queryByText(/\$/)).toBeNull()
})

test("Codex details distinguish context, cumulative subsets, and stale readings", async () => {
  render(
    session({
      context_tokens: 70_000,
      context_level: "green",
      usage: {
        total_tokens: 300_000,
        first_context: 20_000,
        peak_context: 90_000,
        model_context_window: 475_000,
        scope: "thread",
        context_stale: true,
        compactions: 1,
        totals: {
          input_tokens: 80_000,
          cache_read_tokens: 200_000,
          output_tokens: 20_000,
          reasoning_tokens: 10_000,
        },
      },
    }),
  )
  fireEvent.click(badge()!)
  const details = within(await screen.findByRole("dialog", { name: "Context usage" }))
  for (const label of [
    "First request",
    "Peak request",
    "Remaining to smart zone",
    "Reported model capacity",
    "This thread only",
    "Reasoning, included in output",
    "1 compaction",
  ]) {
    expect(details.getByText(label)).toBeTruthy()
  }
  expect(details.getByRole("status").textContent).toContain("Waiting for a new request reading")
  expect(details.queryByText("Cache writes")).toBeNull()
  expect(details.queryByText("Subagents")).toBeNull()
})

test("Claude details show main and subagent accounting including a reported zero", async () => {
  render(
    session({
      context_tokens: 70_000,
      context_level: "green",
      usage: {
        total_tokens: 300_000,
        scope: "conversation",
        main_tokens: 300_000,
        subagent_tokens: 0,
        totals: { input_tokens: 0, output_tokens: 50_000 },
      },
    }),
  )
  fireEvent.click(badge()!)
  const details = within(await screen.findByRole("dialog", { name: "Context usage" }))
  expect(details.getByText("Conversation, including subagents")).toBeTruthy()
  expect(details.getByText("Main agent")).toBeTruthy()
  expect(details.getByText("Subagents")).toBeTruthy()
  expect(details.getByText("Uncached input")).toBeTruthy()
  expect(details.queryByText("First request")).toBeNull()
  expect(details.queryByText("Reported model capacity")).toBeNull()
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
