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

  // page: the Schedule page of the seeded Weather check.
  const pageFixture = await start("schedules", { shape: "page" })
  await page.evaluate(() => (location.hash = "#/schedules"))
  await row("Weather check").click()
  await expect(page).toHaveURL(/#\/schedules\/demo:weather-check$/)
  const name = page.getByLabel("Name", { exact: true })
  const time = (n: number) => page.getByLabel(`Time ${n}`, { exact: true })
  const lateLimit = page.getByLabel("Late limit", { exact: true })
  const prompt = page.getByLabel("Prompt", { exact: true })
  await expect(name).toHaveValue("Weather check")
  await expect(time(2)).toBeVisible()
  const t1 = await time(1).inputValue()
  const t2 = await time(2).inputValue()
  await expect(page.locator('[data-mock-part="row-project"]')).toContainText("demo")
  await expect(page.getByLabel("Harness", { exact: true })).toHaveValue("")
  await expect(page.getByLabel("Harness", { exact: true }).locator("option").first()).toHaveText(
    /^Default \(.+\)$/,
  )
  await expect(lateLimit).toHaveValue("2")
  await expect(prompt).toHaveValue(/Check the weather for tomorrow\./)
  // Back from the Schedule page repaints the list (it once crashed with React error #185).
  await page.evaluate(() => history.back())
  await expect(page).toHaveURL(/#\/schedules$/)
  await expect(row("Weather check")).toBeVisible()
  await row("Weather check").click()
  await expect(page).toHaveURL(/#\/schedules\/demo:weather-check$/)
  await expect(name).toHaveValue("Weather check")
  const runs = page.locator('[data-mock-part="last-runs"] > li')
  await expect(runs).toHaveCount(4)
  await expect(runs.nth(0)).toContainText("Finished")
  await expect(runs.nth(0).getByRole("link")).toHaveAttribute("href", /one-offs/)
  await expect(runs.nth(1)).toContainText("Finished")
  await expect(runs.nth(2)).toContainText("Ran late")
  await expect(runs.nth(2).getByRole("link")).toHaveCount(1)
  await expect(runs.nth(3)).toContainText("Skipped")
  await expect(runs.nth(3).getByRole("link")).toHaveCount(0)

  // Add a second... third time, remove one; the save bar appears and Discard restores.
  const saveBar = page.locator('[data-mock-part="save-bar"]')
  await expect(saveBar).toHaveCount(0)
  await page.getByRole("button", { name: "Add time", exact: true }).click()
  await time(3).fill("23:30")
  await expect(saveBar).toBeVisible()
  await page.getByRole("button", { name: "Remove time 1", exact: true }).click()
  await expect(time(3)).toHaveCount(0)
  await expect(time(1)).toHaveValue(t2)
  await saveBar.getByRole("button", { name: "Discard", exact: true }).click()
  await expect(saveBar).toHaveCount(0)
  await expect(time(1)).toHaveValue(t1)
  await expect(time(2)).toHaveValue(t2)

  // Edit every field and Save.
  await name.fill("Rain check")
  await page.getByRole("button", { name: "Mon", exact: true }).click()
  await time(1).fill("23:45")
  await page.getByLabel("Harness", { exact: true }).selectOption("codex")
  await lateLimit.fill("3")
  await prompt.fill("Is it raining?")
  await saveBar.getByRole("button", { name: "Save", exact: true }).click()
  await expect(saveBar).toHaveCount(0)
  await page
    .getByRole("link", { name: /Scheduled prompts/ })
    .first()
    .click()
  await expect(row("Rain check")).toBeVisible()
  const saved = await pageFixture.cli("schedules", "show", "demo:weather-check", "--json")
  expect(saved).toContain("Rain check")
  expect(saved).toContain("23:45")
  expect(saved).toContain("codex")
  expect(saved).toContain("Is it raining?")
  await row("Rain check").click()
  await expect(name).toHaveValue("Rain check")
  await expect(prompt).toHaveValue("Is it raining?")
  await expect(lateLimit).toHaveValue("3")

  // The switch off: Run now still works and the page says so. The held worker keeps that run
  // live, so the refusal below needs no release (a released run asks a question instead).
  const runNow = page.getByRole("button", { name: "Run now", exact: true })
  await page.getByRole("switch").click()
  await expect(page.getByText("Runs once now. The switch is off on this machine.")).toBeVisible()
  await runNow.click()
  await expect(runs).toHaveCount(5)
  await expect(runs.nth(0).getByRole("link")).toHaveCount(1)

  // The switch on: a second Run now is refused.
  await page.getByRole("switch").click()
  await expect(page.getByText("Runs once now. The switch is off on this machine.")).toHaveCount(0)
  await runNow.click()
  await expect(page.getByRole("alert").filter({ hasText: "previous run started" })).toBeVisible()

  // New: the project defaults to scratch, Create lands on the new Schedule.
  await page
    .getByRole("link", { name: /Scheduled prompts/ })
    .first()
    .click()
  await page.getByRole("button", { name: "New", exact: true }).click()
  await expect(page).toHaveURL(/#\/schedules\/new$/)
  await expect(page.getByLabel("Project", { exact: true })).toHaveValue("scratch")
  await name.fill("Evening note")
  await page.getByRole("button", { name: "Mon", exact: true }).click()
  await time(1).fill("21:00")
  await lateLimit.fill("1")
  await prompt.fill("Say good evening.")
  await page.getByRole("button", { name: "Create", exact: true }).click()
  await expect(page).toHaveURL(/#\/schedules\/scratch:evening-note$/)
  await expect(name).toHaveValue("Evening note")
  await page
    .getByRole("link", { name: /Scheduled prompts/ })
    .first()
    .click()
  await expect(row("Evening note")).toBeVisible()

  // Delete: an inline confirm naming the effect, then back to the list without the row.
  await row("Evening note").click()
  await page.getByRole("button", { name: "Delete this schedule", exact: true }).click()
  await expect(page.getByText(/Queued runs that never started are cancelled/)).toBeVisible()
  await page.getByRole("button", { name: "Delete", exact: true }).click()
  await expect(page).toHaveURL(/#\/schedules$/)
  await expect(row("Evening note")).toHaveCount(0)

  // paused: the paused One-off opens its ticket page; Cancel there turns the row Cancelled.
  await start("schedules", { shape: "paused" })
  await page.evaluate(() => (location.hash = "#/schedules/demo:weather-check"))
  await expect(runs).toHaveCount(5)
  await expect(runs.nth(0)).toContainText("Paused")
  await runs.nth(0).getByRole("link").click()
  await expect(page).toHaveURL(/one-offs/)
  await page.getByRole("button", { name: "Cancel", exact: true }).click()
  await page.getByRole("button", { name: "Cancel run", exact: true }).click()
  await page.goBack()
  await expect(runs.nth(0)).toContainText("Cancelled")
})
