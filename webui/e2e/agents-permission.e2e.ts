import { mkdtempSync, readFileSync } from "node:fs"
import { tmpdir } from "node:os"
import { join } from "node:path"
import type { Page } from "@playwright/test"
import type { SessionInfo } from "../src/generated/server"
import { expect, test, type Fixture } from "./fixture"
import { cardSession, chatOf, claudeIdle, go, sessions } from "./agentsKit"

const conversationOf = async (fixture: Fixture, id: string) =>
  (await chatOf(fixture, id)).conversation_id
const stateOf = async (fixture: Fixture, id: string) =>
  (await sessions(fixture)).find((s) => s.id === id)!.status.state
// An answer waits up to the prompt grace for the session to move on, and a loaded machine slows that.
test.describe.configure({ timeout: 180_000 })
const SLOW = { timeout: 45_000 }
const leavesBlocked = (fixture: Fixture, id: string) =>
  expect.poll(() => stateOf(fixture, id), SLOW).not.toBe("blocked")

/** What the fake read after `before`, the input log it had then. */
const readSince = async (fixture: Fixture, conv: string, before: string) =>
  (await fixture.input(conv)).slice(before.length)

const failure = (page: Page) => page.getByRole("region", { name: "Session failure" })
const noticeButton = (page: Page, name: string) =>
  failure(page).getByRole("button", { name, exact: true })
const screenDialog = (page: Page) => page.getByRole("dialog", { name: "Screen" })

const BASH_DIALOG = (command: string) => [
  "────────────────────",
  "Bash command",
  `  ${command}`,
  "Do you want to proceed?",
  "❯ 1. Yes",
  "  2. No",
  "Esc to cancel",
]
const UNREADABLE = ["▒▒▒▒ nothing gx can read ▒▒▒▒"]
const QUESTION = ["Select a thing", "❯ 1. One", "  2. Two", "Esc to cancel"]

/** Claude's AskUserQuestion dialog as the screen shows it, as agentfake.AskDialogRows draws it. */
const askDialog = (question: string, ...options: string[]) => [
  "←  ☐ Q  ✔ Submit  →",
  "",
  question,
  ...options.flatMap((o, i) => [`${i === 0 ? "❯ " : "  "}${i + 1}. ${o}`, `     about ${o}`]),
  `  ${options.length + 1}. Type something.`,
  "────────────",
  `  ${options.length + 2}. Chat about this`,
  "",
  "Enter to select · Tab/Arrow keys to navigate · Esc to cancel",
]

const NOT_APPROVABLE = "This request needs a real answer. Use the key bar in Screen."
const NOT_DELIVERED = "The answer did not reach the session. Check Screen."

const plainBlocked = (s: SessionInfo) => !s.design && !s.queue && s.failure?.kind === "blocked"

/** The seeded Blocked session whose permission prompt ends in `text`; "" is a prompt gx cannot read. */
async function blockedAt(fixture: Fixture, harness: string, text: string) {
  for (const s of (await sessions(fixture)).filter(
    (x) =>
      plainBlocked(x) && x.failure?.why === "permission" && (x.harness || "claude") === harness,
  )) {
    const perm = JSON.parse(await fixture.cli("agents", "permission", s.id, "--json")) as {
      text: string
    }
    if (perm.text.split("\n").at(-1) === text) return s
  }
  throw new Error(`no ${harness} session blocked at ${JSON.stringify(text)}`)
}

/** Blocks the first idle Claude session at a dialog. */
async function blockIdle(fixture: Fixture, dialog: string[], waitingFor = "permission prompt") {
  const s = (await claudeIdle(fixture))!
  const conv = await conversationOf(fixture, s.id)
  await fixture.controlClaude(conv, { action: "block", waiting_for: waitingFor, dialog })
  await expect.poll(() => stateOf(fixture, s.id)).toBe("blocked")
  return { s, conv }
}

const open = async (page: Page, hash: string) => {
  await go(page, hash)
  await expect(failure(page)).toBeVisible()
}

