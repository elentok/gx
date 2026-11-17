import type { Page } from "@playwright/test"
import { freshVisit } from "./agentsKit"
import { expect, renderedText, test } from "./fixture"

// Titled "N. " flat (no describe) so `gx brief playwright` can filter by number.

const PARTS = [
  "mascot-image",
  "message",
  "submessage",
  "time-label",
  "time",
  "rule-note",
  "unlock-link",
]

const part = (page: Page, name: string) => page.locator(`[data-part="${name}"]`)
const restDialog = (page: Page) => page.getByRole("dialog", { name: "Rest" })

// Parts top to bottom, then the mascot's width. Each part waits on its own locator before the read.
async function layout(page: Page, parts: string[]) {
  for (const name of parts) await expect(part(page, name)).toBeVisible()
  return page.evaluate((names) => {
    const boxes = names.map((n) => {
      const r = document.querySelector(`[data-part="${n}"]`)!.getBoundingClientRect()
      return { n, top: r.top, width: r.width }
    })
    return { tops: boxes.map((b) => b.top), mascotWidth: boxes[0]!.width }
  }, parts)
}

function expectOrder(tops: number[]) {
  for (let i = 1; i < tops.length; i++) expect(tops[i]!).toBeGreaterThan(tops[i - 1]!)
}

async function restPeriod(fixture: { cli: (...args: string[]) => Promise<string> }) {
  const out = JSON.parse(await fixture.cli("rest", "--json"))
  return out.status.period
}

test("206. Rest screen, Break (R1)", async ({ page, start }) => {
  const fixture = await start("rest", { shape: "break" })
  await expect(restDialog(page)).toBeVisible()
  await expect(part(page, "time")).toHaveText(/^\d\d:\d\d$/)

  // A second page of the same server shows the same screen and message.
  const other = await freshVisit(page, "#/")
  await expect(restDialog(other)).toBeVisible()
  await expect(part(other, "message")).toHaveText(await renderedText(part(page, "message")))

  // 900px: parts in order, the page behind is covered (inert).
  await page.setViewportSize({ width: 900, height: 900 })
  const wide = await layout(page, PARTS)
  expectOrder(wide.tops)
  await expect(page.locator("#root")).toHaveAttribute("inert", "")

  // 375px: same order, smaller mascot.
  await page.setViewportSize({ width: 375, height: 812 })
  await expect(part(page, "mascot-image")).toBeVisible()
  const narrow = await layout(page, PARTS)
  expectOrder(narrow.tops)
  expect(narrow.mascotWidth).toBeLessThan(wide.mascotWidth)

  // Unlock anyway: hint, the sentence enables the button, the real round trip clears both pages.
  await part(page, "unlock-link").click()
  await expect(part(page, "unlock-box")).toContainText("Type exactly")
  const button = page.getByRole("button", { name: /^Unlock until/ })
  await expect(button).toBeDisabled()
  await page.getByLabel("Unlock sentence").fill("I'm tired and I'm addicted to AI")
  await expect(button).toBeEnabled()
  await button.click()
  await expect(restDialog(page)).toHaveCount(0)
  await expect(restDialog(other)).toHaveCount(0)
  expect(await restPeriod(fixture)).toBeFalsy()

  // Stats: one Break and one Break unlock today.
  await page.evaluate(() => {
    location.hash = "#/stats?range=7"
  })
  const section = page.getByRole("region", { name: "Mental health" })
  await expect(section).toContainText("1 Breaks · 1 unlocks")
  await expect(section).toContainText("1 Break, 0 Bedtime")
})

test("207. Rest screen, Bedtime (R2)", async ({ page, start }) => {
  const fixture = await start("rest", { shape: "bedtime" })
  await expect(restDialog(page)).toBeVisible()
  await expect(part(page, "time-label")).toHaveText("Back at")
  await expect(part(page, "time")).toHaveText(/^\d\d:\d\d$/)
  expect((await restPeriod(fixture)).kind).toBe("bedtime")

  // The server splits the default message into a message and a sub-message that names the End time.
  await expect(part(page, "message")).toHaveText("That's enough AI for today.")
  await expect(part(page, "submessage")).toHaveText(
    `Go to sleep and come back tomorrow at ${await renderedText(part(page, "time"))}.`,
  )

  await page.setViewportSize({ width: 375, height: 812 })
  await expect(part(page, "mascot-image")).toBeVisible()
  const narrow = await layout(page, PARTS)
  expectOrder(narrow.tops)

  const other = await freshVisit(page, "#/")
  await expect(restDialog(other)).toBeVisible()
  await expect(part(other, "message")).toHaveText(await renderedText(part(page, "message")))

  await part(page, "unlock-link").click()
  await page.getByLabel("Unlock sentence").fill("I'm up late and I'm addicted to AI")
  const button = page.getByRole("button", { name: /^Unlock until/ })
  await expect(button).toBeEnabled()
  await button.click()
  await expect(restDialog(page)).toHaveCount(0)
  await expect(restDialog(other)).toHaveCount(0)
  expect(await restPeriod(fixture)).toBeFalsy()
})
