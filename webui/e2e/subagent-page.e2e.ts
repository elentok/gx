import { appendFileSync } from "node:fs"
import type { Locator, Page } from "@playwright/test"
import type { SessionChatResponse } from "../src/generated/server"
import { expect, test } from "./fixture"
import { go, sessions } from "./agentsKit"

const DESIGN = "#/d/demo:design-epic"
const FINISHED = "Check the wire types"
const TICKETS = "Explore tickets package"
const NAMES = "Find where task rows lose the name"
const PROBE = "Probe Codex rollout format"
const RUNNING = "Survey the sessions code"
const MISSING = "Read the old notes"
const NESTED = "Map the server packages"
const GRANDCHILD = "Read the wire package"
/** The running subagent of the card-pages scenario. */
const WATCH = "Watch the build in the background"

const namedLink = (page: Page, description: string) =>
  page.getByRole("link", { name: `${description} ↗`, exact: true })
/** The chat entry's link; the working card links a running subagent by the same name. */
const callLink = (page: Page, description: string) =>
  namedLink(page, description).and(page.locator(":not([data-mock-part='card'] *)"))
const cardLink = (page: Page, description: string) =>
  namedLink(page, description).and(page.locator("[data-mock-part='card'] *"))
const backLink = (page: Page) => page.getByRole("link", { name: "← Back to parent chat" })
const title = (page: Page, text: string) =>
  expect(page.getByRole("heading", { level: 1, name: text })).toBeVisible()
/** A subagent URL; `origin` is the router path of the page whose link was followed. */
const subUrl = (session: string, callId: string, origin?: string) =>
  `#/agents/${session}/subagents/${callId}${origin ? `?from=${encodeURIComponent(origin)}` : ""}`
/** Matches the end of the address bar, or of an href, exactly. */
const ending = (hash: string) => new RegExp(`${hash.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}$`)
const noSideScroll = (page: Page) =>
  expect
    .poll(() => page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth))
    .toBe(false)
/** Opens every folded tools entry so the marked calls show as links. */
const openTools = async (page: Page) => {
  const entries = page.getByTestId("tool-calls")
  await expect(entries.first()).toBeVisible()
  for (const summary of await entries.locator("summary").all()) await summary.click()
}

