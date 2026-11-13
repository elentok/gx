import type { Locator, Page } from "@playwright/test"
import { sessions } from "./agentsKit"
import { blockEvents, expect, nav, swipe, test } from "./fixture"
import { sidebarKit, stillMotion } from "./sidebarKit"

const goto = (page: Page, hash: string) => page.evaluate((h) => (window.location.hash = h), hash)

// The sidebar at narrow widths and its footer over the `workspaces` scenario: swipes, the 744px and 390px shells, forms after
// a menu closes, and the footer's sizes; split from the Workspaces flow so each stays inside its time limit.
test("Workspaces touch", async ({ page, start }) => {
  test.setTimeout(120_000)
  const fx = await start("workspaces")
  await stillMotion(page)
  const { sidebar, row, switcher, closeMenus, pick, rowMenu, moveTo, jump } = sidebarKit(page)
  await fx.cli("server", "workspaces", "create", "scratch")
  await expect(page.getByRole("button", nav("Workspace: Default workspace"))).toBeVisible()
  await expect(row("ready-epic")).toBeVisible()

  // At 1280px the sidebar is docked: no swipe moves it.
  await swipe(page, [300, 600], [100, 610])
  await expect(sidebar).toBeVisible()
  await swipe(page, [150, 600], [300, 610])
  await expect(sidebar).toBeVisible()

  // At 744px the sidebar slides over: picking a row closes it, the jump menu is a popover, no page scrolls sideways.
  await page.setViewportSize({ width: 744, height: 900 })
  await goto(page, "#/epics")
  await expect(sidebar).toBeHidden()
  await page.getByRole("button", nav("Show sidebar")).click()
  await expect(sidebar).toBeVisible()
  await swipe(page, [300, 600], [100, 610])
  await expect(sidebar).toBeHidden()
  await swipe(page, [150, 600], [300, 610])
  await expect(sidebar).toBeVisible()
  await row("paused-epic").click()
  await expect(page).toHaveURL(/#\/e\/demo:paused-epic$/)
  await expect(sidebar).toBeHidden()
  await page.keyboard.press("ControlOrMeta+k")
  const popover = (await jump.boundingBox())!
  expect(popover.width).toBeLessThan(744)
  expect(popover.height).toBeLessThan(900)
  await page.keyboard.press("Escape")
  const sideways = () =>
    page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth)
  const ticket = page.locator('a[href^="#/t/"]').first()
  await expect(ticket).toBeVisible()
  const ticketHash = (await ticket.getAttribute("href"))!
  // Stats and Settings are checked for sideways scroll in their own flows.
  for (const hash of [ticketHash, "#/e/demo:ws-design"]) {
    await goto(page, hash)
    await expect(page.getByRole("heading").first()).toBeVisible()
    expect(await sideways(), hash).toBe(false)
  }

  // At 390px an item outside the active workspace opens through the switcher: its row is not in the sidebar.
  await page.setViewportSize({ width: 390, height: 844 })
  await goto(page, "#/epics")
  await page.getByRole("button", nav("Show sidebar")).click()
  // Opening ws-design above switched the sidebar to its workspace.
  await pick("Default workspace")
  // The header's search button opens the jump menu above the sliding sidebar; its tooltip names the key.
  const search = sidebar.getByRole("button", nav("Search"))
  await expect(search).toHaveAttribute("title", /^Search \((⌘K|Ctrl\+K)\)$/)
  await search.click()
  await expect(jump).toBeVisible()
  await page.keyboard.press("Escape")
  await expect(jump).toHaveCount(0)
  await expect(sidebar).toBeVisible()
  // Swipe left closes it; a swipe right from x=150 opens it and one from the system's back-swipe edge (x=5) does not.
  await swipe(page, [200, 700], [200, 300])
  await expect(sidebar).toBeVisible()
  await swipe(page, [250, 600], [100, 610])
  await expect(sidebar).toBeHidden()
  await swipe(page, [5, 600], [150, 610])
  await expect(sidebar).toBeHidden()
  await swipe(page, [150, 600], [300, 610])
  await expect(sidebar).toBeVisible()
  // The gear menu's dim covers the Hide sidebar toggle: a tap on the toggle and a swipe through the dim still close the menu and the sidebar.
  const gearMenu = page.getByRole("menu")
  await page.getByRole("button", nav("Menu")).click()
  await expect(gearMenu).toBeVisible()
  // Measured once the sidebar has finished sliding in.
  const toggleBox = (await sidebar.getByRole("button", nav("Hide sidebar")).boundingBox())!
  await page.mouse.click(toggleBox.x + toggleBox.width / 2, toggleBox.y + toggleBox.height / 2)
  await expect(gearMenu).toHaveCount(0)
  await expect(sidebar).toBeHidden()
  await page.getByRole("button", nav("Show sidebar")).click()
  await page.getByRole("button", nav("Menu")).click()
  await expect(gearMenu).toBeVisible()
  await swipe(page, [380, 700], [250, 705])
  await expect(gearMenu).toHaveCount(0)
  await expect(sidebar).toBeHidden()
  await page.getByRole("button", nav("Show sidebar")).click()
  await expect(sidebar).toBeVisible()
  // A tap outside an open menu closes it; Move swaps in place with a Back row, Back returns.
  await rowMenu("paused-epic").click()
  await expect(page.getByRole("menuitem", nav("Move to workspace"))).toBeVisible()
  await page.mouse.click(380, 600)
  await expect(page.getByRole("menuitem", nav("Move to workspace"))).toHaveCount(0)
  await rowMenu("paused-epic").click()
  await page.getByRole("menuitem", nav("Move to workspace")).click()
  await expect(page.getByRole("menuitem", nav("Hold"))).toHaveCount(0)
  await page.getByRole("menuitem", nav("‹ Back")).click()
  await expect(page.getByRole("menuitem", nav("Hold"))).toBeVisible()
  await page.mouse.click(380, 600)
  await expect(page.getByRole("menuitem", nav("Hold"))).toHaveCount(0)
  await moveTo(rowMenu("paused-epic"), "scratch")
  await expect(row("paused-epic")).toHaveCount(0)
  await pick("scratch")
  await row("paused-epic").click()
  await expect(page).toHaveURL(/#\/e\/demo:paused-epic$/)
  await expect(sidebar).toBeHidden()

  // A tap on a chat row opens it without the keyboard: the prompt is not the active element. The fixture page has no touch context, so a click stands in.
  await goto(page, "#/epics")
  await page.getByRole("button", nav("Show sidebar")).click()
  await pick("Default workspace")
  const chatRow = sidebar.locator("a[data-chat]").first()
  const prompt = page.getByRole("form", nav("Composer")).getByRole("textbox")
  await chatRow.click()
  await expect(page).toHaveURL(/#\/agents\//)
  await expect(prompt).toBeVisible()
  await expect(prompt).not.toBeFocused()

  // The chat row's "..." menu: Stop ends the live session, Remove confirms and deletes it, and removing the open chat leaves its page.
  const chatId = page.url().split("/agents/")[1]
  // A rename re-sorts the rows, so the row is found by its id and not by its place.
  const ownRow = sidebar.locator(`a[href$="/agents/${chatId}"]`)
  const chatMenu = () =>
    ownRow.locator("xpath=ancestor::li").getByRole("button", nav("More actions"))
  await goto(page, "#/epics")
  await page.getByRole("button", nav("Show sidebar")).click()
  // Rename from the row's menu at 390px: the popup fits the screen, the page does not scroll sideways, and the rename shows in the row.
  const renameDialog = page.getByRole("dialog", nav("Rename chat"))
  const renameField = renameDialog.getByRole("textbox", nav("Name"))
  const renameFromMenu = async (name: string) => {
    await chatMenu().click()
    await page.getByRole("menuitem", nav("Rename")).click()
    await expect(renameDialog).toBeVisible()
    await renameField.fill(name)
    await page.keyboard.press("Enter")
    await expect(renameDialog).toHaveCount(0)
    await expect(ownRow).toContainText(name)
  }
  await chatMenu().click()
  await page.getByRole("menuitem", nav("Rename")).click()
  await expect(renameDialog).toBeVisible()
  await expect(renameField).toBeVisible()
  const sheet = (await renameDialog.boundingBox())!
  expect(sheet.x).toBeGreaterThanOrEqual(0)
  expect(sheet.x + sheet.width).toBeLessThanOrEqual(390)
  expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(390)
  await page.keyboard.press("Escape")
  await expect(renameDialog).toHaveCount(0)
  await renameFromMenu("touch-renamed")
  await expect.poll(() => fx.cli("agents", "list")).toContain("touch-renamed")
  await chatMenu().click()
  await page.getByRole("menuitem", nav("Stop")).click()
  await expect(ownRow).toContainText("Exited")
  // An Exited chat renames in gx only: the verb succeeds and no session is touched.
  await renameFromMenu("touch-exited")
  await expect.poll(() => fx.cli("agents", "list")).toContain("touch-exited")
  await chatMenu().click()
  await expect(page.getByRole("menuitem", nav("Remove"))).toBeVisible()
  await expect(page.getByRole("menuitem", nav("Stop"))).toHaveCount(0)
  await closeMenus()
  await goto(page, `#/agents/${chatId}`)
  await chatMenu().click()
  await page.getByRole("menuitem", nav("Remove")).click()
  await expect(page.getByRole("dialog")).toBeVisible()
  expect(await fx.cli("agents", "list")).toContain(chatId)
  await page.getByRole("dialog").getByRole("button", nav("Remove")).click()
  await expect(page).not.toHaveURL(new RegExp(`#/agents/${chatId}$`))
  await expect.poll(() => fx.cli("agents", "list")).not.toContain(chatId)

  // Returning to the app restarts the stream and refetches: with the stream blocked, a rename from the CLI
  // reaches the sidebar only through visibilitychange and pageshow, with no reload.
  await goto(page, "#/epics")
  const unblock = await blockEvents(page)
  await fx.cli("server", "workspaces", "rename", "scratch", "archive")
  await page.evaluate(() => document.dispatchEvent(new Event("visibilitychange")))
  await page.evaluate(() => window.dispatchEvent(new Event("pageshow")))
  await unblock()
  await switcher.click()
  await expect(page.getByRole("menuitem", { name: /^archive/ })).toBeVisible()
  await closeMenus()

  // After a real menu closes, focus lands in the dialog: the first field at 1280px, the panel itself at 390px.
  await page.setViewportSize({ width: 1280, height: 900 })
  await expect(sidebar).toBeVisible()
  const dialog = page.getByRole("dialog")
  const newItem = async (name: string) => {
    await sidebar.getByRole("button", { name: "+ New" }).click()
    await page.getByRole("menuitem", { name }).click()
    await expect(dialog).toBeVisible()
  }
  const closeDialog = async () => {
    await page.keyboard.press("Escape")
    await expect(dialog).toHaveCount(0)
  }
  await newItem("New workspace…")
  await expect(dialog.getByRole("textbox", nav("Workspace name"))).toBeFocused()
  await closeDialog()
  await switcher.click()
  await page.getByRole("menuitem", { name: "New workspace…" }).click()
  await expect(dialog.getByRole("textbox", nav("Workspace name"))).toBeFocused()
  await closeDialog()
  await newItem("New chat")
  await expect(dialog.getByRole("textbox", nav("Name"))).toBeVisible()
  await expect(dialog.getByRole("textbox", nav("Name"))).toBeFocused()
  await closeDialog()
  for (let n = 0; n < 4; n++) {
    await newItem("New design")
    await expect(dialog.getByRole("textbox", nav("Epic slug"))).toBeFocused()
    await closeDialog()
  }
  await page.setViewportSize({ width: 390, height: 844 })
  await expect(sidebar).toBeVisible()
  for (const name of ["New chat", "New workspace…"]) {
    await newItem(name)
    await expect(dialog).toBeFocused()
    await closeDialog()
  }
  await page.setViewportSize({ width: 1280, height: 900 })

  // New chat and New design start on the page's project, then the Epics filter's single project, then the last used one.
  // Every check opens a blank form: the previous one is closed with nothing typed, so no draft carries over.
  const chatForm = page.getByRole("dialog", { name: "New session" })
  const designForm = dialog.getByRole("textbox", nav("Epic slug"))
  const chatProject = chatForm.getByRole("combobox", nav("Project"))
  const designProject = dialog.getByRole("combobox", nav("Project"))
  const formsShow = async (project: string) => {
    await newItem("New chat")
    await expect(chatProject).toHaveValue(project)
    await closeDialog()
    await newItem("New design")
    await expect(designForm).toBeVisible()
    await expect(designProject).toHaveValue(project)
    await closeDialog()
  }
  const sideChat = (await sessions(fx)).find((s) => s.project === "side" && !s.design)!
  await goto(page, "#/d/side:side-design")
  await expect(page.getByRole("heading", { name: /side-design/ }).first()).toBeVisible()
  await formsShow("side")
  await goto(page, `#/agents/${sideChat.id}`)
  await expect(page).toHaveURL(new RegExp(`#/agents/${sideChat.id}$`))
  await newItem("New chat")
  await expect(chatProject).toHaveValue("side")
  await closeDialog()

  // On the Epics page, a filter that picks the one project decides.
  await goto(page, "#/epics")
  await page.getByRole("button", nav("Project")).click()
  const projectList = page.getByRole("dialog", nav("Projects"))
  const allProjects = projectList.getByRole("button", nav("All projects"))
  if (await allProjects.isEnabled()) await allProjects.click()
  await projectList.getByRole("checkbox", nav("side")).check()
  await page.keyboard.press("Escape")
  await expect(projectList).toHaveCount(0)
  await formsShow("side")

  // With the filter cleared, a chat created in the second project becomes the last used one, which a Stats page form starts on.
  await page.getByRole("button", nav("Project")).click()
  await projectList.getByRole("button", nav("All projects")).click()
  await page.keyboard.press("Escape")
  await expect(projectList).toHaveCount(0)
  await goto(page, "#/stats")
  await newItem("New chat")
  await expect(chatProject).toHaveValue("demo")
  await chatProject.selectOption("side")
  await chatForm.getByRole("textbox", nav("Name")).fill("side-made")
  await chatForm.getByRole("textbox", nav("First prompt")).fill("hi")
  await chatForm.getByRole("button", nav("Create session")).click()
  await expect(chatForm).toHaveCount(0)
  await goto(page, "#/stats")
  await formsShow("side")

  // The footer's boxes are one size with their icons centered, at 1280px and 390px, and the badges agree.
  const footer = sidebar.locator("footer")
  const epicsBox = footer.getByRole("button", { name: /^Epics/ })
  const followUpsBox = footer.getByRole("button", { name: /^Follow-ups/ })
  const statsBox = footer.getByRole("link", { name: /^Stats/ })
  const gearBox = footer.getByRole("button", nav("Menu"))
  const boxes = [epicsBox, followUpsBox, statsBox, gearBox]
  const measure = async (locator: Locator) => {
    await expect(locator).toBeVisible()
    return (await locator.boundingBox())!
  }
  // `wrapped`: with every indicator shown off desktop the row wraps and the menu drops to a second line, so only the sizes must agree.
  const checkBoxes = async (wrapped = false) => {
    const sizes = []
    for (const box of boxes) {
      const b = await measure(box)
      const i = await measure(box.locator("svg"))
      expect(i.x + i.width / 2).toBeCloseTo(b.x + b.width / 2, 0)
      expect(i.y + i.height / 2).toBeCloseTo(b.y + b.height / 2, 0)
      sizes.push(wrapped ? [b.width, b.height] : [b.width, b.height, b.y])
    }
    for (const s of sizes) expect(s).toEqual(sizes[0])
    return sizes[0][0]
  }
  const badgeOf = async (box: Locator) => {
    const badge = box.locator('[data-part="badge"]')
    await expect(badge).toBeVisible()
    const b = (await badge.boundingBox())!
    const p = (await box.boundingBox())!
    const css = await badge.evaluate((el) => {
      const s = getComputedStyle(el)
      return [s.fontSize, s.fontWeight, s.lineHeight]
    })
    return { w: b.width, h: b.height, top: b.y - p.y, right: p.x + p.width - (b.x + b.width), css }
  }
  await page.setViewportSize({ width: 1280, height: 900 })
  await expect(epicsBox).toHaveCSS("width", "25px")
  expect(await checkBoxes()).toBe(25)
  expect(await badgeOf(epicsBox)).toEqual(await badgeOf(followUpsBox))
  // The menu icon is three stacked dots and no path.
  await expect(gearBox.locator("svg circle")).toHaveCount(3)
  await expect(gearBox.locator("svg path")).toHaveCount(0)

  // With the health warning, paused and offline indicators all shown, no box shrinks and nothing overflows the sidebar.
  const width = await checkBoxes()
  await fx.setHealth("cpu=75 mem=85")
  await fx.cli("server", "queue", "pause")
  const indicators = footer.getByRole("img", {
    name: /^(System health is high or critical|Server queue is paused|Disconnected from the server)$/,
  })
  await expect(indicators).toHaveCount(2)
  await fx.stopServer()
  await expect(indicators).toHaveCount(3)
  const sidebarBox = (await measure(sidebar))!
  expect(sidebarBox.width).toBeLessThanOrEqual(280)
  expect(await checkBoxes()).toBe(width)
  for (let n = 0; n < 3; n++) {
    const b = await measure(indicators.nth(n))
    expect(b.width).toBe(width)
    expect(b.x + b.width).toBeLessThanOrEqual(sidebarBox.x + sidebarBox.width)
  }
  await expect(footer).toBeVisible()
  expect(await footer.evaluate((el) => el.scrollWidth <= el.clientWidth)).toBe(true)

  // Repeat at 390px.
  await page.setViewportSize({ width: 390, height: 844 })
  await expect(sidebar).toBeVisible()
  await expect(epicsBox).toHaveCSS("width", "34px")
  expect(await checkBoxes(true)).toBe(34)
  // The touch footer keeps its boxes at 34px with every indicator shown; only the 280px sidebar is held to fit.
  for (let n = 0; n < 3; n++) expect((await measure(indicators.nth(n))).width).toBe(34)
})
