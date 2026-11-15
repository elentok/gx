import type { Page } from "@playwright/test"
import { composer, go, messageBox, sessions } from "./agentsKit"
import { card, expect, type Fixture, menu, menuButton, nav, openMenu, test, touch } from "./fixture"

// Most vi-editor behaviour (open keys, closers, Done, backdrop, mode text, bar, vim motions and
// edits) is covered in vitest under src/ui/expandable-text. These flows keep what needs a real
// server or a real browser layout.

const TOLERANCE = 1

// `gx vimode` changes the server; the page follows, which the menu switch shows.
const setViMode = async (fixture: Fixture, page: Page, on: boolean) => {
  await fixture.cli("vimode", on ? "on" : "off")
  await openMenu(page)
  await expect(menuSwitch(page)).toHaveAttribute("aria-checked", String(on))
  // The menu's dim covers the button, so Escape closes it once focus is inside the menu.
  await expect(async () => {
    await page.keyboard.press("Escape")
    await expect(menu(page)).toHaveCount(0, { timeout: 500 })
  }).toPass()
  // A sidebar that slid over a narrow page would block the page behind its scrim.
  if (await page.getByTestId("scrim").isVisible())
    await page.getByRole("button", nav("Hide sidebar")).click()
  await expect(page.getByTestId("scrim")).toHaveCount(0)
}

type Locator = ReturnType<Page["getByRole"]>

const MESSAGE = "Message to the agent"

// Opens the design-working message box in the large editor.
const openEditor = async (page: Page, width: number, height = 900) => {
  await page.setViewportSize({ width, height })
  await expect(card(page, "design-epic")).toBeVisible()
  await page.evaluate(() => (location.hash = "#/d/demo:design-epic"))
  await page.getByRole("button", nav("Start design session")).click()
  // The first turn disables the field; wait it out so the field stays enabled for the whole test.
  const working = page.getByRole("status").filter({ hasText: "Working:" })
  await expect(working).toBeVisible()
  await expect(working).toBeHidden({ timeout: 45_000 })
  await page.getByRole("button", nav("Note")).click()
  await page.getByRole("button", nav("Open the note in a large editor")).click()
  const dialog = page.getByRole("dialog", nav(MESSAGE))
  await expect(dialog).toBeVisible()
  await expect(dialog.getByRole("textbox", nav(`${MESSAGE} text`))).toBeVisible()
  return dialog
}

const expectDoneCloses = async (page: Page, dialog: Locator) => {
  const done = dialog.getByRole("button", nav("Done"))
  await expect(done).toBeVisible()
  await done.click()
  await expect(page.getByRole("dialog")).toHaveCount(0)
}

// Opens the design-working editor with `text` in the field and the caret after `caret` characters.
const openWith = async (page: Page, text: string, caret: number) => {
  await expect(card(page, "design-epic")).toBeVisible()
  await page.evaluate(() => (location.hash = "#/d/demo:design-epic"))
  await page.getByRole("button", nav("Start design session")).click()
  // The session is idle for a moment before its first turn starts, and that turn disables the field.
  // Wait the turn out so the field stays enabled for the whole test.
  const working = page.getByRole("status").filter({ hasText: "Working:" })
  await expect(working).toBeVisible()
  await expect(working).toBeHidden({ timeout: 45_000 })
  await page.getByRole("button", nav("Note")).click()
  const field = page.getByRole("textbox", nav("Message"))
  await expect(field).toBeEnabled({ timeout: 45_000 })
  // With vi on the field is the inline editor, which has no value to fill: type the text and walk the caret.
  await field.click()
  await page.keyboard.insertText(text)
  await page.keyboard.press("ControlOrMeta+Home")
  for (let i = 0; i < caret; i++) await page.keyboard.press("ArrowRight")
  await page.keyboard.press("ControlOrMeta+g")
  const dialog = page.getByRole("dialog", nav(MESSAGE))
  await expect(dialog.getByRole("textbox", nav(`${MESSAGE} text`))).toBeFocused()
  return { field, dialog }
}

