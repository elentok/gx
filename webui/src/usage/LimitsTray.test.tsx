import { act, cleanup, fireEvent, screen } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"
import type { Connection } from "../connection"
import { UsageModeLimits, UsageModeSpending, type UsageSummary } from "../generated/server"
import { renderThemed } from "../ui/render"
import { barFill, LimitsTray } from "./LimitsTray"
import type { FetchSummary } from "./useUsageSummary"

afterEach(cleanup)

const NOW = Date.now()
const win = (used: number) => ({
  used_percentage: used,
  resets_at: NOW / 1000 + 3600,
  received_at: NOW / 1000,
})
const limits = (fiveHour?: number) => ({
  five_hour: fiveHour === undefined ? undefined : win(fiveHour),
})
const summary = (mode: string, claude?: number, codex?: number): UsageSummary =>
  ({
    mode,
    limits: limits(claude),
    codex: codex === undefined ? undefined : limits(codex),
    spend: { today: 4.82, since_sunday: 9, since_first: 20 },
  }) as unknown as UsageSummary

const conn = { onEvent: () => () => {} } as unknown as Connection

async function setup(fetcher: FetchSummary) {
  renderThemed(<LimitsTray conn={conn} fetcher={fetcher} />)
  await act(async () => {})
}

const bar = (letter: string) => document.querySelector(`[data-meter=${letter}]`)!

describe("barFill", () => {
  it.each([
    [79, "done"],
    [80, "waiting"],
    [94.4, "waiting"],
    [100, "deadlocked"],
  ])("%i%% is %s", (pct, tone) => {
    expect(barFill(limits(pct) as never, NOW).tone).toBe(tone)
  })

  it("keeps a sliver at 0% and when the window is missing", () => {
    expect(barFill(limits(0) as never, NOW).percent).toBeGreaterThan(0)
    expect(barFill(undefined, NOW).percent).toBeGreaterThan(0)
  })
})

describe("LimitsTray", () => {
  it("renders nothing until a fetch succeeds", async () => {
    await setup(async () => {
      throw new Error("down")
    })
    expect(screen.queryByRole("button", { name: "Usage limits" })).toBeNull()
  })

  it("shows C and X bars that follow each provider's usage", async () => {
    await setup(async () => summary(UsageModeLimits, 34, 85))
    expect(bar("C").textContent).toBe("C")
    expect(bar("X").textContent).toBe("X")
    expect(bar("C").getAttribute("data-tone")).toBe("done")
    expect(bar("X").getAttribute("data-tone")).toBe("waiting")
  })

  it("opens a popover with the limits and mounts no popover body while closed", async () => {
    await setup(async () => summary(UsageModeLimits, 34, 85))
    expect(screen.queryByText("Claude limits")).toBeNull()
    fireEvent.click(screen.getByRole("button", { name: "Usage limits" }))
    expect(await screen.findByText("Claude limits")).toBeTruthy()
    expect(screen.getByText("Codex limits")).toBeTruthy()
  })

  it("in spending mode the bars show limits and the popover shows spend", async () => {
    const fetcher = vi.fn<FetchSummary>(async () => summary(UsageModeSpending, 90))
    await setup(fetcher)
    expect(bar("C").getAttribute("data-tone")).toBe("waiting")
    fireEvent.click(screen.getByRole("button", { name: "Usage limits" }))
    expect(await screen.findByText(/GX spend \(est\.\)/)).toBeTruthy()
  })
})
