import { execFileSync } from "node:child_process"
import { createHash } from "node:crypto"
import { existsSync, readFileSync, statSync, writeFileSync } from "node:fs"
import { join, resolve } from "node:path"
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

/** Opens the epic page at `address` and follows its View diff link. */
async function openDiff(page: Page, hash: string) {
  await go(page, hash)
  await page.getByRole("link", { name: "View diff" }).click()
  await expect(page.getByTestId("diff-header")).toBeVisible()
}

/** Opens the ref inputs through the edit link unless they are already open. */
async function editRefs(page: Page) {
  if (!(await baseRef(page).isVisible()))
    await page.getByRole("button", { name: "Edit refs" }).click()
  await expect(baseRef(page)).toBeVisible()
}

const chipOf = (page: Page) => page.getByTestId("range-chip")

test("Diff page", async ({ page, start }) => {
  // start() resets the allowed reload count on every call, so each start declares the one reload.
  const opts = { reloads: 1 }
  await page.setViewportSize({ width: 1200, height: 900 })

  const fixture = await start("epic-diff", { ...opts, shape: "merged" })

  // A forced failed first load shows Try again inside its notice, and the button loads the page data.
  let failing = true
  await page.route("**/v1/diff/files*", (route) =>
    failing ? route.fulfill({ status: 500, body: "boom" }) : route.continue(),
  )
  await go(page, `#/e/${EPIC}`)
  await page.getByRole("link", { name: "View diff" }).click()
  const failedNotice = page.getByTestId("notice")
  await expect(failedNotice).toHaveAttribute("data-kind", "failed")
  await expect(treeFiles(page)).toHaveCount(0)
  failing = false
  await failedNotice.getByRole("button", { name: "Try again" }).click()
  await expect(treeFiles(page).first()).toBeVisible()
  await expect(failedNotice).toHaveCount(0)
  await page.unroute("**/v1/diff/files*")

  await go(page, `#/e/${EPIC}`)
  await page.getByRole("link", { name: "View diff" }).click()
  await expect(chipOf(page)).toHaveText(/^[0-9a-f]{40}→[0-9a-f]{40}$/)
  await expect(treeFiles(page).first()).toBeVisible()
  await editRefs(page)
  const base = await baseRef(page).inputValue()
  const tip = await tipRef(page).inputValue()
  expect(base).toMatch(/^[0-9a-f]{40}$/)
  expect(tip).toMatch(/^[0-9a-f]{40}$/)
  const allFiles = await treeFiles(page).count()

  // Entry 1: a different base ref and Enter; the tree and the URL follow and the inputs close.
  await baseRef(page).fill(`${tip}~5`)
  await baseRef(page).press("Enter")
  await expect(page).toHaveURL(new RegExp(`${tip}~5`.replace("~", "(?:~|%7E)")))
  await expect(baseRef(page)).toHaveCount(0)
  await expect(treeFiles(page)).not.toHaveCount(allFiles)
  await editRefs(page)
  await baseRef(page).fill(base)
  await baseRef(page).press("Enter")
  await expect(treeFiles(page)).toHaveCount(allFiles)

  // Edit keys: Enter in the tip input applies and closes, Esc and a second click on the edit link
  // close with the chip and URL unchanged, and an unknown ref marks its input invalid.
  const chipText = (await chipOf(page).textContent())!
  const urlBefore = page.url()
  await editRefs(page)
  await tipRef(page).fill(`${tip}~1`)
  await tipRef(page).press("Enter")
  await expect(page).toHaveURL(new RegExp(`${tip}~1`.replace("~", "(?:~|%7E)")))
  await expect(tipRef(page)).toHaveCount(0)
  await editRefs(page)
  await tipRef(page).fill(tip)
  await tipRef(page).press("Enter")
  await expect(tipRef(page)).toHaveCount(0)
  await expect(page).toHaveURL(urlBefore)
  await editRefs(page)
  await baseRef(page).fill("changed-but-escaped")
  await baseRef(page).press("Escape")
  await expect(baseRef(page)).toHaveCount(0)
  await expect(chipOf(page)).toHaveText(chipText)
  await expect(page).toHaveURL(urlBefore)
  await editRefs(page)
  await tipRef(page).fill("changed-but-cancelled")
  await page.getByRole("button", { name: "Edit refs" }).click()
  await expect(tipRef(page)).toHaveCount(0)
  await expect(chipOf(page)).toHaveText(chipText)
  await expect(page).toHaveURL(urlBefore)
  await editRefs(page)
  await baseRef(page).fill("no-such-ref")
  await baseRef(page).press("Enter")
  await expect(page.getByTestId("notice")).toContainText('Unknown ref "no-such-ref"')
  await expect(baseRef(page)).toHaveAttribute("aria-invalid", "true")
  await expect(tipRef(page)).not.toHaveAttribute("aria-invalid", "true")
  await baseRef(page).fill(base)
  await baseRef(page).press("Enter")
  await expect(treeFiles(page)).toHaveCount(allFiles)
  await expect(page.getByTestId("notice")).toHaveCount(0)

  // Entry 12: the commit list is collapsed at load; a ticket narrows the tree and the highlighted row is the only sign, and All commits clears it.
  const disclosure = page.getByTestId("commits-disclosure")
  const toggle = disclosure.getByRole("button", { name: /^\d+ commits?$/ })
  await expect(toggle).toHaveAttribute("aria-expanded", "false")
  await toggle.click()
  await expect(disclosure.getByRole("button", { name: /^All commits/ })).toBeVisible()
  await expect(disclosure.getByRole("button", { name: /^Design/ })).toBeVisible()
  await disclosure.getByRole("button", { name: /^every-commit-stamped/ }).click()
  await expect(disclosure.getByRole("button", { name: /^every-commit-stamped/ })).toHaveAttribute(
    "aria-current",
    "true",
  )
  // Ticket 01's commits touch the logo, the big file, the list, app.tsx and state.ts.
  const narrowed = 5
  await expect(treeFiles(page)).toHaveCount(narrowed)
  await disclosure.getByRole("button", { name: /^All commits/ }).click()
  await expect(treeFiles(page)).toHaveCount(allFiles)
  await expect(page).not.toHaveURL(/commits=/)
  await disclosure.getByRole("button", { name: /^every-commit-stamped/ }).click()
  await expect(treeFiles(page)).toHaveCount(narrowed)

  // Entry 15: no filter at load (the range lists 15 files or fewer); `/` shows it focused with no
  // slash typed, and the filter rebuilds the tree.
  const filter = page.getByTestId("file-filter")
  await expect(filter).toHaveCount(0)
  await page.keyboard.press("/")
  await expect(filter).toBeFocused()
  await expect(filter).toHaveValue("")
  await filter.fill("state")
  await expect(treeFiles(page)).toHaveCount(1)
  await filter.fill("")
  await expect(treeFiles(page)).toHaveCount(narrowed)

  // Entry 16: p and n move and wrap, the counter follows, and a focused filter ignores the keys.
  const area = page.getByTestId("diff-area")
  const first = (await area.getAttribute("data-file"))!
  expect(first).toBeTruthy()
  await filter.focus()
  await page.keyboard.press("n")
  await expect(filter).toHaveValue("n")
  // Stepping would write the file into the URL.
  await expect(page).not.toHaveURL(/file=/)
  await filter.fill("")
  await expect(treeFiles(page)).toHaveCount(narrowed)
  await filter.blur()
  await page.keyboard.press("n")
  await expect(area).not.toHaveAttribute("data-file", first)
  await page.keyboard.press("p")
  await expect(area).toHaveAttribute("data-file", first)
  await page.keyboard.press("p")
  await expect(area).not.toHaveAttribute("data-file", first)
  await page.keyboard.press("n")
  await expect(area).toHaveAttribute("data-file", first)

  // Entry 16: the URL holds the file and the narrowing; the one declared reload shows the same file.
  await page.keyboard.press("n")
  await expect(area).not.toHaveAttribute("data-file", first)
  await expect(page).toHaveURL(/file=/)
  await expect(page).toHaveURL(/commits=/)
  const file = await area.getAttribute("data-file")
  expect(file).toBeTruthy()
  await expect(area).toHaveAttribute("data-view", "unified")
  await page.getByTestId("view-toggle").getByRole("button", { name: "Split" }).click()
  await expect(area).toHaveAttribute("data-view", "split")
  await page.reload()
  await expect(area).toHaveAttribute("data-file", file!)
  await expect(area).toHaveAttribute("data-view", "split")
  await expect(treeFiles(page)).toHaveCount(narrowed)
  await toggle.click()
  await expect(disclosure.getByRole("button", { name: /^every-commit-stamped/ })).toHaveAttribute(
    "aria-current",
    "true",
  )
  await disclosure.getByRole("button", { name: /^All commits/ }).click()
  await expect(treeFiles(page)).toHaveCount(allFiles)

  // Entry 18: unified by default at 1200px and a picked Split survives the reload (checked above), the switch changes it, the diff matches the app surface, 12.5px/19px code in the mono font, accents in the done and deadlocked tones.
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

  // Entry 21: expand context works un-narrowed (a real blob round trip). The reload hid the filter again.
  await expect(filter).toHaveCount(0)
  await page.keyboard.press("/")
  await expect(filter).toBeFocused()
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

  // Independent scroll: at a short desktop window the page does not scroll, the diff pane does, the tree column stays put, the path line sticks to the pane's top and a file picked low in the tree starts at the top. Whether the loaded big file overflows the pane is read in the run, so all boxes come from one poll.
  await page.setViewportSize({ width: 1200, height: 320 })
  const pane = page.getByTestId("diff-pane")
  const left = page.getByTestId("diff-left")
  const pathLine = page.getByTestId("file-path")
  await expect(pathLine).toHaveText(BIG_FILE)
  await expect(pane).toBeVisible()
  await expect(left).toBeVisible()
  const read = async () => {
    // The earlier click may have scrolled the tree to its row; only the pane's own scroll is under test.
    await left.evaluate((el) => (el.scrollTop = 0))
    await pane.evaluate((el) => (el.scrollTop = 10))
    return page.evaluate(() => {
      const box = (id: string) => document.querySelector(`[data-testid="${id}"]`)!
      const pane = box("diff-pane")
      return {
        docOverflow: document.documentElement.scrollHeight - document.documentElement.clientHeight,
        paneScroll: pane.scrollTop,
        leftScroll: box("diff-left").scrollTop,
        lineTop: box("file-path").getBoundingClientRect().top - pane.getBoundingClientRect().top,
      }
    })
  }
  await expect
    .poll(read)
    .toEqual({ docOverflow: expect.any(Number), paneScroll: 10, leftScroll: 0, lineTop: 0 })
  expect((await read()).docOverflow).toBeLessThanOrEqual(0)
  await treeFiles(page).last().click()
  await expect(area).not.toHaveAttribute("data-file", BIG_FILE)
  await expect(pathLine).toHaveText((await area.getAttribute("data-file"))!)
  await expect.poll(() => pane.evaluate((el) => el.scrollTop)).toBe(0)
  await expect(left).toBeVisible()
  await page.setViewportSize({ width: 1200, height: 900 })

  // Entry 5: a done ticket's page opens the epic's Diff page narrowed to it; the commitless one has no link.
  await go(page, `#/t/${EPIC}/01`)
  await page.getByRole("link", { name: "View diff" }).click()
  await page
    .getByTestId("commits-disclosure")
    .getByRole("button", { name: /^\d+ commits?$/ })
    .click()
  await expect(
    page.getByTestId("commits-disclosure").getByRole("button", { name: /^every-commit-stamped/ }),
  ).toHaveAttribute("aria-current", "true")
  await expect(treeFiles(page)).toHaveCount(narrowed)
  await go(page, `#/t/${EPIC}/04`)
  await expect(page.getByRole("link", { name: /^Epic: / })).toBeVisible()
  await expect(page.getByRole("link", { name: "View diff" })).toHaveCount(0)

  // Entry 4: the archived epic's link opens its stamped range.
  await go(page, `#/archive/${ARCHIVED}`)
  await page.getByRole("link", { name: "View diff" }).click()
  await expect(chipOf(page)).toHaveText(/^[0-9a-f]{40}→[0-9a-f]{40}$/)
  await expect(page.locator('[data-testid="tree-file"][data-path="docs/old.md"]')).toBeVisible()

  // The choice is stored in the browser, so put it back to Unified before the phone step.
  await page.getByTestId("view-toggle").getByRole("button", { name: "Unified" }).click()
  await expect(area).toHaveAttribute("data-view", "unified")

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
  // The range chip sits at the header's left edge, as in the mock, not centred on its own row.
  const headBox = await page.getByTestId("diff-header").boundingBox()
  const chipBox = await chipOf(page).boundingBox()
  expect(chipBox!.x - headBox!.x).toBeLessThanOrEqual(1)

  // At a short phone window each tab scrolls on its own and the document never does.
  await page.setViewportSize({ width: 390, height: 300 })
  const scrolls = (id: string) =>
    page.evaluate((testId) => {
      const el = document.querySelector(`[data-testid="${testId}"]`)!
      el.scrollTop = 30
      return {
        own: el.scrollTop,
        doc: document.documentElement.scrollHeight - document.documentElement.clientHeight,
      }
    }, id)
  await page.getByRole("tab", { name: /^Files/ }).click()
  await expect.poll(() => scrolls("diff-left")).toEqual({ own: 30, doc: expect.any(Number) })
  expect((await scrolls("diff-left")).doc).toBeLessThanOrEqual(0)
  await treeFiles(page).filter({ hasText: "long.go" }).click()
  await expect.poll(() => scrolls("diff-pane")).toEqual({ own: 30, doc: expect.any(Number) })
  expect((await scrolls("diff-pane")).doc).toBeLessThanOrEqual(0)
  await page.setViewportSize({ width: 1200, height: 900 })

  // Focus refetch: the repo moves while the page is open and a window focus event updates the tree
  // with no reload. The tip is `main`, so commits on main move the range.
  const projects: { name: string; repo: string }[] = JSON.parse(
    await fixture.cli("project", "list", "--json"),
  )
  const repo = projects.find((p) => p.name === "demo")!.repo
  const git = (...args: string[]) =>
    execFileSync("git", ["-C", repo, "-c", "user.name=t", "-c", "user.email=t@t", ...args])
  const focus = () => page.evaluate(() => window.dispatchEvent(new Event("focus")))
  await openDiff(page, `#/e/${EPIC}`)
  await editRefs(page)
  await tipRef(page).fill("main")
  await tipRef(page).press("Enter")
  await expect(chipOf(page)).toContainText("main")
  await expect(treeFiles(page)).toHaveCount(allFiles)
  await treeFiles(page).filter({ hasText: "state.ts" }).click()
  await expect(area).toHaveAttribute("data-file", "src/web/state.ts")
  writeFileSync(join(repo, "added-on-focus.txt"), "one\n")
  git("add", "added-on-focus.txt")
  git("commit", "-m", "add a file while open")
  await focus()
  await expect(treeFiles(page)).toHaveCount(allFiles + 1)
  await expect(area).toHaveAttribute("data-file", "src/web/state.ts")
  git("checkout", base, "--", "src/web/state.ts")
  git("commit", "-m", "restore state.ts")
  await focus()
  await expect(page.locator('[data-testid="tree-file"][data-path="src/web/state.ts"]')).toHaveCount(
    0,
  )
  await expect(area).toHaveAttribute(
    "data-file",
    (await treeFiles(page).first().getAttribute("data-path"))!,
  )
  // With a ticket narrowing active a focus keeps the narrowed tree and the highlighted row.
  await disclosure.getByRole("button", { name: /^\d+ commits?$/ }).click()
  await disclosure.getByRole("button", { name: /^every-commit-stamped/ }).click()
  await expect(page).toHaveURL(/commits=/)
  // Ticket 01's commits still touch state.ts, so the narrowed tree keeps its five files.
  await expect(treeFiles(page)).toHaveCount(narrowed)
  writeFileSync(join(repo, "added-on-focus-2.txt"), "two\n")
  git("add", "added-on-focus-2.txt")
  git("commit", "-m", "add another file while narrowed")
  await focus()
  await expect(disclosure.getByRole("button", { name: /^every-commit-stamped/ })).toHaveAttribute(
    "aria-current",
    "true",
  )
  await expect(treeFiles(page)).toHaveCount(narrowed)
  await expect(page).toHaveURL(/commits=/)

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
    await expect(chipOf(page)).toHaveText(`${first}→${second}`)
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
  await expect(page.getByTestId("notice")).not.toContainText("Reload")
  await editRefs(page)
  await expect(baseRef(page)).toBeEditable()
  await expect(tipRef(page)).toBeEditable()
})