test.describe("124. Vi editor: open, close and submit", () => {
  test.use({ viewport: { width: 1280, height: 900 } })

  test("Cmd-Enter in the agent composer's editor closes it and sends; in a Working session the first Esc leaves insert mode and the second opens the interrupt confirm", async ({
    page,
    start,
  }) => {
    const fixture = await start("agents")
    const idle = (await sessions(fixture))[0]
    await setViMode(fixture, page, true)
    await go(page, `#/agents/${idle.id}`)
    await messageBox(page).fill("sent from the editor")
    await composer(page).getByRole("button", nav("Open the note in a large editor")).click()
    const dialog = page.getByRole("dialog", { name: /^Message to / })
    await expect(dialog.getByRole("textbox", { name: /text$/ })).toBeFocused()
    const sent = page.waitForRequest(
      (r) => r.method() === "POST" && r.url().includes("/v1/sessions/prompt"),
    )
    await page.keyboard.press("ControlOrMeta+Enter")
    await sent
    await expect(page.getByRole("dialog")).toHaveCount(0)
    await expect
      .poll(() => fixture.cli("agents", "chat", idle.id))
      .toContain("sent from the editor")
    // A Working session: the first Esc leaves insert mode, the second opens the interrupt confirm.
    const working = (await sessions(fixture))[1]
    await go(page, `#/agents/${working.id}`)
    await expect(page.getByRole("status").filter({ hasText: /^Working/ })).toBeVisible()
    await composer(page).getByRole("button", nav("Open the note in a large editor")).click()
    const workingDialog = page.getByRole("dialog", { name: /^Message to / })
    await expect(workingDialog.getByRole("textbox", { name: /text$/ })).toBeFocused()
    const confirm = page.getByRole("alertdialog", nav("Interrupt the agent?"))
    await page.keyboard.press("Escape")
    await expect(confirm).toHaveCount(0)
    await expect(workingDialog).toBeVisible()
    await page.keyboard.press("Escape")
    await expect(confirm).toBeVisible()
    await expect(confirm.getByRole("button", { name: "Interrupt", exact: true })).toBeFocused()
  })
})

test.describe("123. Vi editor: layouts and touch bar", () => {
  // Also covers 126 "Cmd-K does not open the Jump menu over the editor".
  test("1280px mouse: a 860 by 680 modal; mode text left of the footer; Cmd-K over it; 1280px touch is at most 680px high", async ({
    page,
    start,
  }) => {
    const fixture = await start("design-working")
    await setViMode(fixture, page, true)
    const dialog = await openEditor(page, 1280, 900)
    const box = (await dialog.boundingBox())!
    expect(Math.abs(box.width - 860)).toBeLessThanOrEqual(TOLERANCE)
    expect(Math.abs(box.height - 680)).toBeLessThanOrEqual(TOLERANCE)
    await expect(dialog.getByText(MESSAGE, { exact: true })).toBeVisible()
    await expect(dialog.getByText("Saved as you type")).toBeVisible()
    await expect(dialog.getByRole("button", nav("Done"))).toBeVisible()
    await expect(dialog.getByRole("button")).toHaveCount(1)

    const text = dialog.getByRole("status")
    await expect(text).toHaveText("INSERT")
    const saved = (await dialog.getByText("Saved as you type").boundingBox())!
    expect((await text.boundingBox())!.x).toBeLessThan(saved.x)

    const jump = page.getByRole("dialog", nav("Jump to"))
    await page.keyboard.press("ControlOrMeta+k")
    await expect(jump).toHaveCount(0)
    await expect(dialog).toBeVisible()

    // The same open editor on a touch device: the bar shows and the modal is shorter.
    const phone = await page
      .context()
      .browser()!
      .newContext({
        ...touch,
        ignoreHTTPSErrors: true,
        viewport: { width: 1280, height: 900 },
        storageState: await page.context().storageState(),
      })
    const second = await phone.newPage()
    await second.goto(page.url())
    await second.getByRole("button", nav("Note")).click()
    await second.getByRole("button", nav("Open the note in a large editor")).click()
    const touched = second.getByRole("dialog", nav(MESSAGE))
    await expect(touched.getByTestId("editor-bar")).toBeVisible()
    expect((await touched.boundingBox())!.height).toBeLessThanOrEqual(680 + TOLERANCE)
    await phone.close()

    await expectDoneCloses(page, dialog)
    await page.keyboard.press("ControlOrMeta+k")
    await expect(jump).toBeVisible()
  })
})

// The editor text, one line per `.cm-line`.
const editorText = async (editable: Locator) => {
  await expect(editable).toBeVisible()
  return editable.evaluate((el) =>
    [...el.querySelectorAll(".cm-line")].map((l) => l.textContent).join("\n"),
  )
}

