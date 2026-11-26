import { appendFileSync } from "node:fs"
import type { Locator, Page } from "@playwright/test"
import { card, expect, test } from "./fixture"
import { chatOf, go, sessions, sidebar } from "./agentsKit"

const list = (scope: Page | Locator) => scope.getByRole("list", { name: "Tasks" })
const rows = (scope: Page | Locator) => list(scope).getByRole("listitem")
const rowOf = (scope: Page | Locator, label: string) =>
  rows(scope).filter({ hasText: label }).first()
/** The elapsed text is the last cell of a row's first line, a button or a plain box. */
const elapsed = (page: Page, label: string) =>
  rowOf(page, label).locator(":scope > :first-child > span").last()
const noSideScroll = (page: Page) =>
  expect
    .poll(() => page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth))
    .toBe(false)

// Why a flow: a real server round trip from the fixture's transcripts and rollouts to the chat read to the page,
// the same rows on two pages, a real clock ticking and real layout at 390px. WorkingCard.test.tsx (vitest) covers
// order, marks, formats, tick, details and command alone and cannot see those. The group lifecycle is a server
// rule, asserted by the Go tests of the assembler and readers.
test("208. Task list", async ({ page, start }) => {
  // Claude shape: the same list on the Design page and the Agents page.
  const fixture = await start("bg-progress", { shape: "claude" })
  const claude = (await sessions(fixture)).find((s) => s.harness !== "codex")!
  const pages = [`#/d/demo:design-epic`, `#/agents/${claude.id}`]
  let first: (string | null | undefined)[] = []
  for (const hash of pages) {
    await go(page, hash)
    // Every row shows with no click and no summary button, the 4 running ones first.
    const items = rows(page)
    await expect(items).toHaveCount(7)
    await expect(page.getByRole("button", { name: /tasks done/ })).toHaveCount(0)
    const marks = list(page).getByRole("img")
    await expect(marks).toHaveCount(7)
    for (let i = 0; i < 4; i++) await expect(marks.nth(i)).toHaveAccessibleName("running")
    await expect(list(page).getByRole("img", { name: "running" })).toHaveCount(4)
    const labels = await items.evaluateAll((els) =>
      els.map((e) => e.querySelector("[title]")?.getAttribute("title")),
    )
    if (first.length === 0) first = labels
    else expect(labels).toEqual(first)
    // The subagent name links to its page and its row has no details button.
    const sub = rowOf(page, "Survey the sessions code")
    await expect(sub.getByRole("link", { name: "Survey the sessions code ↗" })).toHaveAttribute(
      "href",
      new RegExp(`^#/agents/${claude.id}/subagents/.+`),
    )
    await expect(sub.getByRole("button")).toHaveCount(0)
    // A shell row opens its details and command.
    const shell = rowOf(page, "python3")
    const shellButton = shell.getByRole("button").first()
    await shellButton.click()
    await expect(shellButton).toHaveAttribute("aria-expanded", "true")
    await expect(shell.getByTestId("task-details")).toBeVisible()
    await expect(shell).toContainText("time.sleep(5)")
    await shellButton.click()
    await expect(shellButton).toHaveAttribute("aria-expanded", "false")
    const working = page.getByRole("status").filter({ hasText: /^Working/ })
    await expect(working).toBeVisible()
    await expect(working).not.toContainText("waiting for sub-agent")
    await expect(working).not.toContainText("running shell:")
  }

  // An open row survives a chat reload.
  const kept = rowOf(page, "python3").getByRole("button").first()
  await kept.click()
  await expect(kept).toHaveAttribute("aria-expanded", "true")
  appendFileSync(
    fixture.transcripts.claude,
    `${JSON.stringify({
      type: "assistant",
      uuid: "bg-reload",
      timestamp: new Date().toISOString(),
      message: {
        id: "msg_reload",
        role: "assistant",
        content: [{ type: "text", text: "Still watching the log." }],
      },
    })}\n`,
  )
  await expect(page.getByText("Still watching the log.")).toBeVisible()
  await expect(kept).toHaveAttribute("aria-expanded", "true")

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

  // 390px, the long command open, on both pages.
  await page.setViewportSize({ width: 390, height: 844 })
  for (const hash of pages) {
    await go(page, hash)
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

  // One task: one row.
  await page.setViewportSize({ width: 1280, height: 800 })
  const single = await start("bg-progress", { shape: "single-task" })
  const one = (await sessions(single)).find((s) => s.harness !== "codex")!
  await go(page, `#/agents/${one.id}`)
  await expect(rows(page)).toHaveCount(1)

  // Codex: rows with label and time.
  const codex = await start("bg-progress", { shape: "codex" })
  const cx = (await sessions(codex)).filter((s) => s.harness === "codex")
  await go(page, `#/agents/${cx[0].id}`)
  await expect(rows(page)).toHaveCount(3)
  await expect(rowOf(page, "make build")).toBeVisible()
  await expect(elapsed(page, "make build")).toBeVisible()
  await expect(elapsed(page, "make build")).toHaveText(/\S/)

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

const TICKETS = "Explore tickets package"
const NAMES = "Find where task rows lose the name"
const PROBE = "Probe Codex rollout format"
const READ = "Read tickets/frontier.go"

// Why a flow: the card's rows come from the fixture's subagent files through the server's chat read, and a result
// appended to one of them must reach the page. WorkingCard.test.tsx (vitest) renders given props and cannot see that.
test("217. Working card states", async ({ page, start }) => {
  const open = async (shape: string) => {
    const fixture = await start("subagents", { shape })
    await go(page, `#/agents/${fixture.subagents!.sessionId}`)
    const working = page.locator("[data-mock-part='card']")
    await expect(working.getByRole("status")).toContainText("Working")
    return { fixture, working }
  }

  // Two running subagents around a finished one.
  const { working: two } = await open("card-two-running")
  await expect(rows(two)).toHaveCount(3)
  for (const [name, call] of [
    [TICKETS, READ],
    [PROBE, "rg -n spawn_agent codexsession"],
  ]) {
    await expect(two.getByRole("link", { name: `${name} ↗`, exact: true })).toBeVisible()
    await expect(rowOf(two, name)).toContainText(call)
  }
  const done = rowOf(two, NAMES)
  await expect(done.getByRole("img", { name: "done" })).toBeVisible()
  await expect(done.getByRole("link")).toHaveCount(0)
  const faint = await page.evaluate(() => {
    const probe = document.body.appendChild(document.createElement("span"))
    probe.style.color = "var(--chakra-colors-text-faint)"
    const color = getComputedStyle(probe).color
    probe.remove()
    return color
  })
  await expect(done.getByText(NAMES)).toHaveCSS("color", faint)
  // Scoped to the card: the log's tools entry names the calls too.
  for (const name of [TICKETS, NAMES, PROBE]) await expect(two.getByText(name)).toHaveCount(1)

  // No task: the header alone.
  const { working: none } = await open("card-no-tasks")
  await expect(list(none)).toHaveCount(0)

  // One subagent: its row loses the call line once the call has a result.
  const { fixture, working: one } = await open("card-one-subagent")
  await expect(rows(one)).toHaveCount(1)
  const row = rowOf(one, TICKETS)
  await expect(row).toContainText(READ)
  await expect(row.locator(":scope > *")).toHaveCount(2)
  const timestamp = new Date().toISOString()
  appendFileSync(
    fixture.taskFiles["subagent-card-tickets-file"],
    `${JSON.stringify({
      type: "user",
      isSidechain: true,
      timestamp,
      message: {
        role: "user",
        content: [
          { type: "tool_result", tool_use_id: "toolu_card_read", content: "package tickets" },
        ],
      },
    })}\n`,
  )
  // A subagent file change does not reload the chat; a parent line does.
  appendFileSync(
    fixture.subagents!.parentPath,
    `${JSON.stringify({
      type: "assistant",
      uuid: "card-reload",
      timestamp,
      message: {
        id: "msg_card_reload",
        role: "assistant",
        content: [{ type: "text", text: "Still waiting on the explorer." }],
      },
    })}\n`,
  )
  await expect(page.getByText("Still waiting on the explorer.")).toBeVisible()
  await expect(row.locator(":scope > *")).toHaveCount(1)
  await expect(row).not.toContainText(READ)
})
