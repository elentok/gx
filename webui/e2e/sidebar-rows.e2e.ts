import type { Locator, Page } from "@playwright/test"
import { card, expect, nav, test, touch } from "./fixture"
import { frameButton, sidebarKit, stillMotion } from "./sidebarKit"

const goto = (page: Page, hash: string) => page.evaluate((h) => (window.location.hash = h), hash)

// The sidebar rows over the `workspaces` scenario: focus ring, expand and collapse, Alt numbers, keyboard moves and tree
// rows; split from the Workspaces flow so each stays inside its time limit.
test("Sidebar rows", async ({ page, start }) => {
  test.setTimeout(120_000)
  await start("workspaces")
  await stillMotion(page)
  const { sidebar, row, itemOf, nestedLinks, switcher, menuFocused, closeMenus, pick } =
    sidebarKit(page)

  // One global ring for keyboard focus: inset on a sidebar row, gap plus blue ring elsewhere, never after a click.
  const blue = "rgb(108, 180, 255)"
  const ring = new RegExp(`${blue.replace(/[()]/g, "\\$&")} 0px 0px 0px 4px`)
  await row("ready-epic").focus()
  // The ring sits on the row's frame, the link's parent, so it covers the whole row.
  await expect(row("ready-epic").locator("xpath=..")).toHaveCSS(
    "box-shadow",
    new RegExp(`${blue.replace(/[()]/g, "\\$&")} 0px 0px 0px 2px inset`),
  )
  const iconButton = card(page, "ready-epic").getByRole("button", nav("More actions"))
  await iconButton.focus()
  await expect(iconButton).toHaveCSS("box-shadow", ring)
  const epicCard = card(page, "ready-epic")
  await epicCard.getByRole("link", nav("ready-epic")).focus()
  await expect(epicCard.getByRole("link", nav("ready-epic"))).toHaveCSS("box-shadow", ring)
  await switcher.focus()
  await expect(switcher).toHaveCSS(
    "box-shadow",
    new RegExp(`${blue.replace(/[()]/g, "\\$&")} 0px 0px 0px 2px inset`),
  )
  await page.keyboard.press("Enter")
  await menuFocused()
  await expect(page.getByRole("menuitem").first()).toHaveCSS("box-shadow", ring)
  await closeMenus()
  await pick("Default workspace")
  await expect(switcher).toHaveCSS("box-shadow", "none")
  await page.keyboard.press("Tab")
  await expect(page.locator(":focus")).toHaveCSS(
    "box-shadow",
    /rgb\(108, 180, 255\) 0px 0px 0px (2px inset|4px)/,
  )

  // Clicking an epic row only navigates; the chevron expands and collapses its agents, and a page visit leaves the row as left.
  // Counted inside the epic's own item, so a row still loading elsewhere cannot skew the count.
  const links = itemOf("inv-epic").getByRole("link")
  const expand = sidebar.getByRole("button", nav("Expand inv-epic"))
  await expect(expand).toHaveAttribute("aria-expanded", "false")
  await expect(links).toHaveCount(1)
  await row("inv-epic").click()
  await expect(page).toHaveURL(/#\/e\/demo:inv-epic$/)
  await expect(expand).toHaveAttribute("aria-expanded", "false")
  await expect(links).toHaveCount(1)
  await expand.click()
  await expect(sidebar.getByRole("button", nav("Collapse inv-epic"))).toHaveAttribute(
    "aria-expanded",
    "true",
  )
  await expect(links).toHaveCount(2)
  await goto(page, "#/epics")
  await expect(links).toHaveCount(2)
  await sidebar.getByRole("button", nav("Collapse inv-epic")).click()
  await expect(links).toHaveCount(1)
  await goto(page, "#/e/demo:inv-epic")
  await expect(links).toHaveCount(1)
  await goto(page, "#/epics")

  // Rows show their numbers while Alt is held, and Alt+1 opens the first one.
  // Numbers take the menu-button slot; nothing else changes: no /v1 request, no DOM change outside the sidebar.
  const numbers = sidebar.getByTestId("row-number")
  const status = row("ready-epic").locator("span").last()
  await expect(numbers).toHaveCount(0)
  const wordsBefore = await status.textContent()
  // The Epics page's own loads (epics, checks) land after the goto above; count requests only once its cards are drawn.
  await expect(card(page, "ready-epic")).toBeVisible()
  await page.waitForResponse((r) => r.url().includes("/v1/checks/status")).catch(() => {})
  const requests: string[] = []
  page.on(
    "request",
    (r) => void (new URL(r.url()).pathname.startsWith("/v1/") && requests.push(r.url())),
  )
  await page.evaluate(() => {
    const w = window as unknown as { __outside: MutationRecord[] }
    w.__outside = []
    const side = document.querySelector('[aria-label="Sidebar"]')
    new MutationObserver((rs) =>
      w.__outside.push(...rs.filter((r) => !side?.contains(r.target))),
    ).observe(document.body, {
      subtree: true,
      childList: true,
      attributes: true,
      characterData: true,
    })
  })
  await page.keyboard.down("Alt")
  await expect(numbers.first()).toHaveText("1")
  await expect(status).toHaveText(wordsBefore!)
  await page.keyboard.up("Alt")
  await expect(numbers).toHaveCount(0)
  expect(requests).toEqual([])
  expect(
    await page.evaluate(
      () => (window as unknown as { __outside: MutationRecord[] }).__outside.length,
    ),
  ).toBe(0)
  // A focused text field makes Alt do nothing.
  await page.evaluate(() => {
    const i = document.createElement("input")
    i.id = "alt-probe"
    document.body.appendChild(i)
    i.focus()
  })
  await page.keyboard.down("Alt")
  await expect(numbers).toHaveCount(0)
  await page.keyboard.up("Alt")
  await page.evaluate(() => document.getElementById("alt-probe")?.remove())
  const firstHref = (await sidebar.getByRole("link").first().getAttribute("href"))!.slice(1)
  await page.keyboard.press("Alt+1")
  await expect.poll(() => new URL(page.url()).hash.slice(1)).toBe(firstHref)
  await goto(page, "#/epics")

  // Cmd/Ctrl+B hides the sidebar and brings it back with focus on a row; j/k, l/h and Enter move, expand and open.
  // The roving stop is the last row focused: the earlier row click left it on inv-epic, so put it back on the first.
  const navs = sidebar.locator("[data-nav]")
  await navs.first().focus()
  await page.locator("body").click({ position: { x: 600, y: 600 } })
  await page.keyboard.press("ControlOrMeta+b")
  await expect(sidebar).toBeHidden()
  await page.keyboard.press("ControlOrMeta+b")
  await expect(sidebar).toBeVisible()
  await expect(navs.first()).toBeFocused()
  await page.keyboard.press("j")
  await expect(navs.nth(1)).toBeFocused()
  await page.keyboard.press("ArrowUp")
  await expect(navs.first()).toBeFocused()
  await page.keyboard.press("k")
  await expect(navs.first()).toBeFocused()
  await row("inv-epic").focus()
  await page.keyboard.press("l")
  await expect(sidebar.getByRole("button", nav("Collapse inv-epic"))).toHaveAttribute(
    "aria-expanded",
    "true",
  )
  await page.keyboard.press("h")
  await expect(sidebar.getByRole("button", nav("Expand inv-epic"))).toHaveAttribute(
    "aria-expanded",
    "false",
  )
  await page.keyboard.press("Enter")
  await expect(page).toHaveURL(/#\/e\/demo:inv-epic$/)
  await goto(page, "#/epics")

  // Enter on a chat row focuses its prompt; a click opens the chat and leaves the prompt alone.
  const chatRow = sidebar.locator("a[data-chat]").first()
  const prompt = page.getByRole("form", nav("Composer")).getByRole("textbox")
  await chatRow.focus()
  await page.keyboard.press("Enter")
  await expect(page).toHaveURL(/#\/agents\//)
  await expect(prompt).toBeFocused()
  await goto(page, "#/epics")
  await chatRow.click()
  await expect(prompt).toBeVisible()
  await expect(prompt).not.toBeFocused()
  await goto(page, "#/epics")

  // Tree rows: a nested agent row sits on its parent's columns, has its own "..." menu (none for a Queue agent), and the button follows its own row's hover and focus.
  const x = async (l: Locator) => {
    await expect(l).toBeVisible()
    return (await l.boundingBox())!.x
  }
  const right = async (l: Locator) => {
    await expect(l).toBeVisible()
    const box = (await l.boundingBox())!
    return box.x + box.width
  }
  const titleOf = (link: Locator) => link.locator(":scope > span").first()
  const wordsOf = (link: Locator) => link.locator("span").last()
  // The button hides through an ancestor's opacity (shown on hover, focus or without hover), so multiply up to the row.
  const shown = (button: Locator) =>
    button.evaluate((el) => {
      let o = 1
      for (let n: Element | null = el; n && n.tagName !== "LI"; n = n.parentElement)
        o *= Number(getComputedStyle(n).opacity)
      return o
    })
  expect(await x(titleOf(chatRow))).toBe(await x(titleOf(row("ready-epic"))))
  await sidebar.getByRole("button", nav("Expand paused-epic")).click()
  await sidebar.getByRole("button", nav("Expand inv-epic")).click()
  const working = nestedLinks("paused-epic").filter({ hasText: "Working" })
  const queueAgent = nestedLinks("inv-epic")
  await expect(working).toHaveCount(1)
  await expect(queueAgent).toHaveCount(1)
  await expect(queueAgent.locator("[data-testid=icon-slot] svg")).toHaveCount(0)
  await expect(working.locator("[data-testid=icon-slot] svg")).toHaveCount(1)
  const parentMenu = frameButton(row("paused-epic"))
  // The guide line runs in the parent's icon column, 3px left of its centre so it clears a Fix agent's wrench.
  const centre = async (l: Locator) => {
    await expect(l).toBeVisible()
    const box = (await l.boundingBox())!
    return box.x + box.width / 2
  }
  const guide = itemOf("paused-epic").locator("[data-testid=nested-guide]").first()
  expect(
    Math.abs(
      (await centre(guide)) +
        3 -
        (await centre(row("paused-epic").locator("[data-testid=icon-slot]"))),
    ),
  ).toBeLessThanOrEqual(0.5)
  expect(await x(titleOf(working))).toBe(await x(titleOf(row("paused-epic"))))
  // The words differ in length, so their right edges are the shared column.
  expect(await right(wordsOf(working))).toBe(await right(wordsOf(row("paused-epic"))))
  expect(await x(frameButton(working))).toBe(await x(parentMenu))
  await expect(frameButton(queueAgent)).toHaveCount(0)
  // At 1280px the button shows on its own row only; Tab onto it shows it; without hover every row shows its button.
  const size = page.viewportSize()!
  await page.setViewportSize({ width: 1280, height: size.height })
  await working.hover()
  await expect.poll(() => shown(frameButton(working))).toBe(1)
  await expect.poll(() => shown(parentMenu)).toBe(0)
  await page.mouse.move(700, 5)
  await expect.poll(() => shown(frameButton(working))).toBe(0)
  await working.focus()
  await page.keyboard.press("Tab")
  await expect(frameButton(working)).toBeFocused()
  await expect.poll(() => shown(frameButton(working))).toBe(1)
  await expect.poll(() => shown(parentMenu)).toBe(0)
  await page.setViewportSize(size)
  // A touch context has no hover: the same sidebar shows every row's button, nested included.
  const phone = await page
    .context()
    .browser()!
    .newContext({
      ...touch,
      viewport: { width: 1280, height: size.height },
      storageState: await page.context().storageState(),
    })
  const tablet = await phone.newPage()
  await tablet.goto(new URL(page.url()).origin + "/#/epics")
  const tabletSidebar = tablet.getByRole("complementary", nav("Sidebar"))
  await tabletSidebar.getByRole("button", nav("Expand paused-epic")).click()
  const tabletWorking = tabletSidebar
    .getByRole("listitem")
    .filter({ has: tablet.getByRole("button", nav("Collapse paused-epic")) })
    .locator("ul")
    .getByRole("link", { name: /Working/ })
  await expect(tabletWorking).toHaveCount(1)
  for (const button of [
    frameButton(tabletWorking),
    tabletSidebar.getByRole("button", nav("More actions")).first(),
  ])
    await expect.poll(() => shown(button)).toBe(1)
  await phone.close()
})
