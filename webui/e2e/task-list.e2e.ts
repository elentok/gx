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
/** The computed color of the `text.faint` token, the grey of a finished row. */
const faintColor = (page: Page) =>
  page.evaluate(() => {
    const probe = document.body.appendChild(document.createElement("span"))
    probe.style.color = "var(--chakra-colors-text-faint)"
    const color = getComputedStyle(probe).color
    probe.remove()
    return color
  })

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
  // The turn is over and its subagent still runs: rows alone, with no header and no moving border.
  await go(page, `#/agents/${worker.id}`)
  const waiting = page.locator("[data-mock-part='card']")
  await expect(rows(waiting).first()).toBeVisible()
  await expect(waiting.getByRole("status")).toHaveCount(0)
  await expect(
    page.locator("[data-mock-part='card'][data-live], [data-mock-part='card'] [data-live]"),
  ).toHaveCount(0)
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
  await expect(waiting).toHaveCount(0)
  await go(page, "#/d/demo:design-epic")
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

  // Old Codex: the card's header names the open call and there are no rows. The seeded fake Codex reads Idle in
  // the page, so a prompt starts a turn, which it holds until a finish control.
  const old = await start("bg-progress", { shape: "old-codex" })
  const oc = (await sessions(old)).find((s) => s.harness === "codex")!
  await go(page, `#/agents/${oc.id}`)
  await expect(page.getByRole("heading", { level: 1 })).toBeVisible()
  await old.cli("agents", "prompt", oc.id, "one more")
  const oldCard = page.locator("[data-mock-part='card']")
  await expect(oldCard.getByRole("status")).toContainText("Working")
  // The new turn drops the seeded open call, so the rollout gets another one, after the prompt's own line.
  await expect(page.getByText("one more", { exact: true })).toBeVisible()
  appendFileSync(
    old.transcripts["old-codex"],
    `${JSON.stringify({
      type: "response_item",
      timestamp: new Date().toISOString(),
      payload: {
        type: "function_call",
        name: "exec_command",
        call_id: "old2",
        arguments: JSON.stringify({ cmd: "make old" }),
      },
    })}\n`,
  )
  await expect(oldCard.getByRole("status")).toContainText("make old")
  await expect(list(page)).toHaveCount(0)
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
  await expect(done.getByText(NAMES)).toHaveCSS("color", await faintColor(page))
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

// Why a flow: Codex rollouts and Claude subagent files reach the card through the server, and a file path is
// shortened by real layout. WorkingCard.test.tsx (vitest) renders given props in jsdom and sees neither.
test("218. Working card shapes", async ({ page, start }) => {
  const open = async (shape: string) => {
    const fixture = await start("subagents", { shape })
    const { sessionId, calls } = fixture.subagents!
    await go(page, `#/agents/${sessionId}`)
    return { fixture, sessionId, calls, working: page.locator("[data-mock-part='card']") }
  }

  // Codex child (bgtx.CodexSubRunningCmd and CodexSubInheritedCmd): its own open call, none it inherited.
  const codex = await open("codex-running-call")
  await expect(rows(codex.working)).toHaveCount(1)
  await expect(codex.working).toContainText("go vet ./...")
  await expect(codex.working).not.toContainText("git log --oneline")
  await expect(codex.working.getByRole("status")).toHaveCount(0)
  const chat = JSON.parse(await codex.fixture.cli("agents", "chat", codex.sessionId, "--json"))
  expect(chat.tasks.map((t: { current_call?: string }) => t.current_call)).toEqual(["go vet ./..."])

  // A subagent started with a string flag (bgtx.SubStringDesc, SubStringShellDesc) beside a background shell.
  const str = await open("string-background")
  await expect(rows(str.working)).toHaveCount(2)
  await expect(
    str.working.getByRole("link", { name: "Audit the string flags ↗", exact: true }),
  ).toHaveAttribute(
    "href",
    `#/agents/${str.sessionId}/subagents/${str.calls.string}?from=%2Fagents%2F${str.sessionId}`,
  )
  await expect(rowOf(str.working, "Lint in the background")).toBeVisible()

  // File calls (bgtx.SubReadDesc and SubEditDesc), the older one first.
  const { working } = await open("file-call")
  const [read, edit] = [rows(working).nth(0), rows(working).nth(1)]
  await expect(read).toContainText("Read the task rows")
  await expect(edit).toContainText("Edit the shared notes")
  // The server cuts the long path's start; the browser may cut more of the middle part, never the ends.
  for (const width of [920, 390]) {
    await page.setViewportSize({ width, height: 844 })
    await expect(read.locator(":scope > *").nth(1)).toHaveText("Read server/sessions/chat/tasks.go")
    await edit.scrollIntoViewIfNeeded()
    for (const part of ["call-tool", "call-file"])
      await expect(edit.locator(`[data-part='${part}']`)).toBeInViewport({ ratio: 0.99 })
    await expect(edit.locator("[data-part='call-tool']")).toHaveText(/^Edit /)
    await expect(edit.locator("[data-part='call-file']")).toHaveText("/current-call-labels.md")
    await expect(edit.locator(":scope > *").nth(1)).toHaveText(
      "Edit …/decisions/server/sessions/current-call-labels.md",
    )
    await noSideScroll(page)
  }
})

