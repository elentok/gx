import { fireEvent, waitFor, within } from "@testing-library/react"
import { describe, expect, it } from "vitest"
import type { UsageSpend, UsageSummary } from "../generated/server"
import { UsageModeSpending } from "../generated/server"
import { renderThemed } from "../ui/render"
import { SpendBlock, spendCaveat } from "./SpendBlock"

const show = (spend: UsageSpend) =>
  renderThemed(
    <SpendBlock summary={{ mode: UsageModeSpending, limits: {}, spend } as UsageSummary} />,
  )

const normal: UsageSpend = {
  today: 4.82,
  since_sunday: 21.5,
  since_first: 96.1,
  first_day: "2026-09-01",
}
const short: UsageSpend = {
  today: 1.2,
  since_sunday: 3.4,
  since_sunday_short: true,
  since_first: 3.4,
  since_first_short: true,
  first_day: "2026-10-30",
}
const empty: UsageSpend = { today: 0, since_sunday: 0, since_first: 0 }

const openInfo = async (v: ReturnType<typeof show>) => {
  fireEvent.click(v.getByRole("button", { name: "About gx spend" }))
  await v.findByText(spendCaveat)
}

describe("SpendBlock", () => {
  it("normal: one line with today's amount; the other periods sit in the info popover", async () => {
    const v = show(normal)
    expect(v.getByRole("group", { name: "Today" }).textContent).toBe(
      "GX spend (est.) · Today $4.82",
    )
    expect(v.queryByRole("group", { name: "Since Sunday" })).toBeNull()
    await openInfo(v)
    expect(v.getByRole("group", { name: "Since Sunday" }).textContent).toBe("Since Sunday$21.50")
    expect(v.getByRole("group", { name: "Since the 1st" }).textContent).toBe("Since the 1st$96.10")
    expect(v.queryByText(/^since [A-Z]/)).toBeNull()
  })

  it("short ledger: notes on flagged rows only, never Today", async () => {
    const v = show(short)
    expect(v.getByRole("group", { name: "Today" }).textContent).not.toMatch(/since/)
    await openInfo(v)
    expect(
      within(v.getByRole("group", { name: "Since Sunday" })).getByText("since Fri, Oct 30"),
    ).toBeTruthy()
    expect(
      within(v.getByRole("group", { name: "Since the 1st" })).getByText("since Fri, Oct 30"),
    ).toBeTruthy()
  })

  it("flags drive the notes row by row", async () => {
    const v = show({ ...normal, since_first_short: true, first_day: "2026-10-30" })
    await openInfo(v)
    expect(within(v.getByRole("group", { name: "Since Sunday" })).queryByText(/since/)).toBeNull()
    expect(
      within(v.getByRole("group", { name: "Since the 1st" })).getByText("since Fri, Oct 30"),
    ).toBeTruthy()
  })

  it("empty: $0.00 for every period and no notes", async () => {
    const v = show(empty)
    await openInfo(v)
    expect(v.getAllByText(/\$0\.00/)).toHaveLength(3)
    expect(v.queryByText(/^since [A-Z]/)).toBeNull()
  })

  it("tooltip holds the approved sentence and opens on focus and click", async () => {
    expect(spendCaveat).toBe(
      "Estimated cost of Claude runs started by gx. Not the bill. Codex runs and Claude sessions outside gx are not counted.",
    )
    const v = show(normal)
    const marker = v.getByRole("button", { name: "About gx spend" })
    fireEvent.focus(marker)
    expect(await v.findByText(spendCaveat)).toBeTruthy()
    fireEvent.blur(marker)
    await waitFor(() => expect(v.queryByText(spendCaveat)).toBeNull())
    fireEvent.click(marker)
    expect(await v.findByText(spendCaveat)).toBeTruthy()
  })

  it("shows no budget limit words", () => {
    const v = show(short)
    expect(v.container.textContent).not.toMatch(/budget|limit|soft|hard|over|remaining/i)
  })
})