// [anchor, head] of the editor's selection as character offsets in the text; `\n` counts as one.
const editorSelection = async (editable: Locator) => {
  await expect(editable).toBeVisible()
  return editable.evaluate((el) => {
    const lines = [...el.querySelectorAll(".cm-line")]
    const offset = (node: Node, at: number) => {
      const line = lines.findIndex((l) => l.contains(node))
      let n = 0
      for (let i = 0; i < line; i++) n += (lines[i].textContent ?? "").length + 1
      const range = document.createRange()
      range.setStart(lines[line], 0)
      range.setEnd(node, at)
      return n + range.toString().length
    }
    const s = document.getSelection()!
    return [offset(s.anchorNode!, s.anchorOffset), offset(s.focusNode!, s.focusOffset)]
  })
}

const head = async (editable: Locator) => (await editorSelection(editable))[1]

test.describe("125. Vi editor: commands", () => {
  // Also covers "a click resets the column that j and k keep".
  test("390px: j moves one screen line inside a wrapped paragraph; a click resets the column j keeps", async ({
    page,
    start,
  }) => {
    const fixture = await start("design-working")
    await page.setViewportSize({ width: 390, height: 800 })
    await setViMode(fixture, page, true)
    const paragraph = "word ".repeat(100).trim()
    const text = `${paragraph}\naaaaaaaaaa\nbbbbbbbbbb\ncccccccccc\ndddddddddd`
    const { dialog } = await openWith(page, text, 0)
    const editable = dialog.getByRole("textbox", nav(`${MESSAGE} text`))
    const mode = dialog.locator('[data-testid^="vi-mode-"]')
    await page.keyboard.press("Escape")
    await expect(mode).toHaveText("NORMAL")
    await page.keyboard.type("gg")

    const cursor = dialog.locator(".cm-cursor, .cm-fat-cursor").first()
    await expect(cursor).toBeVisible()
    const before = (await cursor.boundingBox())!
    await page.keyboard.press("j")
    await expect.poll(() => head(editable)).toBeGreaterThan(0)
    expect(await head(editable)).toBeLessThan(paragraph.length / 2)
    const lineHeight = await editable
      .locator(".cm-line")
      .first()
      .evaluate((l) => parseFloat(getComputedStyle(l).lineHeight))
    // The cursor element repaints after the selection changes, so wait for it to move.
    await expect
      .poll(async () => Math.abs(((await cursor.boundingBox())?.y ?? NaN) - before.y - lineHeight))
      .toBeLessThanOrEqual(2)

    // Lines a to d start at paragraph.length + 1 and are 11 characters apart.
    const a = paragraph.length + 1
    await editable
      .locator(".cm-line")
      .nth(1)
      .click({ position: { x: 1, y: 5 } })
    await expect.poll(() => head(editable)).toBe(a)
    await page.keyboard.type("5lj")
    await expect.poll(() => head(editable)).toBe(a + 11 + 5)
    await editable
      .locator(".cm-line")
      .nth(3)
      .click({ position: { x: 1, y: 5 } })
    await expect.poll(() => head(editable)).toBe(a + 22)
    await page.keyboard.press("j")
    await expect.poll(() => head(editable)).toBe(a + 33)
  })
})

// Presses `key` `times` times; after each press focus must sit inside `layer`.
const expectFocusStays = async (page: Page, layer: Locator, key: string, times: number) => {
  for (let i = 0; i < times; i++) {
    await page.keyboard.press(key)
    await expect.poll(() => layer.evaluate((el) => el.contains(document.activeElement))).toBe(true)
  }
}

const PAUSED_TICKET = "#/t/demo:paused-epic/08"
const START_OVER = "Start over reason"

// Opens the start-over dialog on the paused ticket; returns it.
const openStartOver = async (page: Page) => {
  await page.evaluate((hash) => (location.hash = hash), PAUSED_TICKET)
  await page
    .getByRole("form", { name: "Recovery actions" })
    .getByRole("button", nav("Start over"))
    .click()
  const dialog = page.getByRole("dialog", nav("Start over"))
  await expect(dialog).toBeVisible()
  return dialog
}

// Opens the large editor over the Reason field of the start-over dialog.
const openReasonEditor = async (page: Page, dialog: Locator) => {
  await dialog.getByRole("button", nav("Open the note in a large editor")).click()
  const editor = page.getByRole("dialog", nav(START_OVER))
  await expect(editor.getByRole("textbox", nav(`${START_OVER} text`))).toBeFocused()
  return editor
}

