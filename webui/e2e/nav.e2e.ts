import { type BrowserContext, type Page, webkit } from "@playwright/test"
import { expect, nav, test } from "./fixture"
import { sidebarKit, stillMotion } from "./sidebarKit"

const LEADER_TOOLTIP = /^Back \((⌘E|Ctrl\+E) ,\)$/

/** Adds the web login of `from` to `to`. The `__Host-` cookie takes a URL, never a domain. */
async function signIn(from: Page, to: BrowserContext, origin: string) {
  const [{ name, value }] = await from.context().cookies()
  await to.addCookies([{ name, value, url: origin, secure: true }])
}

/** `page` landed on the Epics page with nothing behind it. */
async function onFirstPage(page: Page) {
  await expect(page).toHaveURL(/#\/epics$/)
  await expect(sidebarKit(page).sidebar.getByRole("button", nav("Back"))).toBeDisabled()
  expect(await page.evaluate(() => navigation.canGoBack)).toBe(false)
}

/**
 * The Show sidebar button of the settled page titled `title`. During a page change the shell's
 * stand-in button and the page's own can both be there for a moment.
 */
async function showSidebarButton(page: Page, title: string) {
  await expect(page.getByRole("heading", { level: 1, name: title })).toBeVisible()
  const button = page.getByRole("button", nav("Show sidebar"))
  await expect(button).toHaveCount(1)
  return button
}

async function hoverShowsTooltip(page: Page) {
  await sidebarKit(page).sidebar.getByRole("button", nav("Back")).hover()
  await expect(page.getByRole("tooltip")).toHaveText(LEADER_TOOLTIP)
}

// The sidebar header's Back and Forward buttons against real session history, over the `workspaces` scenario.
test("Nav buttons", async ({ page, start, browser }) => {
  test.setTimeout(120_000)
  await page.setViewportSize({ width: 1280, height: 800 })
  await start("workspaces", { reloads: 1 })
  await stillMotion(page)
  const { sidebar, row } = sidebarKit(page)
  const button = (name: string) => sidebar.getByRole("button", nav(name))
  const [back, forward] = [button("Back"), button("Forward")]
  const header = [button("Search"), button("Hide sidebar"), back, forward]
  const at = async (url: RegExp, canBack: boolean, canForward: boolean) => {
    await expect(page).toHaveURL(url)
    await expect(back).toBeEnabled({ enabled: canBack })
    await expect(forward).toBeEnabled({ enabled: canForward })
  }
  const epics = /#\/epics$/
  const ready = /#\/e\/demo:ready-epic$/
  const paused = /#\/e\/demo:paused-epic$/

  await at(epics, false, false)
  for (const b of header) await expect(b).toBeVisible()
  await expect
    .poll(async () => {
      const xs = await Promise.all(header.map(async (b) => (await b.boundingBox())!.x))
      return xs.every((x, i) => i === 0 || x > xs[i - 1])
    })
    .toBe(true)

  await row("ready-epic").click()
  await at(ready, true, false)
  await row("paused-epic").click()
  await at(paused, true, false)
  await page.locator('a[href^="#/t/"]').first().click()
  await at(/#\/t\/demo:paused-epic\//, true, false)
  await back.click()
  await at(paused, true, true)
  await page.goBack()
  await at(ready, true, true)
  await page.reload()
  await at(ready, true, true)
  await stillMotion(page)
  await back.click()
  await at(epics, false, true)

  // gx leaves the browser's own history chords alone.
  await page.evaluate(() => {
    const seen: string[] = []
    Object.assign(window, { navKeys: seen })
    window.addEventListener("keydown", (e) => {
      if (e.metaKey && (e.key === "[" || e.key === "]"))
        seen.push(`${e.key} prevented=${e.defaultPrevented}`)
    })
  })
  await page.keyboard.press("Meta+[")
  await page.keyboard.press("Meta+]")
  await expect
    .poll(() => page.evaluate(() => (window as unknown as { navKeys: string[] }).navKeys))
    .toEqual(["[ prevented=false", "] prevented=false"])

  await button("Hide sidebar").click()
  await expect(page.getByRole("button", nav("Back"))).toHaveCount(0)
  await expect(page.getByRole("button", nav("Forward"))).toHaveCount(0)

  // `data-open`, not visibility: a closing sidebar stays visible while it slides out.
  const open = () => expect(sidebar).toHaveAttribute("data-open", "true")
  const show = async (title: string) => {
    await (await showSidebarButton(page, title)).click()
    await open()
  }
  for (const size of [
    { width: 744, height: 900 },
    { width: 390, height: 844 },
  ]) {
    await page.setViewportSize(size)
    await show("Epics")
    for (const b of header) await expect(b).toBeVisible()
    await row("paused-epic").click()
    await expect(sidebar).toBeHidden()
    await show("paused-epic")
    await back.click()
    await at(epics, false, true)
    await open()
    await forward.click()
    await at(paused, true, false)
    await open()
    await back.click()
    await at(epics, false, true)
    await button("Hide sidebar").click()
    await expect(sidebar).toBeHidden()
  }

  const origin = new URL(page.url()).origin
  let first!: Page
  for (const path of ["/", "/#/no-such-page"]) {
    first = await page.context().newPage()
    await first.goto(`${origin}${path}`, { waitUntil: "domcontentloaded" })
    await onFirstPage(first)
  }
  await hoverShowsTooltip(first)

  // The Safari Dock app runs WebKit.
  const safari = await webkit.launch()
  try {
    const context = await safari.newContext({ ignoreHTTPSErrors: true, serviceWorkers: "block" })
    await signIn(page, context, origin)
    const other = await context.newPage()
    await other.goto(origin, { waitUntil: "domcontentloaded" })
    await onFirstPage(other)
    await hoverShowsTooltip(other)
  } finally {
    await safari.close()
  }

  const context = await browser.newContext({
    hasTouch: true,
    isMobile: true,
    viewport: { width: 390, height: 844 },
    ignoreHTTPSErrors: true,
    serviceWorkers: "block",
  })
  try {
    await signIn(page, context, origin)
    const phone = await context.newPage()
    // The tooltip opens after a delay, so the test owns the clock to get past it.
    await phone.clock.install()
    await phone.goto(origin, { waitUntil: "domcontentloaded" })
    await expect(phone).toHaveURL(epics)
    const kit = sidebarKit(phone)
    await (await showSidebarButton(phone, "Epics")).tap()
    await kit.row("ready-epic").tap()
    await expect(phone).toHaveURL(ready)
    // The page must be there before the step back, or the two page changes overlap.
    await showSidebarButton(phone, "ready-epic")
    await phone.goBack()
    await expect(phone).toHaveURL(epics)
    await (await showSidebarButton(phone, "Epics")).tap()
    await kit.sidebar.getByRole("button", nav("Forward")).tap()
    await expect(phone).toHaveURL(ready)
    await expect(kit.sidebar).toHaveAttribute("data-open", "true")
    await phone.clock.runFor(2_000)
    await expect(phone.getByRole("tooltip")).toHaveCount(0)
  } finally {
    await context.close()
  }
})
