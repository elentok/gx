import { appendFile, mkdir } from "node:fs/promises"
import { dirname } from "node:path"
import type { Locator, Page } from "@playwright/test"
import { expect, nav, renderedText, test } from "./fixture"

// Seeded totals are in the grilling-stats comment in testutil/gxfixture/scenarios.go.
const grilling = (page: import("@playwright/test").Page) =>
  page.getByRole("region", nav("Grilling"))
const statCard = (page: import("@playwright/test").Page, title: string) =>
  grilling(page).locator("[data-tone]").filter({ hasText: title })

// The limit-burn `stats` shape is relative to today: day 0 of a 30-day range is 29 days ago, history starts
// at day 7, the weekly windows reset at days 12, 19 and 26, the 5-hour windows at 15 and 29. The ledger has
// stamp-only days 0-8 (day 4 an unstamped ticket), rebuilt days 9-18, live days 19-29 (day 24 quiet).
const costCard = (page: Page, title: RegExp) =>
  page.getByRole("region", nav("Cost and effort")).locator("[data-tone]").filter({ hasText: title })
const LIMIT_CARDS = [
  /^Claude weekly burn/,
  /^Codex weekly burn/,
  /^Claude 5-hour burn/,
  /^Codex 5-hour burn/,
]

async function tip(card: Locator, day: number): Promise<string> {
  await card.locator(`[data-hit="${day}"]`).hover({ force: true })
  return renderedText(card.getByRole("tooltip"))
}