test.describe("126. Vi editor: layers and focus", () => {
  test.use({ viewport: { width: 1280, height: 900 } })

  test("Escape in the editor over the question sheet closes neither", async ({ page, start }) => {
    // Kept as e2e: the Escape routing between the editor and the page's Questions view needs the
    // whole design page, which a component test would only imitate.
    const fixture = await start("design", { autoAdvanceDelay: "1h" })
    await setViMode(fixture, page, true)
    await expect(card(page, "design-epic")).toBeVisible()
    await page.evaluate(() => (location.hash = "#/d/demo:design-epic"))
    await page.getByRole("button", nav("Start design session")).click()
    const questions = page.getByRole("group", nav("Questions"))
    // Answering Q8 moves the sheet on to Q8b.
    await questions.getByRole("button", { name: /SQLite/ }).click()
    await questions.getByRole("button", nav("Note on question 8b")).click()
    await questions.getByRole("button", nav("Open the note in a large editor")).click()
    const editor = page.getByRole("dialog", nav("Note on Q8b - Migrate at startup?"))
    await expect(
      editor.getByRole("textbox", nav("Note on Q8b - Migrate at startup? text")),
    ).toBeFocused()
    await page.keyboard.press("Escape")
    await expect(editor).toBeVisible()
    await expect(questions).toBeVisible()
    // Escape in the Questions view would show the chat; it did not.
    await expect(
      page.getByRole("group", nav("View")).getByRole("button", { name: /^Questions/ }),
    ).toHaveAttribute("aria-pressed", "true")
  })

  // Also covers the touch Tab flows: Tab cycles editor, Esc and Done; Shift-Tab wraps to Done.
  test("Tab and Shift-Tab never leave the editor; Tab leaves no tab character; touch cycles editor, Esc, Done", async ({
    page,
    start,
  }) => {
    const fixture = await start("design-working")
    await setViMode(fixture, page, true)
    const dialog = await openEditor(page, 1280, 900)
    const editable = dialog.getByRole("textbox", nav(`${MESSAGE} text`))
    await expect(editable).toBeFocused()
    await page.keyboard.type("ab")
    await expectFocusStays(page, dialog, "Tab", 6)
    await expectFocusStays(page, dialog, "Shift+Tab", 6)
    await editable.focus()
    await page.keyboard.press("Tab")
    expect(await editable.evaluate((el) => el.textContent)).not.toContain("\t")
    await expect(editable).toHaveText("ab")

    // The same open editor on a touch device, where the bar adds Esc and Done to the cycle.
    const phone = await page
      .context()
      .browser()!
      .newContext({
        ...touch,
        ignoreHTTPSErrors: true,
        viewport: { width: 1280, height: 900 },
        storageState: await page.context().storageState(),
      })
    const second = await phone.newPage()
    await second.goto(page.url())
    // The "ab" draft is shared across tabs, so the Note box is already open here.
    await second.getByRole("button", nav("Open the note in a large editor")).click()
    const touched = second.getByRole("dialog", nav(MESSAGE))
    const touchedEditable = touched.getByRole("textbox", nav(`${MESSAGE} text`))
    await expect(touchedEditable).toBeFocused()
    const seen = new Set<string>()
    for (let i = 0; i < 6; i++) {
      await second.keyboard.press("Tab")
      await expect
        .poll(() => touched.evaluate((el) => el.contains(document.activeElement)))
        .toBe(true)
      seen.add(
        await second.evaluate(
          () =>
            document.activeElement?.textContent ||
            document.activeElement?.getAttribute("aria-label") ||
            "editor",
        ),
      )
    }
    expect([...seen]).toEqual(expect.arrayContaining(["Done", "Esc"]))
    await expectFocusStays(second, touched, "Shift+Tab", 6)
    await touchedEditable.focus()
    await second.keyboard.press("Shift+Tab")
    await expect(touched.getByRole("button", { name: "Done" })).toBeFocused()
    await second.keyboard.press("Tab")
    await expect(touchedEditable).toBeFocused()
    await phone.close()
  })

  // Also covers "a backdrop press closes only the editor over the start-over dialog" and "Tab never
  // leaves a plain modal".
  test("Escape closes the start-over dialog, Tab stays in it, and the editor over it keeps both open", async ({
    page,
    start,
  }) => {
    const fixture = await start("paused")
    await setViMode(fixture, page, true)
    const dialog = await openStartOver(page)
    await expectFocusStays(page, dialog, "Tab", 8)
    await expectFocusStays(page, dialog, "Shift+Tab", 8)
    await page.keyboard.press("Escape")
    await expect(dialog).toHaveCount(0)

    const again = await openStartOver(page)
    const editor = await openReasonEditor(page, again)
    await page.keyboard.press("Escape")
    await expect(editor).toBeVisible()
    await expect(again).toBeVisible()

    // Last, so a failure here hides nothing above. The press lands beside the 860px panel, mid-height,
    // on the right edge: the docked sidebar covers the left one.
    await page.mouse.click(page.viewportSize()!.width - 5, 450)
    await expect(editor).toHaveCount(0)
    await expect(again).toBeVisible()
  })
})

