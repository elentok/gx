import { PNG } from "./agentsKit"
import { draftPage } from "./draftKit"
import { expect, nav, swipe, test } from "./fixture"

// 375 px is below the wide layout's 62em breakpoint.
test.use({ viewport: { width: 375, height: 812 } })

// Phone 23 and 53a (the "united" scenario) live in jump.e2e.ts flow 22, which already pays the settle wait.
test.describe("53. Phone, epic page", () => {
  test("a card's meta sits below its title, and answer and fix buttons are full-width tap targets", async ({
    page,
    start,
  }) => {
    // Recovery would turn 07 into a waiting parent, so it stays off as in flow 2.
    const fixture = await start("mixed", { config: { "recovery.enabled": "false" } })
    const long = "A very long ticket title that would wrap onto a second line on a phone screen"
    await fixture.cli(
      "tickets",
      "add",
      "demo:mixed-epic",
      "--slug",
      "long-title",
      "--body",
      `# ${long}\n\n## What to build\n\nx`,
    )
    await page.evaluate(() => (location.hash = "#/e/demo:mixed-epic"))
    const link = page.getByRole("link", { name: "02 open ready" })
    await expect(link).toBeVisible()
    const title = (await link.getByText("open", nav("open")).boundingBox())!
    const meta = (await link.getByText("ready", nav("ready")).boundingBox())!
    expect(meta.y).toBeGreaterThanOrEqual(title.y + title.height)
    // The meta is indented 80px under the title.
    await expect(link.locator("[data-meta]")).toHaveCSS("padding-left", "80px")

    // A long title wraps, and its meta sits below the whole title.
    const longTitle = page.getByText(long, nav(long))
    await expect(longTitle).toBeVisible()
    const box = (await longTitle.boundingBox())!
    const lineHeight = await longTitle.evaluate((el) => parseFloat(getComputedStyle(el).lineHeight))
    expect(box.height).toBeGreaterThan(lineHeight * 1.5)
    const longMeta = (await longTitle
      .locator("xpath=ancestor::a")
      .locator("[data-meta]")
      .boundingBox())!
    expect(longMeta.y).toBeGreaterThanOrEqual(box.y + box.height)

    // The bar's buttons wrap to a full-width row of touch-sized buttons.
    const queue = page.getByRole("button", nav("Queue"))
    expect((await queue.boundingBox())!.height).toBeGreaterThanOrEqual(44)
    await queue.click()
    const answer = page.getByRole("button", nav("Answer"))
    const fix = page.getByRole("button", { name: "Fix it with an agent" })
    // 07 only starts once 02 and 03 have merged, each after the server's 3s finish debounce, as in flow 2.
    await expect(page.getByText("4/8")).toBeVisible({ timeout: 30_000 })
    await expect(answer).toBeVisible()
    await expect(fix).toBeVisible()

    const card = (button: typeof answer) =>
      page
        .locator("div")
        .filter({ has: button })
        .filter({ has: page.locator("a") })
        .last()
    // Each button is as wide as the note row it sits in, less the 80px indent.
    for (const button of [answer, fix]) {
      const row = (await button.locator("xpath=ancestor::*[@data-note-row]").boundingBox())!
      const box = (await button.boundingBox())!
      expect(box.height).toBeGreaterThanOrEqual(44)
      expect(box.width).toBeCloseTo(row.width - 80, 0)
    }

    // In 06's card the meta, question and Answer stack in that order, their text 80px from the content edge.
    const askCard = card(answer)
    const metaEl = askCard.locator("[data-meta]")
    await expect(metaEl).toHaveCSS("padding-left", "80px")
    const askMeta = (await metaEl.boundingBox())!
    const question = (await askCard
      .getByText("Which option should the fake worker take?")
      .boundingBox())!
    const ab = (await answer.boundingBox())!
    expect(askMeta.y + askMeta.height).toBeLessThanOrEqual(question.y)
    expect(question.y + question.height).toBeLessThanOrEqual(ab.y)
    expect(askMeta.x + 80).toBeCloseTo(question.x, 0)
    expect(ab.x).toBeCloseTo(question.x, 0)
  })
})