// Why a flow: a real server round trip from the fixture's transcripts, meta.json files and rollouts to the chat read to
// the page, a page-to-page journey across the Agents, Design and subagent pages with the browser Back button and a
// reload, a live file growth seen with no reload, real layout at 390px, and the back link's origin carried through
// real URLs from the Design, chat and Triage pages. SubagentPage.test.tsx, AgentsPage.subagent.test.tsx,
// WorkingCard.test.tsx, ChatLog.test.tsx and the Go server tests cover each rule alone and cannot see these together.
test("216. Subagent page", async ({ page, start }) => {
  await page.setViewportSize({ width: 1280, height: 800 })
  const fixture = await start("subagents", { shape: "claude", reloads: 1 })
  const sub = fixture.subagents!
  const session = sub.sessionId
  const runningUrl = ending(subUrl(session, sub.calls.running))
  const fromDesign = ending(subUrl(session, sub.calls.running, DESIGN.slice(1)))

  // 1. A running call is linked by the working card only, on the Agents page and on the Design page.
  await go(page, `#/agents/${session}`)
  await openTools(page)
  await expect(cardLink(page, RUNNING)).toHaveAttribute(
    "href",
    ending(subUrl(session, sub.calls.running, `/agents/${session}`)),
  )
  await expect(callLink(page, RUNNING)).toHaveCount(0)
  await go(page, DESIGN)
  await expect(page).toHaveURL(new RegExp(`${DESIGN}$`))
  await expect(cardLink(page, RUNNING)).toHaveAttribute("href", fromDesign)
  await expect(callLink(page, RUNNING)).toHaveCount(0)

  // 2. Open from the Design page, Back, open again, the back link, then a reload.
  await cardLink(page, RUNNING).click()
  await expect(page).toHaveURL(fromDesign)
  await title(page, RUNNING)
  await page.goBack()
  await expect(page).toHaveURL(new RegExp(`${DESIGN}$`))
  await cardLink(page, RUNNING).click()
  await expect(page).toHaveURL(fromDesign)
  await backLink(page).click()
  await expect(page).toHaveURL(new RegExp(`${DESIGN}$`))
  await go(page, subUrl(session, sub.calls.running))
  await expect(page).toHaveURL(runningUrl)
  await title(page, RUNNING)
  await page.reload()
  await expect(page).toHaveURL(runningUrl)
  await title(page, RUNNING)

  // 6. Live: the subagent file grows and the new message shows with no reload.
  const live = "Live line from the fixture."
  appendFileSync(
    fixture.taskFiles["subagent-running-file"],
    `${JSON.stringify({
      type: "assistant",
      isSidechain: true,
      timestamp: new Date().toISOString(),
      message: {
        id: "msg_live",
        model: "claude-sonnet-5-5",
        role: "assistant",
        content: [{ type: "text", text: live }],
        usage: { input_tokens: 5000, output_tokens: 90 },
      },
    })}\n`,
  )
  await expect(page.getByText(live)).toBeVisible()

  // 7. Phone: the running page and the page of a call whose file is gone.
  await page.setViewportSize({ width: 390, height: 844 })
  const header = page.locator("[data-part='subagent-header']")
  await expect
    .poll(async () => {
      const box = await header.boundingBox()
      const h1 = await page.getByRole("heading", { level: 1 }).boundingBox()
      return !!box && !!h1 && box.x >= 0 && box.x + box.width <= 390 && h1.x + h1.width <= 390
    })
    .toBe(true)
  await noSideScroll(page)
  await go(page, `#/agents/${session}/subagents/${sub.calls.missing}`)
  await expect(page.getByText("Transcript no longer available")).toBeVisible()
  await title(page, MISSING)
  await expect
    .poll(async () => {
      const box = await header.boundingBox()
      return !!box && box.x >= 0 && box.x + box.width <= 390
    })
    .toBe(true)
  await noSideScroll(page)
  await page.setViewportSize({ width: 1280, height: 800 })

  // 3. A finished call: the Design page and the folded chat log link it once, and the link opens the page.
  const ended = await start("subagents", { shape: "finished-handback" })
  const endedSub = ended.subagents!
  await go(page, DESIGN)
  await expect(page).toHaveURL(new RegExp(`${DESIGN}$`))
  await expect(callLink(page, FINISHED)).toHaveCount(1)
  await go(page, `#/agents/${endedSub.sessionId}`)
  await expect(page).toHaveURL(new RegExp(`#/agents/${endedSub.sessionId}$`))
  await expect(callLink(page, FINISHED)).toBeVisible()
  await openTools(page)
  await expect(page.getByTestId("tool-calls").getByRole("link")).toHaveCount(1)
  await expect(callLink(page, FINISHED)).toHaveCount(1)
  await callLink(page, FINISHED).click()
  await expect(page).toHaveURL(
    ending(subUrl(endedSub.sessionId, endedSub.calls.finished, `/agents/${endedSub.sessionId}`)),
  )
  await title(page, FINISHED)

  // 9. Two running calls around a finished one: only the finished one is a log link, on both pages.
  const two = await start("subagents", { shape: "card-two-running" })
  const ts = two.subagents!
  const chatUrl = new RegExp(`#/agents/${ts.sessionId}$`)
  const card = page.locator("[data-mock-part='card']")
  const states = async () => {
    const chat: SessionChatResponse = JSON.parse(
      await two.cli("agents", "chat", ts.sessionId, "--json"),
    )
    return Object.fromEntries(
      chat.messages.flatMap((m) => m.subagent_calls ?? []).map((c) => [c.call_id, c.state]),
    )
  }
  expect(await states()).toEqual({
    [ts.calls.tickets]: "running",
    [ts.calls.names]: "finished",
    [ts.calls.probe]: "running",
  })

  await go(page, `#/agents/${ts.sessionId}`)
  await expect(page).toHaveURL(chatUrl)
  await expect(callLink(page, NAMES)).toBeVisible()
  for (const name of [TICKETS, PROBE]) {
    await expect(cardLink(page, name)).toBeVisible()
    await expect(callLink(page, name)).toHaveCount(0)
    await expect(page.getByText(`Agent · Explore · ${name}`)).toHaveCount(0)
  }
  await openTools(page)
  await expect(page.getByTestId("tool-calls").getByRole("link")).toHaveCount(1)
  await expect(callLink(page, NAMES)).toHaveCount(1)
  for (const name of [TICKETS, PROBE])
    await expect(page.getByText(`Agent · Explore · ${name}`, { exact: true })).toBeVisible()

  await go(page, `#/agents/${ts.sessionId}/subagents/${ts.calls.tickets}`)
  await title(page, TICKETS)
  await expect(page.getByRole("status").filter({ hasText: /^Working/ })).toHaveText(
    "Working · Read tickets/frontier.go",
  )
  await expect(card).toHaveCount(0)

  // 10. One subagent ends while the Design page is open: its log link appears with no reload, and the chat page agrees.
  await go(page, DESIGN)
  await expect(page).toHaveURL(new RegExp(`${DESIGN}$`))
  await expect(callLink(page, NAMES)).toHaveCount(1)
  await expect(cardLink(page, TICKETS)).toBeVisible()
  await expect(callLink(page, TICKETS)).toHaveCount(0)
  await expect(callLink(page, PROBE)).toHaveCount(0)
  appendFileSync(
    ts.parentPath,
    `${JSON.stringify({
      type: "user",
      timestamp: new Date().toISOString(),
      message: { content: [{ type: "tool_result", tool_use_id: ts.calls.tickets, content: "ok" }] },
      toolUseResult: { status: "completed" },
    })}\n`,
  )
  await expect(callLink(page, TICKETS)).toBeVisible()
  await expect(cardLink(page, TICKETS)).toHaveCount(0)
  await expect(cardLink(page, PROBE)).toBeVisible()
  await expect(callLink(page, PROBE)).toHaveCount(0)
  await go(page, `#/agents/${ts.sessionId}`)
  await expect(page).toHaveURL(chatUrl)
  await expect(callLink(page, TICKETS)).toBeVisible()
  expect((await states())[ts.calls.tickets]).toBe("finished")

  // 4. Nested Claude subagent: a link of the same shape, the back links, and a direct open with the breadcrumb.
  const nested = await start("subagents", { shape: "nested" })
  const ns = nested.subagents!
  const parentUrl = new RegExp(`#/agents/${ns.sessionId}/subagents/${ns.calls.nested}$`)
  await go(page, `#/agents/${ns.sessionId}/subagents/${ns.calls.nested}`)
  await expect(page).toHaveURL(parentUrl)
  await openTools(page)
  await callLink(page, GRANDCHILD).click()
  await expect(page).toHaveURL(new RegExp(`/subagents/${ns.calls.grandchild}$`))
  await title(page, GRANDCHILD)
  await backLink(page).click()
  await expect(page).toHaveURL(parentUrl)
  await backLink(page).click()
  await expect(page).toHaveURL(new RegExp(`#/agents/${ns.sessionId}$`))
  await go(page, `#/agents/${ns.sessionId}/subagents/${ns.calls.grandchild}`)
  await title(page, GRANDCHILD)
  const owner = (await sessions(nested)).find((s) => s.id === ns.sessionId)!
  await expect(page.getByText(/ › /).first()).toContainText(owner.name || owner.id)
  await expect(page.getByRole("link", { name: NESTED, exact: true })).toBeVisible()

  // 11. Nested from the Design page: back to the parent subagent, then again, then to the Design page.
  const origin = DESIGN.slice(1)
  const fromParent = ending(subUrl(ns.sessionId, ns.calls.nested, origin))
  await go(page, DESIGN)
  await expect(page).toHaveURL(ending(DESIGN))
  await cardLink(page, NESTED).click()
  await expect(page).toHaveURL(fromParent)
  await openTools(page)
  await callLink(page, GRANDCHILD).click()
  await expect(page).toHaveURL(ending(subUrl(ns.sessionId, ns.calls.grandchild, origin)))
  await expect(page.getByRole("link", { name: NESTED, exact: true })).toHaveAttribute(
    "href",
    fromParent,
  )
  await backLink(page).click()
  await expect(page).toHaveURL(fromParent)
  await title(page, NESTED)
  await backLink(page).click()
  await expect(page).toHaveURL(ending(DESIGN))

  // 8. A current file call keeps its tool and file name; on a phone the long path is cut.
  const files = await start("subagents", { shape: "file-call" })
  const fs = files.subagents!
  const working = page.getByRole("status").filter({ hasText: /^Working/ })
  const calls = [
    [fs.calls.read, "Read server/sessions/chat/tasks.go"],
    [fs.calls.edit, "Edit …/decisions/server/sessions/current-call-labels.md"],
  ]
  for (const width of [920, 390]) {
    await page.setViewportSize({ width, height: 844 })
    for (const [call, label] of calls) {
      await go(page, `#/agents/${fs.sessionId}/subagents/${call}`)
      await expect(page).toHaveURL(new RegExp(`/subagents/${call}$`))
      await expect(working).toHaveText(`Working · ${label}`)
      if (width !== 390) continue
      for (const part of ["call-tool", "call-file"])
        await expect(working.locator(`[data-part='${part}']`)).toBeInViewport({ ratio: 0.99 })
      await noSideScroll(page)
    }
  }
  const dir = working.locator("[data-part='call-dir']")
  await expect(dir).toBeVisible()
  await expect.poll(() => dir.evaluate((e) => e.scrollWidth > e.clientWidth)).toBe(true)
  await page.screenshot({ path: test.info().outputPath("file-call-390.png") })
  await page.setViewportSize({ width: 1280, height: 800 })

  // 5. Codex child: its nested call has a line and no link.
  const codex = await start("subagents", { shape: "codex-nested" })
  const cs = codex.subagents!
  await go(page, `#/agents/${cs.sessionId}/subagents/${cs.calls.spawn}`)
  await expect(page).toHaveURL(new RegExp(`/subagents/${cs.calls.spawn}$`))
  await expect(page.getByRole("heading", { level: 1 })).toBeVisible()
  await openTools(page)
  const entry = page.getByTestId("tool-calls")
  await expect(entry.getByText("spawn_agent", { exact: true })).toBeVisible()
  await expect(entry.getByRole("link")).toHaveCount(0)

  // 12. Back link, from the card and from a log link on the Design, chat and Triage pages.
  const pages = await start("card-pages", { reloads: 1 })
  const plain = (await sessions(pages)).find((s) => !s.flow)!
  const view = page.getByRole("group", { name: "View" })
  // Opens a link, checks the origin its URL carries, and follows the back link to that page.
  const roundTrip = async (link: Locator, origin: string) => {
    await link.click()
    await expect(page).toHaveURL(ending(`?from=${encodeURIComponent(origin)}`))
    await expect(page).toHaveURL(/#\/agents\/[^/]+\/subagents\//)
    await backLink(page).click()
    await expect(page).toHaveURL(ending(`#${origin}`))
  }

  for (const origin of [DESIGN.slice(1), `/agents/${plain.id}`, "/triage/demo"]) {
    await go(page, `#${origin}`)
    await expect(page).toHaveURL(ending(`#${origin}`))
    await roundTrip(cardLink(page, WATCH), origin)
    // The Triage session has an open round, so its page comes back in the Questions view, which has no log.
    if (origin.startsWith("/triage")) await view.getByRole("button", { name: "Chat" }).click()
    await roundTrip(callLink(page, FINISHED), origin)
  }

  // A reload keeps the target. No origin, or an address outside the app, opens the chat page.
  await go(page, DESIGN)
  await cardLink(page, WATCH).click()
  await expect(page).toHaveURL(/\/subagents\/[^?]+\?from=/)
  const [bare] = new URL(page.url()).hash.split("?")
  await page.reload()
  await expect(backLink(page)).toHaveAttribute("href", DESIGN)
  await backLink(page).click()
  await expect(page).toHaveURL(ending(DESIGN))
  const chatHref = `#/agents/${bare.split("/")[2]}`
  for (const query of ["", `?from=${encodeURIComponent("https://example.com/d/x")}`]) {
    await go(page, bare + query)
    await expect(backLink(page)).toHaveAttribute("href", chatHref)
    await backLink(page).click()
    await expect(page).toHaveURL(ending(chatHref))
  }

  // 13. The page of a call whose file is gone follows the origin.
  const gone = await start("subagents", { shape: "file-gone" })
  await go(page, DESIGN)
  await expect(page).toHaveURL(ending(DESIGN))
  await callLink(page, MISSING).click()
  await expect(page.getByText("Transcript no longer available")).toBeVisible()
  await expect(page).toHaveURL(new RegExp(`/agents/${gone.subagents!.sessionId}/subagents/`))
  await backLink(page).click()
  await expect(page).toHaveURL(ending(DESIGN))

  // 14. A parent with no transcript has no log link, so the address is typed.
  const orphan = (await start("subagents", { shape: "parent-gone" })).subagents!
  await go(page, subUrl(orphan.sessionId, orphan.calls.running, DESIGN.slice(1)))
  await expect(page.getByText("Transcript no longer available")).toBeVisible()
  await expect(page.getByRole("heading", { level: 1 })).toHaveCount(0)
  await backLink(page).click()
  await expect(page).toHaveURL(ending(DESIGN))
})

// Why a flow: an old Codex rollout reaches the chat read and two pages through the server, which decides that the call
// has no record. ChatLog.test.tsx, AgentChat.test.tsx and codexchild_internal_test.go each cover one half with given
// input.
test("221. Subagent call with no record", async ({ page, start }) => {
  const fixture = await start("subagents", { shape: "codex-old-call" })
  const { sessionId, calls } = fixture.subagents!
  const line = page.getByText(/^Agent · /)

  const chat: SessionChatResponse = JSON.parse(
    await fixture.cli("agents", "chat", sessionId, "--json"),
  )
  expect(chat.messages.flatMap((m) => m.subagent_calls ?? [])).toMatchObject([
    { call_id: calls.spawn, state: "no_record" },
  ])

  await go(page, `#/agents/${sessionId}`)
  await expect(page).toHaveURL(new RegExp(`#/agents/${sessionId}$`))
  await expect(page.getByTestId("tool-calls")).toBeVisible()
  await expect(line).toHaveCount(0)
  await openTools(page)
  await expect(line).toHaveText("Agent · review")
  await expect(line.getByRole("link")).toHaveCount(0)
  await expect(page.getByRole("link", { name: /↗/ })).toHaveCount(0)

  await go(page, DESIGN)
  await expect(page).toHaveURL(new RegExp(`${DESIGN}$`))
  await expect(line).toHaveText("Agent · review")
  await expect(page.getByTestId("tool-calls")).toHaveCount(0)
  await expect(page.getByRole("link", { name: /↗/ })).toHaveCount(0)
})
