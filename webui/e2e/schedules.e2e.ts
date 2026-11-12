import { epicsButton, expect, test } from "./fixture"

// Part 1: menu and the list in each fixture shape. Titled "N. " flat (no describe) so `gx brief playwright` can filter by number.
test("203. Schedules journey", async ({ page, start }) => {
  const fixture = await start("schedules", { shape: "list" })
  const row = (name: string) => page.getByRole("listitem", { name, exact: true })
  const switchOf = (name: string) => row(name).getByRole("switch")
  const banner = page.locator('[data-mock-part="off-banner"]')

  // Menu: a separator after Archive, then Scheduled prompts.
  await epicsButton(page).click()
  const archive = page.getByRole("menuitem", { name: /^Archive/ })
  await expect(archive).toBeVisible()
  await expect(page.getByRole("separator")).toHaveCount(1)
  await page.getByRole("menuitem", { name: "Scheduled prompts", exact: true }).click()
  await expect(page).toHaveURL(/#\/schedules$/)

  // list: three rows; the third is off here.
  await expect(page.getByRole("heading", { name: "Scheduled prompts", exact: true })).toBeVisible()
  await expect(row("Weather check")).toBeVisible()
  await expect(row("Morning PR digest")).toContainText(/Next /)
  await expect(row("Weekly cleanup report")).toContainText("Off here")
  await expect(page.getByRole("listitem")).toHaveCount(3)

  // A switch flip swaps "Off here" and the next run, and the CLI agrees.
  const weekly = switchOf("Weekly cleanup report")
  await weekly.click()
  await expect(weekly).toHaveAttribute("aria-checked", "true")
  await expect(row("Weekly cleanup report")).toContainText(/Next /)
  await expect(row("Weekly cleanup report")).not.toContainText("Off here")
  const shown = JSON.parse(
    await fixture.cli("schedules", "show", "demo:weekly-cleanup-report", "--json"),
  )
  expect(JSON.stringify(shown)).toMatch(/"switch":\s*true/)
  await weekly.click()
  await expect(weekly).toHaveAttribute("aria-checked", "false")
  await expect(row("Weekly cleanup report")).toContainText("Off here")

  // "New" opens the new-Schedule page.
  await page.getByRole("button", { name: "New", exact: true }).click()
  await expect(page).toHaveURL(/#\/schedules\/new$/)
  await page.goBack()
  await expect(page).toHaveURL(/#\/schedules$/)

  // Turning the two on-rows off shows the banner.
  await expect(banner).toHaveCount(0)
  await switchOf("Weather check").click()
  await expect(switchOf("Weather check")).toHaveAttribute("aria-checked", "false")
  await expect(banner).toHaveCount(0)
  await switchOf("Morning PR digest").click()
  await expect(banner).toBeVisible()
  await expect(banner).toContainText("Every schedule is off")

  // all-off: the banner from the start and "Off here" on every row.
  await start("schedules", { shape: "all-off" })
  await page.evaluate(() => (location.hash = "#/schedules"))
  await expect(banner).toBeVisible()
  await expect(page.getByText("Off here")).toHaveCount(3)

  // empty: the empty text with its example and New.
  await start("schedules", { shape: "empty" })
  await page.evaluate(() => (location.hash = "#/schedules"))
  await expect(page.getByText("No scheduled prompts yet.")).toBeVisible()
  await expect(page.getByText(/for example every day at 07:00 and 21:00/)).toBeVisible()
  await expect(page.getByRole("button", { name: "New", exact: true })).toBeVisible()

  // missing-project: "project not found" in place of the next run.
  await start("schedules", { shape: "missing-project" })
  await page.evaluate(() => (location.hash = "#/schedules"))
  await expect(page.getByRole("listitem")).toHaveCount(1)
  await expect(page.getByRole("listitem")).toContainText("project not found")
})