/** Stops the page from learning anything new, so its notice outlives the Blocked state. */
const freeze = (page: Page) => page.route("**/v1/snapshot*", () => new Promise(() => {}))

async function closeDialogTwice(
  page: Page,
  fixture: Fixture,
  hash: string,
  s: SessionInfo,
  conv: string,
) {
  const before = await fixture.input(conv)
  await open(page, hash)
  await freeze(page)
  await noticeButton(page, "Close dialog").click()
  await expect.poll(() => stateOf(fixture, s.id)).toBe("idle")
  await expect
    .poll(async () => JSON.stringify(await readSince(fixture, conv, before)))
    .toBe('"\\u001b"')

  // The notice still shows, so the button is there; the guard refuses it.
  await noticeButton(page, "Close dialog").click()
  await expect(failure(page).getByRole("alert")).toBeVisible()
  expect(await readSince(fixture, conv, before)).toBe("\x1b")
  await page.unrouteAll({ behavior: "ignoreErrors" })
}

// Hosts: 157 (Close dialog, Agents page; also covers 157 design page and 158 design page) and 158 (readable Claude prompt;
// also covers 158 Allow once refused, Ignored Allow key, Codex hook and screen). Both run on agents-blocked.
test("157. Close dialog, Agents page and design page; 158. design Allow once and Deny", async ({
  page,
  start,
}) => {
  const fixture = await start("agents-blocked", { autoAdvanceDelay: "1h" })
  await page.setViewportSize({ width: 1280, height: 800 })
  const { s, conv } = await blockIdle(fixture, QUESTION, "input needed")
  await closeDialogTwice(page, fixture, `#/agents/${s.id}`, s, conv)

  // 157 design page.
  const design = (await sessions(fixture)).find((x) => x.ticket === "demo:design-auto")!
  const designConv = await conversationOf(fixture, design.id)
  await fixture.controlClaude(designConv, {
    action: "block",
    waiting_for: "input needed",
    dialog: QUESTION,
  })
  await expect.poll(() => stateOf(fixture, design.id)).toBe("blocked")
  await closeDialogTwice(page, fixture, "#/d/demo:design-auto", design, designConv)

  // 158 design page.
  const block = async (dialog: string[]) => {
    await fixture.controlClaude(designConv, {
      action: "block",
      waiting_for: "permission prompt",
      dialog,
    })
    await expect.poll(() => stateOf(fixture, design.id)).toBe("blocked")
  }
  await go(page, "#/epics")
  await block(BASH_DIALOG("rg design-allow"))
  let before = await fixture.input(designConv)
  await open(page, "#/d/demo:design-auto")
  await expect(failure(page)).toContainText("rg design-allow")
  await noticeButton(page, "Allow once").click()
  await leavesBlocked(fixture, design.id)
  await expect.poll(() => readSince(fixture, designConv, before)).toBe("\r")

  // A prompt gx cannot read: Allow is refused, Deny works.
  await block(UNREADABLE)
  before = await fixture.input(designConv)
  // A fresh visit: the notice of the first prompt is still being replaced.
  await go(page, "#/epics")
  await open(page, "#/d/demo:design-auto")
  await expect(failure(page)).not.toContainText("rg design-allow")
  await expect(noticeButton(page, "Allow once")).toBeVisible()
  await noticeButton(page, "Allow once").click()
  await expect(failure(page).getByRole("alert")).toHaveText(NOT_APPROVABLE)
  expect(await readSince(fixture, designConv, before)).toBe("")
  await noticeButton(page, "Deny").click()
  await leavesBlocked(fixture, design.id)
  await expect.poll(() => readSince(fixture, designConv, before)).toBe("\x1b")
})

