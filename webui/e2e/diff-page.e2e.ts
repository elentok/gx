import type { Page } from "@playwright/test"
import { expect, test } from "./fixture"

// The names are the constants of testutil/gxfixture/spec/epicdiff.go, documented in CATALOG.md.
const EPIC = "demo:widgets-impl"
const ARCHIVED = "demo:widgets-old-impl"
const LONG_FILE = "src/long/long.go"
const BIG_FILE = "src/gen/big.go"

const go = (page: Page, hash: string) => page.evaluate((h) => (location.hash = h), hash)
const baseRef = (page: Page) => page.getByRole("textbox", { name: "Base ref" })
const tipRef = (page: Page) => page.getByRole("textbox", { name: "Tip ref" })
const treeFiles = (page: Page) => page.getByTestId("tree-file")
const counter = (page: Page) => page.getByTestId("file-nav")

/** Opens the epic page at `address` and follows its View diff link. */
async function openDiff(page: Page, hash: string) {
  await go(page, hash)
  await page.getByRole("link", { name: "View diff" }).click()
  await expect(page.getByTestId("ref-bar")).toBeVisible()
}

test("Diff page", async ({ page, start }) => {
  // start() resets the allowed reload count on every call, so each start declares the one reload.
  const opts = { reloads: 1 }
  await page.setViewportSize({ width: 1200, height: 900 })

  await start("epic-diff", { ...opts, shape: "merged" })
  await go(page, `#/e/${EPIC}`)
  await page.getByRole("link", { name: "View diff" }).click()
  await expect(baseRef(page)).toHaveValue(/^[0-9a-f]{40}$/)
  await expect(tipRef(page)).toHaveValue(/^[0-9a-f]{40}$/)
  await expect(treeFiles(page).first()).toBeVisible()
  const base = await baseRef(page).inputValue()
  const tip = await tipRef(page).inputValue()
  const allFiles = await treeFiles(page).count()

  // Entry 1: a different base ref and Reload; the tree and the URL follow.
  await baseRef(page).fill(`${tip}~5`)
  await page.getByRole("button", { name: "Reload" }).click()
  await expect(page).toHaveURL(new RegExp(`${tip}~5`.replace("~", "(?:~|%7E)")))
  await expect(treeFiles(page)).not.toHaveCount(allFiles)
  await baseRef(page).fill(base)
  await page.getByRole("button", { name: "Reload" }).click()
  await expect(treeFiles(page)).toHaveCount(allFiles)

  // Entry 12: the commit list is collapsed at load; a ticket narrows the tree and the chip clears it.
  const disclosure = page.getByTestId("commits-disclosure")
  const toggle = disclosure.getByRole("button", { name: /^COMMITS BY TICKET/ })
  await expect(toggle).toHaveAttribute("aria-expanded", "false")
  await toggle.click()
  await expect(disclosure.getByRole("button", { name: /^All commits/ })).toBeVisible()
  await expect(disclosure.getByRole("button", { name: /^Design/ })).toBeVisible()
  await disclosure.getByRole("button", { name: /^every-commit-stamped/ }).click()
  const chip = page.getByTestId("narrow-chip")
  await expect(chip).toContainText("Showing: ")
  // Ticket 01's commits touch the logo, the big file, the list, app.tsx and state.ts.
  const narrowed = 5
  await expect(treeFiles(page)).toHaveCount(narrowed)
  await chip.getByRole("button", { name: "Clear narrowing" }).click()
  await expect(chip).toHaveCount(0)
  await expect(treeFiles(page)).toHaveCount(allFiles)
  await disclosure.getByRole("button", { name: /^every-commit-stamped/ }).click()
  await expect(treeFiles(page)).toHaveCount(narrowed)

  // Entry 15: the file filter rebuilds the tree.
  const filter = page.getByTestId("file-filter")
  await filter.fill("state")
  await expect(treeFiles(page)).toHaveCount(1)
  await filter.fill("")
  await expect(treeFiles(page)).toHaveCount(narrowed)

  // Entry 16: p and n move and wrap, the counter follows, and a focused filter ignores the keys.
  await expect(counter(page)).toContainText(`1 of ${narrowed}`)
  await filter.focus()
  await page.keyboard.press("n")
  await expect(filter).toHaveValue("n")
  // Stepping would write the file into the URL.
  await expect(page).not.toHaveURL(/file=/)
  await filter.fill("")
  await expect(treeFiles(page)).toHaveCount(narrowed)
  await filter.blur()
  await page.keyboard.press("n")
  await expect(counter(page)).toContainText(`2 of ${narrowed}`)
  await page.keyboard.press("p")
  await expect(counter(page)).toContainText(`1 of ${narrowed}`)
  await page.keyboard.press("p")
  await expect(counter(page)).toContainText(`${narrowed} of ${narrowed}`)
  await page.keyboard.press("n")
  await expect(counter(page)).toContainText(`1 of ${narrowed}`)

  // Entry 16: the URL holds the file and the narrowing; the one declared reload shows the same file.
  await page.keyboard.press("n")
  await expect(counter(page)).toContainText(`2 of ${narrowed}`)
  await expect(page).toHaveURL(/file=/)
  await expect(page).toHaveURL(/commits=/)
  const area = page.getByTestId("diff-area")
  const file = await area.getAttribute("data-file")
  expect(file).toBeTruthy()
  await page.reload()
  await expect(counter(page)).toContainText(`2 of ${narrowed}`)
  await expect(area).toHaveAttribute("data-file", file!)
  await expect(chip).toContainText("Showing: ")
  await chip.getByRole("button", { name: "Clear narrowing" }).click()
  await expect(treeFiles(page)).toHaveCount(allFiles)

  // Entry 18: split by default at 1200px, the switch changes it, the diff matches the app surface, 12.5px/19px code in the mono font, accents in the done and deadlocked tones.
  await expect(area).toHaveAttribute("data-view", "split")
  const split = page.getByTestId("view-toggle")
  await split.getByRole("button", { name: "Unified" }).click()
  await expect(area).toHaveAttribute("data-view", "unified")
  await split.getByRole("button", { name: "Split" }).click()
  await expect(area).toHaveAttribute("data-view", "split")
  await treeFiles(page).filter({ hasText: "app.tsx" }).click()
  await expect(area).toHaveAttribute("data-file", "src/web/app.tsx")
  const host = area.locator("pre").first()
  await expect(host).toBeVisible()
  const surface = await page.evaluate(() => {
    const probe = document.createElement("div")
    probe.style.backgroundColor = "var(--chakra-colors-surface)"
    document.body.append(probe)
    const color = getComputedStyle(probe).backgroundColor
    probe.remove()
    return color
  })
  await expect(host).toHaveCSS("background-color", surface)
  const code = area.locator("code").first()
  await expect(code).toHaveCSS("font-size", "12.5px")
  await expect(code).toHaveCSS("line-height", "19px")
  // Tokens read through a probe element, so the comparison uses the browser's resolved values.
  const tokens = await page.evaluate(() => {
    const probe = document.createElement("div")
    probe.style.fontFamily = "var(--chakra-fonts-mono)"
    probe.style.color = "var(--chakra-colors-tone-done-fg)"
    probe.style.borderColor = "var(--chakra-colors-tone-deadlocked-fg)"
    document.body.append(probe)
    const s = getComputedStyle(probe)
    const out = { mono: s.fontFamily, done: s.color, deadlocked: s.borderTopColor }
    probe.remove()
    return out
  })
  await expect(code).toHaveCSS("font-family", tokens.mono)
  // The accents reach the rendered diff: the host resolves them to the done and deadlocked tones.
  const accents = await area
    .locator("diffs-container")
    .first()
    .evaluate((el) => {
      const probe = document.createElement("div")
      el.shadowRoot!.append(probe)
      const read = (name: string) => {
        probe.style.color = `var(${name})`
        return getComputedStyle(probe).color
      }
      const out = {
        add: read("--diffs-addition-color-override"),
        del: read("--diffs-deletion-color-override"),
      }
      probe.remove()
      return out
    })
  expect(accents).toEqual({ add: tokens.done, del: tokens.deadlocked })

  // Entry 21: expand context works un-narrowed (a real blob round trip).
  await page.getByTestId("file-filter").fill("long")
  await treeFiles(page).filter({ hasText: "long.go" }).click()
  await expect(area).toHaveAttribute("data-file", LONG_FILE)
  await expect(area.getByText("// long 100 changed").first()).toBeVisible()
  await expect(area.getByText("// long 50", { exact: true })).toHaveCount(0)
  await area.locator("[data-expand-button]").first().click()
  await expect(area.getByText("// long 9", { exact: true }).first()).toBeVisible()
  await page.getByTestId("file-filter").fill("")

  // Entry 20: the 5000-line file loads on request in a 70vh box, and scrolling the box reaches the middle.
  await page.getByTestId("file-filter").fill("big")
  await treeFiles(page).filter({ hasText: "big.go" }).click()
  await expect(area).toHaveAttribute("data-file", BIG_FILE)
  await area.getByRole("button", { name: "Load diff" }).click()
  const box = area.locator('[style*="70vh"]')
  await expect(box).toBeVisible()
  const viewport = page.viewportSize()!
  await expect(async () => {
    const h = (await box.boundingBox())!.height
    expect(Math.abs(h - viewport.height * 0.7)).toBeLessThan(2)
  }).toPass()
  await box.evaluate((el) => (el.scrollTop = el.scrollHeight / 2))
  await expect(area.getByText("// generated 25", { exact: false }).first()).toBeVisible()
  await page.getByTestId("file-filter").fill("")

  // Entry 5: a done ticket's page opens the epic's Diff page narrowed to it; the commitless one has no link.
  await go(page, `#/t/${EPIC}/01`)
  await page.getByRole("link", { name: "View diff" }).click()
  await expect(page.getByTestId("narrow-chip")).toContainText("every-commit-stamped")
  await expect(treeFiles(page)).toHaveCount(narrowed)
  await go(page, `#/t/${EPIC}/04`)
  await expect(page.getByRole("link", { name: /^Epic: / })).toBeVisible()
  await expect(page.getByRole("link", { name: "View diff" })).toHaveCount(0)

  // Entry 4: the archived epic's link opens its stamped range.
  await go(page, `#/archive/${ARCHIVED}`)
  await page.getByRole("link", { name: "View diff" }).click()
  await expect(baseRef(page)).toHaveValue(/^[0-9a-f]{40}$/)
  await expect(page.locator('[data-testid="tree-file"][data-path="docs/old.md"]')).toBeVisible()

  // Entries 18 and 19: at 390px the page is unified by default, has Files and Diff tabs and no sideways scroll.
  await page.setViewportSize({ width: 390, height: 844 })
  await go(page, `#/e/${EPIC}`)
  await page.getByRole("link", { name: "View diff" }).click()
  await expect(page.getByRole("tab", { name: "Files" })).toHaveAttribute("aria-selected", "true")
  await expect(page.getByRole("tab", { name: "Diff" })).toBeVisible()
  await expect(treeFiles(page).first()).toBeVisible()
  await treeFiles(page).first().click()
  await expect(page.getByRole("tab", { name: "Diff" })).toHaveAttribute("aria-selected", "true")
  await expect(page.getByTestId("diff-area")).toHaveAttribute("data-view", "unified")
  await expect(page.getByTestId("file-card")).toBeVisible()
  const widths = await page.evaluate(() => ({
    scroll: document.documentElement.scrollWidth,
    client: document.documentElement.clientWidth,
  }))
  expect(widths.scroll).toBeLessThanOrEqual(widths.client)
  await page.setViewportSize({ width: 1200, height: 900 })

  // Entry 3: each running epic's link opens its derived refs; the quiet epic has none.
  await start("epic-diff", { ...opts, shape: "running" })
  const refs: Array<[string, string, string]> = [
    ["alpha-impl", "main", "alpha-impl"],
    ["beta-impl", "alpha-impl", "beta-impl"],
    ["gamma-impl", "main", "gamma-impl"],
    ["custom-impl", "release", "custom-impl"],
  ]
  for (const [epic, first, second] of refs) {
    await openDiff(page, `#/e/demo:${epic}`)
    await expect(baseRef(page)).toHaveValue(first)
    await expect(tipRef(page)).toHaveValue(second)
  }
  await go(page, "#/e/demo:quiet-impl")
  await expect(page.getByRole("heading", { name: /quiet-impl/ }).first()).toBeVisible()
  await expect(page.getByRole("link", { name: "View diff" })).toHaveCount(0)

  // Entry 9: a merged epic with no stamped range opens the page with empty ref fields.
  await start("epic-diff", { ...opts, shape: "unstamped" })
  await openDiff(page, `#/e/${EPIC}`)
  await expect(baseRef(page)).toHaveValue("")
  await expect(tipRef(page)).toHaveValue("")

  // Entry 6: an empty range has no link.
  await start("epic-diff", { ...opts, shape: "empty" })
  await go(page, `#/e/${EPIC}`)
  await expect(page.getByRole("heading", { name: /widgets-impl/ }).first()).toBeVisible()
  await expect(page.getByRole("link", { name: "View diff" })).toHaveCount(0)

  // Entry 10: a tip SHA that was never committed opens the page with no diff and editable ref fields.
  await start("epic-diff", { ...opts, shape: "missing-sha" })
  await openDiff(page, `#/e/${EPIC}`)
  await expect(page.getByTestId("notice")).toBeVisible()
  await expect(treeFiles(page)).toHaveCount(0)
  await expect(baseRef(page)).toBeEditable()
  await expect(tipRef(page)).toBeEditable()
})
