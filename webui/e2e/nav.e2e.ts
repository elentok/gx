import { type BrowserContext, type Page, webkit } from "@playwright/test"
import { chordKit } from "./chordKit"
import { expect, nav, test } from "./fixture"
import { goto, sidebarKit, stillMotion } from "./sidebarKit"

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

/** The hashes of the sidebar's rendered rows, top to bottom; read from the DOM so a hidden sidebar still answers. */
async function rowOrder(page: Page) {
  const rows = page.locator("[data-nav-list] a[data-nav]")
  await expect(rows).not.toHaveCount(0)
  return rows.evaluateAll((links) => links.map((a) => a.getAttribute("href")!))
}

const atHash = (page: Page, hash: string) => expect(page).toHaveURL((url) => url.hash === hash)

/** Leader then `j` or `k`, and the wait for the row it must open. */
async function step(page: Page, key: "j" | "k", target: string) {
  const chords = chordKit(page)
  await chords.open(key === "j" ? chords.next : chords.prev)
  await page.keyboard.press(key)
  await atHash(page, target)
  // The address changes before a loading page renders, and a chord until then reads the old page.
  await expect(page.locator(`a[data-nav][href="${target}"]`)).toHaveAttribute(
    "aria-current",
    "page",
  )
}

/**
 * Presses `key` from the page at `from` until no row is left that way, checking each page against
 * the sidebar's order at that moment. Then checks that the end offers no step. Returns the hashes
 * visited.
 */
async function walk(page: Page, key: "j" | "k", from: string) {
  const visited: string[] = []
  let here = from
  for (;;) {
    const order = await rowOrder(page)
    const at = order.indexOf(here)
    // From a page no row owns, `j` opens the first row and `k` the last.
    const target = at < 0 ? order.at(key === "j" ? 0 : -1) : order[at + (key === "j" ? 1 : -1)]
    if (!target) break
    expect(visited).not.toContain(target)
    await step(page, key, target)
    visited.push(target)
    here = target
  }
  const chords = chordKit(page)
  // A one-row sidebar has neither item row, so the history row says the box is up.
  await chords.open(chords.back)
  await expect(key === "j" ? chords.next : chords.prev).toHaveCount(0)
  await page.keyboard.press(key)
  await expect(chords.box).toHaveCount(0)
  await atHash(page, here)
  return visited
}

