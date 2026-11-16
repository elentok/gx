import type { Page } from "@playwright/test"
import { expect, test } from "./fixture"
import { go, sessions } from "./agentsKit"

const summary = (page: Page, text: string) =>
  page.getByRole("button", { name: new RegExp(`^.?\\s*${text}`) })
const list = (page: Page) => page.getByRole("list", { name: "Tasks" })
const rowOf = (page: Page, label: string) =>
  list(page).getByRole("listitem").filter({ hasText: label }).first()
/** The elapsed text is the last cell of a row's header. */
const elapsed = (page: Page, label: string) => rowOf(page, label).locator("button > span").last()

/** The labels and kinds of every row, in order. */
const rowsOf = (page: Page) =>
  list(page)
    .getByRole("listitem")
    .evaluateAll((els) => els.map((e) => e.querySelector("button")?.textContent ?? ""))
const noSideScroll = (page: Page) =>
  expect
    .poll(() => page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth))
    .toBe(false)

// Why a flow: a real server round trip from the fixture's transcripts and rollouts to the chat read to the page,
// the same rows on two pages, a real clock ticking and real layout at 390px. TaskList.test.tsx (vitest) covers
// summary, order, marks, formats, tick, details and command alone and cannot see those. The group lifecycle is a
// server rule, asserted by the Go tests of the assembler and readers.
test("208. Task list", async ({ page, start }) => {
  // Claude shape: the same list on the Design page and the Agents page.
  const fixture = await start("bg-progress", { shape: "claude" })
  const claude = (await sessions(fixture)).find((s) => s.harness !== "codex")!
  const pages = [`#/d/demo:design-epic`, `#/agents/${claude.id}`]
  let first: string[] = []
  for (const hash of pages) {
    await go(page, hash)
    const sum = summary(page, "3 of 7 tasks done")
    await expect(sum).toBeVisible()
    await expect(sum).toHaveAttribute("aria-expanded", "false")
    await sum.click()
    await expect(sum).toHaveAttribute("aria-expanded", "true")
    await expect(list(page)).toBeVisible()
    await expect(list(page).getByRole("listitem")).toHaveCount(7)
    const rows = await rowsOf(page)
    if (first.length === 0) first = rows
    else expect(rows).toEqual(first)
    const working = page.getByRole("status").filter({ hasText: /^Working/ })
    await expect(working).toBeVisible()
    await expect(working).not.toContainText("waiting on")
    await expect(working).not.toContainText("running shell:")
  }

  // The elapsed time of a running row moves with no request meanwhile.
  for (const label of ["Survey the sessions code", "Watch the app log", "python3"]) {
    const cell = elapsed(page, label)
    await expect(cell).toBeVisible()
    const before = await cell.textContent()
    await expect.poll(() => cell.textContent()).not.toBe(before)
  }
  const cell = elapsed(page, "Survey the sessions code")
  let requests = 0
  page.on("request", (r) => {
    if (r.url().includes("/v1/")) requests++
  })
  const t0 = await cell.textContent()
  await expect.poll(() => cell.textContent()).not.toBe(t0)
  const t1 = await cell.textContent()
  await expect.poll(() => cell.textContent()).not.toBe(t1)
  expect(requests).toBe(0)

  // 390px, list open and the long command open, on both pages.
  await page.setViewportSize({ width: 390, height: 844 })
  for (const hash of pages) {
    await go(page, hash)
    const sum = summary(page, "3 of 7 tasks done")
    await expect(sum).toBeVisible()
    if ((await sum.getAttribute("aria-expanded")) === "false") await sum.click()
    const script = rowOf(page, "bash").locator("button").first()
    await expect(script).toBeVisible()
    await script.click()
    await page.getByRole("button", { name: /^Show all \(\d+ lines\)$/ }).click()
    await noSideScroll(page)
  }

  // One task: the row alone, no summary button.
  await page.setViewportSize({ width: 1280, height: 800 })
  const single = await start("bg-progress", { shape: "single-task" })
  const one = (await sessions(single)).find((s) => s.harness !== "codex")!
  await go(page, `#/agents/${one.id}`)
  await expect(list(page).getByRole("listitem")).toHaveCount(1)
  await expect(summary(page, "tasks done")).toHaveCount(0)

  // Codex: a summary and rows with label and time.
  const codex = await start("bg-progress", { shape: "codex" })
  const cx = (await sessions(codex)).filter((s) => s.harness === "codex")
  await go(page, `#/agents/${cx[0].id}`)
  const csum = summary(page, "1 of 3 tasks done")
  await expect(csum).toBeVisible()
  await csum.click()
  await expect(list(page).getByRole("listitem")).toHaveCount(3)
  await expect(rowOf(page, "make build")).toBeVisible()

  // Old Codex: no list, the Working row keeps its in-flight text.
  const old = await start("bg-progress", { shape: "old-codex" })
  const oc = (await sessions(old)).find((s) => s.harness === "codex")!
  await go(page, `#/agents/${oc.id}`)
  await expect(page.getByRole("heading", { level: 1 })).toBeVisible()
  await expect(list(page)).toHaveCount(0)
  // The fake Codex reads Idle in the page, so no Working row renders; the chat read still labels the open call.
  await expect
    .poll(async () => {
      const chat = JSON.parse(await old.cli("agents", "chat", oc.id, "--json"))
      return chat.current?.label
    })
    .toBe("make old")
})
