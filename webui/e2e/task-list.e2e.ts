import { appendFileSync } from "node:fs"
import type { Locator, Page } from "@playwright/test"
import { card, expect, test } from "./fixture"
import { chatOf, go, sessions, sidebar } from "./agentsKit"

const summary = (page: Page, text: string) =>
  page.getByRole("button", { name: new RegExp(`^.?\\s*${text}`) })
const list = (page: Page) => page.getByRole("list", { name: "Tasks" })
const rowOf = (page: Page, label: string) =>
  list(page).getByRole("listitem").filter({ hasText: label }).first()
/** The elapsed text is the last cell of a row's header. */
const elapsed = (page: Page, label: string) => rowOf(page, label).locator("button > span").last()

/** The kind and label of every row, in order; the ticking elapsed cell (the last span) is left out. */
const rowsOf = (page: Page) =>
  list(page)
    .getByRole("listitem")
    .evaluateAll((els) =>
      els.map((e) => {
        const spans = [...(e.querySelector("button")?.querySelectorAll(":scope > span") ?? [])]
        return spans
          .slice(0, -1)
          .map((s) => s.textContent)
          .join("|")
      }),
    )
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

  // The working Design is purple everywhere and follows its session on the open page: the step the session reports
  // sets the word, a turn start and end flip the card, the sidebar row and the Design header with no reload.
  await page.setViewportSize({ width: 1440, height: 900 })
  const designCard = card(page, "design-epic")
  const designRow = sidebar(page).getByRole("link", { name: /design-epic/ })
  const cardRing = designCard.locator("[data-kind='design']").first()
  const rowRing = designRow.locator("[data-kind='design']")
  const headerRing = page
    .locator("[data-kind='design']:not([data-testid='sidebar-body'] *)")
    .first()
  const purple = async (ring: Locator) => {
    await expect(ring).toHaveAttribute("data-arc", "kind.design.fg")
    await expect(ring.getByTestId("comet")).toBeVisible()
  }
  // The Design page keeps its tone while a session lives; only the arc and the comet show that a turn runs.
  const stopped = async (ring: Locator) => {
    await expect(ring).not.toHaveAttribute("data-arc", /.+/)
    await expect(ring.getByTestId("comet")).toHaveCount(0)
  }
  const grey = async (ring: Locator) => {
    await expect(ring).toHaveAttribute("data-tone", "muted")
    await stopped(ring)
  }
  await go(page, "#/epics")
  // No step reported: the map's step word, "..." on the card only.
  await expect(designCard.getByTestId("status")).toHaveText("Researching...")
  await expect(designRow).toContainText("Researching")
  await expect(designRow).not.toContainText("...")
  await purple(cardRing)
  await purple(rowRing)
  await fixture.cliEnv({ GX_AGENT_SESSION: claude.id }, "agent", "step", "spec")
  await expect(designCard.getByTestId("status")).toHaveText("Writing spec...")
  await expect(designRow).toContainText("Writing spec")
  await expect(designRow).not.toContainText("...")
  await go(page, "#/d/demo:design-epic")
  await purple(headerRing)
  await purple(rowRing)

  // A turn and its background task end, then a turn starts, with the page open. The shape with one subagent is the
  // one whose task a flow can end (the claude shape's tasks run throughout).
  const turn = await start("bg-progress", { shape: "subagent-with-inner-task" })
  const worker = (await sessions(turn)).find((s) => s.harness !== "codex")!
  const workerConv = (await chatOf(turn, worker.id)).conversation_id
  await go(page, "#/epics")
  await purple(cardRing)
  await purple(rowRing)
  await go(page, "#/d/demo:design-epic")
  await purple(headerRing)
  await turn.controlClaude(workerConv, { action: "finish" })
  appendFileSync(
    turn.transcripts["subagent-with-inner-task"],
    `${JSON.stringify({
      type: "queue-operation",
      operation: "enqueue",
      timestamp: new Date().toISOString(),
      content:
        "<task-notification>\n<task-id>a1ner0001</task-id>\n<status>completed</status>\n<summary>Agent done</summary>\n<usage>\n<subagent_tokens>900</subagent_tokens>\n<tool_uses>1</tool_uses>\n</usage>\n</task-notification>",
    })}\n`,
  )
  await grey(rowRing)
  await stopped(headerRing)
  await go(page, "#/epics")
  await grey(cardRing)
  // The turn starts while the Design page is open: its header follows without a reload.
  await go(page, "#/d/demo:design-epic")
  await grey(rowRing)
  await stopped(headerRing)
  await expect
    .poll(() =>
      turn.cli("agents", "prompt", worker.id, "[slow] one more").then(
        () => true,
        () => false,
      ),
    )
    .toBe(true)
  await purple(headerRing)
  await purple(rowRing)
  await go(page, "#/epics")
  await purple(cardRing)

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
