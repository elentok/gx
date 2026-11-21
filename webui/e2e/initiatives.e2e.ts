import type { Page } from "@playwright/test"
import { card, epicsButton, expect, section, test, type Fixture } from "./fixture"
import { goto } from "./sidebarKit"

// Titled "N. " flat (no describe) so `gx brief playwright` can filter by number.
// Render details (wording, colours, empty states) are pinned in InitiativePage.test.tsx and InitiativesPage.test.tsx;
// these flows only add the server payload, the CLI agreeing with the page, and navigation across pages.

const ADDRESS = "demo:ship-sync"

type CliMilestone = { number: number; slug: string; state: string }
type CliPage = { state: string; milestones: CliMilestone[] }

const region = (page: Page, name: string) => page.getByRole("region", { name, exact: true })

async function cliPage(fixture: Fixture, address: string): Promise<CliPage> {
  return JSON.parse(await fixture.cli("server", "initiative", address, "--json"))
}

/** The Initiative page is one centred column about 860px wide at 1280px (read in one poll, like flow 24). */
async function expectOneColumn(page: Page) {
  const column = region(page, "Milestones")
  await expect
    .poll(async () => {
      const box = await column.boundingBox()
      return box ? Math.round(box.width) : 0
    })
    .toBeGreaterThan(780)
  await expect
    .poll(async () => {
      const box = await column.boundingBox()
      return box ? Math.round(box.width) : 0
    })
    .toBeLessThanOrEqual(880)
}

/** The page's pill and the Milestone pills equal what the CLI read verb prints for the same server. */
async function expectPageMatchesCli(page: Page, fixture: Fixture, address: string, pill: string) {
  const cli = await cliPage(fixture, address)
  await expect(page.getByText(pill, { exact: true }).first()).toBeVisible()
  expect(cli.state).toBe(pill.toLowerCase())
  const milestones = region(page, "Milestones")
  for (const m of cli.milestones) {
    await expect(milestones.getByText(`${m.number}. `).first()).toBeVisible()
    await expect(milestones.getByText(m.state, { exact: true }).first()).toBeVisible()
  }
  return cli
}

