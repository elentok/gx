import { expect, nav, test } from "./fixture"
import {
  ask,
  composer,
  created,
  createButton,
  go,
  messageBox,
  moreActions,
  newForm,
  projectSelect,
  promptBox,
  REPLY,
  row,
  sendButton,
  sessions,
  sidebar,
  openNewAgent,
  worktreeBox,
} from "./agentsKit"

// Flow 85 also covers 86 (worktree), 98 (model label) and 90 (offline, last); flow 87 moved to
// AgentsPage.newSession.test.tsx and NewSessionForm.test.tsx.
test("85. Create session", async ({ page, start }) => {
  const fixture = await start("agents")
  const before = await sessions(fixture)
  await page.setViewportSize({ width: 1280, height: 800 })
  await go(page, "#/agents")

  // The page's own "New session" button opens the same form, with Name focused (not Project or First prompt).
  await page.getByRole("main").getByRole("button", { name: "New session", exact: true }).click()
  await expect(newForm(page)).toBeVisible()
  await expect(projectSelect(page)).not.toHaveValue("")
  await expect(newForm(page).getByRole("textbox", { name: "Name", exact: true })).toBeFocused()
  await newForm(page).getByRole("button", { name: "Close" }).click()
  await expect(newForm(page)).toHaveCount(0)

  await openNewAgent(page, "demo")
  await expect(projectSelect(page)).toHaveValue("demo")
  await expect(worktreeBox(page)).toHaveValue("Project root")
  await promptBox(page).fill("first words")
  await createButton(page).click()

  const made = await created(fixture, before)
  expect(made.project).toBe("demo")
  await expect(page).toHaveURL(new RegExp(`#/agents/${made.id}$`))
  await expect(newForm(page)).toHaveCount(0)
  await expect(row(page, made)).toHaveAttribute("aria-current", "page")
  await expect(page.getByText("first words", { exact: true })).toBeVisible()
  expect(await fixture.cli("agents", "list")).toContain(made.id)

  // 86. Create in a new worktree.
  const beforeTree = await sessions(fixture)
  await openNewAgent(page, "demo")

  // A bad name shows the refusal in the form and keeps the input.
  await worktreeBox(page).fill("bad name")
  await page.getByRole("option", { name: /Create worktree/ }).click()
  await createButton(page).click()
  await expect(newForm(page).getByRole("alert")).toBeVisible()
  await expect(worktreeBox(page)).toHaveValue("bad name")
  expect(await sessions(fixture)).toHaveLength(beforeTree.length)

  // A first prompt that starts with ! is refused too.
  await worktreeBox(page).fill("fresh-tree")
  await page.getByRole("option", { name: /Create worktree/ }).click()
  await promptBox(page).fill("!ls")
  await createButton(page).click()
  await expect(newForm(page).getByRole("alert")).toBeVisible()
  await expect(promptBox(page)).toHaveValue("!ls")
  await expect(worktreeBox(page)).toHaveValue("fresh-tree")
  expect(await sessions(fixture)).toHaveLength(beforeTree.length)

  // A good one creates the worktree and runs the session in it.
  await promptBox(page).fill("hello")
  await createButton(page).click()
  const inTree = await created(fixture, beforeTree)
  expect(inTree.dir).toMatch(/fresh-tree$/)
  await expect(page).toHaveURL(new RegExp(`#/agents/${inTree.id}$`))
  await expect(newForm(page)).toHaveCount(0)

  // 98. A project with no tier config runs the default worker model, and a reply keeps the label.
  const beforeOther = await sessions(fixture)
  await openNewAgent(page, "other")
  await createButton(page).click()
  const interactive = await created(fixture, beforeOther)
  const label = page.getByText("Claude Sonnet · medium", { exact: true })
  const tierShown = async () => {
    await moreActions(page).click()
    await expect(
      page.getByRole("menu", nav("More actions")).getByText("Worker", { exact: true }),
    ).toBeVisible()
    // Escape goes to the menu once focus is inside it.
    await expect(async () => {
      await page.keyboard.press("Escape")
      await expect(page.getByRole("menu", nav("More actions"))).toHaveCount(0, { timeout: 500 })
    }).toPass()
  }
  await expect(label).toBeVisible()
  await tierShown()
  await ask(fixture, interactive.id, "hello")
  await expect(label).toBeVisible()
  await tierShown()

  // 90. Offline comes last: the server stops. The create form and the composer are disabled.
  const idle = before[0]
  await go(page, `#/agents/${idle.id}`)
  await expect(page.getByText(REPLY)).toBeVisible()
  await messageBox(page).fill("typed before the drop")
  await openNewAgent(page, "demo")
  await promptBox(page).fill("a first prompt")
  await expect(createButton(page)).toBeEnabled()

  await fixture.stopServer()
  await expect(page.getByText("Offline, reconnecting…")).toBeVisible({ timeout: 15_000 })
  await expect(createButton(page)).toBeDisabled()
  await expect(promptBox(page)).toHaveValue("a first prompt")
  await newForm(page).getByRole("button", { name: "Close" }).click()

  // The last data stays; Send is off with its reason and the draft is kept.
  await expect(row(page, idle)).toBeVisible()
  await expect(page.getByText(REPLY)).toBeVisible()
  await expect(composer(page).getByText("Offline. Send turns on when gx reconnects.")).toBeVisible()
  await expect(sendButton(page)).toBeDisabled()
  await expect(messageBox(page)).toHaveValue("typed before the drop")
})

test("88. Phone, Agents page", async ({ page, start }) => {
  const fixture = await start("agents")
  const list = await sessions(fixture)
  const target = list[1]
  await page.setViewportSize({ width: 375, height: 812 })
  const { width, height } = page.viewportSize()!

  // The page alone, with the shared sidebar slid away.
  await go(page, "#/agents")
  await expect(page.getByRole("heading", { name: "Pick a session" })).toBeVisible()
  await expect(row(page, target)).toBeHidden()

  // The sidebar's row opens the session on its own and slides the sidebar closed.
  await page.getByRole("button", { name: "Show sidebar", exact: true }).click()
  await row(page, target).click()
  await expect(page).toHaveURL(new RegExp(`#/agents/${target.id}$`))
  await expect(row(page, target)).toBeHidden()
  await expect(composer(page)).toBeVisible()

  // The footer "+ New" menu is an anchored popover that flips above its button, not a sheet.
  await page.getByRole("button", { name: "Show sidebar", exact: true }).click()
  const add = sidebar(page).getByRole("button", { name: "+ New", exact: true })
  await add.click()
  const menu = page.getByRole("menu", { name: "+ New" })
  await expect(menu).toBeVisible()
  await expect(page.getByTestId("grab-handle")).toHaveCount(0)
  await expect
    .poll(
      async () =>
        (await menu.boundingBox())!.y + (await menu.boundingBox())!.height <=
        (await add.boundingBox())!.y,
    )
    .toBe(true)

  // Create opens as a sheet at the bottom of the screen.
  await page.getByRole("menuitem", { name: /^New chat/ }).click()
  await expect(newForm(page)).toBeVisible()
  await expect(page.getByTestId("grab-handle")).toBeVisible()
  await expect
    .poll(async () => {
      const box = (await newForm(page).boundingBox())!
      return [box.x, Math.round(box.y + box.height), box.width]
    })
    .toEqual([0, height, width])
  await newForm(page).getByRole("button", { name: "Close" }).click()
  await expect(newForm(page)).toHaveCount(0)
})
