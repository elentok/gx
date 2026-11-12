import type { Page } from "@playwright/test"
import { sessions } from "./agentsKit"
import { expect, nav, test } from "./fixture"

const goto = (page: Page, hash: string) => page.evaluate((h) => (window.location.hash = h), hash)

// The Leader key's Chord box over the `workspaces` scenario; split from the Workspaces flow so each stays inside its time limit.
test("Chords", async ({ page, start }) => {
  test.setTimeout(120_000)
  const fx = await start("workspaces")
  // Every menu close waits out its animation; the flow checks end states, not motion.
  await page.addStyleTag({
    content:
      "*,*::before,*::after{animation-duration:0s!important;animation-delay:0s!important;transition-duration:0s!important}",
  })
  const sidebar = page.getByRole("complementary", nav("Sidebar"))
  const chatRow = sidebar.locator("a[data-chat]").first()
  const prompt = page.getByRole("form", nav("Composer")).getByRole("textbox")
  const nestedLinks = (title: string) =>
    sidebar
      .getByRole("link", { name: new RegExp(`^(Fix agent )?${title}(?![\\w-])`) })
      .locator("xpath=ancestor::li[1]")
      .locator("ul")
      .getByRole("link")
  await sidebar.getByRole("button", nav("Expand paused-epic")).click()
  await sidebar.getByRole("button", nav("Expand inv-epic")).click()
  const working = nestedLinks("paused-epic").filter({ hasText: "Working" })
  const queueAgent = nestedLinks("inv-epic")
  await expect(working).toHaveCount(1)
  await expect(queueAgent).toHaveCount(1)

  // Chords: the Leader key opens the Chord box, any other key closes it, and open layers keep it shut.
  const chordBox = page.getByRole("menu", nav("Chords"))
  const leader = "ControlOrMeta+e"
  // The chip follows the platform: a Mac context reads ⌘E and Meta+E opens the box, any other reads Ctrl+E.
  for (const [platform, chip, key] of [
    ["MacIntel", "⌘E", "Meta+e"],
    ["Linux x86_64", "Ctrl+E", "Control+e"],
  ] as const) {
    const tab = await page.context().newPage()
    // isMac() reads userAgentData.platform before navigator.platform, so set both.
    await tab.addInitScript((p) => {
      Object.defineProperty(navigator, "platform", { get: () => p })
      Object.defineProperty(navigator, "userAgentData", { get: () => ({ platform: p }) })
    }, platform)
    await tab.goto(page.url())
    await expect(tab.getByRole("complementary", nav("Sidebar"))).toBeVisible()
    await expect(async () => {
      await tab.keyboard.press(key)
      await expect(tab.getByRole("menu", nav("Chords"))).toBeVisible({ timeout: 1_000 })
    }).toPass({ timeout: 10_000 })
    await expect(
      tab.getByRole("menu", nav("Chords")).getByText(chip, { exact: true }),
    ).toBeVisible()
    await tab.close()
  }
  // The Leader key opens the box at the bottom right.
  await goto(page, "#/epics")
  await expect(page).toHaveURL(/#\/epics$/)
  await expect(async () => {
    await page.keyboard.press(leader)
    await expect(chordBox).toBeVisible({ timeout: 1_000 })
  }).toPass({ timeout: 10_000 })
  const viewport = page.viewportSize()!
  const chordRect = (await chordBox.boundingBox())!
  expect(chordRect.x + chordRect.width).toBeGreaterThan(viewport.width / 2)
  expect(chordRect.y + chordRect.height).toBeGreaterThan(viewport.height / 2)
  // A mouse move leaves it open (no timer closes it).
  await page.mouse.move(300, 300)
  await expect(chordBox).toBeVisible()
  await page.keyboard.press("Escape")
  await expect(chordBox).toHaveCount(0)

  // In the Message box the Leader key opens the box and the draft stays; Esc and an unknown key close it and change nothing.
  await chatRow.click()
  await expect(prompt).toBeVisible()
  await prompt.fill("draft text")
  const chatUrl = page.url()
  await page.keyboard.press(leader)
  await expect(chordBox).toBeVisible()
  await expect(prompt).toHaveValue("draft text")
  await page.keyboard.press("Escape")
  await expect(chordBox).toHaveCount(0)
  await page.keyboard.press(leader)
  await expect(chordBox).toBeVisible()
  await page.keyboard.press("z")
  await expect(chordBox).toHaveCount(0)
  await expect(prompt).toHaveValue("draft text")
  expect(page.url()).toBe(chatUrl)
  await expect(page.getByRole("dialog")).toHaveCount(0)
  // Leader then c runs New chat and types no c; a modal open then keeps the Leader key shut.
  await page.keyboard.press(leader)
  await expect(chordBox).toBeVisible()
  await page.keyboard.press("c")
  const chordDialog = page.getByRole("dialog")
  await expect(chordDialog).toBeVisible()
  await expect(chordBox).toHaveCount(0)
  await page.keyboard.press(leader)
  await expect(chordDialog).toBeVisible()
  await expect(chordBox).toHaveCount(0)
  await expect(async () => {
    await page.keyboard.press("Escape")
    await expect(chordDialog).toHaveCount(0, { timeout: 1_000 })
  }).toPass({ timeout: 10_000 })
  await expect(prompt).toHaveValue("draft text")
  // With the command palette open the Leader key opens no box; the shell's own keys still work when closed.
  const chordPalette = page.getByRole("dialog", nav("Commands"))
  await page.keyboard.press("ControlOrMeta+p")
  await expect(chordPalette).toBeVisible()
  await page.keyboard.press(leader)
  await expect(chordPalette).toBeVisible()
  await expect(chordBox).toHaveCount(0)
  await page.keyboard.press("ControlOrMeta+p")
  await expect(chordPalette).toHaveCount(0)
  // With the box open, Cmd/Ctrl+K and Cmd/Ctrl+P close it and open nothing.
  for (const key of ["ControlOrMeta+k", "ControlOrMeta+p"]) {
    await page.keyboard.press(leader)
    await expect(chordBox).toBeVisible()
    await page.keyboard.press(key)
    await expect(chordBox).toHaveCount(0)
    await expect(page.getByRole("dialog")).toHaveCount(0)
  }
  // The large editor sheet keeps the Leader key: no box, and its text is unchanged.
  await prompt.click()
  await page
    .getByRole("form", nav("Composer"))
    .getByRole("button", nav("Open the note in a large editor"))
    .click()
  const sheet = page.getByRole("dialog", { name: /^Message to / })
  const sheetText = sheet.getByRole("textbox", { name: /text$/ })
  await expect(sheetText).toBeFocused()
  await expect(sheetText).toHaveText("draft text")
  await page.keyboard.press(leader)
  await expect(sheet).toBeVisible()
  await expect(chordBox).toHaveCount(0)
  await expect(sheetText).toHaveText("draft text")
  await sheet.getByRole("button", nav("Done")).click()
  await expect(sheet).toHaveCount(0)
  // On a Working chat the box takes the Esc: it closes and no interrupt confirm opens.
  await working.click()
  await expect(page.getByRole("status").filter({ hasText: /^Working/ })).toBeVisible()
  await page.getByRole("form", nav("Composer")).getByRole("textbox").click()
  await page.keyboard.press(leader)
  await expect(chordBox).toBeVisible()
  await page.keyboard.press("Escape")
  await expect(chordBox).toHaveCount(0)
  await expect(page.getByRole("alertdialog", nav("Interrupt the agent?"))).toHaveCount(0)
  // In the Questions view the Esc closes only the box: the view stays on Questions.
  await goto(page, "#/d/demo:design-new")
  await page.getByRole("button", nav("Start design session")).click()
  await expect(page.getByRole("group", nav("Questions"))).toBeVisible()
  await page.keyboard.press(leader)
  await expect(chordBox).toBeVisible()
  await page.keyboard.press("Escape")
  await expect(chordBox).toHaveCount(0)
  await expect(page.getByRole("group", nav("Questions"))).toBeVisible()
  await expect(
    page.getByRole("group", nav("View")).getByRole("button", { name: /^Questions/ }),
  ).toHaveAttribute("aria-pressed", "true")
  await goto(page, "#/epics")

  // The chords: c and d open the new chat and design forms, a click on the c row does the same, and the
  // palette's New chat opens the same form on the same project.
  const openBox = async () => {
    await expect(async () => {
      await page.keyboard.press(leader)
      await expect(chordBox).toBeVisible({ timeout: 1_000 })
    }).toPass({ timeout: 10_000 })
  }
  const chordForm = page.getByRole("dialog", { name: "New session" })
  const chordProject = chordForm.getByRole("combobox", nav("Project"))
  await goto(page, "#/d/side:side-design")
  await expect(page.getByRole("heading", { name: /side-design/ }).first()).toBeVisible()
  await openBox()
  await page.keyboard.press("c")
  await expect(chordForm).toBeVisible()
  await expect(chordProject).toHaveValue("side")
  await page.keyboard.press("Escape")
  await expect(chordForm).toHaveCount(0)
  await openBox()
  await chordBox.getByRole("menuitem", { name: /New chat/ }).click()
  await expect(chordForm).toBeVisible()
  await expect(chordProject).toHaveValue("side")
  await page.keyboard.press("Escape")
  await expect(chordForm).toHaveCount(0)
  await page.keyboard.press("ControlOrMeta+p")
  await page.getByRole("dialog", nav("Commands")).getByRole("combobox").fill("New chat")
  await page.keyboard.press("Enter")
  await expect(chordForm).toBeVisible()
  await expect(chordProject).toHaveValue("side")
  await page.keyboard.press("Escape")
  await expect(chordForm).toHaveCount(0)
  await openBox()
  await page.keyboard.press("d")
  const chordDesignSlug = page.getByRole("dialog").getByRole("textbox", nav("Epic slug"))
  await expect(chordDesignSlug).toBeVisible()
  await page.keyboard.press("Escape")
  await expect(chordDesignSlug).toHaveCount(0)

  // On a live chat: Leader then x asks first with Keep focused, Keep leaves the chat, Remove deletes it.
  await goto(page, "#/epics")
  await openBox()
  await page.keyboard.press("c")
  await chordForm.getByRole("textbox", nav("Name")).fill("chord-chat")
  await chordForm.getByRole("textbox", nav("First prompt")).fill("hi")
  await chordForm.getByRole("button", nav("Create session")).click()
  await expect(chordForm).toHaveCount(0)
  const chordChat = (await sessions(fx)).find((s) => s.name === "chord-chat")!
  const chordChatUrl = new RegExp(`#/agents/${chordChat.id}$`)
  await goto(page, `#/agents/${chordChat.id}`)
  await expect(page).toHaveURL(chordChatUrl)
  const removeRow = chordBox.getByRole("menuitem", { name: /^x\s*Remove/ })
  const removeConfirm = page.getByRole("dialog").filter({ hasText: "Stop and remove" })
  await openBox()
  await expect(removeRow).toContainText("Remove chat")
  const plainKey = chordBox.getByRole("menuitem", { name: /^c\s*New chat/ }).locator("b")
  await expect(plainKey).toBeVisible()
  const plainColor = await plainKey.evaluate((e) => getComputedStyle(e).color)
  await expect(removeRow.locator("b")).not.toHaveCSS("color", plainColor)
  await page.keyboard.press("x")
  await expect(removeConfirm).toBeVisible()
  await expect(removeConfirm.getByRole("button", nav("Keep"))).toBeFocused()
  await page.keyboard.press("Enter")
  await expect(removeConfirm).toHaveCount(0)
  await expect(page).toHaveURL(chordChatUrl)
  expect(await fx.cli("agents", "list")).toContain(chordChat.id)
  await openBox()
  await page.keyboard.press("x")
  await removeConfirm.getByRole("button", nav("Remove")).click()
  await expect(page).toHaveURL(/#\/agents$/)
  await expect.poll(() => fx.cli("agents", "list")).not.toContain(chordChat.id)

  // Design page, Queue and a Queue agent: the box has c, d and s only, and x does nothing.
  const queueAgentHash = (await queueAgent.getAttribute("href"))!
  for (const hash of ["#/d/side:side-design", queueAgentHash]) {
    await goto(page, hash)
    await openBox()
    await expect(chordBox.getByRole("menuitem")).toHaveCount(3)
    await expect(chordBox.getByRole("menuitem", { name: /^x/ })).toHaveCount(0)
    await page.keyboard.press("x")
    await expect(page.getByRole("dialog")).toHaveCount(0)
    await page.keyboard.press("Escape")
    await expect(chordBox).toHaveCount(0)
  }
  await goto(page, "#/epics")

  // On a Fix agent page the x row reads "Remove" and opens the same confirm.
  await working.click()
  await expect(working).toHaveAttribute("aria-current", "page")
  await openBox()
  await expect(chordBox.getByRole("menuitem", { name: /^x\s*Remove$/ })).toBeVisible()
  await page.keyboard.press("x")
  await expect(removeConfirm).toBeVisible()
  await removeConfirm.getByRole("button", nav("Keep")).click()
  await expect(removeConfirm).toHaveCount(0)
})
