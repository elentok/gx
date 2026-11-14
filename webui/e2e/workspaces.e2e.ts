import { sessions } from "./agentsKit"
import { card, expect, nav, sessionRows, test } from "./fixture"
import { frameButton, goto, sidebarKit, stillMotion } from "./sidebarKit"

// One journey over the `workspaces` scenario: the sidebar's rows follow the active workspace, and
// Move, Remove and Hold work from every place an item shows.
test("Workspaces", async ({ page, start }) => {
  test.setTimeout(120_000)
  const fx = await start("workspaces")
  // Every menu close and sidebar slide waits out its animation (a third of this flow's run time); the flow checks end states, not motion.
  await stillMotion(page)
  const {
    sidebar,
    row,
    nestedLinks,
    switcher,
    menuFocused,
    closeMenus,
    pick,
    rowMenu,
    openSubmenu,
    moveTo,
    jump,
  } = sidebarKit(page)

  // The sidebar shows the Default workspace's rows on the Epics page.
  await expect(page.getByRole("button", nav("Workspace: Default workspace"))).toBeVisible()
  await expect(row("ready-epic")).toBeVisible()
  await expect(row("ws-design")).toHaveCount(0)

  // The footer's inbox button sums the project's follow-ups (2 bugs, 2 ideas); the popover line opens the Follow-ups card, section open and in view.
  const inbox = page.getByRole("button", { name: /^Follow-ups/ })
  const followUpsLine = page.getByRole("menuitem", { name: /^demo/ })
  await expect(inbox).toHaveAttribute("aria-label", "Follow-ups · 4 open")
  await expect(inbox).toContainText("4")
  await inbox.click()
  await expect(followUpsLine).toContainText("2 bugs · 2 ideas")
  await expect(followUpsLine).toContainText("4")
  await followUpsLine.click()
  await expect(page).toHaveURL(/#\/epics$/)
  const followUpsCard = page.locator('[data-focus="follow-ups:demo"]')
  await expect(followUpsCard).toBeVisible()
  await expect(followUpsCard).toBeInViewport()
  await expect(page.getByRole("menu")).toHaveCount(0)

  // The nested menu: Agent block, Stop while live, Remove; Remove asks first, then the row leaves and so does the session.
  await sidebar.getByRole("button", nav("Expand paused-epic")).click()
  const working = nestedLinks("paused-epic").filter({ hasText: "Working" })
  await expect(working).toHaveCount(1)
  const agentId = (await working.getAttribute("href"))!.split("/agents/")[1]
  expect(await sessionRows(fx, agentId)).toHaveLength(1)
  // Its status words change when it stops, so find the row by its link from here on.
  const agent = sidebar.locator(`a[href="#/agents/${agentId}"]`)
  await agent.click()
  await expect(page).toHaveURL(new RegExp(`#/agents/${agentId}$`))
  await expect(agent).toHaveAttribute("aria-current", "page")
  await frameButton(agent).click()
  await expect(page.getByRole("menu").getByText("Agent", { exact: true })).toBeVisible()
  await expect(page.getByRole("menuitem", nav("Stop"))).toBeVisible()
  await page.getByRole("menuitem", nav("Remove")).click()
  await expect(page.getByRole("dialog")).toBeVisible()
  await page.getByRole("dialog").getByRole("button", nav("Remove")).click()
  await expect(page).toHaveURL(/#\/agents$/)
  await expect(agent).toHaveCount(0)
  await expect.poll(() => sessionRows(fx, agentId)).toHaveLength(0)
  await goto(page, "#/epics")

  // Alt+Shift+2 picks the second workspace.
  await page.keyboard.press("Alt+Shift+2")
  await expect(page.getByRole("button", nav("Workspace: gx bugs"))).toBeVisible()
  await expect(row("ws-design")).toBeVisible()
  await page.keyboard.press("Alt+Shift+1")
  await expect(page.getByRole("button", nav("Workspace: Default workspace"))).toBeVisible()

  // Switching workspace changes the rows, on the Epics page and after in-app navigation to a Design.
  await pick("gx bugs")
  await expect(row("ws-design")).toBeVisible()
  await expect(row("ready-epic")).toHaveCount(0)
  // The badge covers only this workspace's projects: demo has cards here, so its follow-ups show, matching the card's words.
  await expect(inbox).toHaveAttribute("aria-label", "Follow-ups · 4 open")
  await inbox.click()
  await expect(followUpsLine).toContainText("2 bugs · 2 ideas")
  await closeMenus()
  await row("ws-design").click()
  // A Design row opens the Design page with its composer, not the epic page.
  await expect(page).toHaveURL(/#\/d\/demo:ws-design$/)
  await expect(page.getByRole("form", { name: "Composer" })).toBeVisible()
  await expect(row("ws-design")).toHaveAttribute("aria-current", "page")
  // Picking another workspace on the open item's page switches to it and the pick sticks.
  await pick("research")
  await expect(page.getByRole("button", nav("Workspace: research"))).toBeVisible()
  await expect(row("ws-impl-epic")).toBeVisible()
  await expect(row("ws-design")).toHaveCount(0)
  await expect(page).toHaveURL(/#\/d\/demo:ws-design$/)
  await expect(inbox).toHaveAttribute("aria-label", "Follow-ups · 4 open")

  // Move from a sidebar row: the row leaves this workspace and shows in the target.
  // The submenu opens to the side of its item, lists every workspace (the Default workspace too) and marks the current one.
  await rowMenu("ws-impl-epic").click()
  const moveItem = page.getByRole("menuitem", nav("Move to workspace"))
  const current = page
    .getByRole("menu")
    .last()
    .getByRole("menuitem", { name: /^research/ })
  await menuFocused()
  await openSubmenu(moveItem, current)
  await expect(current).toHaveAttribute("aria-current", "true")
  await expect(page.getByRole("menuitem", nav("Default workspace"))).toHaveAttribute(
    "aria-current",
    "false",
  )
  await expect(page.getByRole("menuitem", nav("gx bugs"))).toBeVisible()
  // Both menus animate in; measure once they hold still.
  await expect(async () => {
    const [itemBox, subBox] = [await moveItem.boundingBox(), await current.boundingBox()]
    expect(subBox!.x).toBeGreaterThanOrEqual(itemBox!.x + itemBox!.width - 1)
  }).toPass({ timeout: 5_000 })
  await expect(page.getByRole("menuitem", nav("Remove from workspace"))).toHaveCount(0)
  await page.keyboard.press("Escape")
  await page.keyboard.press("Escape")

  await moveTo(rowMenu("ws-impl-epic"), "gx bugs")
  await expect(row("ws-impl-epic")).toHaveCount(0)
  await pick("gx bugs")
  await expect(row("ws-impl-epic")).toBeVisible()
  // Move from an Epics page card is covered by the EpicCard tests; ws-impl-epic stays in gx bugs for the jump dialog.
  await goto(page, "#/epics")

  // Jump dialog: Move from a row, then opening an item in another workspace switches the sidebar.
  const jumpRow = (title: string) => jump.getByRole("option").filter({ hasText: title }).first()
  const openJump = async (filter: string) => {
    await page.keyboard.press("ControlOrMeta+k")
    await jump.getByRole("combobox").fill(filter)
  }
  await pick("Default workspace")
  await openJump("held-epic")
  await jumpRow("held-epic").click()
  await expect(page).toHaveURL(/#\/e\/demo:held-epic$/)
  // A held item does not switch the workspace.
  await expect(page.getByRole("button", nav("Workspace: Default workspace"))).toBeVisible()
  await goto(page, "#/epics")
  await openJump("ws-impl-epic")
  await jumpRow("ws-impl-epic").getByRole("button", nav("More actions")).click()
  await menuFocused()
  await openSubmenu(
    page.getByRole("menuitem", nav("Move to workspace")),
    page.getByRole("menuitem", nav("research")),
  )
  await page.getByRole("menuitem", nav("research")).click()
  await expect(jumpRow("ws-impl-epic")).toContainText("research")
  await jumpRow("ws-impl-epic").click()
  await expect(page).toHaveURL(/#\/e\/demo:ws-impl-epic$/)
  await expect(page.getByRole("button", nav("Workspace: research"))).toBeVisible()
  await expect(row("ws-impl-epic")).toBeVisible()
  await goto(page, "#/epics")

  // A chat row shows in its workspace only.
  await pick("Default workspace")
  await expect(sidebar.getByRole("img", nav("Chat"))).toHaveCount(2) // one per project
  await pick("research")
  await expect(sidebar.getByRole("img", nav("Chat"))).toHaveCount(0)

  // Creates land in the active workspace: the sidebar's + New (design, chat); the Epics page's New Design is covered by the EpicsPage tests.
  const members = async () =>
    JSON.parse(await fx.cli("server", "workspaces", "list", "--json")).workspaces.find(
      (w: { name: string }) => w.name === "research",
    ).members as number
  const startDesign = async (slug: string) => {
    const dialog = page.getByRole("dialog", nav("New Design"))
    await dialog.getByRole("textbox", nav("Epic slug")).fill(slug)
    await dialog.getByRole("textbox", nav("Rough idea")).fill("an idea")
    await dialog.getByRole("button", nav("Start design")).click()
    await expect(dialog).toHaveCount(0)
  }
  const before = await members()
  await sidebar.getByRole("button", { name: "+ New", exact: true }).click()
  await page.getByRole("menuitem", { name: /^New design/ }).click()
  await startDesign("made-from-sidebar")
  await expect.poll(members).toBe(before + 1)
  await goto(page, "#/epics")
  await sidebar.getByRole("button", { name: "+ New", exact: true }).click()
  await page.getByRole("menuitem", { name: /^New chat/ }).click()
  await page.getByRole("button", { name: "Create session" }).click()
  await expect(sidebar.getByRole("img", nav("Chat"))).toHaveCount(1)
  await expect.poll(members).toBe(before + 2)
  // Later steps expect Default's chat alone, since deleting this workspace moves its items there.
  const madeId = (await sidebar.locator("a[data-chat]").getAttribute("href"))!.split("/agents/")[1]
  await fx.cli("agents", "stop", madeId)
  await fx.cli("agents", "remove", madeId)
  await expect(sidebar.getByRole("img", nav("Chat"))).toHaveCount(0)
  await goto(page, "#/epics")

  // Hold from an Epics card: the row moves to the collapsed On hold section.
  await pick("Default workspace")
  const onHold = sidebar.getByRole("button", { name: /^On hold \(\d+\)$/ })
  await expect(onHold).toHaveCount(0)
  await card(page, "ready-epic").getByRole("button", nav("More actions")).click()
  await page.getByRole("menuitem", nav("Hold")).click()
  await expect(onHold).toHaveText("On hold (1)")
  await expect(onHold).toHaveAttribute("aria-expanded", "false")
  await expect(row("ready-epic")).toHaveCount(0)
  await onHold.click()
  await expect(row("ready-epic")).toBeVisible()

  // Take it off hold from the sidebar row, then hold it again from the same row.
  await rowMenu("ready-epic").click()
  await page.getByRole("menuitem", nav("Take off hold")).click()
  await expect(onHold).toHaveCount(0)
  await expect(row("ready-epic")).toBeVisible()
  await rowMenu("ready-epic").click()
  await page.getByRole("menuitem", nav("Hold")).click()
  await expect(onHold).toHaveText("On hold (1)")

  // Hold from the page header and from a jump row is covered by the EpicPage and JumpPalette tests.

  // A Design with live sessions asks first.
  await pick("gx bugs")
  await rowMenu("ws-design").click()
  await page.getByRole("menuitem", nav("Hold")).click()
  const holding = page.getByRole("dialog", nav("Hold ws-design"))
  await holding.getByRole("button", nav("Cancel")).click()
  await expect(row("ws-design")).toBeVisible()
  await rowMenu("ws-design").click()
  await page.getByRole("menuitem", nav("Hold")).click()
  await holding.getByRole("button", nav("Hold and end sessions")).click()
  await expect(onHold).toHaveText("On hold (2)")
  if ((await onHold.getAttribute("aria-expanded")) === "false") await onHold.click()
  await expect(row("ws-design")).toBeVisible()
  // Another held Design (held-design) may sort first, so look for the row anywhere below the header.
  await expect(onHold.locator("xpath=following::a").filter({ hasText: "ws-design" })).toHaveCount(1)

  // Other sits below On hold and is the same in every workspace. Its Fix agents have a menu without Move or Hold; the Queue agent has none.
  const other = sidebar.getByRole("button", nav("Other"))
  const otherRows = ["fix-demo_cone-offs_s04", "fix-demo", "one-offs-iter-01"]
  const showOther = async () => {
    if ((await other.getAttribute("aria-expanded")) === "false") await other.click()
    for (const name of otherRows) await expect(row(name)).toBeVisible()
  }
  await showOther()
  for (const name of otherRows)
    await expect(
      row(name).locator("xpath=..").getByRole("button", nav("More actions")),
    ).toHaveCount(name === "one-offs-iter-01" ? 0 : 1)
  await row("fix-demo").locator("xpath=..").getByRole("button", nav("More actions")).click()
  await expect(page.getByRole("menuitem", nav("Remove"))).toBeVisible()
  await expect(page.getByRole("menuitem", { name: /^(Move|Hold)/ })).toHaveCount(0)
  await closeMenus()
  await pick("Default workspace")
  await showOther()
  // Opening an Other row leaves the workspace alone.
  await row("fix-demo").click()
  await expect(page).toHaveURL(/#\/agents\//)
  await expect(page.getByRole("button", nav("Workspace: Default workspace"))).toBeVisible()
  await goto(page, "#/epics")
  // The Fix agent dialogs (refusal, nesting under the epic) are covered by the pause and restart-run flows.

  // Manage workspaces: order, rename, reorder, delete naming its members; Default has no controls.
  const manage = page.getByRole("dialog", nav("Manage workspaces"))
  const names = manage.getByRole("textbox")
  const openManage = async () => {
    await switcher.click()
    await page.getByRole("menuitem", nav("Manage workspaces")).click()
  }
  await openManage()
  await expect(names).toHaveCount(2)
  await expect(names.nth(0)).toHaveValue("gx bugs")
  await expect(names.nth(1)).toHaveValue("research")
  await expect(manage.getByRole("button", { name: /^(Rename|Delete) Default/ })).toHaveCount(0)
  await expect(manage.getByRole("button", nav("Move Default workspace up"))).toBeDisabled()
  await names.nth(1).fill("labs")
  await names.nth(1).press("Enter")
  await expect(names.nth(1)).toHaveValue("labs")
  await manage.getByRole("button", nav("Move labs up")).click()
  await expect(names.nth(0)).toHaveValue("labs")
  await expect(names.nth(1)).toHaveValue("gx bugs")
  await manage.getByRole("button", nav("Delete gx bugs")).click()
  const deleting = page.getByRole("dialog", nav("Delete gx bugs"))
  await expect(deleting).toContainText(/Its \d+ items? go(es)? back to the Default workspace/)
  await deleting.getByRole("button", nav("Keep")).click()
  await expect(names).toHaveCount(2)
  await manage.getByRole("button", nav("Delete gx bugs")).click()
  await deleting.getByRole("button", nav("Delete")).click()
  await expect(names).toHaveCount(1)
  await manage.getByRole("button", nav("Close")).click()

  // CLI changes show in the open page without a reload; deleting the active workspace drops to Default.
  await pick("labs")
  await fx.cli("server", "workspaces", "rename", "labs", "studies")
  // The renamed workspace is a new name to the page: the old one vanished, so the active one is Default.
  await expect(page.getByRole("button", nav("Workspace: Default workspace"))).toBeVisible()
  await pick("studies")
  await fx.cli("server", "workspaces", "create", "scratch")
  await fx.cli("server", "workspaces", "reorder", "scratch", "Default workspace", "studies")
  await switcher.click()
  const items = page
    .getByRole("menuitem")
    .filter({ hasText: /^(scratch|Default workspace|studies)/ })
  await expect(items.nth(0)).toHaveText(/^scratch/)
  await expect(items.nth(2)).toHaveText(/^studies/)
  await closeMenus()
  await fx.cli("server", "workspaces", "delete", "studies")
  await expect(page.getByRole("button", nav("Workspace: Default workspace"))).toBeVisible()

  // The gear menu holds AFK, Vi mode, Sort, Restart server, More settings, Cleanup and Tips; Sort is per workspace and follows the CLI.
  const gear = page.getByRole("button", nav("Menu"))
  const gearItems = page.getByRole("menu").locator('[role^="menuitem"]')
  await gear.click()
  await expect(gearItems).toHaveText([
    /^AFK$/,
    /^Vi mode$/,
    /^Sort · Default workspace/,
    /^Restart server$/,
    /^More settings\.\.\.$/,
    /^Cleanup/,
    /^Tips$/,
    /^Force refresh$/,
  ])
  const sortOptions = page.getByRole("menuitemradio")
  const openSort = async (workspace: string) => {
    await closeMenus()
    await gear.click()
    await menuFocused()
    await openSubmenu(
      page.getByRole("menuitem", { name: new RegExp(`^Sort · ${workspace}`) }),
      sortOptions.first(),
    )
  }
  await openSort("Default workspace")
  await expect(sortOptions).toHaveText([
    "Needs you first, then by name",
    "By last activity, newest first",
    "Alphabetically by name",
    "By project, then by name",
    "By status, then by name",
  ])
  await expect(sortOptions.nth(0)).toHaveAttribute("aria-checked", "true")
  await sortOptions.nth(1).click()
  await expect
    .poll(() => fx.cli("server", "workspaces", "sort", "Default workspace"))
    .toContain("activity")
  await fx.cli("server", "workspaces", "sort", "Default workspace", "name")
  await openSort("Default workspace")
  await expect(sortOptions.nth(2)).toHaveAttribute("aria-checked", "true")
  await closeMenus()
  await fx.cli("server", "workspaces", "sort", "Default workspace", "needs-you")

  // Rows reorder live and keyboard focus stays on the same row: the last row, which moves furthest when the order flips.
  const navRows = sidebar.locator("a[data-nav]")
  const last = navRows.last()
  await last.focus()
  const lastName = (await last.getAttribute("href"))!
  const focusedRow = sidebar.locator("a[data-nav]:focus")
  for (const order of ["name", "activity", "needs-you"]) {
    await fx.cli("server", "workspaces", "sort", "Default workspace", order)
    await expect(focusedRow).toHaveAttribute("href", lastName)
  }

  // Under "By last activity", Alt+1 opens the first row of the sorted list.
  await fx.cli("server", "workspaces", "sort", "Default workspace", "activity")
  await goto(page, "#/epics")
  const sortedFirst = (await sidebar.getByRole("link").first().getAttribute("href"))!.slice(1)
  await page.keyboard.press("Alt+1")
  await expect.poll(() => new URL(page.url()).hash.slice(1)).toBe(sortedFirst)
  await fx.cli("server", "workspaces", "sort", "Default workspace", "needs-you")
  await goto(page, "#/epics")

  // A row menu opens from the keyboard: press "." on the focused row, arrow to an item, Enter.
  const heldBefore = Number(/\d+/.exec((await onHold.textContent())!)![0])
  const held = onHold.locator("xpath=following::a[1]")
  if ((await onHold.getAttribute("aria-expanded")) === "false") await onHold.click()
  await held.focus()
  // The menu button is not a Tab stop (item 80); "." opens the focused row's menu.
  await expect(held.locator("xpath=..").getByRole("button", nav("More actions"))).toHaveAttribute(
    "tabindex",
    "-1",
  )
  await page.keyboard.press(".")
  await menuFocused()
  // "." opens the menu with nothing highlighted: the first item is the Move submenu, then Take off hold.
  await page.keyboard.press("ArrowDown")
  await page.keyboard.press("ArrowDown")
  await expect(page.getByRole("menuitem", nav("Take off hold"))).toHaveAttribute(
    "data-highlighted",
    "",
  )
  await page.keyboard.press("Enter")
  await expect(page.getByRole("menu")).toHaveCount(0)
  await expect(onHold).toHaveText(`On hold (${heldBefore - 1})`)
  await pick("scratch")
  await openSort("scratch")
  await expect(sortOptions.nth(0)).toHaveAttribute("aria-checked", "true")
  await closeMenus()
  await pick("Default workspace")
  await gear.click()
  await page.getByRole("menuitem", nav("More settings...")).click()
  // The wide shell replaces /settings with its first section, so the URL is either.
  await expect(page).toHaveURL(/#\/settings(\/s\/[\w-]+)?$/)
  await goto(page, "#/epics")
})