// Why a flow: the fixture's sessions reach three routes through the server, each page wiring its own header and the
// Triage page's open round its own view. SteppedPage.test.tsx and WorkingCard.test.tsx (vitest) render given props.
test("220. Working card pages", async ({ page, start }) => {
  const fixture = await start("card-pages")
  const plain = (await sessions(fixture)).find((s) => !s.flow)!
  const working = page.locator("[data-mock-part='card']")
  const sameCard = async (header: RegExp) => {
    await expect(working).toHaveCount(1)
    await expect(working.getByRole("status")).toHaveText(header)
    await expect(rows(working)).toHaveCount(2)
    const running = rowOf(working, "Watch the build in the background")
    await expect(running.getByRole("img", { name: "running" })).toBeVisible()
    await expect(running.getByRole("link")).toHaveAttribute("href", /\/subagents\//)
    const done = rowOf(working, "Check the wire types")
    await expect(done.getByRole("img", { name: "done" })).toBeVisible()
    await expect(done.getByRole("link")).toHaveCount(0)
    await expect(done.getByText("Check the wire types")).toHaveCSS("color", await faintColor(page))
    await expect(working).toBeInViewport()
  }

  await go(page, "#/d/demo:design-epic")
  await expect(page).toHaveURL(/#\/d\/demo:design-epic$/)
  await sameCard(/^Working: Resolve tickets/)

  await go(page, `#/agents/${plain.id}`)
  await expect(page).toHaveURL(new RegExp(`#/agents/${plain.id}$`))
  await sameCard(/^Working(?!:)/)

  // The Triage session has an open round, so its page opens in the Questions view. Its header has no title.
  await go(page, "#/triage/demo")
  const view = page.getByRole("group", { name: "View" })
  const questions = view.getByRole("button", { name: /^Questions/ })
  await expect(questions).toHaveAttribute("aria-pressed", "true")
  await sameCard(/^Working: Questions(?! ·)/)
  await view.getByRole("button", { name: "Chat" }).click()
  await expect(questions).toHaveAttribute("aria-pressed", "false")
  await sameCard(/^Working: Questions(?! ·)/)
})

/** Two Claude transcript lines: a background shell and its launch result. */
function longShell(command: string): string {
  const timestamp = new Date().toISOString()
  const id = "toolu_long"
  return [
    {
      type: "assistant",
      uuid: "long-use",
      timestamp,
      message: {
        id: "msg_long",
        role: "assistant",
        content: [
          {
            type: "tool_use",
            id,
            name: "Bash",
            input: { command, description: "Run a long script", run_in_background: true },
          },
        ],
      },
    },
    {
      type: "user",
      uuid: "long-result",
      timestamp,
      message: {
        role: "user",
        content: [
          {
            type: "tool_result",
            tool_use_id: id,
            content: "Command running in background with ID: blong0001.",
          },
        ],
      },
      toolUseResult: { backgroundTaskId: "blong0001" },
    },
  ]
    .map((line) => `${JSON.stringify(line)}\n`)
    .join("")
}

// Why a flow: the cap, the scrolling rows and the header that stays put are real layout and scroll, which jsdom
// (WorkingCard.test.tsx, Card.test.tsx) cannot see.
test("219. Working card height cap", async ({ page, start }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  const fixture = await start("bg-progress", { shape: "many" })
  const claude = (await sessions(fixture)).find((s) => s.harness !== "codex")!
  await go(page, `#/agents/${claude.id}`)
  const working = page.locator("[data-mock-part='card']")
  const header = working.getByRole("status")
  await expect(rows(working)).toHaveCount(50)
  await expect(header).toContainText("Working")
  await expect(list(working)).toBeVisible()

  const layout = () =>
    expect
      .poll(async () => {
        const box = await working.boundingBox()
        // The scrolling log is the card's previous sibling.
        const log = await working.locator("xpath=preceding-sibling::*[1]").boundingBox()
        const scrolls = await list(working).evaluate((el) => el.scrollHeight > el.clientHeight)
        return {
          capped: !!box && box.height <= 844 * 0.4 + 0.5,
          scrolls,
          log: !!box && !!log && log.height > 100 && log.y + log.height <= box.y + 0.5,
        }
      })
      .toEqual({ capped: true, scrolls: true, log: true })
  await layout()

  await rows(working).last().scrollIntoViewIfNeeded()
  await expect(rows(working).last()).toBeInViewport()
  await expect(header).toBeInViewport({ ratio: 1 })
  await layout()

  // An open shell row's long command scrolls inside the card too. The shape's commands are one short line, so the
  // transcript gets a long one.
  appendFileSync(
    fixture.transcripts.many,
    longShell(Array.from({ length: 60 }, (_, i) => `echo line-${i}`).join("\n")),
  )
  const shell = rowOf(working, "Run a long script")
  await shell.getByRole("button").first().click()
  await shell.getByRole("button", { name: "Show all (60 lines)" }).click()
  await layout()
  await shell.getByText("echo line-59").scrollIntoViewIfNeeded()
  await expect(shell.getByText("echo line-59")).toBeInViewport()
  await expect(header).toBeInViewport({ ratio: 1 })
  await layout()
  await noSideScroll(page)
})