test("210. Initiatives journey", async ({ page, start }) => {
  const fixture = await start("initiative-list")

  // Nav: the Epics menu opens Initiatives with no reload.
  await epicsButton(page).click()
  await page.getByRole("menuitem", { name: "Initiatives", exact: true }).click()
  await expect(page).toHaveURL(/#\/initiatives$/)

  // List: Active and Stopped rows, no button, and the CLI list agrees.
  await expect(page.getByRole("heading", { name: "Initiatives", level: 1 })).toBeVisible()
  const project = region(page, "demo")
  const active = project
    .getByRole("row")
    .filter({ has: page.getByRole("link", { name: "ship-sync", exact: true }) })
  const stopped = project
    .getByRole("row")
    .filter({ has: page.getByRole("link", { name: "stopped-sync", exact: true }) })
  await expect(active).toContainText("Active")
  await expect(active).toContainText("1 of 3 done")
  await expect(active).toContainText("Milestone 2: retry (designing)")
  await expect(stopped).toContainText("Stopped")
  await expect(stopped).toContainText("Stopped at Milestone 1 gate")
  await expect(page.getByRole("button", { name: /start/i })).toHaveCount(0)
  const list = await fixture.cli("server", "initiatives")
  expect(list).toContain("demo:ship-sync\tactive\t1 of 3 done")
  expect(list).toContain("demo:stopped-sync\tstopped at Milestone 1 gate\t1 of 3 done")

  // Gated Active page, reached from its row; back returns to the list.
  await page.getByRole("link", { name: "ship-sync", exact: true }).click()
  await expect(page).toHaveURL(/#\/initiatives\/demo:ship-sync$/)
  await expect(page.getByRole("heading", { name: "ship-sync", level: 1 })).toBeVisible()
  const gated = await expectPageMatchesCli(page, fixture, ADDRESS, "Active")
  expect(gated.milestones.map((m) => m.state)).toEqual(["gated", "designing", "sketch"])
  await expectOneColumn(page)
  const milestones = region(page, "Milestones")
  await expect(milestones.getByRole("link", { name: "Design: sync-core" })).toBeVisible()
  await expect(milestones.getByRole("link", { name: "Impl epic: sync-core-impl" })).toBeVisible()
  await expect(milestones.getByRole("link", { name: "Design: sync-retry" })).toBeVisible()
  await expect(milestones.getByRole("link")).toHaveCount(3)
  await expect(region(page, "Gate notes")).toContainText("core · outcome continue")
  await page.goBack()
  await expect(page).toHaveURL(/#\/initiatives$/)

  // Stopped page, reached from its row.
  await page.getByRole("link", { name: "stopped-sync", exact: true }).click()
  await expect(page).toHaveURL(/#\/initiatives\/demo:stopped-sync$/)
  const stoppedPage = await expectPageMatchesCli(page, fixture, "demo:stopped-sync", "Stopped")
  expect(stoppedPage.milestones.map((m) => m.state)).toEqual(["gated", "sketch", "sketch"])
  await expectOneColumn(page)
  await expect(region(page, "Gate notes")).toContainText("outcome stop")
  await expect(region(page, "Deferred")).toContainText("dropped")

  // Finished page on a fresh server (no nav needed: the journey was checked above).
  const finished = await start("initiative-finished")
  await goto(page, `#/initiatives/${ADDRESS}`)
  await expect(page.getByRole("heading", { name: "ship-sync", level: 1 })).toBeVisible()
  const done = await expectPageMatchesCli(page, finished, ADDRESS, "Finished")
  expect(done.milestones.map((m) => m.state)).toEqual(["gated", "gated"])
  await expectOneColumn(page)
  await expect(region(page, "Deferred")).toContainText("Nothing deferred.")
  await expect(region(page, "Gate notes")).toContainText("outcome continue")
  await expect(region(page, "Gate notes")).toContainText("outcome done")
})

test("211. Initiative on the Epics page", async ({ page, start }) => {
  const fixture = await start("initiative-epics-cards")
  await goto(page, "#/epics")

  // Cards: the line sits inside the Implementation and Design cards, a plain Design card has none, no Initiative card.
  const impl = card(page, "sync-core")
  await expect(impl).toContainText("Milestone 1 of")
  await expect(impl.getByRole("link", { name: "ship-sync", exact: true })).toHaveAttribute(
    "href",
    `#/initiatives/${ADDRESS}`,
  )
  await section(page, "Ready to design")
  const design = card(page, "sync-retry")
  await expect(design).toContainText("Milestone 2 of")
  await expect(design.getByRole("link", { name: "ship-sync", exact: true })).toBeVisible()
  await expect(card(page, "plain-design")).toBeVisible()
  await expect(card(page, "plain-design")).not.toContainText("Milestone")
  // The Initiative has no card of its own: its slug appears only as the two back-links.
  await expect(page.getByRole("link", { name: "ship-sync", exact: true })).toHaveCount(2)
  // The grid: the Implementation card's line is inside the card box, and the cards share a row layout.
  await expect
    .poll(async () => {
      const outer = await impl.boundingBox()
      const line = await impl.getByRole("link", { name: "ship-sync", exact: true }).boundingBox()
      return (
        !!outer && !!line && line.x >= outer.x && line.x + line.width <= outer.x + outer.width + 1
      )
    })
    .toBe(true)

  // The card's link opens the Initiative page, implementing, and the CLI agrees.
  await impl.getByRole("link", { name: "ship-sync", exact: true }).click()
  await expect(page).toHaveURL(/#\/initiatives\/demo:ship-sync$/)
  const cli = await expectPageMatchesCli(page, fixture, ADDRESS, "Active")
  expect(cli.milestones[0].state).toBe("implementing")
  await expectOneColumn(page)
  await expect(region(page, "Gate notes")).toContainText("/gx-initiative demo:ship-sync")

  // The Milestone links lead to the epic and Design headers, which show the same line and lead back.
  await region(page, "Milestones").getByRole("link", { name: "Impl epic: sync-core-impl" }).click()
  await expect(page).toHaveURL(/#\/e\/demo:sync-core-impl$/)
  await expect(page.getByText("Milestone 1 of")).toBeVisible()
  await page.getByRole("link", { name: "ship-sync", exact: true }).click()
  await expect(page).toHaveURL(/#\/initiatives\/demo:ship-sync$/)
  await region(page, "Milestones").getByRole("link", { name: "Design: sync-retry" }).click()
  await expect(page).toHaveURL(/#\/d\/demo:sync-retry$/)
  await expect(page.getByText("Milestone 2 of")).toBeVisible()
  await page.getByRole("link", { name: "ship-sync", exact: true }).click()
  await expect(page).toHaveURL(/#\/initiatives\/demo:ship-sync$/)
})
