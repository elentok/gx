import type { Locator, Page } from "@playwright/test"
import { expect, nav } from "./fixture"

// The Chord box helpers shared by the Chords and Nav keys flows.
export const chordKit = (page: Page) => {
  const box = page.getByRole("menu", nav("Chords"))
  const item = (name: RegExp) => box.getByRole("menuitem", { name })
  const leader = "ControlOrMeta+e"
  // After a hash change the old route can still render for a moment, and the route change then closes a box opened on it.
  // `ready` is a row of the new page's box: the retry opens the box again until that row shows.
  const open = async (ready: Locator = box) => {
    await expect(async () => {
      if (await box.isVisible()) await page.keyboard.press("Escape")
      await page.keyboard.press(leader)
      await expect(ready).toBeVisible({ timeout: 1_000 })
    }).toPass({ timeout: 10_000 })
  }
  return {
    box,
    leader,
    open,
    back: item(/^,\s*Back$/),
    forward: item(/^\.\s*Forward$/),
    next: item(/^j\s*Next item$/),
    prev: item(/^k\s*Previous item$/),
  }
}
