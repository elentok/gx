import type { Page } from "@playwright/test"
import { chatOf, sessions } from "./agentsKit"
import { history } from "./draftKit"
import { expect, flag, nav, test, type Fixture } from "./fixture"
import { goto, sidebarKit, stillMotion } from "./sidebarKit"

const draftPage = (page: Page) => ({
  brief: page.getByRole("textbox", nav("Brief")),
  start: page.getByRole("button", nav("Start chat")),
  remove: page.getByRole("button", nav("Remove")),
  saved: page.getByText("Saved", { exact: true }),
  error: page.getByRole("alert"),
})

const drafts = async (fx: Fixture) =>
  (JSON.parse(await fx.cli("server", "snapshot", "--json")).chat_drafts ?? []) as {
    id: string
    label: string
  }[]

const draftId = async (fx: Fixture, label: string) =>
  (await drafts(fx)).find((d) => d.label === label)!.id

/** The id of the chat the page went to: the last segment of `#/agents/<id>`. */
const chatIdOf = async (page: Page) => {
  await expect(page).toHaveURL(/#\/agents\/[^/]+$/)
  return page.url().split("/agents/")[1]!
}

// One journey over the `chat-draft` scenario: the draft page edits a brief, Start launches the chat
// on the draft's model and effort, and a failed Start keeps the draft with its worktree.
test("209. Chat draft page", async ({ page, start }) => {
  test.setTimeout(120_000)
  const fx = await start("chat-draft")
  await stillMotion(page)
  const { sidebar } = sidebarKit(page)
  // A draft row is found by its link: the chat it becomes keeps the draft's name.
  const draftRow = (id: string) => sidebar.locator(`a[href$="/chat-drafts/${id}"]`)
  const d = draftPage(page)
  const before = (await sessions(fx)).length

  // The Claude draft: edit the brief, wait for the save, then Start.
  const claudeId = await draftId(fx, "claude-draft")
  await goto(page, `#/chat-drafts/${claudeId}`)
  await expect(d.brief).toHaveValue("Brief of the Claude draft")
  await expect(d.start).toBeVisible()
  await expect(d.remove).toBeVisible()
  await d.brief.fill("Edited brief of the Claude draft")
  await expect(d.saved).toBeVisible()
  await d.start.click()
  const claudeChat = await chatIdOf(page)
  await expect
    .poll(async () => (await sessions(fx)).find((s) => s.id === claudeChat)?.model)
    .toBe("opus")
  expect((await sessions(fx)).find((s) => s.id === claudeChat)!.effort).toBe("max")
  await expect.poll(async () => (await fx.launches()).length).toBeGreaterThan(0)
  const args = (await fx.launches()).at(-1)!
  expect(flag(args, "model")).toBe("opus")
  expect(flag(args, "effort")).toBe("max")
  const conversation = (await chatOf(fx, claudeChat)).conversation_id
  await expect.poll(() => fx.input(conversation)).toContain("Edited brief of the Claude draft")
  await expect(draftRow(claudeId)).toHaveCount(0)
  expect((await drafts(fx)).map((x) => x.id)).not.toContain(claudeId)

  // The old draft link lands on the new chat.
  await goto(page, "#/epics")
  await goto(page, `#/chat-drafts/${claudeId}`)
  await expect(page).toHaveURL(new RegExp(`#/agents/${claudeChat}$`))

  // A Start that fails after the new worktree exists keeps the draft and names the worktree; Start again runs there.
  const worktreeId = await draftId(fx, "worktree-draft")
  await fx.forkHook("start-fail", true)
  await goto(page, `#/chat-drafts/${worktreeId}`)
  // The started Claude draft is the one entry so far: the newest History row on a new draft page.
  const popup = history(page, "Chat drafts")
  await popup.open()
  await expect(popup.rows).toHaveCount(1)
  await expect(popup.rows.first()).toContainText("Edited brief of the Claude draft")
  await popup.close()
  await d.start.click()
  await expect(d.error).toContainText("test hook")
  await expect(d.error).toContainText("Start made a worktree at")
  const made = /worktree at (\S+?);/.exec((await d.error.textContent())!)![1]!
  await expect(page).toHaveURL(new RegExp(`#/chat-drafts/${worktreeId}$`))
  await expect(draftRow(worktreeId)).toBeVisible()
  await fx.forkHook("start-fail", false)
  await d.start.click()
  const worktreeChat = await chatIdOf(page)
  expect((await sessions(fx)).find((s) => s.id === worktreeChat)!.dir).toBe(made)
  await expect(draftRow(worktreeId)).toHaveCount(0)

  // Remove deletes the draft and starts no chat.
  const removeMe = (
    await fx.cliInput(
      "Brief of a draft to remove",
      "agents",
      "drafts",
      "create",
      "--project",
      "demo",
      "--name",
      "remove-me",
      "--brief",
      "-",
    )
  ).trim()
  const count = (await sessions(fx)).length
  await goto(page, `#/chat-drafts/${removeMe}`)
  await expect(d.brief).toHaveValue("Brief of a draft to remove")
  await d.remove.click()
  // The footer's Remove opens a popover with its own Remove; it is the one inside the dialog.
  await page
    .getByRole("alertdialog")
    .getByRole("button", { name: /^Remove/ })
    .click()
  await expect(page).toHaveURL(/#\/epics/)
  await expect(draftRow(removeMe)).toHaveCount(0)
  expect((await drafts(fx)).map((x) => x.id)).not.toContain(removeMe)
  expect((await sessions(fx)).length).toBe(count)
  expect(count).toBe(before + 2)

  // An empty draft removes at once, with no popover.
  const emptyId = (
    await fx.cliInput(
      "x",
      "agents",
      "drafts",
      "create",
      "--project",
      "demo",
      "--name",
      "empty-me",
      "--brief",
      "-",
    )
  ).trim()
  await goto(page, `#/chat-drafts/${emptyId}`)
  await d.brief.fill("")
  await expect(d.saved).toBeVisible()
  await d.remove.click()
  await expect(page).toHaveURL(/#\/epics/)
  await expect(page.getByRole("alertdialog")).toHaveCount(0)
  expect((await drafts(fx)).map((x) => x.id)).not.toContain(emptyId)

  // Codex: the same Start journey launches codex on the draft's model and effort.
  const codexId = await draftId(fx, "codex-draft")
  await goto(page, `#/chat-drafts/${codexId}`)
  // The Remove above added one row with its brief, newest first; the failed Start added none.
  await popup.open()
  await expect(popup.rows).toHaveCount(3)
  await expect(popup.rows.first()).toContainText("Brief of a draft to remove")
  await popup.close()
  await d.brief.fill("Edited brief of the Codex draft")
  await expect(d.saved).toBeVisible()
  await d.start.click()
  const codexChat = await chatIdOf(page)
  const codexConversation = (await chatOf(fx, codexChat)).conversation_id
  await expect.poll(async () => (await fx.codexLaunches(codexConversation)).length).toBe(1)
  const codexArgs = (await fx.codexLaunches(codexConversation))[0]!
  expect(flag(codexArgs, "model")).toBe("gpt-5.5-codex")
  expect(codexArgs).toContain('model_reasoning_effort="high"')
  await expect(draftRow(codexId)).toHaveCount(0)
})
