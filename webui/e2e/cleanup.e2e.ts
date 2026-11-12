import { access } from "node:fs/promises"
import { expect, nav, openMenu, test, type Fixture } from "./fixture"
import type { Page } from "@playwright/test"

// The sidebar re-renders as the route changes, which can detach the open menu; reopen until the click lands.
const gearItem = async (page: Page, name: string) => {
  await expect(async () => {
    await openMenu(page)
    await page.getByRole("menuitem", nav(name)).click({ timeout: 3_000 })
  }).toPass()
}

const group = (page: Page, title: string) => page.getByRole("region", nav(title))

// The fixture keeps the project's trunk checkout at <root>/repo, and the cleanup scenario's worktrees under it.
const worktreeDir = async (fixture: Fixture, branch: string) => {
  const path = (await fixture.cli("tickets", "show", "demo:cleanup-epic/01")).match(
    /^path: (.+)$/m,
  )![1]
  return `${path.split("/data/gx/tickets")[0]}/repo/.worktrees/${branch}`
}

const exists = (path: string) =>
  access(path).then(
    () => true,
    () => false,
  )

// Row rendering and grouping are in webui/src/cleanup/*.test.tsx and the server's rules in server/cleanup; this
// flow covers the pieces that need the real server: the gear menu, the size scan, the writes and the one-off.
test.describe("198. Cleanup", () => {
  test("gear menu pages, groups, remove unmerged worktree, cleanup agent", async ({
    page,
    start,
  }) => {
    const fixture = await start("cleanup")

    await openMenu(page)
    for (const name of ["More settings...", "Cleanup", "Tips"])
      await expect(page.getByRole("menuitem", nav(name))).toBeVisible()
    await page.getByRole("menuitem", nav("Tips")).click()
    await expect(page.getByRole("heading", { name: "Tips" })).toBeVisible()

    await gearItem(page, "Cleanup")
    await expect(page.getByRole("heading", { name: "Cleanup" })).toBeVisible()
    await expect(page.getByRole("combobox", nav("Project"))).toHaveValue("demo")
    for (const title of [
      "Merged worktrees",
      "Done epics",
      "Unmerged idle worktrees",
      "Needs the cleanup agent",
    ]) {
      await expect(group(page, title)).toBeVisible()
    }
    await expect(group(page, "Unmerged idle worktrees").getByText("idle-wt")).toBeVisible()
    await expect(group(page, "Merged worktrees").getByText("merged-wt")).toBeVisible()

    // Sizes arrive after the first report.
    await expect(group(page, "Unmerged idle worktrees").getByTestId("cleanup-size")).toHaveText(
      /\d/,
    )

    const idle = await worktreeDir(fixture, "idle-wt")
    const idleRow = group(page, "Unmerged idle worktrees")
      .getByRole("listitem")
      .filter({ hasText: "idle-wt" })
    await idleRow.getByRole("button", nav("Remove")).click()
    const dialog = page.getByRole("dialog", nav("Remove unmerged worktree"))
    await expect(dialog.getByText("Remove the worktree of idle-wt?")).toBeVisible()
    await dialog.getByRole("button", nav("Cancel")).click()
    await expect(dialog).toBeHidden()
    await expect(idleRow).toBeVisible()
    expect(await exists(idle)).toBe(true)

    await idleRow.getByRole("button", nav("Remove")).click()
    await dialog.getByRole("button", nav("Confirm remove")).click()
    await expect(group(page, "Unmerged idle worktrees")).toHaveCount(0)
    expect(await exists(idle)).toBe(false)
    // The branch stays, so it now waits for the agent as a worktree-less unresolved branch.
    await expect(group(page, "Needs the cleanup agent").getByText("idle-wt")).toBeVisible()

    await group(page, "Needs the cleanup agent")
      .getByRole("button", nav("Start cleanup agent"))
      .click()
    // The one-off is a ticket in the one-offs epic; its body is the prompt.
    const prompt = () => fixture.cli("tickets", "show", "demo:one-offs/01").catch(() => "")
    await expect.poll(prompt).toContain("Run /gx-cleanup for project demo")
    const text = await prompt()
    expect(text).toContain("ralph-loop/cleanup-epic-item-02")
    expect(text).toContain("idle-wt")
    expect(text).toContain("unmerged-epic")
    // Whole list lines: "merged-epic" is a substring of "unmerged-epic".
    expect(text).not.toMatch(/^- (merged-wt|squash-one|merged-epic)$/m)

    // The cleanup fake asks at the confirm, so the one-off waits for an answer on its page.
    await page.evaluate(() => (location.hash = "#/t/demo:one-offs/01"))
    const answer = page.getByRole("form", nav("Answer"))
    await answer.getByRole("textbox", { name: "Your answer" }).fill("Remove them")
    await answer.getByRole("button", nav("Send answer")).click()
    await expect(page.getByText(/^Answer sent/)).toBeVisible()
  })
})
