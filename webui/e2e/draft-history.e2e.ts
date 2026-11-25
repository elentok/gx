import type { Locator, Page } from "@playwright/test"
import { composer, go, messageBox, sessions } from "./agentsKit"
import { draftPage, editedAt, history } from "./draftKit"
import { expect, nav, test, type Fixture } from "./fixture"
import { stillMotion } from "./sidebarKit"

const leader = "ControlOrMeta+e"
const chordBox = (page: Page) => page.getByRole("menu", nav("Chords"))
const hRow = (page: Page) => chordBox(page).getByRole("menuitem", { name: /^h\s*History$/ })
const historyButton = (page: Page) => page.getByRole("button", nav("History"))
const anyHistoryDialog = (page: Page) => page.getByRole("dialog", { name: /history$/ })

/** The Leader key opens the Chord box; the first press can land before the page listens, so it retries. */
const openChords = (page: Page) =>
  expect(async () => {
    if (await chordBox(page).isVisible()) return
    await page.keyboard.press(leader)
    await expect(chordBox(page)).toBeVisible({ timeout: 1_000 })
  }).toPass({ timeout: 10_000 })

/** Leader then h opens the popup of the page. */
async function leaderH(page: Page, list: string) {
  await openChords(page)
  await expect(hRow(page)).toBeVisible()
  await page.keyboard.press("h")
  await expect(history(page, list).popup).toBeVisible()
  await expect(chordBox(page)).toHaveCount(0)
  await history(page, list).close()
}

/** On a page with no History: no button, no `h` row, and `h` opens nothing. */
async function noHistory(page: Page) {
  await expect(historyButton(page)).toHaveCount(0)
  await openChords(page)
  await expect(hRow(page)).toHaveCount(0)
  await page.keyboard.press("h")
  await expect(chordBox(page)).toHaveCount(0)
  await expect(anyHistoryDialog(page)).toHaveCount(0)
}

const rowOf = (popup: Locator, text: string) =>
  popup.getByRole("option", { name: new RegExp(text) })

const box = async (l: Locator) => (await l.boundingBox())!

/** The History button and the save icon's slot (always rendered, empty while idle) share one row of the title. */
async function besideSaveIcon(page: Page) {
  const button = historyButton(page)
  const slot = draftPage(page).saveStatus
  await expect(button).toBeVisible()
  await expect(slot).toBeAttached()
  await expect
    .poll(async () => {
      const [b, s] = [await box(button), await box(slot)]
      return Math.abs(b.y + b.height / 2 - (s.y + s.height / 2)) < 8
    })
    .toBe(true)
}

const draftList = async (fx: Fixture) => (await fx.cli("designs", "drafts", "list")).split("\n")
const showDraft = async (fx: Fixture, id: string) =>
  (
    JSON.parse(await fx.cli("designs", "drafts", "show", id, "--json")) as {
      draft: { idea: string; slug: string; project: string; images: string[] }
    }
  ).draft