const menuSwitch = (page: Page) => menu(page).getByRole("menuitemcheckbox", nav("Vi mode"))

test.describe("127. Vi editor: setting", () => {
  test.use({ viewport: { width: 1280, height: 900 } })

  // Also covers "the menu has a Vi mode switch that turns on" and "a second page of the same login
  // shows the switch on".
  test("flipping the setting with the editor open shows and hides vi at once", async ({
    page,
    start,
  }) => {
    const fixture = await start("design-working")
    await expect(card(page, "design-epic")).toBeVisible()
    await menuButton(page).click()
    const item = menuSwitch(page)
    await expect(item).toHaveAttribute("aria-checked", "false")
    await item.click()
    await expect(item).toHaveAttribute("aria-checked", "true")
    const second = await page.context().newPage()
    await second.goto(page.url())
    await expect(card(second, "design-epic")).toBeVisible()
    await menuButton(second).click()
    await expect(menuSwitch(second)).toHaveAttribute("aria-checked", "true")
    await second.close()
    await menuButton(page).click()
    await expect(menu(page)).toHaveCount(0)

    const { dialog } = await openWith(page, "hello", 2)
    const editable = dialog.getByRole("textbox", nav(`${MESSAGE} text`))
    const mode = dialog.getByTestId("vi-mode-text")
    const same = async () => {
      expect(await editorText(editable)).toBe("hello")
      expect(await editorSelection(editable)).toEqual([2, 2])
    }
    await expect(mode).toHaveText("INSERT")
    await same()

    await fixture.cli("vimode", "off")
    await expect(mode).toHaveCount(0)
    await expect(editable).toBeFocused()
    await same()
    // Without vi, Escape does nothing and `x` types a letter.
    await page.keyboard.press("Escape")
    await page.keyboard.press("x")
    await expect(dialog).toBeVisible()
    expect(await editorText(editable)).toBe("hexllo")
    await page.keyboard.press("Backspace")
    await same()

    await fixture.cli("vimode", "on")
    await expect(mode).toHaveText("INSERT")
    await same()
    await page.keyboard.press("Escape")
    await expect(mode).toHaveText("NORMAL")
    await page.keyboard.press("x")
    // Vi `x` deletes the character under the caret instead of typing.
    expect(await editorText(editable)).toBe("hllo")
  })
})

