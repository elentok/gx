import type { Locator, Page } from "@playwright/test"
import { expect, nav, type Fixture } from "./fixture"

export const draftPage = (page: Page) => ({
  prompt: page.getByRole("textbox", nav("Prompt")),
  slug: page.getByRole("textbox", nav("Epic slug")),
  project: page.getByRole("combobox", nav("Project")),
  rename: page.getByRole("button", nav("Rename")),
  start: page.getByRole("button", nav("Start design")),
  remove: page.getByRole("button", nav("Remove draft")),
  /** The save icon's slot: always rendered, empty while idle. */
  saveStatus: page.locator("[data-save-shown]"),
})

/** A blocked Start's reason is its tooltip. Focus opens it, as do hover and tap. */
export const startReason = async (page: Page, start: Locator) => {
  await start.focus()
  return page.getByRole("tooltip")
}

/** The id in a draft page's URL, `#/drafts/<id>`. */
export const designDraftId = (page: Page) => page.url().split("/drafts/")[1]!

/** Polls the server until the Design draft holds `text`: its prompt (`show`) or its slug (`list`). */
export const designDraftHolds = (fx: Fixture, id: string, text: string) =>
  expect
    .poll(
      async () =>
        (await fx.cli("designs", "drafts", "show", id)) +
        (await fx.cli("designs", "drafts", "list")),
    )
    .toContain(text)

/** The chat draft's `edited_at`, which moves on every save: read it before an edit, poll for a change after. */
export const editedAt = async (fx: Fixture, id: string) =>
  (
    JSON.parse(await fx.cli("server", "snapshot", "--json")).chat_drafts as {
      id: string
      edited_at: string
    }[]
  ).find((x) => x.id === id)!.edited_at

/** The History popup of a draft page or composer: `list` is "Design drafts", "Chat drafts" or "Sent messages". */
export const history = (page: Page, list: string) => {
  const popup = page.getByRole("dialog", nav(`${list} history`))
  return {
    popup,
    rows: popup.getByRole("option"),
    open: async () => {
      await page.getByRole("button", nav("History")).click()
      await expect(popup).toBeVisible()
    },
    close: async () => {
      await page.keyboard.press("Escape")
      await expect(popup).toHaveCount(0)
    },
  }
}

/** Sets the draft's slug through the pencil, which focuses the slug field. */
export const renameDraft = async (page: Page, slug: string) => {
  const d = draftPage(page)
  await d.rename.click()
  await expect(d.slug).toBeFocused()
  await d.slug.fill(slug)
}

/** Starts a Design from the Epics page through the draft page, which lands on its Design page. */
export const startDesign = async (page: Page, slug: string) => {
  const d = draftPage(page)
  await page.getByRole("button", nav("New design")).click()
  await expect(page).toHaveURL(/#\/drafts\//)
  await expect(d.prompt).toBeFocused()
  await d.prompt.fill("An idea.")
  await renameDraft(page, slug)
  await d.start.click()
  await expect(page).toHaveURL(new RegExp(`#/d/demo:${slug}$`))
}