// The names are the constants of testutil/gxfixture/spec/worktreediff.go, documented in CATALOG.md.
const WT_MODIFIED = "src/mod.txt"
const WT_RENAMED = "src/renamed.txt"
const WT_DELETED = "src/gone.txt"
const WT_NEW = "notes/new.txt"
const WT_IGNORED = "debug.log"
const WT_PNG = "shot.png"

const openWorktree = (page: Page, target: string) =>
  go(page, `#/diff?project=demo&worktree=${target}`)
const treeFile = (page: Page, path: string) =>
  page.locator(`[data-testid="tree-file"][data-path="${path}"]`)
const git = (dir: string, ...args: string[]) => execFileSync("git", ["-C", dir, ...args]).toString()
const hasNoSidewaysScroll = (page: Page) =>
  page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)

/** What a read of the worktree must leave alone: its status, its index file and the lock beside it. */
function worktreeState(dir: string) {
  const index = resolve(dir, git(dir, "rev-parse", "--git-path", "index").trim())
  return {
    status: git(dir, "status", "--porcelain"),
    mtime: statSync(index).mtimeMs,
    sha: createHash("sha256").update(readFileSync(index)).digest("hex"),
    locked: existsSync(`${index}.lock`),
  }
}

test("Worktree diff targets", async ({ page, start }) => {
  await page.setViewportSize({ width: 1200, height: 900 })
  const fixture = await start("worktree-diff", { reloads: 1 })
  const wt = fixture.worktreeDiff!
  const chip = chipOf(page)
  const disclosure = page.getByTestId("commits-disclosure")
  const uncommitted = disclosure.getByRole("button", { name: /Uncommitted changes/ })

  await test.step("a chat: the chip, the tree, story 42 and a read-only worktree", async () => {
    const before = worktreeState(wt.chat.path)
    await openWorktree(page, `chat:${wt.chat.id}`)
    await expect(chip).toHaveText(
      new RegExp(`^Worktree · (?!${wt.chat.id} )\\S.* since main@[0-9a-f]{7}$`),
    )
    await expect(baseRef(page)).toHaveCount(0)
    await expect(tipRef(page)).toHaveCount(0)
    await expect(page.getByRole("button", { name: "Edit refs" })).toHaveCount(0)
    for (const path of ["src/added.txt", WT_MODIFIED, WT_RENAMED, WT_DELETED, WT_NEW, WT_PNG])
      await expect(treeFile(page, path)).toBeVisible()
    await expect(treeFile(page, WT_IGNORED)).toHaveCount(0)
    await expect(treeFile(page, WT_NEW).getByTestId("file-dot")).toHaveAttribute(
      "data-tone",
      "done",
    )
    await expect(treeFile(page, WT_DELETED).getByTestId("file-dot")).toHaveAttribute(
      "data-tone",
      "waiting",
    )

    // Story 42: a renamed path line, a deleted row's dot (above) and an image body.
    await treeFile(page, WT_RENAMED).click()
    await expect(page.getByTestId("file-path")).toHaveText("src/old.txt → src/renamed.txt")
    await treeFile(page, WT_PNG).click()
    await expect(page.getByTestId("diff-area")).toHaveAttribute("data-file", WT_PNG)
    await expect(page.getByTestId("image-after")).toBeVisible()

    await disclosure.getByRole("button", { name: /^\d+ commits?$/ }).click()
    await expect(disclosure.getByRole("button").last()).toHaveText(/Uncommitted changes$/)
    await uncommitted.click()
    await expect(page).toHaveURL(/commits=uncommitted/)
    await expect(treeFiles(page)).toHaveCount(5)
    await expect(treeFile(page, "src/added.txt")).toHaveCount(0)

    await page.reload()
    await expect(chip).toHaveText(new RegExp(`^Worktree · (?!${wt.chat.id} )\\S.* since main@`))
    await expect(treeFiles(page)).toHaveCount(5)

    // Reading the worktree leaves its status and index alone.
    expect(worktreeState(wt.chat.path)).toEqual(before)
  })

  await test.step("a Design and a claimed ticket", async () => {
    await openWorktree(page, `design:${wt.design.address!.slice("demo:".length)}`)
    await expect(chip).toContainText("Worktree · wtd since main@")
    await expect(baseRef(page)).toHaveCount(0)
    await expect(treeFile(page, WT_MODIFIED)).toBeVisible()
    await expect(treeFile(page, "work/wtd.txt")).toBeVisible()

    await openWorktree(page, `ticket:${wt.ticket.address}`)
    await expect(chip).toHaveText(
      new RegExp(`^Worktree · ${wt.ticket.address} since wtd-claimed-impl@[0-9a-f]{7}$`),
    )
    await expect(baseRef(page)).toHaveCount(0)
    await expect(page.getByRole("button", { name: "Edit refs" })).toHaveCount(0)
    await expect(treeFile(page, "work/wtd-claimed-impl.txt")).toBeVisible()
    await expect(treeFile(page, WT_MODIFIED)).toBeVisible()
    await disclosure.getByRole("button", { name: /^\d+ commits?$/ }).click()
    await expect(disclosure.getByRole("button").last()).toHaveText(/Uncommitted changes$/)
    await uncommitted.click()
    await expect(treeFiles(page)).toHaveCount(1)
    await expect(treeFile(page, WT_MODIFIED)).toBeVisible()
  })

  await test.step("390 px: the claimed ticket fits", async () => {
    await page.setViewportSize({ width: 390, height: 844 })
    await openWorktree(page, `ticket:${wt.ticket.address}`)
    await expect(page.getByRole("tab", { name: "Files" })).toHaveAttribute("aria-selected", "true")
    await expect(page.getByRole("tab", { name: "Diff" })).toBeVisible()
    await expect(chip).toBeVisible()
    await expect(disclosure).toBeVisible()
    await expect(treeFiles(page).first()).toBeVisible()
    await expect.poll(() => hasNoSidewaysScroll(page)).toBe(true)
    const box = await chip.boundingBox()
    expect(box!.x + box!.width).toBeLessThanOrEqual(390)
  })
})

