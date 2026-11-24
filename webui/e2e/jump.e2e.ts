import { card, expect, menu, menuButton, nav, test } from "./fixture"
import { draftPage, renameDraft } from "./draftKit"
import { goto, sidebarKit } from "./sidebarKit"

test("22. Back link count, then the phone layout", async ({ page, start }) => {
  const fx = await start("united")
  const { closeMenus, menuFocused, openSubmenu, row, rowMenu } = sidebarKit(page)
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
  // Settings redirects to its first section after it loads; a goto sent before that is overwritten.
  await expect(page).toHaveURL(/#\/settings\/s\//)

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
  await closeMenus()

  // Clicking the count text itself opens the Epics menu, whose Epics item opens the page.
  await back(9).getByText("9", { exact: true }).click()
  await menuFocused()
  await page.getByRole("menuitem", { name: /^Epics/ }).click()
  await expect(page).toHaveURL(/#\/epics$/)

  // Design drafts: New design from the Command palette makes one draft and opens its page, and the list has no draft rows.
  const draftIds = async () =>
    (await fx.cli("designs", "drafts", "list"))
      .split("\n")
      .filter((l) => l.includes("\t"))
      .map((l) => l.split("\t")[0]!.trim())
  const d = draftPage(page)
  const newDraft = async (slug: string, prompt: string, onDraft = false) => {
    await page.keyboard.press("ControlOrMeta+p")
    const newDesign = rows.filter({ hasText: /^New design/ })
    await expect(newDesign).toBeVisible()
    // On a draft page the entity rows come first; Start design is the page's button, never a row.
    await expect(rows.filter({ hasText: /^Start design/ })).toHaveCount(0)
    if (onDraft)
      await expect(rows.filter({ hasText: /^(Rename|Move to workspace|Remove)/ })).toHaveCount(3)
    else await expect(rows.filter({ hasText: /^(Rename|Move|Remove)/ })).toHaveCount(0)
    await newDesign.click()
    await expect(palette).toHaveCount(0)
    await expect(page).toHaveURL(/#\/drafts\/[^/]+$/)
    await expect(d.prompt).toBeFocused()
    await d.prompt.fill(prompt)
    await renameDraft(page, slug)
    await expect(page.getByText("Saved")).toBeVisible()
  }
  await newDraft("near-draft", "A near idea.")
  const nearUrl = page.url()
  expect(await draftIds()).toHaveLength(1)
  await newDraft("far-draft", "A far idea.", true)
  expect(await draftIds()).toHaveLength(2)
  await goto(page, "#/epics")

  // Cmd/Ctrl+K with an empty filter lists the active workspace's drafts in a Design drafts section.
  const jumpRow = (title: string) => jump.getByRole("option").filter({ hasText: title })
  const drafts = jump.getByRole("group", nav("Design drafts"))
  await page.keyboard.press("ControlOrMeta+k")
  await expect(jump).toBeVisible()
  await expect(drafts.getByRole("option")).toHaveText([/^far-draft/, /^near-draft/])
  // The draft row has the icon, the label and "Draft", and its menu lists Rename, Move to workspace and Remove.
  const near = jumpRow("near-draft")
  await expect(near.getByRole("img", nav("Design draft"))).toBeVisible()
  await expect(near).toContainText("Draft")
  await near.getByRole("button", nav("More actions")).click()
  await menuFocused()
  await expect(page.getByRole("menuitem")).toHaveText([/^Rename/, /^Move to workspace/, /^Remove/])
  await page.keyboard.press("Escape")
  await expect(page.getByRole("menu")).toHaveCount(0)
  // Escape closes the menu and the jump menu with it.
  await expect(jump).toHaveCount(0)
  await page.keyboard.press("ControlOrMeta+k")
  await expect(jump).toBeVisible()

  // A draft of another workspace is absent until a search, then shows under Other workspaces with the workspace name.
  await fx.cli("server", "workspaces", "create", "scratch")
  await jumpRow("far-draft").getByRole("button", nav("More actions")).click()
  await menuFocused()
  await openSubmenu(
    page.getByRole("menuitem", nav("Move to workspace")),
    page.getByRole("menuitem", nav("scratch")),
  )
  await page.getByRole("menuitem", nav("scratch")).click()
  await expect
    .poll(async () =>
      JSON.parse(await fx.cli("server", "workspaces", "list", "--json")).workspaces.map(
        (w: { name: string; members: number }) => `${w.name}:${w.members}`,
      ),
    )
    .toContain("scratch:1")
  await expect(jumpRow("far-draft")).toHaveCount(0)
  await jump.getByRole("combobox").fill("far-dr")
  await expect(jump.getByRole("group", nav("Other workspaces")).getByRole("option")).toHaveText([
    /^far-draft.*scratch/,
  ])

  // Search finds a draft by its label and by its project name.
  await jump.getByRole("combobox").fill("near-dr")
  await expect(jumpRow("near-draft")).toBeVisible()
  const project = (await fx.cli("designs", "drafts", "list")).split("\t")[1]!
  await jump.getByRole("combobox").fill(project)
  await expect(jumpRow("near-draft")).toBeVisible()

  // Remove from the row menu names the draft and deletes it.
  await jump.getByRole("combobox").fill("far-dr")
  await jumpRow("far-draft").getByRole("button", nav("More actions")).click()
  await menuFocused()
  await page.getByRole("menuitem", nav("Remove")).click()
  const confirm = page.getByRole("dialog", nav("Remove far-draft"))
  await confirm.getByRole("button", { name: /^Remove/ }).click()
  await expect(confirm).toHaveCount(0)
  await expect.poll(draftIds).toHaveLength(1)

  // The confirm is a modal, so the jump menu closed before it opened. Picking the draft row opens its page.
  await expect(jump).toHaveCount(0)
  await page.keyboard.press("ControlOrMeta+k")
  await expect(jump).toBeVisible()
  await jump.getByRole("combobox").fill("near-dr")
  await jumpRow("near-draft").click()
  await expect(page).toHaveURL(nearUrl)

  // Cmd/Ctrl+P on the Epic page lists the epic's registry actions above the eight commands.
  await goto(page, "#/e/demo:running-epic")
  await page.keyboard.press("ControlOrMeta+p")
  await expect(rows.filter({ hasText: /^Hold/ })).toBeVisible()
  await expect(rows.filter({ hasText: /^Take off hold/ })).toHaveCount(0)
  const epicRows = await rows.allInnerTexts()
  expect(epicRows.findIndex((t) => t.startsWith("Hold"))).toBeLessThan(
    epicRows.findIndex((t) => t.startsWith("New design")),
  )
  await expect(rows.last()).toHaveText(/^Toggle AFK/)
  await page.keyboard.press("Escape")
  await expect(palette).toHaveCount(0)

  // On the Design page it offers Stop and Hold.
  await goto(page, "#/d/demo:design-live")
  await page.keyboard.press("ControlOrMeta+p")
  await expect(rows.filter({ hasText: /^Stop/ })).toBeVisible()
  await expect(rows.filter({ hasText: /^Hold/ })).toBeVisible()
  await page.keyboard.press("Escape")
  await expect(palette).toHaveCount(0)

  // Cmd/Ctrl+K rows: an epic offers Hold, a Design offers Stop with one live session, and a chat draft offers Rename, Move to workspace and Remove.
  await page.keyboard.press("ControlOrMeta+e")
  await page.keyboard.press("c")
  await expect(page).toHaveURL(/#\/chat-drafts\/[^/]+$/)
  await page.keyboard.press("ControlOrMeta+k")
  await expect(jump).toBeVisible()
  const rowMenuItems = async (title: string) => {
    await jumpRow(title).getByRole("button", nav("More actions")).click()
    await menuFocused()
    const items = await page.getByRole("menuitem").allInnerTexts()
    await page.keyboard.press("Escape")
    await expect(jump).toHaveCount(0)
    await page.keyboard.press("ControlOrMeta+k")
    await expect(jump).toBeVisible()
    return items
  }
  expect(await rowMenuItems("running-epic")).toContainEqual(expect.stringMatching(/^Hold/))
  expect(await rowMenuItems("design-live")).toContainEqual(expect.stringMatching(/^Stop/))
  const chatDraftRow = jump.getByRole("group", nav("Chat drafts")).getByRole("option")
  await expect(chatDraftRow).toHaveCount(1)
  await expect(chatDraftRow).toContainText("Draft")
  await chatDraftRow.getByRole("button", nav("More actions")).click()
  await menuFocused()
  await expect(page.getByRole("menuitem")).toHaveText([/^Rename/, /^Move to workspace/, /^Remove/])
  await page.keyboard.press("Escape")
  await expect(jump).toHaveCount(0)
  await goto(page, "#/epics")

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
  // The sidebar opens from the left edge over most of the phone and lists the draft row; Remove from its menu deletes the last draft.
  const sidebar = page.getByRole("complementary", nav("Sidebar"))
  await expect(row("near-draft")).toBeVisible()
  await expect.poll(async () => (await sidebar.boundingBox())?.x).toBe(0)
  expect((await sidebar.boundingBox())!.width).toBeGreaterThan(375 * 0.85)
  await expect(row("near-draft")).toBeVisible()
  await rowMenu("near-draft").click()
  await page.getByRole("menuitem", nav("Remove")).click()
  await page
    .getByRole("alertdialog", nav("Remove near-draft"))
    .getByRole("button", { name: /^Remove/ })
    .click()
  await expect(row("near-draft")).toHaveCount(0)
  await expect.poll(draftIds).toHaveLength(0)
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