// Moved to vitest: 57 (Grilling section, StatsPage.test.tsx), 182 (limit burn, LimitBurnCards.test.tsx and
// the Go shaping tests), 183 (tokens and spend by group, StatsPage.test.tsx). Also covers 182 phone.
test("184. Live token ledger (and 182. Stats limit burn, phone)", async ({ page, start }) => {
  test.setTimeout(120_000)
  const fixture = await start("limit-burn", { shape: "live", reloads: 1 })
  const groups = { queue: "Queue", design: "Design", one_off: "One-off", other: "Other" }
  const append = async (tokens: number) => {
    for (const path of Object.values(fixture.transcripts)) {
      await mkdir(dirname(path), { recursive: true })
      const line = {
        type: "assistant",
        timestamp: new Date().toISOString(),
        message: { model: "claude-sonnet-4-5", usage: { input_tokens: tokens, output_tokens: 0 } },
      }
      await appendFile(path, `${JSON.stringify(line)}\n`)
    }
  }
  // Today's tokens per group from the CLI, the same numbers the page reads.
  const today = async () => {
    const stats = JSON.parse(await fixture.cli("server", "stats", "--json", "--range", "7"))
    return Object.fromEntries(
      stats.cost.groups.map((g: { group: string; tokens: { value: number }[] }) => [
        g.group,
        g.tokens[g.tokens.length - 1].value,
      ]),
    ) as Record<string, number>
  }
  const waitFor = async (want: (t: Record<string, number>) => boolean) => {
    await expect.poll(async () => want(await today()), { timeout: 60_000 }).toBe(true)
    return today()
  }
  expect(Object.keys(fixture.transcripts).sort()).toEqual(Object.keys(groups).sort())

  // The first poll that sees a transcript only stores its total, so a baseline line goes in first.
  await append(1000)
  await new Promise((r) => setTimeout(r, 3000))
  const before = await today()

  await append(250_000)
  const afterFirst = await waitFor((t) =>
    Object.values(groups).every((g) => t[g] - before[g] === 250_000),
  )

  // Navigate by hash: the one reload this flow declares comes later, before the Web performance counts.
  await page.evaluate(() => (location.hash = "#/stats?range=30"))
  const tokens = costCard(page, /^Tokens/)
  await tokens.waitFor()
  const check = async (totals: Record<string, number>) => {
    const tooltip = await tip(tokens, 29)
    for (const g of Object.values(groups))
      expect(tooltip).toMatch(new RegExp(`${g}: ${totals[g]}(?!\\d)`))
  }
  await check(afterFirst)

  // Only the delta is added.
  await append(100_000)
  const afterSecond = await waitFor((t) =>
    Object.values(groups).every((g) => t[g] - afterFirst[g] === 100_000),
  )
  await page.evaluate(() => (location.hash = "#/"))
  await page.evaluate(() => (location.hash = "#/stats?range=30"))
  await tokens.waitFor()
  // The pointer still rests on the old bar; move it off so the new bar sees a fresh hover.
  await page.mouse.move(0, 0)
  await check(afterSecond)

  // The limit-burn scenario seeds finished launches: a median, a share under target and per-step medians, no "—".
  const launch = page.getByRole("region", nav("Launch"))
  const launchCard = launch.locator("[data-tone]").filter({ hasText: "Launch time" })
  await expect(launchCard).toContainText(/\d+(\.\d+)?s/)
  await expect(launchCard).toContainText(/\d+% under/)
  await expect(launchCard).not.toContainText("—")
  const steps = launch.getByTestId("launch-steps")
  await expect(steps).toContainText("worktree")
  await expect(steps).not.toContainText("—")

  // Web performance: the browser sends its own samples when the page is hidden. Run this first, a section
  // with no samples shows only the empty state, with no tabs or cards.
  const perfCounts = async () => {
    const stats = JSON.parse(
      await fixture.cli("server", "stats", "--json", "--range", "7", "--include-dev"),
    )
    const wp = stats.web_performance
    const by = (cards: { metric: string; count: number }[], m: string) =>
      cards.find((c) => c.metric === m)?.count ?? 0
    return [
      by(wp.hard_loads, "lcp"),
      by(wp.hard_loads, "fcp"),
      by(wp.hard_loads, "cls"),
      by(wp.hard_loads, "app_ready"),
      by(wp.hard_loads, "inp") + by(wp.route_changes, "inp"),
    ]
  }
  // The fixture's first load lands on "/", which redirects to /epics and uses up the hard span, so no app_ready is
  // ever queued. Reload on the stats route (the one declared reload) to get a real hard load.
  await page.evaluate(() => (location.hash = "#/stats?range=30"))
  await page.reload()
  await expect(page.getByRole("region", nav("Web performance"))).toBeVisible()
  const perfBefore = await perfCounts()
  const perfNames = ["lcp", "fcp", "cls", "app_ready", "inp"]
  const describeCounts = (counts: number[]) =>
    perfNames.map((n, i) => `${n}=${counts[i]}`).join(" ")
  // Hide by hand: the queue flush and web-vitals both listen for visibilitychange on document.
  const setHidden = (hidden: boolean) =>
    page.evaluate((h) => {
      Object.defineProperty(document, "visibilityState", {
        value: h ? "hidden" : "visible",
        configurable: true,
      })
      Object.defineProperty(document, "hidden", { value: h, configurable: true })
      document.dispatchEvent(new Event("visibilitychange"))
    }, hidden)
  // The vitals observers start at idle, after the page loads. A click before that makes no INP sample. FCP is
  // reported when they start and the queue sends it when idle, so an FCP count that rises with the page still
  // visible proves they are running. Hiding first would make web-vitals ignore FCP.
  await expect
    .poll(async () => (await perfCounts())[1] > perfBefore[1], { timeout: 60_000 })
    .toBe(true)
  // An element with a gx action name gives INP a target.
  await page.locator("[data-gx-action]:visible").first().click()
  // LCP and INP entries reach the page's observers after a rendered frame; under load the hide can otherwise
  // run first and the sample is never queued. Two frames after the click settle both.
  await page.evaluate(
    () => new Promise<void>((r) => requestAnimationFrame(() => requestAnimationFrame(() => r()))),
  )
  await setHidden(true)
  await expect
    .poll(
      async () => {
        const after = await perfCounts()
        const flat = perfNames.filter((_, i) => after[i] <= perfBefore[i])
        return flat.length === 0
          ? "all rose"
          : `flat: ${flat.join(", ")}; before: ${describeCounts(perfBefore)}; after: ${describeCounts(after)}`
      },
      { timeout: 60_000 },
    )
    .toBe("all rose")
  await setHidden(false)

  await page.evaluate(() => (location.hash = "#/stats?range=30"))
  const perf = page.getByRole("region", nav("Web performance"))
  await expect(perf).toBeVisible()
  const launchBox = (await launch.boundingBox())!
  const perfBox = (await perf.boundingBox())!
  expect(perfBox.y).toBeGreaterThan(launchBox.y)

  const dev = perf.getByRole("checkbox", { name: "Include dev" })
  await expect(dev).not.toBeChecked()
  // The box is driven by the URL and the router commits it after the click, so check()/uncheck() would read it
  // while it still shows the old state. Click, then assert with a retrying expect.
  await dev.click()
  await expect(dev).toBeChecked()
  await expect(page).toHaveURL(/[?&]dev=1/)
  await dev.click()
  await expect(dev).not.toBeChecked()
  await expect(page).not.toHaveURL(/dev=1/)

  const card = (m: string) => perf.getByTestId(`perf-card-${m}`)
  await perf.getByRole("button", { name: "Route changes" }).click()
  await expect(page).toHaveURL(/[?&]perf=route/)
  for (const m of ["route_ready", "inp", "cls"]) await expect(card(m)).toBeVisible()
  await expect(card("lcp")).toHaveCount(0)
  await perf.getByRole("button", { name: "Hard loads" }).click()
  await expect(page).not.toHaveURL(/perf=route/)
  for (const m of ["lcp", "inp", "cls", "fcp", "app_ready"]) await expect(card(m)).toBeVisible()

  // 182 phone: the four limit cards stack in one column with no horizontal overflow.
  await page.setViewportSize({ width: 375, height: 812 })
  // The sidebar slides away on the way to phone width; measure after it has.
  await expect(page.getByRole("button", { name: "Show sidebar", exact: true })).toBeVisible()
  await expect(page.getByRole("complementary", { name: "Sidebar", exact: true })).toBeHidden()
  const boxes = []
  for (const t of LIMIT_CARDS) boxes.push((await costCard(page, t).boundingBox())!)
  for (const b of boxes.slice(1)) expect(b.x).toBe(boxes[0].x)
  for (let i = 1; i < boxes.length; i++) expect(boxes[i].y).toBeGreaterThan(boxes[i - 1].y)
  const overflow = await page.evaluate(
    () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
  )
  expect(overflow).toBeLessThanOrEqual(0)
})

// Kept as its own flow: the Grilling cards only exist in the grilling-stats scenario, not in 184's.
test.describe("phone", () => {
  test.use({ viewport: { width: 375, height: 812 } })

  test("57.Stats, Grilling section, phone", async ({ page, start }) => {
    await start("grilling-stats")
    await page.evaluate(() => (location.hash = "#/stats?range=30"))
    const first = (await statCard(page, "Words read").boundingBox())!
    const second = (await statCard(page, "Questions asked").boundingBox())!
    expect(second.x).toBe(first.x)
    expect(second.y).toBeGreaterThan(first.y)
    const overflow = await page.evaluate(
      () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
    )
    expect(overflow).toBeLessThanOrEqual(0)
  })
})
