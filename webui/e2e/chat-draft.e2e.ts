import type { Page } from "@playwright/test"
import { addInput, chatOf, png, sessions, steerUploads } from "./agentsKit"
import { editedAt, history, startReason } from "./draftKit"
import { expect, flag, holdRoute, nav, test, type Fixture } from "./fixture"
import { goto, sidebarKit, stillMotion } from "./sidebarKit"

const draftPage = (page: Page) => ({
  brief: page.getByRole("textbox", nav("Brief")),
  start: page.getByRole("button", nav("Start chat")),
  remove: page.getByRole("button", nav("Remove draft")),
  saveStatus: page.locator("[data-save-shown]"),
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
  const { sidebar, closeMenus } = sidebarKit(page)
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
  // The ⋯ is the far-right control of the title row, right of the Title field, and lists Rename, Move to workspace, Remove.
  // The sidebar rows have their own ⋯ buttons; the page's comes last in the document.
  const more = page.getByRole("button", nav("More actions")).last()
  const titleBox = (await page.getByRole("textbox", nav("Title")).boundingBox())!
  const moreBox = (await more.boundingBox())!
  expect(moreBox.x).toBeGreaterThan(titleBox.x + titleBox.width - 1)
  await more.click()
  await expect(page.getByRole("menuitem")).toHaveText([/^Rename/, /^Move to workspace/, /^Remove/])
  await closeMenus()
  // Type a new last sentence and press Start at once: Start carries the latest text without waiting for a save.
  const lastSentence = "The last sentence, typed just before Start."
  await d.brief.fill(`Edited brief of the Claude draft. ${lastSentence}`)
  // A held snapshot keeps the new chat out of the list, so the page shows the loader and never "Session not found".
  const releaseSnapshot = await holdRoute(page, "**/v1/snapshot*")
  await d.start.click()
  const claudeChat = await chatIdOf(page)
  await expect(page.getByText("Starting chat…")).toBeVisible()
  await expect(page.getByText("Session not found")).toHaveCount(0)
  releaseSnapshot()
  await expect
    .poll(async () => (await sessions(fx)).find((s) => s.id === claudeChat)?.model)
    .toBe("opus")
  expect((await sessions(fx)).find((s) => s.id === claudeChat)!.effort).toBe("max")
  await expect.poll(async () => (await fx.launches()).length).toBeGreaterThan(0)
  const args = (await fx.launches()).at(-1)!
  expect(flag(args, "model")).toBe("opus")
  expect(flag(args, "effort")).toBe("max")
  const conversation = (await chatOf(fx, claudeChat)).conversation_id
  await expect.poll(() => fx.input(conversation)).toContain(lastSentence)
  await expect(draftRow(claudeId)).toHaveCount(0)
  await expect(page.getByText("Starting chat…")).toHaveCount(0)
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
  const emptyWas = await editedAt(fx, emptyId)
  await d.brief.fill("")
  await expect.poll(() => editedAt(fx, emptyId)).not.toBe(emptyWas)
  await d.remove.click()
  await expect(page).toHaveURL(/#\/epics/)
  await expect(page.getByRole("alertdialog")).toHaveCount(0)
  expect((await drafts(fx)).map((x) => x.id)).not.toContain(emptyId)

  // Leader then x is the same Remove: an empty draft goes at once, with no dialog.
  const chordId = (
    await fx.cliInput(
      "x",
      "agents",
      "drafts",
      "create",
      "--project",
      "demo",
      "--name",
      "chord-me",
      "--brief",
      "-",
    )
  ).trim()
  await goto(page, `#/chat-drafts/${chordId}`)
  const chordWas = await editedAt(fx, chordId)
  await d.brief.fill("")
  await expect.poll(() => editedAt(fx, chordId)).not.toBe(chordWas)
  await page.keyboard.press("ControlOrMeta+e")
  await expect(page.getByRole("menuitem", { name: /^x\s*Remove$/ })).toBeVisible()
  await page.keyboard.press("x")
  await expect(page).toHaveURL(/#\/epics/)
  await expect(page.getByRole("dialog")).toHaveCount(0)
  expect((await drafts(fx)).map((x) => x.id)).not.toContain(chordId)

  // Codex: the same Start journey launches codex on the draft's model and effort.
  const codexId = await draftId(fx, "codex-draft")
  await goto(page, `#/chat-drafts/${codexId}`)
  await expect(page).toHaveURL(new RegExp(`#/chat-drafts/${codexId}$`))
  await expect(d.brief).toBeVisible()
  // The Remove above added one row with its brief, newest first; the failed Start added none.
  await popup.open()
  await expect(popup.rows).toHaveCount(3)
  await expect(popup.rows.first()).toContainText("Brief of a draft to remove")
  await popup.close()
  const codexWas = await editedAt(fx, codexId)
  await d.brief.fill("Edited brief of the Codex draft")
  await expect.poll(() => editedAt(fx, codexId)).not.toBe(codexWas)
  await d.start.click()
  const codexChat = await chatIdOf(page)
  const codexConversation = (await chatOf(fx, codexChat)).conversation_id
  await expect.poll(async () => (await fx.codexLaunches(codexConversation)).length).toBe(1)
  const codexArgs = (await fx.codexLaunches(codexConversation))[0]!
  expect(flag(codexArgs, "model")).toBe("gpt-5.5-codex")
  expect(codexArgs).toContain('model_reasoning_effort="high"')
  await expect(draftRow(codexId)).toHaveCount(0)

  const makeDraft = async (name: string, brief: string) =>
    (
      await fx.cliInput(
        brief,
        "agents",
        "drafts",
        "create",
        "--project",
        "demo",
        "--name",
        name,
        "--brief",
        "-",
      )
    ).trim()

  // A blocked Start has no hint text: its reason is a tooltip that opens on focus.
  const tipId = await makeDraft("tip-me", "Brief of the tip draft")
  await goto(page, `#/chat-drafts/${tipId}`)
  await expect(d.brief).toHaveValue("Brief of the tip draft")
  await expect(d.start).not.toHaveAttribute("aria-disabled", "true")
  const uploads = await steerUploads(page)
  uploads.set("hold")
  await addInput(page).setInputFiles([png(1)])
  await expect(d.start).toHaveAttribute("aria-disabled", "true")
  await expect(await startReason(page, d.start)).toHaveText("Uploading images...")
  await uploads.release("fail")
  const failed = page.locator('[data-state="failed"]')
  await expect(failed).toBeVisible()
  await expect(await startReason(page, d.start)).toHaveText("Remove or retry the failed images.")
  await failed.getByRole("button", nav("Remove image")).click()
  await expect(failed).toHaveCount(0)
  await expect(d.start).not.toHaveAttribute("aria-disabled", "true")

  // A refused edit (a worktree name outside the project) stops Start: the draft stays and the reason shows.
  const refusedId = await makeDraft("refuse-me", "Brief of the refused draft")
  const chats = (await sessions(fx)).length
  await goto(page, `#/chat-drafts/${refusedId}`)
  await expect(d.brief).toHaveValue("Brief of the refused draft")
  const folder = page.getByRole("combobox", nav("Folder"))
  await folder.fill("../out")
  await folder.press("Enter")
  await d.start.click()
  await expect(d.error).toBeVisible()
  await expect(page).toHaveURL(new RegExp(`#/chat-drafts/${refusedId}$`))
  await expect(draftRow(refusedId)).toBeVisible()
  expect((await drafts(fx)).map((x) => x.id)).toContain(refusedId)
  expect((await sessions(fx)).length).toBe(chats)

  // With save-fail armed, the failed-save floppy shows until the hook is removed, then the draft recovers.
  const failId = await makeDraft("fail-me", "Brief of the fail draft")
  await goto(page, `#/chat-drafts/${failId}`)
  await expect(d.brief).toHaveValue("Brief of the fail draft")
  const failWas = await editedAt(fx, failId)
  await fx.forkHook("save-fail", true)
  await d.brief.fill("Brief typed while saves fail")
  await expect(d.saveStatus).toHaveAttribute("data-save-shown", "unsaved")
  await expect(d.saveStatus).toHaveText("Not saved, retrying")
  expect(await editedAt(fx, failId)).toBe(failWas)
  await fx.forkHook("save-fail", false)
  await expect.poll(() => editedAt(fx, failId), { timeout: 15_000 }).not.toBe(failWas)
  await expect(d.saveStatus).not.toHaveAttribute("data-save-shown", "unsaved")
})