test.describe("206. Inline vi box and big prompt box", () => {
  test.use({ viewport: { width: 1280, height: 900 } })

  test("the Message box is a vi editor with a mode pill; Leader b doubles it in every page; Cmd-G hands it to the large editor and back; vi off gives the textarea back", async ({
    page,
    start,
  }) => {
    test.setTimeout(120_000)
    const fixture = await start("workspaces")
    await setViMode(fixture, page, true)
    const sidebar = page.getByRole("complementary", nav("Sidebar"))
    const chatRow = sidebar.locator("a[data-chat]").first()
    await expect(chatRow).toBeVisible()
    const chatHash = (await chatRow.getAttribute("href"))!
    await chatRow.click()
    const box = messageBox(page)
    // The scroller is the text area itself; the editor around it also holds the vim panel.
    const area = composer(page).locator(".cm-scroller")
    const pill = composer(page).getByTestId("vi-mode-text")
    const chordBox = page.getByRole("menu", nav("Chords"))
    const leader = "ControlOrMeta+e"
    const height = async () => {
      await expect(area).toBeVisible()
      return (await area.boundingBox())!.height
    }
    // The editor is `rows` lines plus 20px of padding; the big box doubles the rows, not the padding.
    const PADDING = 20
    const expectHeight = (want: number) =>
      expect
        .poll(async () => Math.abs((await height()) - want), { message: `want ${want}` })
        .toBeLessThanOrEqual(TOLERANCE)
    const dialog = page.getByRole("dialog", { name: /^Message to / })
    const dialogText = dialog.getByRole("textbox", { name: /text$/ })
    const dialogMode = dialog.getByTestId("vi-mode-text")

    // Vi on: the Message box is the editor and the pill reads INSERT.
    await expect(box).toBeVisible()
    await expect(composer(page).locator("textarea")).toHaveCount(0)
    await expect(pill).toHaveText("INSERT")
    await box.click()
    await expect(box).toBeFocused()

    // Esc gives NORMAL, v VISUAL, Esc then i INSERT; typed text appears; dd empties it.
    await page.keyboard.press("Escape")
    await expect(pill).toHaveText("NORMAL")
    await page.keyboard.press("v")
    await expect(pill).toHaveText("VISUAL")
    await page.keyboard.press("Escape")
    await page.keyboard.press("i")
    await expect(pill).toHaveText("INSERT")
    await page.keyboard.type("hello world")
    await expect(box).toHaveText("hello world")
    await page.keyboard.press("Escape")
    await page.keyboard.press("d")
    await page.keyboard.press("d")
    await expect(box).not.toContainText("hello")
    await expect(pill).toHaveText("NORMAL")

    // `/` opens the panel inside the box; `:q` says so and changes nothing else.
    await page.keyboard.press("/")
    await expect(composer(page).locator(".cm-vim-panel input")).toBeVisible()
    await page.keyboard.press("Escape")
    await expect(composer(page).locator(".cm-vim-panel input")).toHaveCount(0)
    await page.keyboard.press(":")
    await page.keyboard.type("q")
    await page.keyboard.press("Enter")
    await expect(composer(page)).toContainText("Not an editor command")
    await expect(pill).toHaveText("NORMAL")
    await expect(box).not.toContainText("hello")

    // With the chat Working, Esc in INSERT opens no confirm; the next Esc in NORMAL does.
    await sidebar.getByRole("button", nav("Expand paused-epic")).click()
    const working = sidebar
      .getByRole("link", { name: /^(Fix agent )?paused-epic(?![\w-])/ })
      .locator("xpath=ancestor::li[1]")
      .locator("ul")
      .getByRole("link")
      .filter({ hasText: "Working" })
    await working.click()
    await expect(page.getByRole("status").filter({ hasText: /^Working/ })).toBeVisible()
    await box.click()
    await page.keyboard.press("i")
    await expect(pill).toHaveText("INSERT")
    const confirm = page.getByRole("alertdialog", nav("Interrupt the agent?"))
    await page.keyboard.press("Escape")
    await expect(pill).toHaveText("NORMAL")
    await expect(confirm).toHaveCount(0)
    await page.keyboard.press("Escape")
    await expect(confirm).toBeVisible()
    await confirm.getByRole("button", nav("Cancel")).click()
    await expect(confirm).toHaveCount(0)

    // Back to the idle chat. Leader then b in INSERT doubles the box; text, caret and mode stay.
    await go(page, chatHash)
    await expect(box).toBeVisible()
    await box.click()
    await page.keyboard.press("i")
    await expect(pill).toHaveText("INSERT")
    await page.keyboard.type("alpha beta gamma")
    await expect(box).toHaveText("alpha beta gamma")
    const small = await height()
    const big = (small - PADDING) * 2 + PADDING
    await expect.poll(() => editorSelection(box)).toEqual([16, 16])
    await page.keyboard.press(leader)
    await expect(chordBox.getByRole("menuitem", { name: /^b\s*Big prompt box/ })).toBeVisible()
    await page.keyboard.press("b")
    await expect(chordBox).toHaveCount(0)
    await expectHeight(big)
    await expect(box).toHaveText("alpha beta gamma")
    await expect(pill).toHaveText("INSERT")
    await expect(box).toBeFocused()
    expect(await editorSelection(box)).toEqual([16, 16])
    await page.keyboard.press(leader)
    await expect(chordBox.getByRole("menuitem", { name: /^b\s*Normal prompt box/ })).toBeVisible()
    await page.keyboard.press("Escape")
    await expect(chordBox).toHaveCount(0)

    // A second page in the same context opens with the box already big, no reload.
    const second = await page.context().newPage()
    await second.goto(page.url())
    await expect(messageBox(second)).toBeVisible()
    await expect
      .poll(async () =>
        Math.abs((await composer(second).locator(".cm-scroller").boundingBox())!.height - big),
      )
      .toBeLessThanOrEqual(TOLERANCE)
    await second.close()

    // The large editor opens in INSERT. Leader then b over it changes nothing.
    await page.keyboard.press("ControlOrMeta+g")
    await expect(dialogText).toBeFocused()
    await expect(dialogMode).toHaveText("INSERT")
    await page.keyboard.press("Escape")
    await expect(dialogMode).toHaveText("NORMAL")
    await page.keyboard.press(leader)
    await page.keyboard.press("b")
    await expect(chordBox).toHaveCount(0)
    await expect(dialog).toBeVisible()
    await expect(dialogText).toHaveText("alpha beta gamma")
    await expectHeight(big)
    // Cmd-G closes it with the caret back in the box.
    const caret = await editorSelection(dialogText)
    await page.keyboard.press("ControlOrMeta+g")
    await expect(dialog).toHaveCount(0)
    await expect(box).toBeFocused()
    await expect(pill).toHaveText("INSERT")
    expect(await editorSelection(box)).toEqual(caret)

    // From NORMAL the pill reads NORMAL after; the large editor still opens in INSERT.
    await page.keyboard.press("Escape")
    await expect(pill).toHaveText("NORMAL")
    await page.keyboard.press("ControlOrMeta+g")
    await expect(dialogMode).toHaveText("INSERT")
    await page.keyboard.press("ControlOrMeta+g")
    await expect(dialog).toHaveCount(0)
    await expect(box).toBeFocused()
    await expect(pill).toHaveText("NORMAL")
    // From a VISUAL selection the pill reads NORMAL after and the selection is the caret.
    await page.keyboard.press("0")
    await page.keyboard.press("v")
    await page.keyboard.press("e")
    await expect(pill).toHaveText("VISUAL")
    await expect
      .poll(async () => {
        const [a, b] = await editorSelection(box)
        return a !== b
      })
      .toBe(true)
    await page.keyboard.press("ControlOrMeta+g")
    await expect(dialogMode).toHaveText("INSERT")
    await page.keyboard.press("ControlOrMeta+g")
    await expect(dialog).toHaveCount(0)
    await expect(pill).toHaveText("NORMAL")
    await expect(box).toBeFocused()
    await expect
      .poll(async () => {
        const [a, b] = await editorSelection(box)
        return a === b
      })
      .toBe(true)

    // The focus ring shows while the editor has focus and is gone after blur.
    const ring = () =>
      area.evaluate((el) => {
        for (let n = el.parentElement; n; n = n.parentElement)
          if (getComputedStyle(n).boxShadow.includes("2px")) return true
        return false
      })
    await expect.poll(ring).toBe(true)
    await box.blur()
    await expect(box).not.toBeFocused()
    await expect.poll(ring).toBe(false)

    // Sending with Cmd-Enter empties the box and the pill returns to INSERT.
    await box.click()
    await page.keyboard.press("Escape")
    await expect(pill).toHaveText("NORMAL")
    const sent = page.waitForRequest(
      (r) => r.method() === "POST" && r.url().includes("/v1/sessions/prompt"),
    )
    await page.keyboard.press("ControlOrMeta+Enter")
    await sent
    await expect(box).not.toContainText("alpha")
    await expect(pill).toHaveText("INSERT")
    const busy = page.getByRole("status").filter({ hasText: /^Working/ })
    await expect(busy).toBeHidden({ timeout: 45_000 })

    // Touch at the same size: a textarea and no pill. A mouse at 390 px still shows the editor.
    await box.click()
    await page.keyboard.type("keep this text")
    await expect(box).toHaveText("keep this text")
    const phone = await page
      .context()
      .browser()!
      .newContext({
        ...touch,
        ignoreHTTPSErrors: true,
        viewport: { width: 1280, height: 900 },
        storageState: await page.context().storageState(),
      })
    const touched = await phone.newPage()
    await touched.goto(page.url())
    await expect(composer(touched).locator("textarea")).toBeVisible()
    await expect(composer(touched).getByTestId("vi-mode-text")).toHaveCount(0)
    await phone.close()
    await page.setViewportSize({ width: 390, height: 900 })
    await expect(messageBox(page)).toBeVisible()
    await expect(area).toBeVisible()
    await expect(composer(page).locator("textarea")).toHaveCount(0)
    await page.setViewportSize({ width: 1280, height: 900 })

    // Vi off turns the box back into a textarea with the same text.
    await fixture.cli("vimode", "off")
    const plain = composer(page).locator("textarea")
    await expect(plain).toHaveValue("keep this text")
    await expect(pill).toHaveCount(0)
  })
})

