import { card, expect, menu, menuButton, nav, test } from "./fixture"
import { goto } from "./sidebarKit"

test("22. Back link count, then the phone layout", async ({ page, start }) => {
  const fx = await start("united")
  const back = (n: number) => page.getByRole("button", nav(`Epics · ${n} need you`))

  // The footer badge counts every card that needs you, whichever page is open.
  // The permission card joins only once its held worker asks, so the full count of nine is the signal that the scenario has settled.
  await goto(page, "#/e/demo:running-epic")
  await expect(back(9)).toBeVisible({ timeout: 60_000 })
  await goto(page, "#/e/demo:paused-epic")
  await expect(back(9)).toBeVisible()
  await goto(page, "#/d/demo:design-live")
  await expect(back(9)).toBeVisible()
  await goto(page, "#/e/demo:running-epic")
  await expect(back(9)).toBeVisible()

  // The count is not the button's colour and sits on the waiting-tone pill (the mock's tab count).
  const style = await back(9)
    .getByText("9", { exact: true })
    .evaluate((el) => {
      const cs = getComputedStyle(el)
      const button = getComputedStyle(el.closest("button")!)
      return { recolored: cs.color !== button.color, bg: cs.backgroundColor }
    })
  expect(style).toEqual({ recolored: true, bg: "rgb(242, 184, 69)" })

  // The page title is not a button and nothing is called waiting-count.
  await expect(page.getByRole("heading", { level: 1 }).getByRole("button")).toHaveCount(0)
  await expect(page.getByTestId("waiting-count")).toHaveCount(0)

  // Cmd/Ctrl+K opens the shell menu on the Epics page and on the Agents page, and Escape closes it.
  const jump = page.getByRole("dialog", nav("Jump to"))
  for (const hash of ["#/epics", "#/agents"]) {
    await goto(page, hash)
    await page.keyboard.press("ControlOrMeta+k")
    await expect(jump).toBeVisible()
    await page.keyboard.press("Escape")
    await expect(jump).toHaveCount(0)
  }
  await goto(page, "#/e/demo:running-epic")

  // Cmd/Ctrl+P opens the Command palette on every page, a second press closes it, and the filter box has focus.
  const palette = page.getByRole("dialog", nav("Commands"))
  for (const hash of ["#/epics", "#/e/demo:running-epic", "#/agents", "#/stats", "#/settings"]) {
    await goto(page, hash)
    await page.keyboard.press("ControlOrMeta+p")
    await expect(palette).toBeVisible()
    await expect(palette.getByRole("combobox")).toBeFocused()
    await page.keyboard.press("ControlOrMeta+p")
    await expect(palette).toHaveCount(0)
  }

  // It works from the Design page's chat box too.
  await goto(page, "#/d/demo:design-live")
  await page.getByRole("form", nav("Composer")).getByRole("button", nav("Note")).click()
  await expect(page.getByRole("textbox", nav("Message"))).toBeFocused()
  await page.keyboard.press("ControlOrMeta+p")
  await expect(palette).toBeVisible()
  await page.keyboard.press("Escape")
  await expect(palette).toHaveCount(0)

  // The palette is centered horizontally and lists the eight commands in order.
  await goto(page, "#/epics")
  await page.keyboard.press("ControlOrMeta+p")
  await expect(palette).toBeVisible()
  const box = (await palette.boundingBox())!
  const viewport = page.viewportSize()!
  expect(Math.abs(box.x + box.width / 2 - viewport.width / 2)).toBeLessThanOrEqual(2)
  const rows = palette.getByRole("option")
  await expect(rows).toHaveText([
    /^New design/,
    /^New chat/,
    /^Next question/,
    /^Go to Epics/,
    /^Go to Agents/,
    /^Go to Stats/,
    /^Go to Settings/,
    /^Toggle AFK/,
  ])

  // Next question lands on a Needs you card of the active workspace.
  await rows.filter({ hasText: "Next question" }).click()
  await expect(palette).toHaveCount(0)
  await expect(page).toHaveURL(/#\/(e|d|a|t)\//)

  // Toggle AFK, then the gear menu shows the AFK switch flipped.
  await goto(page, "#/epics")
  await page.keyboard.press("ControlOrMeta+p")
  await expect(rows.filter({ hasText: "Toggle AFK" })).toContainText("AFK is off")
  await rows.filter({ hasText: "Toggle AFK" }).click()
  await expect(palette).toHaveCount(0)
  await menuButton(page).click()
  await expect(menu(page).getByRole("menuitemcheckbox", nav("AFK"))).toBeChecked()
  await page.keyboard.press("Escape")

  // Clicking the count text itself opens the Epics menu, whose Epics item opens the page.
  await back(9).getByText("9", { exact: true }).click()
  await page.getByRole("menuitem", { name: /^Epics/ }).click()
  await expect(page).toHaveURL(/#\/epics$/)

  // Phone layout on the same settled scenario. 375 px is below the wide layout's 62em breakpoint.
  await page.setViewportSize({ width: 375, height: 812 })

  // The Command palette is a full-screen sheet on the phone.
  await page.keyboard.press("ControlOrMeta+p")
  await expect(palette).toBeVisible()
  await expect
    .poll(() => palette.boundingBox())
    .toMatchObject({ x: 0, y: 0, width: 375, height: 812 })
  await page.keyboard.press("Escape")
  await expect(palette).toHaveCount(0)

  // The grid is one column: two cards of a section sit at the same left edge, one under the other.
  const first = await card(page, "paused-epic").boundingBox()
  const second = await card(page, "question-epic").boundingBox()
  expect(second!.x).toBe(first!.x)
  expect(second!.y).toBeGreaterThan(first!.y)

  // The card's time label stays on one line.
  const time = card(page, "paused-epic").getByTestId("time")
  await expect(time).toBeVisible()
  const timeBox = await time.boundingBox()
  const timeLineHeight = await time.evaluate((el) => parseFloat(getComputedStyle(el).lineHeight))
  expect(timeBox!.height).toBeLessThanOrEqual(timeLineHeight * 1.5)

  // The sidebar slides in over the phone page and its footer keeps the Epics link with its count.
  await goto(page, "#/d/demo:design-live")
  await page.getByRole("button", nav("Show sidebar")).click()
  await expect(back(9)).toBeVisible()
  await page.getByRole("button", nav("Hide sidebar")).click()
  await expect(back(9)).toBeHidden()

  // On the epic page the buttons sit below the stats line.
  await goto(page, "#/e/demo:awaiting-epic")
  const merge = page.getByRole("button", nav("Merge"))
  await expect(merge).toBeVisible()
  const stats = (await page.getByTestId("stats-line").boundingBox())!
  for (const button of await merge.locator("xpath=..").getByRole("button").all()) {
    const box = (await button.boundingBox())!
    expect(box.y).toBeGreaterThanOrEqual(stats.y + stats.height)
    expect(box.height).toBeGreaterThanOrEqual(44)
  }

  // The count follows the server with no reload.
  await goto(page, "#/e/demo:running-epic")
  await page.getByRole("button", nav("Show sidebar")).first().click()
  await expect(back(9)).toBeVisible()
  await fx.cli("tickets", "set", "demo:paused-epic/01", "--status", "open")
  await expect(back(8)).toBeVisible()
})
