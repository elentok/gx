import { epicsButton, expect, nav, openEpics, test } from "./fixture"

// Select mode's rendering and keys are vitest's (SidebarSelect, EpicsPage and ArchivePage tests); this flow covers
// what needs the server and real layout: Move, Hold, Stop and the Archive confirm through the bar, a CLI archive
// leaving the selection, and the two ways into the Archive page.
test("Bulk actions", async ({ page, start }) => {
  test.setTimeout(120_000)
  const fx = await start("bulk-actions")
  await page.addStyleTag({
    content:
      "*,*::before,*::after{animation-duration:0s!important;animation-delay:0s!important;transition-duration:0s!important}",
  })
  const sidebar = page.getByRole("complementary", nav("Sidebar"))
  const box = (name: string) => sidebar.getByRole("checkbox", nav(`Select ${name}`))
  const bar = page.locator('[data-part="select-bar"]')
  const count = bar.getByTestId("select-count")
  const barButton = (name: string) => bar.getByRole("button", { name: new RegExp(`^${name}\\b`) })
  const dialog = (name: string) => page.getByRole("dialog", nav(name))

  const enter = async () => {
    await sidebar.getByRole("button", nav("Select")).click()
    await expect(bar).toBeVisible()
  }

  // Entry 1: the header's Select opens the checkbox column and the bar.
  await expect(box("bulk-merged")).toHaveCount(0)
  await enter()
  await expect(count).toHaveText("0 selected")
  await expect(box("bulk-merged")).toBeVisible()

  // Entry 11: Move opens the workspace menu above the bar and moves the fitting rows.
  await box("bulk-done").check()
  await box("bulk-unmerged").check()
  await expect(count).toHaveText("2 selected")
  await barButton("Move").click()
  const menu = page.getByRole("menu")
  await expect(menu).toBeVisible()
  // The menu opens upward from the Move button, so it never covers the bar's own buttons.
  const moveTop = (await barButton("Move").boundingBox())!.y
  const menuBox = (await menu.boundingBox())!
  expect(menuBox.y + menuBox.height).toBeLessThanOrEqual(moveTop)
  await menu.getByRole("menuitem", { name: /^bulk/ }).click()
  // The moved rows leave the Default workspace's list, and the mode ends because nothing is selected any more.
  await expect(bar).toHaveCount(0)
  await expect(sidebar.getByRole("link", nav("bulk-done Not started"))).toHaveCount(0)
  // The CLI verb takes many rows too.
  const moved = await fx.cli(
    "server",
    "workspaces",
    "move",
    "demo:bulk-done-impl",
    "demo:bulk-unmerged",
    "bulk",
    "--json",
  )
  expect(JSON.parse(moved).results).toHaveLength(2)

  // Entry 12: Hold on a live Design asks and lists its Sessions; on an epic it asks nothing.
  await enter()
  await box("bulk-early").check()
  await barButton("Hold").click()
  await expect(dialog("Hold selected")).toContainText("Holding ends these live sessions")
  await dialog("Hold selected").getByRole("button", nav("Cancel")).click()
  await expect(dialog("Hold selected")).toHaveCount(0)
  await expect(count).toHaveText("1 selected")
  await box("bulk-early").uncheck()
  await box("bulk-clash").check()
  await barButton("Hold").click()
  await expect(dialog("Hold selected")).toHaveCount(0)
  // The held row stays selected, in the collapsed On hold section, so the mode stays on until Done.
  await expect(bar).toBeVisible()
  await expect(count).toHaveText("1 selected")
  await expect(box("bulk-clash")).toHaveCount(0)
  await bar.getByRole("button", nav("Done")).click()
  await expect(bar).toHaveCount(0)

  // Entry 14: Stop confirms once and lists the rows; the Exited agent is skipped as not live.
  await sidebar.getByRole("button", nav("Expand bulk-paused")).click()
  await expect(sidebar.getByRole("link", { name: /^Fix agent / })).toBeVisible()
  await enter()
  await sidebar.getByRole("checkbox", { name: /^Select fix-/ }).check()
  await box("Claude Code").check()
  await barButton("Stop").click()
  await expect(dialog("Stop selected")).toContainText("Stop ")
  await expect(dialog("Stop selected")).not.toContainText("Claude Code")
  await dialog("Stop selected").getByRole("button", nav("Stop")).click()
  await expect(dialog("Stop selected")).toHaveCount(0)
  await expect(page.getByText("Not run on some rows")).toBeVisible()
  await expect(page.getByText(/Claude Code.*not live/)).toBeVisible()
  // Both rows stay in the list (the stopped one as Exited), so both stay selected.
  await expect(count).toHaveText("2 selected")
  await bar.getByRole("button", nav("Done")).click()
  await expect(bar).toHaveCount(0)

  // Entry 17: the Archive confirm lists the rows, the live Sessions and one warning per epic not on trunk.
  // The scenario has no live Session on an epic that fits, so the Sessions line is covered by the rig and vitest.
  await enter()
  await box("bulk-merged").check()
  await box("bulk-done-solo").check()
  await box("bulk-early").check()
  await barButton("Archive").click()
  const archive = dialog("Archive selected")
  // The live Design is not finished, so the bar leaves it out of the dialog.
  await expect(archive).toContainText("Archive 2 epics?")
  for (const name of ["bulk-merged", "bulk-done-solo"])
    await expect(archive.getByRole("listitem").filter({ hasText: name })).toHaveCount(1)
  await expect(archive.getByRole("listitem")).toHaveCount(2)
  await expect(archive.getByText(/is not merged to trunk/)).toHaveCount(1)
  await expect(archive.getByText("⚠ bulk-done-solo is not merged to trunk")).toBeVisible()
  await archive.getByRole("button", nav("Cancel")).click()
  await expect(archive).toHaveCount(0)

  // Entry 22: an epic archived through the CLI leaves the selection.
  await expect(count).toHaveText("3 selected")
  const archived = await fx.cli("server", "epics", "archive", "demo:bulk-merged", "--json")
  expect(JSON.parse(archived).results).toHaveLength(1)
  await expect(count).toHaveText("2 selected")
  await expect(box("bulk-merged")).toHaveCount(0)
  await bar.getByRole("button", nav("Done")).click()
  await expect(bar).toHaveCount(0)

  // Entry 26: the footer keeps its width while the Epics menu is open, and Archive opens the Archive page.
  const footer = page.locator('[data-part="sidebar-footer"]')
  const width = async () => (await footer.boundingBox())!.width
  const before = await width()
  await epicsButton(page).click()
  const epicsMenu = page.getByRole("menuitem", { name: /^Archive/ })
  await expect(epicsMenu).toBeVisible()
  expect(await width()).toBe(before)
  await epicsMenu.click()
  await expect(page).toHaveURL(/#\/archive$/)
  await expect(page.getByRole("heading", nav("Archive"))).toBeVisible()

  // Entry 25: the Epics page header's Archive link opens the Archive page.
  await openEpics(page)
  await expect(page).toHaveURL(/#\/epics$/)
  await page.getByRole("link", nav("Archive")).click()
  await expect(page).toHaveURL(/#\/archive$/)
  await expect(page.getByRole("heading", nav("Archive"))).toBeVisible()

  // Entry 27: "← Epics" returns to the Epics page.
  await page.getByRole("link", nav("← Epics")).click()
  await expect(page).toHaveURL(/#\/epics$/)
  await page.getByRole("link", nav("Archive")).click()
  await expect(page).toHaveURL(/#\/archive$/)

  // Entry 28: newest first, with project, kind, date and workspace; the order and fields match the CLI.
  const rows = page.locator("li[aria-label]")
  const cliRows = async (): Promise<
    { title: string; address: string; workspace: string; kind: string; archived_at?: string }[]
  > => JSON.parse(await fx.cli("server", "epics", "archived", "--json")).epics
  const listed = await cliRows()
  expect(listed.length).toBeGreaterThan(3)
  await expect(rows).toHaveCount(listed.length)
  await expect(rows).toHaveText(listed.map((r) => new RegExp(`^${r.title}`)))
  for (const r of listed) {
    const row = rows.filter({ hasText: r.title }).first()
    await expect(row).toContainText(`demo · ${r.kind === "map" ? "Design" : "Epic"} ·`)
    await expect(row).toContainText(r.workspace)
    if (r.archived_at) {
      const date = await page.evaluate(
        (iso) =>
          new Date(iso).toLocaleDateString(undefined, {
            year: "numeric",
            month: "short",
            day: "numeric",
          }),
        r.archived_at,
      )
      await expect(row).toContainText(date)
    }
  }
  const rowOf = (title: string) => rows.filter({ has: page.getByText(title, { exact: true }) })

  // Entry 36: a click on a row, and Enter on the focused row, open the read-only detail.
  await rowOf("arch-bare").click()
  await expect(page).toHaveURL(/#\/archive\/demo:arch-bare$/)
  await expect(page.getByTestId("archived-stamp")).toHaveCount(0)
  await page.goBack()
  await expect(page).toHaveURL(/#\/archive$/)
  await rowOf("arch-stamped").focus()
  await page.keyboard.press("Enter")
  await expect(page).toHaveURL(/#\/archive\/demo:arch-stamped$/)

  // Entry 37: the detail has summary, tickets, cost and times, and Unarchive as the only action; tickets are not links.
  await expect(page.getByRole("heading", nav("arch-stamped"))).toBeVisible()
  await expect(page.getByTestId("archived-stamp")).toContainText("Archived")
  await expect(page.getByRole("button", nav("Unarchive"))).toBeVisible()
  await expect(page.locator('a[href^="#/t/"]')).toHaveCount(0)
  for (const name of ["Queue", "Hold", "Merge", "Fix", "Open design"]) {
    await expect(page.getByRole("button", nav(name))).toHaveCount(0)
    await expect(page.getByRole("link", nav(name))).toHaveCount(0)
  }
  await expect(page.getByText("built", { exact: true })).toBeVisible()

  // Entry 32 and 38: Unarchive from the detail puts the epic back in its named workspace, active on the Epics page.
  const membersBefore =
    JSON.parse(await fx.cli("server", "workspaces", "list", "--json")).workspaces.find(
      (w: { name: string }) => w.name === "bulk",
    ).members + 1
  await page.getByRole("button", nav("Unarchive")).click()
  await expect(page).toHaveURL(/#\/e\/demo:arch-stamped$/)
  await epicsButton(page).click()
  await page.getByRole("menuitem", { name: /^Archive/ }).click()
  await expect(rowOf("arch-stamped")).toHaveCount(0)
  const bulkMembers = async () => {
    const all: { name: string; members: number }[] = JSON.parse(
      await fx.cli("server", "workspaces", "list", "--json"),
    ).workspaces
    return all.find((w) => w.name === "bulk")!.members
  }
  await expect.poll(bulkMembers).toBe(membersBefore)
  await openEpics(page)
  await expect(page.getByRole("link", nav("arch-stamped")).first()).toBeVisible()
  await page.getByRole("link", nav("Archive")).click()

  // Entry 39: Unarchive from a row menu.
  await rowOf("arch-stale")
    .getByRole("button", { name: /actions/i })
    .click()
  await page.getByRole("menuitem", nav("Unarchive")).click()
  await expect(rowOf("arch-stale")).toHaveCount(0)

  // Entry 41: an epic archived through the CLI appears without a reload, and leaves after a CLI Unarchive.
  const cliArchived = await fx.cli("server", "epics", "archive", "demo:bulk-unmerged", "--json")
  expect(JSON.parse(cliArchived).results).toHaveLength(1)
  await expect(rowOf("bulk-unmerged")).toHaveCount(1)
  const address = (await cliRows()).find((r) => r.title === "bulk-unmerged")!.address
  await fx.cli("server", "epics", "unarchive", address, "--json")
  await expect(rowOf("bulk-unmerged")).toHaveCount(0)

  // Entry 34: the Fix agent Exited by the Archive shows nowhere while archived, and under its row after Unarchive.
  const fixLinks = sidebar.getByRole("link", { name: /^Fix agent / })
  const fixBefore = await fixLinks.count()
  await rowOf("bulk-agents")
    .getByRole("button", { name: /actions/i })
    .click()
  await page.getByRole("menuitem", nav("Unarchive")).click()
  await expect(rowOf("bulk-agents")).toHaveCount(0)
  await sidebar.getByRole("button", nav("Expand bulk-agents")).click()
  await expect(fixLinks).toHaveCount(fixBefore + 1)

  // Entry 57: at 744 px and 390 px the checkboxes and the bar work and nothing scrolls sideways, on both pages.
  const pageSelect = page.getByTestId("page-header-actions").getByRole("button", nav("Select"))
  const sideways = () =>
    page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth)
  for (const width of [744, 390]) {
    await page.setViewportSize({ width, height: 844 })
    for (const hash of ["#/archive", "#/epics"]) {
      await page.evaluate((h) => (location.hash = h), hash)
      await expect(
        page.getByRole("heading", nav(hash === "#/archive" ? "Archive" : "Epics")),
      ).toBeVisible()
      await pageSelect.click()
      await expect(bar).toBeVisible()
      await page
        .getByRole("checkbox", { name: /^Select / })
        .first()
        .check()
      await expect(count).toHaveText("1 selected")
      await expect(bar).toBeInViewport()
      expect(await sideways(), `${hash} at ${width}`).toBe(false)
      await bar.getByRole("button", nav("Done")).click()
      await expect(bar).toHaveCount(0)
    }
    // The sidebar's own bar and checkboxes, in the slide-over.
    await page.evaluate(() => (location.hash = "#/epics"))
    await page.getByRole("button", nav("Show sidebar")).click()
    await enter()
    await box("bulk-done-solo").check()
    await expect(count).toHaveText("1 selected")
    await expect(bar).toBeInViewport()
    expect(await sideways(), `sidebar at ${width}`).toBe(false)
    await bar.getByRole("button", nav("Done")).click()
    await expect(bar).toHaveCount(0)
  }
  await page.setViewportSize({ width: 1280, height: 720 })

  // Entry 30: after the last row is unarchived the page shows the empty state.
  await page.evaluate(() => (location.hash = "#/archive"))
  await expect(page.getByRole("heading", nav("Archive"))).toBeVisible()
  await pageSelect.click()
  for (const r of await cliRows())
    await page.getByRole("checkbox", nav(`Select ${r.title}`)).check()
  await barButton("Unarchive").click()
  await expect(page.getByText("Nothing is archived")).toBeVisible()
})