test("158. Allow once and Deny, readable Claude prompt; refused; Ignored Allow key; Codex hook and screen", async ({
  page,
  start,
}) => {
  const fixture = await start("agents-blocked", { autoAdvanceDelay: "1h" })
  await page.setViewportSize({ width: 1280, height: 800 })

  // The Agents-page question sheet: hook dialog card, then the keystroke-fallback card.
  const sheet = page.getByRole("group", { name: "Questions" })
  const cardOf = (source: string, text: string) => cardSession(page, fixture, source, text)
  const hookCard = await cardOf("claude-dialog", "Which store?")
  await go(page, `#/agents/${hookCard.id}`)
  await expect(sheet).toBeVisible()
  await expect(sheet).toContainText("0 of 3 answered")
  // Submit, Dismiss and the terminal button sit beside the group, not in it.
  await expect(page.getByRole("button", { name: "Submit", exact: true })).toBeDisabled()
  await expect(page.getByRole("button", { name: "Huh?" })).toHaveCount(0)
  await expect(page.getByRole("button", { name: "Answer in the terminal" })).toBeVisible()
  await sheet.getByRole("button", { name: "Other", exact: true }).first().click()
  await sheet.getByPlaceholder("Other", { exact: true }).first().fill("Postgres")
  await expect(sheet).toContainText("1 of 3 answered")
  // Picking an answer moves to the next question.
  await sheet.getByRole("button", { name: "Note on question Checks" }).click()
  await expect(sheet.getByPlaceholder("Note on this question")).toBeVisible()

  const fallback = await cardOf("claude-dialog", "Which color?")
  await go(page, `#/agents/${fallback.id}`)
  await expect(sheet).toBeVisible()
  await expect(sheet.getByRole("button", { name: /^Note on question/ })).toHaveCount(0)
  await expect(page.getByRole("button", { name: "Dismiss", exact: true })).toBeVisible()
  await expect(page.getByRole("button", { name: "Huh?" })).toHaveCount(0)
  await expect(page.getByRole("button", { name: "Answer in the terminal" })).toHaveCount(0)
  await expect(failure(page)).toBeVisible()

  // Permission and plan cards: exact labels, nothing preselected, no Huh?.
  const wrapped = await cardOf("permission", "ask again for this command")
  await go(page, `#/agents/${wrapped.id}`)
  await expect(sheet).toBeVisible()
  await expect(sheet.getByRole("button", { pressed: true })).toHaveCount(0)
  await expect(page.getByRole("button", { name: "Huh?" })).toHaveCount(0)
  await expect(page.getByRole("button", { name: "Dismiss", exact: true })).toBeVisible()
  const plan = await cardOf("permission", "auto-accept edits")
  await go(page, `#/agents/${plan.id}`)
  // Story 24: the card shows the text of the plan file the dialog names.
  await expect(page.getByText("second step")).toBeVisible()
  await expect(sheet.getByRole("button", { name: /Yes, and auto-accept edits/ })).toBeVisible()
  await expect(sheet.getByRole("button", { name: /Yes, and manually approve edits/ })).toBeVisible()
  await expect(page.getByRole("button", { name: "Submit", exact: true })).toBeDisabled()
  await sheet.getByRole("button", { name: /Tell Claude what to change/ }).click()
  await expect(page.getByRole("button", { name: "Submit", exact: true })).toBeDisabled()
  await sheet.getByPlaceholder("Tell Claude what to change").fill("fewer steps")
  await expect(page.getByRole("button", { name: "Submit", exact: true })).toBeEnabled()

  // Codex question card: Note, no Huh?; the Queue agent's ticket ask: Submit only; the printed round: Huh? and nothing else.
  const codexCard = await cardOf("codex-question", "Which store?")
  await go(page, `#/agents/${codexCard.id}`)
  await expect(sheet).toContainText("0 of 3 answered")
  await expect(sheet.getByRole("button", { name: "Note on question Store" })).toBeVisible()
  await expect(page.getByRole("button", { name: "Huh?" })).toHaveCount(0)
  const ticketAsk = await cardOf("ticket-ask", "")
  await go(page, `#/agents/${ticketAsk.id}`)
  await expect(sheet).toBeVisible()
  await expect(page.getByRole("button", { name: "Submit", exact: true })).toBeDisabled()
  await expect(page.getByRole("button", { name: "Huh?" })).toHaveCount(0)
  await expect(page.getByRole("button", { name: "Dismiss", exact: true })).toHaveCount(0)
  await expect(page.getByRole("button", { name: "Answer in the terminal" })).toHaveCount(0)
  const round = await cardOf("round", "Which store?")
  await go(page, `#/agents/${round.id}`)
  await expect(sheet).toContainText("Which store?")
  await expect(sheet.getByRole("button", { name: "Huh?" }).first()).toBeVisible()
  await expect(page.getByRole("button", { name: "Dismiss", exact: true })).toHaveCount(0)

  // On a phone the sheet fits the width.
  await page.setViewportSize({ width: 390, height: 844 })
  await expect(sheet).toBeVisible()
  expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(
    true,
  )
  await page.setViewportSize({ width: 1280, height: 800 })

  // Story 8: Answer in the terminal makes the hook print nothing and exit 0, and the released dialog lists no fallback card.
  const termSession = (await claudeIdle(fixture, 1))!
  const termConv = await conversationOf(fixture, termSession.id)
  const termOut = join(mkdtempSync(join(tmpdir(), "ask-term-")), "hook.json")
  await fixture.controlClaude(termConv, {
    action: "askquestion",
    ask: {
      tool_use_id: "toolu_terminal",
      out_file: termOut,
      dialog: askDialog("Which term?", "x", "y"),
      questions: [
        {
          header: "Term",
          question: "Which term?",
          multiSelect: false,
          options: [
            { label: "x", description: "x" },
            { label: "y", description: "y" },
          ],
        },
      ],
    },
  })
  await go(page, `#/agents/${termSession.id}`)
  await expect(sheet).toContainText("Which term?")
  await page.getByRole("button", { name: "Answer in the terminal" }).click()
  await expect(sheet).toHaveCount(0)
  await expect.poll(() => readFileSync(`${termOut}.exit`, "utf8"), SLOW).toBe("0")
  expect(readFileSync(termOut, "utf8")).toBe("")
  await expect.poll(() => stateOf(fixture, termSession.id), SLOW).toBe("blocked")
  const termChat = await page.evaluate(
    (id) => fetch(`/v1/sessions/chat?session=${encodeURIComponent(id)}`).then((r) => r.text()),
    termSession.id,
  )
  expect(termChat).not.toContain("toolu_terminal")
  expect(termChat).not.toContain("Which term?")

  // Dismiss on a hook card: a second ask on an idle session, so the first hook card can still move to the terminal below.
  const asked = (await claudeIdle(fixture))!
  const askedConv = await conversationOf(fixture, asked.id)
  const outFile = join(mkdtempSync(join(tmpdir(), "ask-out-")), "hook.json")
  await fixture.controlClaude(askedConv, {
    action: "askquestion",
    ask: {
      tool_use_id: "toolu_dismiss",
      out_file: outFile,
      questions: [
        {
          header: "Pick",
          question: "Which pick?",
          multiSelect: false,
          options: [
            { label: "A", description: "a" },
            { label: "B", description: "b" },
          ],
        },
      ],
    },
  })
  await go(page, `#/agents/${asked.id}`)
  await expect(sheet).toContainText("Which pick?")
  await page.getByRole("button", { name: "Dismiss", exact: true }).click()
  await expect(sheet).toHaveCount(0)
  await expect
    .poll(() => readFileSync(outFile, "utf8"), SLOW)
    .toContain("The user doesn't want to proceed with this question")
  expect(JSON.parse(readFileSync(outFile, "utf8")).hookSpecificOutput.permissionDecision).toBe(
    "deny",
  )

  // A new dialog id after a retype drops the draft: the same session blocks again on another dialog.
  await fixture.controlClaude(askedConv, {
    action: "block",
    waiting_for: "input needed",
    dialog: askDialog("Which size?", "small", "large"),
  })
  // The fallback card lists once the dialog has waited out the server's delay, and nothing pushes the page when it has.
  const revisitUntil = (text: string) =>
    expect(async () => {
      await go(page, "#/agents")
      await go(page, `#/agents/${asked.id}`)
      await expect(sheet).toContainText(text, { timeout: 3_000 })
    }).toPass(SLOW)
  await revisitUntil("Which size?")
  await sheet.getByRole("button", { name: "Other", exact: true }).first().click()
  await sheet.getByPlaceholder("Other", { exact: true }).first().fill("draft text")
  await fixture.controlClaude(askedConv, {
    action: "block",
    waiting_for: "input needed",
    dialog: askDialog("Which shape?", "round", "square"),
  })
  await revisitUntil("Which shape?")
  await sheet.getByRole("button", { name: "Other", exact: true }).first().click()
  await expect(sheet.getByPlaceholder("Other", { exact: true }).first()).toHaveValue("")
  await page.getByRole("button", { name: "Dismiss", exact: true }).click()
  await leavesBlocked(fixture, asked.id)

  // Last, since each answer clears its card: Dismiss on the fallback card, and the hook card moving to the terminal.
  await go(page, `#/agents/${fallback.id}`)
  await expect(sheet).toBeVisible()
  await page.getByRole("button", { name: "Dismiss", exact: true }).click()
  await leavesBlocked(fixture, fallback.id)
  await expect(sheet).toHaveCount(0)
  await go(page, `#/agents/${hookCard.id}`)
  await expect(sheet).toBeVisible()
  await page.getByRole("button", { name: "Answer in the terminal" }).click()
  await expect(sheet).toHaveCount(0)

  // Answered at the terminal: the permission card clears with no click in the page.
  await go(page, `#/agents/${wrapped.id}`)
  await expect(sheet).toBeVisible()
  await fixture.controlClaude(await conversationOf(fixture, wrapped.id), { action: "finish" })
  await leavesBlocked(fixture, wrapped.id)
  await expect(sheet).toHaveCount(0)

  const allow = await blockIdle(fixture, BASH_DIALOG("rg allow-me"))
  let before = await fixture.input(allow.conv)
  await open(page, `#/agents/${allow.s.id}`)
  await expect(failure(page)).toContainText("rg allow-me")
  await expect(failure(page).getByRole("button")).toHaveText(["Screen", "Deny", "Allow once"])
  await noticeButton(page, "Allow once").click()
  await leavesBlocked(fixture, allow.s.id)
  await expect.poll(() => readSince(fixture, allow.conv, before)).toBe("\r")

  const deny = await blockIdle(fixture, BASH_DIALOG("rg deny-me"))
  before = await fixture.input(deny.conv)
  await open(page, `#/agents/${deny.s.id}`)
  await expect(failure(page)).toContainText("rg deny-me")
  await noticeButton(page, "Deny").click()
  await leavesBlocked(fixture, deny.s.id)
  await expect.poll(() => readSince(fixture, deny.conv, before)).toBe("\x1b")

  // Allow once refused for Claude prompts gx cannot approve.
  const refused = async (text: string, notice: string) => {
    const s = await blockedAt(fixture, "claude", text)
    const conv = await conversationOf(fixture, s.id)
    const before = await fixture.input(conv)
    await open(page, `#/agents/${s.id}`)
    await noticeButton(page, "Allow once").click()
    await expect(failure(page).getByRole("alert")).toHaveText(notice)
    expect(await stateOf(fixture, s.id)).toBe("blocked")
    return { s, conv, before }
  }

  // Unreadable screen: no prompt text; Allow is refused and sends nothing; Deny still works.
  const unreadable = await refused("", NOT_APPROVABLE)
  expect(await readSince(fixture, unreadable.conv, unreadable.before)).toBe("")
  await noticeButton(page, "Deny").click()
  await leavesBlocked(fixture, unreadable.s.id)
  await expect.poll(() => readSince(fixture, unreadable.conv, unreadable.before)).toBe("\x1b")

  // A first option that is not a plain Yes would pick a standing permission, so no Enter is sent.
  const standing = await refused("rg --files", NOT_APPROVABLE)
  await expect(failure(page)).toContainText("rg --files")
  expect(await readSince(fixture, standing.conv, standing.before)).toBe("")

  // Down in the key bar reaches the fake. The fake draws no cursor, so the redrawn dialog, cursor on option 2, is armed by hand.
  const moved = await blockedAt(fixture, "claude", "rg readable-command")
  const movedConv = await conversationOf(fixture, moved.id)
  const movedBefore = await fixture.input(movedConv)
  await open(page, `#/agents/${moved.id}`)
  // Any key leaves the session Blocked, and the notice (with the Screen dialog it owns) goes with it unless the page stops learning.
  await freeze(page)
  await noticeButton(page, "Screen").click()
  await screenDialog(page)
    .getByRole("group", { name: "Keys" })
    .getByRole("button", { name: "Down", exact: true })
    .click()
  await expect.poll(() => readSince(fixture, movedConv, movedBefore)).toBe("\x1b[B")
  await screenDialog(page).getByRole("button", { name: "Close", exact: true }).click()
  await page.unrouteAll({ behavior: "ignoreErrors" })
  const redrawn = BASH_DIALOG("rg readable-command")
  redrawn[4] = "  1. Yes"
  redrawn[5] = "❯ 2. No"
  await fixture.controlClaude(movedConv, {
    action: "block",
    waiting_for: "permission prompt",
    dialog: redrawn,
  })
  await go(page, "#/agents")
  await open(page, `#/agents/${moved.id}`)
  await noticeButton(page, "Allow once").click()
  await expect(failure(page).getByRole("alert")).toHaveText(NOT_APPROVABLE)
  expect(await readSince(fixture, movedConv, movedBefore)).toBe("\x1b[B")
  expect(await stateOf(fixture, moved.id)).toBe("blocked")

  // Codex hook and screen.
  const hook = await blockedAt(fixture, "codex", "rg codex-hook")
  await open(page, `#/agents/${hook.id}`)
  await expect(failure(page)).toContainText("rg codex-hook")
  // Story 28: the Agents-page card offers Allow and Deny and nothing else.
  const hookSheet = page.getByRole("group", { name: "Questions" })
  await expect(hookSheet).toBeVisible()
  await expect(hookSheet).toContainText("Allow this?")
  await expect(hookSheet.getByRole("button", { name: /^(Allow|Deny)$/ })).toHaveCount(2)
  await expect(hookSheet.getByRole("button", { name: "Other", exact: true })).toHaveCount(0)
  await noticeButton(page, "Allow once").click()
  await leavesBlocked(fixture, hook.id)
  await expect.poll(() => fixture.codexDecisions()).toEqual([{ behavior: "allow", message: "" }])

  const screen = await blockedAt(fixture, "codex", "rg codex-screen")
  await open(page, `#/agents/${screen.id}`)
  await expect(failure(page)).toContainText("rg codex-screen")
  await noticeButton(page, "Deny").click()
  await leavesBlocked(fixture, screen.id)

  // No hook and a screen gx cannot read: Allow is refused, Deny sends Esc.
  const codexUnreadable = await blockedAt(fixture, "codex", "")
  await open(page, `#/agents/${codexUnreadable.id}`)
  await noticeButton(page, "Allow once").click()
  await expect(failure(page).getByRole("alert")).toHaveText(NOT_APPROVABLE)
  expect(await stateOf(fixture, codexUnreadable.id)).toBe("blocked")
  await noticeButton(page, "Deny").click()
  await leavesBlocked(fixture, codexUnreadable.id)

  // Ignored Allow key shows the not-delivered text. Last: the server waits its whole prompt grace (8s in this scenario).
  const ignored = await blockedAt(fixture, "claude", "rg ignores-allow")
  const ignoredConv = await conversationOf(fixture, ignored.id)
  const ignoredBefore = await fixture.input(ignoredConv)
  await open(page, `#/agents/${ignored.id}`)
  await noticeButton(page, "Allow once").click()
  await expect(failure(page).getByRole("alert")).toHaveText(NOT_DELIVERED, { timeout: 30_000 })
  // The Enter went out; the fake ignored it and no other option was picked.
  await expect.poll(() => readSince(fixture, ignoredConv, ignoredBefore)).toBe("\r")
  expect(await stateOf(fixture, ignored.id)).toBe("blocked")
})