// The Leader key's item and history chords over the sidebar of the `workspaces` scenario.
test("Nav keys", async ({ page, start }) => {
  test.setTimeout(120_000)
  await page.setViewportSize({ width: 1280, height: 800 })
  await start("workspaces")
  await stillMotion(page)
  const { sidebar, row, nestedLinks, pick } = sidebarKit(page)
  const chords = chordKit(page)
  const button = (name: string) => sidebar.getByRole("button", nav(name))
  const prompt = page.getByRole("form", nav("Composer")).getByRole("textbox")
  const chats = sidebar.locator("a[data-chat]")
  const knownRows = async () => {
    for (const title of ["ready-epic", "ws-draft"]) await expect(row(title)).toBeVisible()
    await expect(chats).toHaveCount(2)
  }
  /** The collapsed rows and the closed section are still shut. */
  const stillClosed = async () => {
    for (const title of ["paused-epic", "inv-epic"]) {
      await expect(row(title)).toBeVisible()
      await expect(nestedLinks(title)).toHaveCount(0)
    }
    await expect(sidebar.getByRole("button", { name: /^Other/ })).toHaveAttribute(
      "aria-expanded",
      "false",
    )
  }

  await expect(page).toHaveURL(/#\/epics$/)
  await knownRows()
  await stillClosed()
  const down = await walk(page, "j", "#/epics")
  expect(down).toEqual(await rowOrder(page))
  const up = await walk(page, "k", down.at(-1)!)
  expect([...up].reverse()).toEqual((await rowOrder(page)).slice(0, -1))
  await stillClosed()

  // Back returns to the previous item.
  const [first, second] = await rowOrder(page)
  await step(page, "j", second)
  await chords.open(chords.back)
  await page.keyboard.press("Comma")
  await atHash(page, first)

  // Focus: the two chats are neighbours, and a row sits before them.
  await expect(chats).toHaveCount(2)
  const [chatA, chatB] = await chats.evaluateAll((links) =>
    links.map((a) => a.getAttribute("href")!),
  )
  const order = await rowOrder(page)
  const atA = order.indexOf(chatA)
  expect(atA).toBeGreaterThan(0)
  expect(order[atA + 1]).toBe(chatB)
  const link = (hash: string) => sidebar.locator(`a[data-nav][href="${hash}"]`)
  await link(order[atA - 1]).click()
  await atHash(page, order[atA - 1])
  await goto(page, "#/epics")
  await atHash(page, "#/epics")
  await expect(prompt).toHaveCount(0)
  await step(page, "j", chatA)
  await expect(link(chatA)).toHaveAttribute("aria-current", "page")
  await expect(prompt).toBeVisible()
  await expect(prompt).not.toBeFocused()
  await prompt.click()
  await step(page, "j", chatB)
  await expect(link(chatB)).toHaveAttribute("aria-current", "page")
  await expect(prompt).toBeFocused()

  // A hidden sidebar stays hidden, and Back works there too.
  await button("Hide sidebar").click()
  await expect(sidebar).toBeHidden()
  await step(page, "k", chatA)
  await chords.open(chords.back)
  await page.keyboard.press("Comma")
  await atHash(page, chatB)
  await expect(sidebar).toBeHidden()
  await page.getByRole("button", nav("Show sidebar")).click()
  await expect(sidebar).toHaveAttribute("data-open", "true")

  // Select mode stays on.
  const selected = sidebar.getByTestId("select-count")
  await button("Select").click()
  await expect(selected).toHaveText("0 selected")
  await step(page, "k", chatA)
  await expect(selected).toHaveText("0 selected")
  await button("Done").click()
  await expect(selected).toHaveCount(0)

  // At 390px the sliding sidebar stays closed, then stays open.
  await page.setViewportSize({ width: 390, height: 844 })
  await expect(sidebar).toBeHidden()
  await step(page, "j", chatB)
  await expect(sidebar).toBeHidden()
  // The chat's page is there, so only its own Show sidebar button is.
  await expect(prompt).toBeVisible()
  await expect(page.getByRole("button", nav("Show sidebar"))).toHaveCount(1)
  await page.getByRole("button", nav("Show sidebar")).click()
  await expect(sidebar).toHaveAttribute("data-open", "true")
  await step(page, "k", chatA)
  await expect(sidebar).toHaveAttribute("data-open", "true")
  await page.setViewportSize({ width: 1280, height: 800 })

  // A step to a row below the fold scrolls it into view. A new tab remembers no row.
  const low = await page.context().newPage()
  await low.setViewportSize({ width: 1280, height: 420 })
  await low.goto(`${new URL(page.url()).origin}/#/settings`, { waitUntil: "domcontentloaded" })
  await expect(low).toHaveURL(/#\/settings/)
  await stillMotion(low)
  await goto(low, "#/epics")
  await expect(low).toHaveURL(/#\/epics$/)
  await expect(sidebarKit(low).row("ready-epic")).toBeVisible()
  const last = (await rowOrder(low)).at(-1)!
  const lastLink = sidebarKit(low).sidebar.locator(`a[data-nav][href="${last}"]`)
  await expect(lastLink).not.toBeInViewport()
  await step(low, "k", last)
  await expect(lastLink).toBeInViewport()
  await low.close()

  // Another workspace: its held section is skipped and stays closed.
  await pick("gx bugs")
  await goto(page, "#/epics")
  await atHash(page, "#/epics")
  await expect(row("ws-design")).toBeVisible()
  const onHold = button("On hold (1)")
  await expect(onHold).toHaveAttribute("aria-expanded", "false")
  const bugs = await walk(page, "j", "#/epics")
  expect(bugs).toEqual(await rowOrder(page))
  await expect(onHold).toHaveAttribute("aria-expanded", "false")
  await expect(row("held-design")).toHaveCount(0)
})

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