test.describe("207. Inline vi box in the Questions view", () => {
  test.use({ viewport: { width: 1280, height: 900 } })

  test("n and p stay text in INSERT; Esc keeps or leaves the box without closing the view; a text answer's Esc lands on its row and Cmd-Enter sends", async ({
    page,
    start,
  }) => {
    // Kept as e2e: vitest (PromptBox.test.tsx, QuestionSheet.test.tsx) cannot see the capture-phase
    // window Esc handler in ChatLog, real focus after Esc, or a real server answer round trip.
    test.setTimeout(120_000)
    const viewButton = (name: RegExp) =>
      page.getByRole("group", nav("View")).getByRole("button", { name })

    // Part 1: the note box of Q8b.
    const fixture = await start("design", { autoAdvanceDelay: "1h" })
    await setViMode(fixture, page, true)
    await expect(card(page, "design-epic")).toBeVisible()
    await page.evaluate(() => (location.hash = "#/d/demo:design-epic"))
    await page.getByRole("button", nav("Start design session")).click()
    const questions = page.getByRole("group", nav("Questions"))
    await questions.getByRole("button", { name: /SQLite/ }).click()
    await questions.getByRole("button", nav("Note on question 8b")).click()
    const note = questions.getByRole("textbox", { name: /Note text for question 8b/ })
    const pill = questions.getByTestId("vi-mode-text")
    await note.click()
    await expect(note).toBeFocused()
    await expect(pill).toHaveText("INSERT")
    await page.keyboard.type("np")
    await expect(note).toHaveText("np")
    await expect(note).toBeFocused()
    await expect(questions.getByRole("group", { name: /^Question .*8b of / })).toBeVisible()
    // Esc in INSERT only leaves INSERT: the box keeps focus and the view stays.
    await page.keyboard.press("Escape")
    await expect(pill).toHaveText("NORMAL")
    await expect(note).toBeFocused()
    await expect(questions).toBeVisible()
    await expect(viewButton(/^Questions/)).toHaveAttribute("aria-pressed", "true")
    // Esc in NORMAL leaves the box; the Questions view stays open.
    await page.keyboard.press("Escape")
    await expect(note).not.toBeFocused()
    await expect(questions).toBeVisible()
    await expect(viewButton(/^Questions/)).toHaveAttribute("aria-pressed", "true")

    // Part 2: a text question on a second fixture.
    const second = await start("design-text", { autoAdvanceDelay: "1h" })
    await setViMode(second, page, true)
    await expect(card(page, "design-epic")).toBeVisible()
    await page.evaluate(() => (location.hash = "#/d/demo:design-epic"))
    await page.getByRole("button", nav("Start design session")).click()
    // The first turn must finish before the sheet shows.
    const working = page.getByRole("status").filter({ hasText: "Working:" })
    await expect(working).toBeVisible()
    await expect(working).toBeHidden({ timeout: 45_000 })
    const sheet = page.getByRole("group", nav("Questions"))
    await expect(sheet.getByText("Name the migration")).toBeVisible()
    const answer = sheet.getByRole("textbox")
    await expect(answer).toBeVisible()
    await expect(sheet.getByTestId("vi-mode-text")).toHaveText("INSERT")
    await answer.click()
    await expect(answer).toBeFocused()
    await page.keyboard.type("add_users")
    await expect(answer).toHaveText("add_users")
    await page.keyboard.press("Escape")
    await expect(sheet.getByTestId("vi-mode-text")).toHaveText("NORMAL")
    await page.keyboard.press("Escape")
    await expect(answer).not.toBeFocused()
    // The text question still has its "Huh?" button, so focus lands on it, inside the question's own row.
    const row = sheet.getByRole("group", { name: /^Question .*13 of 1$/ })
    await expect(row.getByRole("button", { name: "Huh?" })).toBeFocused()
    await expect(viewButton(/^Questions/)).toHaveAttribute("aria-pressed", "true")
    // The box has no submit of its own, so Cmd-Enter reaches the sheet and sends.
    await answer.click()
    await expect(answer).toBeFocused()
    await expect(page.getByRole("button", nav("Send answers"))).toBeEnabled()
    await page.keyboard.press("ControlOrMeta+Enter")
    // The server takes the answers and the agent's next turn starts.
    await expect(page.getByRole("status").filter({ hasText: "Working:" })).toBeVisible()
  })
})