test("41. Phone, Design draft page", async ({ page, start }) => {
  await start("ready")
  const d = draftPage(page)
  await page.getByRole("button", nav("New design")).click()
  await expect(page).toHaveURL(/#\/drafts\/[^/]+$/)
  await expect(page.getByRole("dialog")).toHaveCount(0)
  await expect(d.prompt).toBeFocused()

  // The page stack matches the desktop order: header, project and slug bar, Prompt, then the footer.
  const clip = page.getByRole("button", nav("Add image"))
  await expect(clip).toHaveCount(1)
  await expect(d.start).toBeVisible()
  const [head, slugBox, promptBox, foot, rm, s, c] = [
    (await page.getByRole("heading", nav("Untitled design")).boundingBox())!,
    (await d.slug.boundingBox())!,
    (await d.prompt.boundingBox())!,
    (await d.remove.locator("xpath=..").boundingBox())!,
    (await d.remove.boundingBox())!,
    (await d.start.boundingBox())!,
    (await clip.boundingBox())!,
  ]
  expect(head.y + head.height).toBeLessThanOrEqual(slugBox.y)
  expect(slugBox.y + slugBox.height).toBeLessThanOrEqual(promptBox.y)
  expect(promptBox.y + promptBox.height).toBeLessThanOrEqual(foot.y)

  // The footer is the paperclip, Remove, and Start design at its right edge, each at natural width.
  const pad = 12
  expect(c.x + c.width).toBeLessThanOrEqual(rm.x)
  expect(s.x + s.width).toBeCloseTo(foot.x + foot.width - pad, 0)
  // Natural width: the label plus the button's padding, far short of the stretched half the footer once gave it.
  const content = await d.start.evaluate((el) => {
    const range = document.createRange()
    range.selectNodeContents(el)
    const cs = getComputedStyle(el)
    return (
      range.getBoundingClientRect().width +
      parseFloat(cs.paddingLeft) +
      parseFloat(cs.paddingRight) +
      parseFloat(cs.borderLeftWidth) +
      parseFloat(cs.borderRightWidth)
    )
  })
  expect(s.width).toBeCloseTo(content, 0)

  // With the prompt empty the one-line hint shows above the footer, and Start keeps its width.
  await expect(d.start).toBeDisabled()
  const hint = page.getByText("Write a prompt first")
  await expect(hint).toBeVisible()
  const h = (await hint.boundingBox())!
  expect(h.height).toBeLessThan(24)
  expect(h.y + h.height).toBeLessThanOrEqual(foot.y)
  expect((await d.start.boundingBox())!.width).toBeCloseTo(s.width, 0)

  // A file picked through the paperclip gets a thumbnail; a click opens the viewer and closing it starts no design.
  const starts: string[] = []
  page.on("request", (r) => r.url().includes("/v1/designs/open") && starts.push(r.url()))
  const chooser = page.waitForEvent("filechooser")
  await clip.click()
  await (await chooser).setFiles({ name: "picked.png", mimeType: "image/png", buffer: PNG })
  const open = page.getByRole("button", { name: "Open image" })
  await expect(open).toHaveCount(1)
  await open.click()
  const closeViewer = page.getByRole("button", { name: /^Close ✕/ })
  await expect(closeViewer).toBeVisible()
  await closeViewer.click()
  await expect(closeViewer).toBeHidden()
  await expect(page).toHaveURL(/#\/drafts\/[^/]+$/)
  expect(starts).toEqual([])

  // At 744px (tablet) the paperclip is still before Remove.
  await page.setViewportSize({ width: 744, height: 1000 })
  await expect(d.remove).toBeVisible()
  const [rm2, c2] = [(await d.remove.boundingBox())!, (await clip.boundingBox())!]
  expect(c2.x + c2.width).toBeLessThanOrEqual(rm2.x)
})

test("24. Phone, Design page", async ({ page, start }) => {
  const fixture = await start("design", { autoAdvanceDelay: "9s" })

  await page.evaluate(() => (location.hash = "#/d/demo:design-epic"))
  await page.getByRole("button", nav("Start design session")).click()
  // The Questions view is up: the first message is hidden.
  const view = page.getByRole("group", nav("View"))
  const chatText = page.getByText("Two questions before the spec.")
  await expect(view.getByRole("button", { name: /^Questions/ })).toHaveAttribute(
    "aria-pressed",
    "true",
  )
  await expect(chatText).toBeHidden()

  // A step line under the name and the ring open the steps sheet; there is no segmented bar.
  await expect(page.getByRole("region", nav("Progress"))).toHaveCount(0)
  const stepLine = page.getByRole("button", { name: /^Resolve tickets · / })
  const steps = page.getByRole("dialog", nav("Design progress steps"))
  const current = steps.locator('li[data-step="resolve-tickets"][data-state="current"]')
  await stepLine.click()
  await expect(current).toBeVisible()
  await steps.getByRole("button", nav("Close")).click()
  await expect(steps).toHaveCount(0)
  await page.getByRole("button", nav("Show progress steps")).click()
  await expect(current).toBeVisible()
  await page.keyboard.press("Escape")
  await expect(steps).toHaveCount(0)
  await stepLine.click()
  await expect(steps).toBeVisible()
  await page.mouse.click(187, 20)
  await expect(steps).toHaveCount(0)

  // The View toggle replaces the focus buttons: Chat shows the first message, Questions hides it.
  await expect(page.getByRole("button", { name: /focus mode/ })).toHaveCount(0)
  await view.getByRole("button", nav("Chat")).click()
  await expect(chatText).toBeVisible()
  await view.getByRole("button", { name: /^Questions/ }).click()
  await expect(chatText).toBeHidden()

  // Send with gaps: the popover fits the screen and overlaps the sheet.
  const questions = page.getByRole("group", nav("Questions"))
  // Send is dimmed until one question is answered; the other stays open, so the gaps popover shows.
  await expect(page.getByRole("button", nav("Send"))).toBeDisabled()
  await page.getByRole("button", { name: /^SQLite/ }).click()
  await page.getByRole("button", nav("Send")).click()
  const popover = page.getByRole("alertdialog", nav("Unanswered questions"))
  await expect(popover).toBeVisible()
  // It scales in, so it is measured once settled.
  await popover.evaluate((el) => Promise.all(el.getAnimations().map((a) => a.finished)))
  const pop = (await popover.boundingBox())!
  const sheetBox = (await questions.boundingBox())!
  expect(pop.width).toBeLessThanOrEqual(343)
  expect(pop.x).toBeGreaterThanOrEqual(0)
  expect(pop.x + pop.width).toBeLessThanOrEqual(375)
  expect(pop.y).toBeLessThan(sheetBox.y + sheetBox.height)
  expect(pop.y + pop.height).toBeGreaterThan(sheetBox.y)
  await page.keyboard.press("Escape")
  await expect(popover).toHaveCount(0)

  // The question cards own horizontal swipes: a swipe right never opens the sidebar, a swipe left moves to the next
  // question. A wide code block is not in the fixture, so the vitest case in useSidebarSwipe covers a scroller.
  const sidebar = page.getByRole("complementary", nav("Sidebar"))
  const prev = page.getByRole("button", nav("← Prev"))
  const next = page.getByRole("button", nav("Next →"))
  // Picking SQLite moved to the second card; go back to the first.
  await expect(prev).toBeEnabled()
  await prev.click()
  await expect(prev).toBeDisabled()
  const cardsBox = (await questions.getByTestId("question-body").boundingBox())!
  const cardY = cardsBox.y + 40
  await swipe(page, [150, cardY], [300, cardY + 5])
  await expect(sidebar).toBeHidden()
  await expect(prev).toBeDisabled()
  await swipe(page, [340, cardY], [40, cardY + 5])
  await expect(prev).toBeEnabled()
  await expect(next).toBeDisabled()
  await expect(sidebar).toBeHidden()

  // With a single question (the next round) a swipe right still leaves the sidebar closed.
  await questions.getByRole("button", { name: /Yes/ }).click()
  await page.getByRole("button", nav("Send")).click()
  await expect(questions.getByText("0 of 1 answered")).toBeVisible()
  const single = (await questions.getByTestId("question-body").boundingBox())!
  await swipe(page, [150, single.y + 40], [300, single.y + 45])
  await expect(sidebar).toBeHidden()
  await expect(questions.getByText("0 of 1 answered")).toBeVisible()

  // The step line stays above the sheet in the Questions view.
  const line = (await stepLine.boundingBox())!
  expect(line.y + line.height).toBeLessThanOrEqual((await questions.boundingBox())!.y)

  // Note is an icon with no text; the Auto button shows both short names.
  await expect(page.getByRole("button", nav("Note"))).toHaveText("")
  await expect(page.getByRole("button", { name: /^Auto:/ })).toHaveText("AdvanceQueue")

  // The footer is one row at 390px: Add, Note, Auto, Send, with 44px Add and Note.
  await page.setViewportSize({ width: 390, height: 844 })
  const row = ["Add image", "Note"].map((n) => page.getByRole("button", nav(n)))
  const auto = page.getByRole("button", { name: /^Auto:/ })
  const send = page.getByRole("button", nav("Send"))
  const buttons = [...row, auto, send]
  for (const b of buttons) await expect(b).toBeVisible()
  // The footer reflows on the next frame, so read all four boxes together until they share one row.
  let boxes: { x: number; y: number; width: number; height: number }[] = []
  await expect
    .poll(async () => {
      boxes = (await Promise.all(buttons.map((b) => b.boundingBox()))).map((b) => b!)
      const ys = boxes.map((b) => b.y + b.height / 2)
      return Math.max(...ys) - Math.min(...ys) < 0.5
    })
    .toBe(true)
  for (const b of boxes.slice(0, 2)) expect([b.width, b.height]).toEqual([44, 44])
  for (let i = 1; i < boxes.length; i++)
    expect(boxes[i].x).toBeGreaterThanOrEqual(boxes[i - 1].x + boxes[i - 1].width)
  // The paperclip is at the left edge of the footer, which has no separator and 12px above, none below.
  const footer = page.getByRole("form", nav("Composer"))
  const footerBox = (await footer.boundingBox())!
  expect(boxes[0].x - footerBox.x).toBeLessThan(boxes[1].x - footerBox.x)
  await expect(footer).toHaveCSS("border-top-width", "0px")
  await expect(footer).toHaveCSS("padding-top", "12px")
  await expect(footer).toHaveCSS("padding-bottom", "0px")

  // Keyboard: a visual viewport 300px shorter while the question box is focused drops the top bar, title block
  // and images and leaves the tabs with the ⋯; restoring the viewport and leaving focus brings them back.
  const title = page.getByRole("heading", { level: 1 })
  const more = page.getByRole("button", nav("More actions"))
  const topBar = page.getByRole("button", { name: "Show sidebar", exact: true })
  await page.getByRole("button", nav("Note")).click()
  const box = page.getByRole("textbox", nav("Message"))
  await box.focus()
  const setViewport = (shrink: number) =>
    page.evaluate((px) => {
      const vv = window.visualViewport!
      Object.defineProperty(vv, "height", {
        configurable: true,
        get: () => window.innerHeight - px,
      })
      vv.dispatchEvent(new Event("resize"))
    }, shrink)
  await expect(title).toBeVisible()
  await setViewport(300)
  await expect(title).toBeHidden()
  await expect(topBar).toBeHidden()
  await expect(page.getByLabel("Images")).toHaveCount(0)
  await expect(view).toBeVisible()
  await expect(more).toBeVisible()
  await expect(view.locator("xpath=../..").getByRole("button", nav("More actions"))).toBeVisible()
  await setViewport(0)
  await box.blur()
  await expect(title).toBeVisible()
  await expect(topBar).toBeVisible()
  await expect(more).toHaveCount(1)

  // The ⋯ menu holds Effort, Tier and the link to the design epic; the model is in the footer, without effort.
  await more.click()
  const actions = page.getByRole("menu", nav("More actions"))
  await expect(actions.getByText("Effort", { exact: true })).toBeVisible()
  await expect(actions.getByText("Tier", { exact: true })).toBeVisible()
  await expect(actions.getByText("Model", { exact: true })).toHaveCount(0)
  await expect(page.getByText("Claude Sonnet", { exact: true })).toBeVisible()
  await actions.getByRole("menuitem", { name: /^Open design epic/ }).click()
  await expect(page).toHaveURL(/#\/e\/demo:design-epic$/)
  await page.goBack()
  await expect(view).toBeVisible()

  // Auto-queue: the menu shows "Queue" and flips; the countdown strip sits above the footer with a 40px+ Cancel.
  await auto.click()
  const menu = page.getByRole("dialog", nav("Auto"))
  await expect(menu.getByRole("switch", { name: "Auto-queue off" })).toBeEnabled()
  await menu.getByRole("switch", { name: "Auto-queue off" }).click()
  await page.keyboard.press("Escape")
  await expect(page.getByRole("button", { name: /Auto-queue on$/ })).toHaveText("AdvanceQueue")
  await fixture.cli("tickets", "set", "demo:design-epic", "--design-step", "branch")
  const strip = page.locator('[data-part="countdown-strip"]')
  await expect(strip).toContainText(/Queuing design-epic in \ds/)
  const cancel = strip.getByRole("button", nav("Cancel auto-queue"))
  expect((await cancel.boundingBox())!.height).toBeGreaterThanOrEqual(40)
  expect((await strip.boundingBox())!.y + (await strip.boundingBox())!.height).toBeLessThanOrEqual(
    (await send.boundingBox())!.y + 1,
  )

  // Cmd/Ctrl+K opens the jump menu as a full-screen sheet; its close button closes it.
  await page.keyboard.press("ControlOrMeta+k")
  const sheet = page.getByRole("dialog", nav("Jump to"))
  await expect(sheet).toBeVisible()
  expect(await sheet.boundingBox()).toMatchObject({ x: 0, y: 0, width: 390, height: 844 })
  await sheet.getByRole("button", nav("Close")).click()
  await expect(sheet).toHaveCount(0)
})