test("Worktree diff edge states", async ({ page, start }) => {
  await page.setViewportSize({ width: 1200, height: 900 })
  const fixture = await start("worktree-diff", {})
  const wt = fixture.worktreeDiff!
  const chip = chipOf(page)
  const notice = page.getByTestId("notice")

  await test.step("a paused ticket's base: tracked run, blocker, feature branch", async () => {
    await openWorktree(page, `ticket:${wt.paused_run.address}`)
    await expect(chip).toHaveText(
      new RegExp(`^Worktree · ${wt.paused_run.address} since wtd-ask-impl@[0-9a-f]{7}$`),
    )
    await expect(treeFiles(page).first()).toBeVisible()

    await openWorktree(page, `ticket:${wt.paused_stacked.address}`)
    await expect(chip).toHaveText(
      new RegExp(
        `^Worktree · ${wt.paused_stacked.address} since demo:wtd-pstack-impl/01@[0-9a-f]{7}$`,
      ),
    )
    await expect(baseRef(page)).toHaveCount(0)
    // A stacked ticket hides its blocker's commits.
    await expect(treeFile(page, "work/wtd-pstack-impl-02.txt")).toBeVisible()
    await expect(treeFile(page, "work/wtd-pstack-impl-01.txt")).toHaveCount(0)

    await openWorktree(page, `ticket:${wt.paused_feature.address}`)
    await expect(chip).toHaveText(
      new RegExp(
        `^Worktree · ${wt.paused_feature.address} since ${wt.feature_branch}@[0-9a-f]{7}$`,
      ),
    )
  })

  await test.step("a ticket with no worktree and a missing chat show the empty state", async () => {
    await openWorktree(page, `ticket:${wt.unclaimed}`)
    await expect(notice).toHaveAttribute("data-kind", "no-worktree")
    await expect(notice).toContainText(wt.unclaimed)
    await expect(baseRef(page)).toHaveCount(0)
    await expect(page.getByRole("button", { name: "Edit refs" })).toHaveCount(0)
    await expect(treeFiles(page)).toHaveCount(0)

    const url = new URL("/v1/diff/worktree?project=demo&target=chat:no-such-chat", page.url())
    const res = await page.request.get(url.href)
    expect(res.status()).toBeLessThan(500)
    expect(await res.json()).toEqual({ reason: "no_worktree", target_label: "no-such-chat" })
  })

  await test.step("a clean worktree has no changes", async () => {
    await openWorktree(page, `chat:${wt.clean.id}`)
    await expect(notice).toHaveAttribute("data-kind", "no-changes")
    await expect(notice).toHaveText(/No changes since main@[0-9a-f]{7}/)
  })

  await test.step("untracked files left out give a notice and keep the tracked changes", async () => {
    await openWorktree(page, `chat:${wt.capped.id}`)
    const skipped = page.locator('[data-testid="notice"][data-kind="untracked-skipped"]')
    await expect(skipped).toContainText(
      /Untracked files not shown \(\d+\)\. Add them to \.gitignore or commit them\./,
    )
    const capped = (await skipped.textContent())!.match(/\((\d+)\)/)![1]
    await expect(treeFile(page, "src/capped.txt")).toBeVisible()

    await openWorktree(page, `chat:${wt.oversize.id}`)
    await expect(skipped).toContainText(
      "Untracked files not shown (1). Add them to .gitignore or commit them.",
    )
    expect(capped).not.toBe("1")
    await expect(treeFile(page, "big/small.txt")).toBeVisible()
    await expect(treeFile(page, "big/huge.txt")).toHaveCount(0)
  })
})

test("Worktree diff from the Show diff action", async ({ page, start }) => {
  await page.setViewportSize({ width: 1200, height: 900 })
  const fixture = await start("worktree-diff", {})
  const wt = fixture.worktreeDiff!
  const showDiff = async (id: string) => {
    await go(page, `#/agents/${id}`)
    await page.locator("header").getByRole("button", { name: "More actions", exact: true }).click()
    await page
      .getByRole("menu", { name: "More actions" })
      .getByRole("menuitem", { name: "Show diff" })
      .click()
    await expect(page.getByTestId("range-chip")).toBeVisible()
  }

  await showDiff(wt.ticket.id!)
  await expect(page.getByTestId("range-chip")).toHaveText(
    new RegExp(`^Worktree · ${wt.ticket.address} since wtd-claimed-impl@[0-9a-f]{7}$`),
  )
  await expect(treeFile(page, "work/wtd-claimed-impl.txt")).toBeVisible()

  await showDiff(wt.fix_agent.id!)
  await expect(page.getByTestId("range-chip")).toHaveText(
    new RegExp(`^Worktree · (?!${wt.fix_agent.id} )\\S.* since main@[0-9a-f]{7}$`),
  )
  await expect(treeFile(page, WT_MODIFIED)).toBeVisible()
})