// One journey over the `draft-history` scenario: the History popup on the Design draft page, the Chat draft page, a chat, a Design and
// a Codex chat, the pages that have none, one reload, and the phone sheet.
test("Draft history popup", async ({ page, start }) => {
  test.setTimeout(120_000)
  const fx = await start("draft-history", { reloads: 1 })
  await page.setViewportSize({ width: 1280, height: 800 })
  await stillMotion(page)
  const all = await sessions(fx)
  const claude = all.find((s) => s.harness !== "codex" && !s.design && !s.queue && !s.fix_of)!
  const codex = all.find((s) => s.harness === "codex" && !s.design && !s.queue)!
  const fix = all.find((s) => s.fix_of)!
  const queue = all.find((s) => s.queue)!
  const designId = (await draftList(fx)).find((l) => l.includes("history-draft"))!.split("\t")[0]!
  const chatDraft = JSON.parse(await fx.cli("server", "snapshot", "--json")).chat_drafts.find(
    (d: { label: string }) => d.label === "history-chat-draft",
  ) as { id: string; harness: string; project: string }

  await test.step("Design draft page: button, popup size, filter, rows, restore", async () => {
    const d = draftPage(page)
    await go(page, `#/drafts/${designId}`)
    await expect(d.prompt).toHaveValue("Idea of the history draft page")
    await leaderH(page, "Design drafts")

    const popup = history(page, "Design drafts")
    await popup.open()
    await expect(popup.popup.getByRole("combobox", nav("Filter design drafts…"))).toBeFocused()
    await expect
      .poll(async () => {
        const b = await box(popup.popup)
        return { w: b.width, tall: b.height <= 480 }
      })
      .toEqual({ w: 640, tall: true })
    await expect(popup.rows).toHaveCount(8)
    const exists = rowOf(popup.popup, "History design draft whose Design exists")
    await expect(exists).toContainText("history-design")
    await expect(exists).toContainText("demo")
    await expect(rowOf(popup.popup, "images not kept")).toContainText("images not kept")
    const withImages = rowOf(popup.popup, "History design draft with two images")
    await expect(withImages.locator("svg")).toHaveCount(1)
    await expect(withImages).toContainText("2")
    await page.keyboard.type("plain B")
    await expect(popup.rows).toHaveCount(1)
    await page.keyboard.press("Escape")
    await expect(popup.popup).toHaveCount(0)

    // A taken slug: one click restores and closes; the slug is cleared and a notice shows.
    await popup.open()
    await exists.click()
    await expect(popup.popup).toHaveCount(0)
    await expect(d.prompt).toHaveValue("History design draft whose Design exists")
    await expect(d.slug).toHaveValue("")
    await expect(page.getByText(/slug "history-design" is already taken/)).toBeVisible()

    // The page's earlier text is the newest row now.
    await popup.open()
    await expect(popup.rows.first()).toContainText("Idea of the history draft page")
    await popup.close()

    // A deleted image file: the notice counts the missing image.
    await popup.open()
    await rowOf(popup.popup, "History design draft with a deleted image").click()
    await expect(page.getByText("1 images missing")).toBeVisible()

    // Type, wait for the server to hold it, pick: the pick brings text, images and slug and keeps the project.
    await d.prompt.fill("Typed before the pick")
    await expect
      .poll(async () => (await showDraft(fx, designId)).idea)
      .toBe("Typed before the pick")
    await besideSaveIcon(page)
    await popup.open()
    await rowOf(popup.popup, "History design draft with two images").click()
    await expect(popup.popup).toHaveCount(0)
    await expect(page.getByText("2 of 10 images")).toBeVisible()
    await expect(d.prompt).toHaveValue("History design draft with two images")
    await expect(d.prompt).toBeFocused()
    await expect
      .poll(async () => {
        const s = await showDraft(fx, designId)
        return [s.idea, s.slug, s.project, s.images.length]
      })
      .toEqual(["History design draft with two images", "images-slug", "demo", 2])
    expect((await draftList(fx)).join("\n")).toContain("images-slug\t2 images")
    await popup.open()
    await expect(popup.rows.first()).toContainText("Typed before the pick")
    await popup.close()

    // The flow's only reload: the pick survives it.
    await page.reload()
    await stillMotion(page)
    await expect(d.prompt).toHaveValue("History design draft with two images")
    await expect(d.slug).toHaveValue("images-slug")
    await expect(page.getByText("2 of 10 images")).toBeVisible()
  })

  await test.step("Chat draft page: button, popup, rows, restore keeps the settings", async () => {
    const brief = page.getByRole("textbox", nav("Brief"))
    await go(page, `#/chat-drafts/${chatDraft.id}`)
    await expect(brief).toHaveValue("Brief of the history chat draft")
    const was = await editedAt(fx, chatDraft.id)
    await brief.fill("Brief of the history chat draft, edited")
    await expect.poll(() => editedAt(fx, chatDraft.id)).not.toBe(was)
    await besideSaveIcon(page)
    await leaderH(page, "Chat drafts")

    const popup = history(page, "Chat drafts")
    await popup.open()
    await expect(popup.rows).toHaveCount(6)
    const claudeRow = rowOf(popup.popup, "History chat draft of Claude")
    await expect(claudeRow).toContainText("demo / chat-claude")
    await expect(claudeRow).toContainText("claude · opus · max")
    await rowOf(popup.popup, "History chat draft of Codex").click()
    await expect(popup.popup).toHaveCount(0)
    await expect(brief).toHaveValue("History chat draft of Codex")
    await expect(page.getByRole("textbox", nav("Name"))).toHaveValue("chat-codex")
    await popup.open()
    await expect(popup.rows.first()).toContainText("Brief of the history chat draft, edited")
    await rowOf(popup.popup, "History chat draft with an image").click()
    await expect(page.getByText("1 of 10 images")).toBeVisible()
    await expect(brief).toHaveValue("History chat draft with an image")
    // Harness, project and the rest stay the draft's own.
    await expect
      .poll(async () => {
        const row = JSON.parse(await fx.cli("server", "snapshot", "--json")).chat_drafts.find(
          (d: { id: string }) => d.id === chatDraft.id,
        )
        return [row.harness, row.project]
      })
      .toEqual([chatDraft.harness, chatDraft.project])
  })

  await test.step("Chat composer: button after Add, Sent messages, restore", async () => {
    await go(page, `#/agents/${claude.id}`)
    await expect(messageBox(page)).toBeVisible()
    const add = composer(page).getByRole("button", nav("Add image"))
    await expect
      .poll(async () => {
        const [a, h] = [await box(add), await box(historyButton(page))]
        return h.x >= a.x + a.width && h.x - (a.x + a.width) < 24
      })
      .toBe(true)
    await leaderH(page, "Sent messages")

    const popup = history(page, "Sent messages")
    await popup.open()
    await expect(popup.rows).toHaveCount(8)
    await expect(popup.rows.first()).toContainText("/history-command run it")
    await expect(rowOf(popup.popup, "History sent message with images").locator("svg")).toHaveCount(
      1,
    )
    await popup.close()

    await messageBox(page).fill("Unsent chat words")
    await popup.open()
    await rowOf(popup.popup, "History sent chat message").click()
    await expect(popup.popup).toHaveCount(0)
    await expect(messageBox(page)).toHaveValue("History sent chat message")
    await expect(messageBox(page)).toBeFocused()
    await popup.open()
    await expect(popup.rows.first()).toContainText("Unsent chat words")
    await rowOf(popup.popup, "History sent message with images").click()
    await expect(page.getByText("2 of 10 images")).toBeVisible()
    // The session's own settings stay.
    const after = (await sessions(fx)).find((s) => s.id === claude.id)!
    expect([after.harness, after.name]).toEqual([claude.harness, claude.name])
  })

  await test.step("Codex chat: a Claude command keeps its text", async () => {
    await go(page, `#/agents/${codex.id}`)
    await expect(messageBox(page)).toBeVisible()
    const popup = history(page, "Sent messages")
    await popup.open()
    await rowOf(popup.popup, "/history-command run it").click()
    await expect(popup.popup).toHaveCount(0)
    await expect(messageBox(page)).toHaveValue("/history-command run it")
  })

  await test.step("Design composer: button before Note, Design entries", async () => {
    await go(page, "#/d/demo:history-design")
    const form = composer(page)
    await expect(form).toBeVisible()
    const add = form.getByRole("button", nav("Add image"))
    const note = form.getByRole("button", nav("Note"))
    await expect
      .poll(async () => {
        const [a, h, n] = [await box(add), await box(historyButton(page)), await box(note)]
        return h.x >= a.x + a.width && n.x >= h.x + h.width
      })
      .toBe(true)
    await leaderH(page, "Sent messages")
    const popup = history(page, "Sent messages")
    await popup.open()
    await expect(rowOf(popup.popup, "History sent design message")).toContainText(
      "Design · history-sent-slug",
    )
    await expect(popup.rows.filter({ hasText: "Chat · history-sent-chat" })).toHaveCount(1)
    await popup.close()
  })

  await test.step("Epics, Queue agent and Fix agent pages offer no History", async () => {
    await go(page, "#/epics")
    await expect(page.getByRole("heading", { name: "Epics", level: 1 })).toBeVisible()
    await noHistory(page)
    await go(page, `#/agents/${queue.id}`)
    await expect(page).toHaveURL(new RegExp(`/agents/${queue.id}$`))
    await noHistory(page)
    await go(page, `#/agents/${fix.id}`)
    await expect(page).toHaveURL(new RegExp(`/agents/${fix.id}$`))
    await noHistory(page)
  })

  await test.step("Phone: icon-only buttons and the bottom sheet", async () => {
    await page.setViewportSize({ width: 390, height: 844 })
    await go(page, `#/drafts/${designId}`)
    await expect(draftPage(page).prompt).toBeVisible()
    await expect(historyButton(page)).toHaveText("")
    await go(page, "#/d/demo:history-design")
    await expect(composer(page)).toBeVisible()
    await expect(historyButton(page)).toHaveText("")
    await expect
      .poll(async () => {
        const b = await box(historyButton(page))
        return [b.width, b.height]
      })
      .toEqual([44, 44])

    const popup = history(page, "Sent messages")
    await popup.open()
    await expect
      .poll(async () => {
        const b = await box(popup.popup)
        return [b.x, b.y, b.width]
      })
      .toEqual([0, 90, 390])
    await page.setViewportSize({ width: 390, height: 667 })
    await expect
      .poll(async () => {
        const b = await box(popup.popup)
        return [b.x, b.y, b.width]
      })
      .toEqual([0, 90, 390])
    // The Design composer shows its text box only once a Note holds text, so a tap that restores nothing leaves it closed.
    const text = composer(page).getByRole("textbox")
    await rowOf(popup.popup, "History sent plain C").click()
    await expect(rowOf(popup.popup, "History sent plain C")).toHaveAttribute(
      "aria-selected",
      "true",
    )
    await expect(text).toHaveCount(0)
    await popup.popup.getByRole("button", nav("Close")).click()
    await expect(popup.popup).toHaveCount(0)
    await popup.open()
    await rowOf(popup.popup, "History sent plain C").click()
    await popup.popup.getByRole("button", nav("Restore")).click()
    await expect(popup.popup).toHaveCount(0)
    await expect(text).toHaveValue("History sent plain C")
  })
})
