import { access } from "node:fs/promises"
import type { Page } from "@playwright/test"
import { chatOf, PNG, png, steerUploads } from "./agentsKit"
import { draftPage, history, renameDraft, startDesign } from "./draftKit"
import { expect, flag, holdRoute, nav, openEpics, section, sessionRows, test } from "./fixture"
import { sidebarKit } from "./sidebarKit"

const exists = (path: string) =>
  access(path).then(
    () => true,
    () => false,
  )

const newDesign = (page: Page) => page.getByRole("button", nav("New design")).click()

test("26. New Design form", async ({ page, start }) => {
  const fixture = await start("ready", { reloads: 2 })
  const d = draftPage(page)
  const { row, rowMenu, sidebar } = sidebarKit(page)
  const draftIds = async () =>
    (await fixture.cli("designs", "drafts", "list"))
      .split("\n")
      .filter((l) => l.includes("\t"))
      .map((l) => l.split("\t")[0]!.trim())
  const idOf = (p: Page) => p.url().split("/drafts/")[1]!
  const toast = page.locator("[data-scope='toast'][data-part='root']")

  // The button sits right of the filter. It opens a draft page with the Prompt focused and no dialog.
  const filter = await page.getByRole("searchbox", nav("Filter epics")).boundingBox()
  const at = await page.getByRole("button", nav("New design")).boundingBox()
  expect(at!.x).toBeGreaterThanOrEqual(filter!.x + filter!.width)
  await page.setViewportSize({ width: 1280, height: 800 })
  await newDesign(page)
  await expect(page).toHaveURL(/#\/drafts\/[^/]+$/)
  await expect(page.getByRole("dialog")).toHaveCount(0)
  const first = idOf(page)
  await expect(d.prompt).toBeFocused()
  await expect.poll(draftIds).toEqual([first])
  expect(await fixture.cli("server", "epics")).not.toContain(first)

  // The page: header with the label and Rename, project and slug bar, Prompt, image strip and footer.
  await expect(page.getByRole("heading", nav("Untitled design"))).toBeVisible()
  await expect(page).toHaveTitle(/^Untitled design · Design draft · gx/)
  await expect(d.project.locator("option")).toHaveText(["demo", "scratch"])
  await expect(d.slug).toHaveAttribute("placeholder", "my-epic")
  await expect(d.prompt).toHaveAttribute(
    "placeholder",
    "What is the problem? Who hits it? What does done look like?",
  )
  await expect(page.getByRole("button", nav("Add image"))).toBeVisible()
  await expect(d.remove).toBeVisible()
  // With no prompt Start gives the first reason; the footer's Start sits at the right edge.
  await expect(d.start).toBeDisabled()
  await expect(page.getByText("Write a prompt first")).toBeVisible()
  const [foot, rm, st] = [
    (await d.remove.locator("xpath=..").boundingBox())!,
    (await d.remove.boundingBox())!,
    (await d.start.boundingBox())!,
  ]
  expect(rm.x).toBeLessThan(foot.x + foot.width / 2)
  expect(st.x + st.width).toBeCloseTo(foot.x + foot.width, 0)

  // New design on an empty draft's page reuses it: still one row, the Prompt focused.
  await d.slug.click()
  await sidebar.getByRole("button", { name: "+ New", exact: true }).click()
  await page.getByRole("menuitem", { name: /^New design/ }).click()
  await expect(page).toHaveURL(new RegExp(`#/drafts/${first}$`))
  expect(await draftIds()).toEqual([first])
  await expect(d.prompt).toBeFocused()

  // A second context opens the Epics page and presses New design: its sweep leaves this open, leased empty draft alone, and its own draft is a second one.
  const other = await page
    .context()
    .browser()!
    .newContext({ ignoreHTTPSErrors: true, storageState: await page.context().storageState() })
  try {
    const page2 = await other.newPage()
    const d2 = draftPage(page2)
    const side2 = sidebarKit(page2)
    await page2.goto(`${page.url().split("#")[0]!}#/epics`)
    await newDesign(page2)
    await expect(page2).toHaveURL(/#\/drafts\/[^/]+$/)
    const second = idOf(page2)
    expect(second).not.toBe(first)
    await expect.poll(draftIds).toHaveLength(2)
    await expect(page).toHaveURL(new RegExp(`#/drafts/${first}$`))
    await expect(side2.row("Untitled design")).toHaveCount(2)
    // Leaving an empty draft inside the app removes it, and the first context's sidebar follows.
    await openEpics(page2)
    await expect.poll(draftIds).toEqual([first])
    await expect(row("Untitled design")).toHaveCount(1)

    // A reload keeps the empty draft. A slug alone keeps it until it is cleared.
    await page.reload()
    await expect(d.prompt).toBeVisible()
    expect(await draftIds()).toEqual([first])
    await renameDraft(page, "keep-me")
    await expect(page.getByText("Saved")).toBeVisible()
    await openEpics(page)
    expect(await draftIds()).toEqual([first])
    await row("keep-me").click()
    await expect(page).toHaveURL(new RegExp(`#/drafts/${first}$`))
    await d.slug.fill("")
    await expect.poll(() => fixture.cli("designs", "drafts", "list")).not.toContain("keep-me")
    await openEpics(page)
    await expect.poll(draftIds).toEqual([])

    // With the Epics page filtered to one project the new draft is on it; from a non-empty draft's page the new draft takes that draft's project.
    await page.getByRole("button", nav("Project")).click()
    await page.getByRole("checkbox", nav("scratch")).check()
    await page.keyboard.press("Escape")
    await newDesign(page)
    await expect(d.project).toHaveValue("scratch")
    const third = idOf(page)
    await d.prompt.fill("Ready to type now")
    await d.project.selectOption("demo")
    await expect(page.getByText("Saved")).toBeVisible()
    await sidebar.getByRole("button", { name: "+ New", exact: true }).click()
    await page.getByRole("menuitem", { name: /^New design/ }).click()
    await expect(d.prompt).toHaveValue("")
    expect(idOf(page)).not.toBe(third)
    await expect(d.project).toHaveValue("demo")
    await expect(d.prompt).toHaveValue("")
    // Leave that empty draft for the first one.
    await row("Ready to type now").click()
    await expect(page).toHaveURL(new RegExp(`#/drafts/${third}$`))
    await expect.poll(draftIds).toEqual([third])

    // Autosave shows "Saving..." then "Saved". The row and the title take the prompt's first words.
    const release = await holdRoute(page, "**/v1/designs/drafts/save")
    await d.prompt.fill("Ready to type now and more words")
    await expect(page.getByText("Saving...")).toBeVisible()
    release()
    await expect(page.getByText("Saved")).toBeVisible()
    await expect(row("Ready to type now and")).toBeVisible()
    await expect(page).toHaveTitle(/^Ready to type now and · Design draft · gx/)
    // The pencil focuses the slug; a new slug updates the row and the title; clearing it brings the prompt label back.
    await renameDraft(page, "alpha")
    await expect(row("alpha")).toBeVisible()
    await expect(page).toHaveTitle(/^alpha · Design draft · gx/)
    await expect(d.start).toBeEnabled()
    await d.slug.fill("")
    await expect(d.start).toBeDisabled()
    await expect(row("Ready to type now and")).toBeVisible()
    await expect(page).toHaveTitle(/^Ready to type now and · Design draft · gx/)
    await expect(page.getByText("Name the epic to start.")).toBeVisible()
    await d.slug.fill("alpha")
    await expect(page.getByText("Saved")).toBeVisible()

    // A reload brings back the same draft and text, with no second row.
    await page.reload()
    await expect(d.prompt).toHaveValue("Ready to type now and more words")
    await expect(d.slug).toHaveValue("alpha")
    await expect(page).toHaveURL(new RegExp(`#/drafts/${third}$`))
    expect(await draftIds()).toEqual([third])
    await expect(row("alpha")).toHaveCount(1)
    // The server holds the draft, and it is not an epic.
    expect(await fixture.cli("designs", "drafts", "show", third)).toContain("and more words")
    expect(await fixture.cli("server", "epics")).not.toContain("alpha")

    // Two contexts edit one draft: the conflict banner offers Load theirs and Keep mine, and there is no "gone" banner.
    await page2.goto(page.url())
    await expect(d2.prompt).toHaveValue("Ready to type now and more words")
    await d2.prompt.fill("Theirs")
    await expect(page2.getByText("Saved")).toBeVisible()
    await d.prompt.fill("Mine")
    const banner = page.getByRole("alert").filter({ hasText: "changed on another device" })
    await expect(banner).toBeVisible()
    await banner.getByRole("button", { name: "Load theirs" }).click()
    await expect(banner).toBeHidden()
    await expect(d.prompt).toHaveValue("Theirs")
    await d2.prompt.fill("Theirs again")
    await expect(page2.getByText("Saved")).toBeVisible()
    await d.prompt.fill("Mine again")
    await expect(banner).toBeVisible()
    await banner.getByRole("button", { name: "Keep mine" }).click()
    await expect(banner).toBeHidden()
    await expect(d.prompt).toHaveValue("Mine again")
    await expect.poll(() => fixture.cli("designs", "drafts", "show", third)).toContain("Mine again")
    await expect(
      page.getByText(/no longer exists|Keep as new draft|deleted or started/),
    ).toHaveCount(0)

    // The second context's sidebar follows a slug change at once.
    await renameDraft(page, "alpha-two")
    await expect(side2.row("alpha-two")).toBeVisible()
    // A Rename from the row menu in the second context shows in the first page with no banner, and a later keystroke keeps the new slug.
    await side2.rowMenu("alpha-two").click()
    await page2.getByRole("menuitem", nav("Rename")).click()
    const renameDialog = page2.getByRole("dialog", nav("Rename draft"))
    await renameDialog.getByRole("textbox", nav("Slug")).fill("gamma")
    await renameDialog.getByRole("button", nav("Rename")).click()
    await expect(d.slug).toHaveValue("gamma")
    await expect(banner).toHaveCount(0)
    await d.prompt.fill("Mine again, more")
    await expect(page.getByText("Saved")).toBeVisible()
    await expect.poll(() => fixture.cli("designs", "drafts", "list")).toContain("gamma")
    await expect(d.slug).toHaveValue("gamma")

    // Images: a pasted PNG and an Add image file each get a thumbnail, and the line counts them.
    const open = page.getByRole("button", { name: "Open image" })
    const uploads = await steerUploads(page)
    await d.prompt.evaluate((el, b64) => {
      const data = new DataTransfer()
      data.items.add(
        new File([Uint8Array.from(atob(b64), (c) => c.charCodeAt(0))], "pasted.png", {
          type: "image/png",
        }),
      )
      el.dispatchEvent(
        new ClipboardEvent("paste", { clipboardData: data, bubbles: true, cancelable: true }),
      )
    }, PNG.toString("base64"))
    await expect(open).toHaveCount(1)
    await page.getByTestId("image-input").setInputFiles([png(1)])
    await expect(open).toHaveCount(2)
    await expect(page.getByText("2 of 10 images")).toBeVisible()

    // Start stays off with each reason in turn.
    await d.slug.fill("")
    await expect(d.start).toBeDisabled()
    await expect(page.getByText("Name the epic to start.")).toBeVisible()
    await d.slug.fill("ready-epic")
    await expect(d.start).toBeEnabled()

    // A held upload shows the spinner and "1 uploading"; a failed one shows the red tile.
    uploads.set("hold")
    await page.getByTestId("image-input").setInputFiles([png(2)])
    await expect(page.getByLabel("Uploading")).toBeVisible()
    await expect(page.getByText("1 uploading")).toBeVisible()
    await expect(d.start).toBeDisabled()
    await expect(page.getByText("Wait for the upload to finish")).toBeVisible()
    await uploads.release("fail")
    const failed = page.locator('[data-state="failed"]')
    await expect(failed).toBeVisible()
    await expect(page.getByText("Remove the failed image first")).toBeVisible()
    await expect(d.start).toBeDisabled()
    await failed.getByRole("button", nav("Remove image")).click()
    await expect(failed).toHaveCount(0)
    await expect(d.start).toBeEnabled()
    uploads.set("pass")

    // A thumbnail click opens the viewer; closing it returns to the page with no new epic and no Start request.
    const starts: string[] = []
    page.on("request", (r) => r.url().includes("/v1/designs/open") && starts.push(r.url()))
    const epicsBefore = await fixture.cli("server", "epics")
    await open.first().click()
    await page.getByRole("button", { name: /^Close ✕/ }).click()
    await expect(page.getByRole("button", { name: /^Close ✕/ })).toBeHidden()
    await expect(d.prompt).toBeVisible()
    expect(await fixture.cli("server", "epics")).toBe(epicsBefore)
    expect(starts).toEqual([])

    // An existing epic is refused: the draft and its images stay.
    await expect(page.getByText("Saved")).toBeVisible()
    await d.start.click()
    await expect(toast.last()).toContainText("already has an epic")
    await expect(open).toHaveCount(2)
    await expect(d.slug).toHaveValue("ready-epic")
    expect(await draftIds()).toEqual([third])
    await expect.poll(() => exists(fixture.draftDir(third))).toBe(true)
    await expect(page).toHaveURL(new RegExp(`#/drafts/${third}$`))
    // The refusal toast would intercept the next click while it is up.
    await expect(async () => {
      for (const closer of await toast.locator("[data-part='close-trigger']").all())
        await closer.click({ timeout: 1000 })
      await expect(toast).toHaveCount(0, { timeout: 1000 })
    }).toPass()

    // A good Start moves the files, opens the Design page and removes the draft row.
    const before = (await fixture.launches()).length
    await d.slug.fill("pictured")
    // The history entry is read from the saved draft, so the slug must be saved before Start.
    await expect.poll(() => fixture.cli("designs", "drafts", "list")).toContain("pictured")
    await d.start.click()
    await expect(page).toHaveURL(/#\/d\/demo:pictured$/)
    await expect(page2).toHaveURL(/#\/d\/demo:pictured$/)
    await expect(row("pictured")).toHaveCount(1)
    await expect(sidebar.getByRole("link", { name: /^Design draft/ })).toHaveCount(0)
    await expect(side2.sidebar.getByRole("link", { name: /^Design draft/ })).toHaveCount(0)
    const shown = await fixture.cli("tickets", "show", "demo:pictured")
    expect(shown).toContain("kind: map")
    expect(shown).toContain("design_step: chart")
    expect(shown).not.toContain("Destination")
    const lines = shown.slice(shown.indexOf("## Initial prompt")).trimEnd().split("\n")
    const after = lines.slice(lines.lastIndexOf("Images:") + 1)
    const listed = after.slice(0, after.includes("") ? after.indexOf("") : undefined)
    expect(lines.lastIndexOf("Images:")).toBeGreaterThan(0)
    expect(listed).toHaveLength(2)
    for (const path of listed) {
      // `gx tickets root` needs a git repo, which the fixture's cwd is not; the store is under the fixture's data dir.
      expect(path).toMatch(/^\/.*\/gx\/tickets\/demo\/pictured\/images\/[^/]+$/)
      await expect(access(path)).resolves.toBeUndefined()
    }
    await expect.poll(() => exists(fixture.draftDir(third))).toBe(false)
    expect(await draftIds()).toEqual([])
    expect(await fixture.cli("server", "epics")).toContain("pictured")
    await expect.poll(async () => (await fixture.launches()).length).toBe(before + 1)
    const launched = (await fixture.launches()).at(-1)!
    // The first prompt is the bare skill command; the image paths live in the map ticket, not the prompt.
    expect(launched).not.toContain("Images:")
    await expect
      .poll(async () => (await sessionRows(fixture, "pictured")).length)
      .toBeGreaterThan(0)
    const conv = (
      await chatOf(fixture, (await sessionRows(fixture, "pictured"))[0]!.split("\t")[0]!)
    ).conversation_id
    await expect.poll(() => fixture.input(conv)).toContain("/gx-design demo:pictured")
    expect(await fixture.input(conv)).not.toContain("Images:")

    // The old draft URL lands on the Design page; an unknown id lands on the Epics page with the note.
    await page.evaluate((id) => (window.location.hash = `#/drafts/${id}`), third)
    await expect(page).toHaveURL(/#\/d\/demo:pictured$/)
    await page.evaluate(() => (window.location.hash = "#/drafts/no-such-draft"))
    await expect(page).toHaveURL(/#\/epics$/)
    await expect(page.getByText("This draft was removed.")).toBeVisible()

    // Remove on a draft with a prompt and images asks first; Keep leaves it, Remove deletes it and its image folder.
    await newDesign(page)
    // The Start saved one entry with its text and slug: the newest History row on a new draft page.
    const popup = history(page, "Design drafts")
    await popup.open()
    await expect(popup.rows).toHaveCount(1)
    await expect(popup.rows.first()).toContainText("Mine again, more")
    await expect(popup.rows.first()).toContainText("pictured")
    await popup.close()
    await d.prompt.fill("To be removed")
    await page.getByTestId("image-input").setInputFiles([png(3)])
    await expect(open).toHaveCount(1)
    const fourth = idOf(page)
    await expect.poll(() => exists(fixture.draftDir(fourth))).toBe(true)
    await expect(page.getByText("Saved")).toBeVisible()
    await page2.goto(page.url())
    await expect(d2.prompt).toHaveValue("To be removed")
    await d.remove.click()
    await expect(page.getByText("Remove this draft and its images?")).toBeVisible()
    await page.getByRole("button", nav("Keep")).click()
    await expect(page.getByText("Remove this draft and its images?")).toHaveCount(0)
    expect(await draftIds()).toEqual([fourth])
    await d.remove.click()
    await page.getByRole("button", nav("Remove")).click()
    await expect(page).toHaveURL(/#\/epics$/)
    await expect.poll(draftIds).toEqual([])
    await expect.poll(() => exists(fixture.draftDir(fourth))).toBe(false)
    // The page open in the second context leaves for the Epics page with the note.
    await expect(page2).toHaveURL(/#\/epics$/)
    await expect(page2.getByText("This draft was removed.")).toBeVisible()

    // Remove on an empty draft acts with no confirmation.
    await newDesign(page)
    await expect(d.prompt).toBeFocused()
    await expect.poll(draftIds).toHaveLength(1)
    // The footer Remove above added a second row, newest first.
    await popup.open()
    await expect(popup.rows).toHaveCount(2)
    await expect(popup.rows.first()).toContainText("To be removed")
    await popup.close()
    await d.remove.click()
    await expect(page).toHaveURL(/#\/epics$/)
    await expect.poll(draftIds).toEqual([])
    await expect(page.getByText("Remove this draft and its images?")).toHaveCount(0)

    // Remove from the sidebar row menu and with Leader then x each add a row with their own text.
    await newDesign(page)
    await d.prompt.fill("Removed from the row menu")
    await expect(page.getByText("Saved")).toBeVisible()
    await expect(row("Removed from the row menu")).toBeVisible()
    await rowMenu("Removed from the row menu").click()
    await page.getByRole("menuitem", nav("Remove")).click()
    await page
      .getByRole("dialog", { name: /^Remove / })
      .getByRole("button", nav("Remove"))
      .click()
    await expect(row("Removed from the row menu")).toHaveCount(0)
    await expect.poll(draftIds).toEqual([])
    await newDesign(page)
    await d.prompt.fill("Removed with Leader x")
    await expect(page.getByText("Saved")).toBeVisible()
    await page.keyboard.press("ControlOrMeta+e")
    await page.keyboard.press("x")
    const confirm = page.getByText("Remove this draft and its images?")
    await expect(confirm).toBeVisible()
    await confirm.locator("xpath=..").getByRole("button", nav("Remove")).click()
    await expect(page).toHaveURL(/#\/epics$/)
    await expect.poll(draftIds).toEqual([])
    await newDesign(page)
    await popup.open()
    await expect(popup.rows).toHaveCount(4)
    await expect(popup.rows.nth(0)).toContainText("Removed with Leader x")
    await expect(popup.rows.nth(1)).toContainText("Removed from the row menu")
    await popup.close()
    await d.remove.click()
    await expect(page).toHaveURL(/#\/epics$/)
  } finally {
    await other.close()
  }

  // The Design chat shows the strip read-only: no Remove, Add or upload states; a click opens the viewer.
  await page.evaluate(() => (window.location.hash = "#/d/demo:pictured"))
  const strip = page.getByLabel("Images", { exact: true })
  const open = page.getByRole("button", { name: "Open image" })
  // The Questions view (the first view with a round) shows no strip; the Chat view does.
  await expect(strip).toHaveCount(0)
  await page.getByRole("button", { name: "Chat", exact: true }).click()
  await expect(open.first()).toBeVisible()
  await expect(strip.getByRole("button", nav("Remove image"))).toHaveCount(0)
  await expect(strip.getByRole("button", nav("Add image"))).toHaveCount(0)
  await expect(strip.getByLabel("Uploading")).toHaveCount(0)
  // The strip is the first entry of the log, and (checked after the expect above) has no scrollbar beside it.
  expect(
    await strip.evaluate(
      (el) => el.parentElement!.querySelector(":scope > :not([aria-hidden])") === el,
    ),
  ).toBe(true)
  // A scrollbar takes layout space, so the box is bigger than its client area.
  expect(
    await strip.evaluate((el) => {
      const box = el as HTMLElement
      return box.offsetWidth === box.clientWidth && box.offsetHeight === box.clientHeight
    }),
  ).toBe(true)
  await open.first().click()
  await page.getByRole("button", { name: /^Close ✕/ }).click()
  await expect(page.getByRole("button", { name: /^Close ✕/ })).toBeHidden()
})

test("94. New Design launches at the strongest tier", async ({ page, start }) => {
  const fixture = await start("design-model-tiers")
  const before = (await fixture.launches()).length
  await startDesign(page, "tier-fresh")

  await expect.poll(async () => (await fixture.launches()).length).toBe(before + 1)
  const args = (await fixture.launches()).at(-1)!
  expect(flag(args, "model")).toBe("opus")
  expect(flag(args, "effort")).toBe("max")
})

test("27. Design a not-started Design", async ({ page, start }) => {
  const fixture = await start("united")
  const ready = await section(page, "Ready to design")
  await ready
    .locator("[data-kind]")
    .filter({ has: page.getByRole("link", nav("design-new")) })
    .getByRole("button", nav("Design"))
    .click()

  await expect(page).toHaveURL(/#\/d\/demo:design-new$/)
  await expect.poll(() => fixture.cli("agents", "list")).toContain("design-new")
})
