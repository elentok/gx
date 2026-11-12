import type { Locator, Page } from "@playwright/test"
import { expect, nav } from "./fixture"

// Zero-length animations: menu closes and sidebar slides wait out their motion, and the flows check end states, not motion.
export const stillMotion = (page: Page) =>
  page.addStyleTag({
    content:
      "*,*::before,*::after{animation-duration:0s!important;animation-delay:0s!important;transition-duration:0s!important}",
  })

// A row's "..." button sits in the frame, the link's parent.
export const frameButton = (link: Locator) =>
  link.locator("xpath=..").getByRole("button", nav("More actions"))

// Sidebar helpers shared by the Workspaces and Sidebar rows flows.
export const sidebarKit = (page: Page) => {
  const sidebar = page.getByRole("complementary", nav("Sidebar"))
  // A Fix agent's link is named "Fix agent <name>": its wrench icon carries the label.
  const row = (title: string) =>
    sidebar.getByRole("link", { name: new RegExp(`^(Fix agent )?${title}(?![\\w-])`) })
  const itemOf = (title: string) => row(title).locator("xpath=ancestor::li[1]")
  const nestedLinks = (title: string) => itemOf(title).locator("ul").getByRole("link")
  const switcher = page.getByRole("button", { name: /^Workspace:/ })
  // The first Escape can land before a menu takes focus, and a closing menu keeps covering the sidebar until it is gone.
  // A click on a submenu item that lands before the menu has taken focus does not open it.
  const menuFocused = () => expect(page.locator('[role="menu"]:focus-within')).toHaveCount(1)
  const closeMenus = () =>
    expect(async () => {
      await page.keyboard.press("Escape")
      await expect(page.getByRole("menu")).toHaveCount(0, { timeout: 1_000 })
    }).toPass({ timeout: 10_000 })
  const pick = async (name: string) => {
    await switcher.click()
    await page.getByRole("menuitem", { name: new RegExp(`^${name}`) }).click()
    await expect(page.getByRole("button", nav(`Workspace: ${name}`))).toBeVisible()
    // The closing menu keeps covering the sidebar, and its items answer later queries, until it is gone.
    await expect(page.getByRole("menu")).toHaveCount(0)
  }
  return { sidebar, row, itemOf, nestedLinks, switcher, menuFocused, closeMenus, pick }
}
