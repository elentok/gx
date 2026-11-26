import { chatOf, row, sessions, titleHeading } from "./agentsKit"
import { chordKit } from "./chordKit"
import { draftPage } from "./draftKit"
import { expect, nav, test } from "./fixture"
import { goto, sidebarKit, stillMotion } from "./sidebarKit"

// The Leader key's Chord box over the `workspaces` scenario; split from the Workspaces flow so each stays inside its time limit.
test("Chords", async ({ page, start, context }) => {
  test.setTimeout(120_000)
  // y copies the chat's ID; the flow reads it back.
  await context.grantPermissions(["clipboard-read", "clipboard-write"])
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
  const { box: chordBox, leader, open: openBox } = chordKit(page)
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
  // Leader then C (Shift+c) runs New chat and types no c; a modal open then keeps the Leader key shut.
  await page.keyboard.press(leader)
  await expect(chordBox).toBeVisible()
  await page.keyboard.press("Shift+C")
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

  // The chords: C (Shift+c) and d open the new chat form and a design draft, a click on the C row does the same, and the
  // palette's New chat opens the same form on the same project.
  const draftIds = async () =>
    (await fx.cli("designs", "drafts", "list"))
      .split("\n")
      .filter((l) => l.includes("\t"))
      .map((l) => l.split("\t")[0]!.trim())
  const chordForm = page.getByRole("dialog", { name: "New chat" })
  const chordProject = chordForm.getByRole("combobox", nav("Project"))
  await goto(page, "#/d/side:side-design")
  await expect(page.getByRole("heading", { name: /side-design/ }).first()).toBeVisible()
  await openBox()
  await page.keyboard.press("Shift+C")
  await expect(chordForm).toBeVisible()
  await expect(chordProject).toHaveValue("side")
  await page.keyboard.press("Escape")
  await expect(chordForm).toHaveCount(0)
  await openBox()
  await chordBox.getByRole("menuitem", { name: /^C\s*New chat$/ }).click()
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
  // d opens a draft page with the Prompt focused, and no dialog.
  const draft = draftPage(page)
  await expect(page).toHaveURL(/#\/drafts\/[^/]+$/)
  await expect(draft.prompt).toBeFocused()
  await expect(page.getByRole("dialog")).toHaveCount(0)
  expect(await draftIds()).toHaveLength(2)

  // On the empty draft: Leader then x removes it at once and leaves for the Epics page.
  await openBox()
  await expect(chordBox.getByRole("menuitem", { name: /^x\s*Remove$/ })).toBeVisible()
  await page.keyboard.press("x")
  await expect(page).toHaveURL(/#\/epics$/)
  await expect(page.getByRole("dialog")).toHaveCount(0)
  await expect.poll(draftIds).toHaveLength(1)
  // The draft page is gone before the next chord reads the page it is on.
  await expect(draft.prompt).toHaveCount(0)

  // On a non-empty draft: r focuses the slug field; x asks first, Keep leaves it, Remove deletes it.
  await openBox()
  await page.keyboard.press("d")
  await expect(page).toHaveURL(/#\/drafts\/[^/]+$/)
  await expect(draft.prompt).toBeFocused()
  await draft.prompt.fill("A chord idea.")
  await expect.poll(draftIds).toHaveLength(2)
  await openBox()
  await page.keyboard.press("r")
  await expect(draft.slug).toBeFocused()
  // A chord has no button to anchor on, so the confirm is a modal dialog.
  const draftRemove = page.getByRole("dialog", { name: /^Remove / })
  await draft.prompt.focus()
  await openBox()
  await page.keyboard.press("x")
  await expect(draftRemove).toBeVisible()
  await draftRemove.getByRole("button", { name: /^Cancel/ }).click()
  await expect(draftRemove).toHaveCount(0)
  await expect(page).toHaveURL(/#\/drafts\/[^/]+$/)
  expect(await draftIds()).toHaveLength(2)
  await openBox()
  await page.keyboard.press("x")
  await draftRemove.getByRole("button", { name: /^Remove/ }).click()
  await expect(page).toHaveURL(/#\/epics$/)
  await expect.poll(draftIds).toHaveLength(1)
  await expect(sidebar.getByRole("link", { name: /Untitled design|A chord idea/ })).toHaveCount(0)
  await expect(page.getByRole("heading", { name: "Epics", level: 1 })).toBeVisible()

  // On a live chat: Leader then x asks first with Cancel focused, Cancel leaves the chat, Remove deletes it.
  await goto(page, "#/epics")
  await openBox()
  await page.keyboard.press("Shift+C")
  await chordForm.getByRole("textbox", nav("Title")).fill("chord-chat")
  await chordForm.getByRole("textbox", nav("First prompt")).fill("hi")
  await chordForm.getByRole("button", nav("Create chat")).click()
  await expect(chordForm).toHaveCount(0)
  const chordChat = (await sessions(fx)).find((s) => s.name === "chord-chat")!
  const chordChatUrl = new RegExp(`#/agents/${chordChat.id}$`)
  await goto(page, `#/agents/${chordChat.id}`)
  await expect(page).toHaveURL(chordChatUrl)

  // Rename chat: Leader then r opens the popup on the open chat with its name selected, Enter renames it
  // everywhere without a reload and types /rename into the Claude chat, Esc and an empty name change nothing.
  const renameDialog = page.getByRole("dialog", { name: "Rename chat" })
  const renameField = renameDialog.getByRole("textbox", nav("Title"))
  const renameButton = renameDialog.getByRole("button", nav("Rename"))
  const otherChat = (await sessions(fx)).find(
    (s) => s.id !== chordChat.id && !s.design && !s.queue && s.harness !== "codex" && s.name,
  )!
  await expect
    .poll(async () => (await sessions(fx)).find((s) => s.id === chordChat.id)?.status.state)
    .toBe("idle")
  const chordConv = (await chatOf(fx, chordChat.id)).conversation_id
  await page.evaluate(() => ((window as unknown as { __kept?: boolean }).__kept = true))
  await prompt.fill("a draft")
  await openBox()
  await expect(chordBox.getByRole("menuitem", { name: /^r\s*Rename$/ })).toBeVisible()
  await page.keyboard.press("r")
  await expect(renameDialog).toBeVisible()
  await expect(renameField).toBeFocused()
  await expect(renameField).toHaveValue("chord-chat")
  expect(
    await renameField.evaluate((e: HTMLInputElement) => [e.selectionStart, e.selectionEnd]),
  ).toEqual([0, "chord-chat".length])
  await expect(prompt).toHaveValue("a draft")
  await page.keyboard.press("Tab")
  await expect(renameDialog.getByRole("button", nav("Cancel"))).toBeFocused()
  await page.keyboard.press("Tab")
  await expect(renameButton).toBeFocused()
  await renameField.fill("")
  await expect(renameButton).toBeDisabled()
  await renameField.fill("zzz")
  await page.keyboard.press("Escape")
  await expect(renameDialog).toHaveCount(0)
  await expect(row(page, chordChat)).toContainText("chord-chat")
  await expect(prompt).toHaveValue("a draft")
  await prompt.fill("")
  await openBox()
  await page.keyboard.press("r")
  await renameField.fill("chord-renamed")
  await page.keyboard.press("Enter")
  await expect(renameDialog).toHaveCount(0)
  await expect(row(page, chordChat)).toContainText("chord-renamed")
  await expect(titleHeading(page)).toContainText("chord-renamed")
  await expect.poll(() => fx.cli("agents", "list")).toContain("chord-renamed")
  await expect.poll(() => fx.input(chordConv)).toContain("/rename chord-renamed")
  expect(await page.evaluate(() => (window as unknown as { __kept?: boolean }).__kept)).toBe(true)
  expect((await sessions(fx)).find((s) => s.id === otherChat.id)!.name).toBe(otherChat.name)
  // The palette's Rename row opens the same popup on the same chat.
  await page.keyboard.press("ControlOrMeta+p")
  await page.getByRole("dialog", nav("Commands")).getByRole("combobox").fill("Rename")
  await page.keyboard.press("Enter")
  await expect(renameDialog).toBeVisible()
  await expect(renameField).toHaveValue("chord-renamed")
  await renameField.fill("chord-palette")
  await page.keyboard.press("Enter")
  await expect(row(page, chordChat)).toContainText("chord-palette")
  await expect(titleHeading(page)).toContainText("chord-palette")
  expect((await sessions(fx)).find((s) => s.id === otherChat.id)!.name).toBe(otherChat.name)

  // A refusal shows its reason in the popup and keeps the text: the chat is removed from the CLI while the popup is open.
  await goto(page, "#/epics")
  await openBox()
  await page.keyboard.press("Shift+C")
  await chordForm.getByRole("textbox", nav("Title")).fill("refuse-chat")
  await chordForm.getByRole("textbox", nav("First prompt")).fill("hi")
  await chordForm.getByRole("button", nav("Create chat")).click()
  await expect(chordForm).toHaveCount(0)
  const refuseChat = (await sessions(fx)).find((s) => s.name === "refuse-chat")!
  await goto(page, `#/agents/${refuseChat.id}`)
  await expect(titleHeading(page)).toContainText("refuse-chat")
  await openBox()
  await page.keyboard.press("r")
  await renameField.fill("refused-name")
  await fx.cli("agents", "remove", "--yes", refuseChat.id)
  await page.keyboard.press("Enter")
  await expect(renameDialog.getByRole("alert")).toBeVisible()
  await expect(renameField).toHaveValue("refused-name")
  await page.keyboard.press("Escape")
  await expect(renameDialog).toHaveCount(0)
  await goto(page, `#/agents/${chordChat.id}`)
  await expect(page).toHaveURL(chordChatUrl)
  const removeRow = chordBox.getByRole("menuitem", { name: /^x\s*Remove/ })
  const removeConfirm = page.getByRole("dialog").filter({ hasText: "Stop and remove" })
  await openBox()
  await expect(removeRow).toHaveText(/^x\s*Remove$/)
  // The chat's own chords come first, a separator, then the app-level group.
  await expect(chordBox.getByRole("separator")).toHaveCount(1)
  for (const key of ["r", "m", "w", "y", "x"]) {
    await expect(chordBox.getByRole("menuitem", { name: new RegExp(`^${key}\\s`) })).toBeVisible()
  }
  const plainKey = chordBox.getByRole("menuitem", { name: /^c\s*New chat/ }).locator("b")
  await expect(plainKey).toBeVisible()
  const plainColor = await plainKey.evaluate((e) => getComputedStyle(e).color)
  await expect(removeRow.locator("b")).not.toHaveCSS("color", plainColor)
  await page.keyboard.press("Escape")
  // y copies the ID and toasts it; w opens the workspace picker, m the model picker.
  await openBox()
  await page.keyboard.press("y")
  await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe(chordChat.id)
  await expect(page.getByText(`Copied ${chordChat.id}`)).toBeVisible()
  await openBox()
  await page.keyboard.press("w")
  const movePicker = page.getByRole("dialog", nav("Move to workspace"))
  await expect(movePicker).toBeVisible()
  await page.keyboard.press("Escape")
  await expect(movePicker).toHaveCount(0)
  await openBox()
  await page.keyboard.press("m")
  const modelPicker = page.getByRole("dialog", nav("Switch model"))
  await expect(modelPicker).toBeVisible()
  await page.keyboard.press("Escape")
  await expect(modelPicker).toHaveCount(0)
  // x confirms in a modal: n cancels, y confirms.
  await openBox()
  await page.keyboard.press("x")
  await expect(removeConfirm).toBeVisible()
  await expect(removeConfirm.getByRole("button", { name: /^Cancel/ })).toBeFocused()
  await page.keyboard.press("n")
  await expect(removeConfirm).toHaveCount(0)
  await expect(page).toHaveURL(chordChatUrl)
  expect(await fx.cli("agents", "list")).toContain(chordChat.id)
  await openBox()
  await page.keyboard.press("x")
  await expect(removeConfirm).toBeVisible()
  await page.keyboard.press("y")
  await expect(page).toHaveURL(/#\/agents$/)
  await expect.poll(() => fx.cli("agents", "list")).not.toContain(chordChat.id)

  // Design page and a Queue agent: no r, m or x (y copies the ID, and w and a need a workspace card), and x does nothing.
  // The palette lists no Rename there; its New chat row proves the filter ran.
  const expectNoRenameInPalette = async () => {
    await page.keyboard.press("ControlOrMeta+p")
    const commands = page.getByRole("dialog", nav("Commands"))
    await commands.getByRole("combobox").fill("chat")
    await expect(commands.getByText("New chat")).toBeVisible()
    await expect(commands.getByText("Rename")).toHaveCount(0)
    await page.keyboard.press("Escape")
    await expect(commands).toHaveCount(0)
  }
  const queueAgentHash = (await queueAgent.getAttribute("href"))!
  for (const hash of ["#/d/side:side-design", queueAgentHash]) {
    await goto(page, hash)
    await openBox(chordBox.getByRole("menuitem", { name: /^c\s*New chat/ }))
    await expect(chordBox.getByRole("menuitem", { name: /^y\s*Copy ID$/ })).toBeVisible()
    for (const key of ["x", "r", "m"]) {
      await expect(chordBox.getByRole("menuitem", { name: new RegExp(`^${key}\\s`) })).toHaveCount(
        0,
      )
    }
    await page.keyboard.press("x")
    await expect(page.getByRole("dialog")).toHaveCount(0)
    await page.keyboard.press("Escape")
    await expect(chordBox).toHaveCount(0)
    await expectNoRenameInPalette()
  }
  // The epics page and the agents list are not a chat page either.
  for (const hash of ["#/epics", "#/agents"]) {
    await goto(page, hash)
    await openBox(chordBox.getByRole("menuitem", { name: /^c\s*New chat/ }))
    await expect(chordBox.getByRole("menuitem", { name: /^r/ })).toHaveCount(0)
    // Switch model is a chat page chord: no row, and the key opens no picker.
    await expect(chordBox.getByRole("menuitem", { name: /^m/ })).toHaveCount(0)
    await page.keyboard.press("m")
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
  await expect(chordBox.getByRole("menuitem", { name: /^r/ })).toHaveCount(0)
  await page.keyboard.press("x")
  await expect(removeConfirm).toBeVisible()
  await removeConfirm.getByRole("button", { name: /^Cancel/ }).click()
  await expect(removeConfirm).toHaveCount(0)
  await expectNoRenameInPalette()

  // Back and forward chords, on a new tab with its own history.
  const tab = await context.newPage()
  await tab.goto(new URL(page.url()).origin, { waitUntil: "domcontentloaded" })
  await expect(tab).toHaveURL(/#\/epics$/)
  await stillMotion(tab)
  const { sidebar: tabSidebar, row: tabRow } = sidebarKit(tab)
  const chords = chordKit(tab)
  const epics = /#\/epics$/
  const ready = /#\/e\/demo:ready-epic$/
  // The address changes before the new page renders, and that render closes an open box.
  const landed = async (url: RegExp, title: string) => {
    await expect(tab).toHaveURL(url)
    await expect(tab.getByRole("heading", { level: 1, name: title })).toBeVisible()
  }
  await expect(tabRow("ready-epic")).toBeVisible()
  await chords.open(chords.next)
  await expect(chords.back).toHaveCount(0)
  await expect(chords.forward).toHaveCount(0)
  await tab.keyboard.press("Escape")
  await tabRow("ready-epic").click()
  await landed(ready, "ready-epic")
  await chords.open(chords.back)
  await expect(chords.forward).toHaveCount(0)
  await tab.keyboard.press("Comma")
  await landed(epics, "Epics")
  await chords.open(chords.forward)
  await expect(chords.back).toHaveCount(0)
  await tab.keyboard.press("Period")
  await landed(ready, "ready-epic")
  // Leader then Shift+, is no chord: the box closes and the page stays.
  await chords.open(chords.back)
  await tab.keyboard.press("Shift+Comma")
  await expect(chords.box).toHaveCount(0)
  await expect(tab).toHaveURL(ready)
  await tabSidebar.getByRole("button", nav("Hide sidebar")).click()
  await expect(tabSidebar).toBeHidden()
  await chords.open(chords.back)
  await tab.keyboard.press("Comma")
  await landed(epics, "Epics")
  await chords.open(chords.forward)
  await tab.keyboard.press("Period")
  await landed(ready, "ready-epic")
  await expect(tabSidebar).toBeHidden()
  await tab.getByRole("button", nav("Show sidebar")).click()
  // A step back to a chat opened with Enter focuses its prompt again.
  const tabPrompt = tab.getByRole("form", nav("Composer")).getByRole("textbox")
  await tabSidebar.locator("a[data-chat]").first().focus()
  await tab.keyboard.press("Enter")
  await expect(tab).toHaveURL(/#\/agents\/[^/]+$/)
  const enteredChat = tab.url()
  await expect(tabPrompt).toBeFocused()
  await tabRow("ready-epic").click()
  await landed(ready, "ready-epic")
  await expect(tabPrompt).toHaveCount(0)
  await chords.open(chords.back)
  await tab.keyboard.press("Comma")
  await expect(tab).toHaveURL(enteredChat)
  await expect(tabPrompt).toBeFocused()
})
