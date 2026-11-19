import type { Page } from "@playwright/test"
import { expect, nav } from "./fixture"

export const draftPage = (page: Page) => ({
  prompt: page.getByRole("textbox", nav("Prompt")),
  slug: page.getByRole("textbox", nav("Epic slug")),
  project: page.getByRole("combobox", nav("Project")),
  rename: page.getByRole("button", nav("Rename")),
  start: page.getByRole("button", nav("Start design")),
  remove: page.getByRole("button", nav("Remove")),
})

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
