import { access } from "node:fs/promises"
import type { Locator, Page } from "@playwright/test"
import { chatOf, PNG, png, steerUploads } from "./agentsKit"
import { card, expect, flag, nav, openEpics, section, sessionRows, test } from "./fixture"

const openDialog = async (page: Page) => {
  await page.getByRole("button", nav("New Design")).click()
  const dialog = page.getByRole("dialog", nav("New Design"))
  await expect(dialog).toBeVisible()
  return dialog
}

const addInput = (dialog: Locator) => dialog.getByTestId("image-input")

const fields = (dialog: Locator) => ({
  slug: dialog.getByRole("textbox", nav("Epic slug")),
  idea: dialog.getByRole("textbox", nav("Rough idea")),
  project: dialog.getByRole("combobox", nav("Project")),
})

test("26. New Design form", async ({ page, start }) => {
  const fixture = await start("ready", { reloads: 2 })

  // The button sits right of the filter and opens the form in a dialog.
  const filter = await page.getByRole("searchbox", nav("Filter epics")).boundingBox()
  const at = await page.getByRole("button", nav("New Design")).boundingBox()
  expect(at!.x).toBeGreaterThanOrEqual(filter!.x + filter!.width)
  await page.setViewportSize({ width: 1280, height: 800 })
  const dialog = await openDialog(page)
  const { slug, idea, project } = fields(dialog)
  // The Epics page button lands focus in Epic slug.
  await expect(slug).toBeFocused()

  // Close is a text button at the right end of the top bar, and there is no "×".
  const closeBtn = dialog.getByRole("button", nav("Close"))
  await expect(closeBtn).toHaveText("Close")
  await expect(dialog).not.toContainText("×")
  const [dlg, closeAt] = [(await dialog.boundingBox())!, (await closeBtn.boundingBox())!]
  expect(Math.abs(closeAt.x + closeAt.width - (dlg.x + dlg.width - 12))).toBeLessThanOrEqual(2)

  // The select lists every project, even one with no epics ("scratch"); the one-project rule is in NewDesignForm.test.
  // The idea box takes the space left, over a third of the dialog beside the header and the image row.
  await expect(project.locator("option")).toHaveText(["demo", "scratch"])
  await expect(slug).toHaveAttribute("placeholder", "my-epic")
  await expect(idea).toHaveAttribute(
    "placeholder",
    "What is the problem? Who hits it? What does done look like?",
  )
  const [box, sheet] = [await idea.boundingBox(), await dialog.boundingBox()]
  expect(box!.height).toBeGreaterThan(sheet!.height / 3)

  // An existing epic is refused by the server, and the dialog keeps what was typed.
  await slug.fill("ready-epic")
  await idea.fill("Same name.")
  // The footer is Discard at its left edge and Start design at its right edge (flow 41 checks the phone).
  const footer = dialog.getByRole("button", nav("Discard")).locator("xpath=..")
  const [foot, d, s] = [
    (await footer.boundingBox())!,
    (await dialog.getByRole("button", nav("Discard")).boundingBox())!,
    (await dialog.getByRole("button", nav("Start design")).boundingBox())!,
  ]
  expect(d.x).toBeCloseTo(foot.x + 12, 0)
  expect(s.x + s.width).toBeCloseTo(foot.x + foot.width - 12, 0)
  await dialog.getByRole("button", nav("Start design")).click()
  await expect(page.locator("[data-scope='toast'][data-part='root']").last()).toContainText(
    "already has an epic",
  )
  await expect(slug).toHaveValue("ready-epic")
  await expect(idea).toHaveValue("Same name.")
  await expect(page).toHaveURL(/#\/epics$/)

  await slug.fill("fresh-design")
  await idea.fill("A rough idea.")
  await dialog.getByRole("button", nav("Start design")).click()

  await expect(page).toHaveURL(/#\/d\/demo:fresh-design$/)
  await expect.poll(() => fixture.cli("server", "epics")).toContain("fresh-design")
  const shown = await fixture.cli("tickets", "show", "demo:fresh-design")
  expect(shown).toContain("kind: map")
  expect(shown).toContain("design_step: chart")
  expect(shown).toContain("## Initial prompt")
  expect(shown).not.toContain("Destination")

  // A reopened modal is empty. The card of the Designing epic opens its epic page.
  await openEpics(page)
  const reopened = fields(await openDialog(page))
  await expect(reopened.slug).toHaveValue("")
  await expect(reopened.idea).toHaveValue("")
  await page.keyboard.press("Escape")
  await card(page, "fresh-design").getByRole("link", nav("fresh-design")).click()
  await expect(page).toHaveURL(/#\/e\/demo:fresh-design$/)

  // Autosave: typed text shows "Saved", survives a reload, and a closed form reopens on the same draft.
  await openEpics(page)
  const first = await openDialog(page)
  await fields(first).slug.fill("alpha")
  await fields(first).idea.fill("First idea")
  await expect(first.getByText("Saved")).toBeVisible()
  await page.reload()
  const again = await openDialog(page)
  await expect(fields(again).slug).toHaveValue("alpha")
  await expect(fields(again).idea).toHaveValue("First idea")
  await page.keyboard.press("Escape")
  await expect(again).toBeHidden()
  const reopenedDraft = fields(await openDialog(page))
  await expect(reopenedDraft.slug).toHaveValue("alpha")

  // The server holds the draft, and it is not an epic.
  await expect.poll(() => fixture.cli("designs", "drafts", "list")).toContain("alpha")
  const id = (await fixture.cli("designs", "drafts", "list")).split("\t")[0]!
  expect(await fixture.cli("designs", "drafts", "show", id)).toContain("First idea")
  expect(await fixture.cli("server", "epics")).not.toContain("alpha")

  // A second browser context on the same server sees the draft.
  const other = await page
    .context()
    .browser()!
    .newContext({ ignoreHTTPSErrors: true, storageState: await page.context().storageState() })
  try {
    const page2 = await other.newPage()
    await page2.goto(page.url())
    const theirs = await openDialog(page2)
    await expect(fields(theirs).slug).toHaveValue("alpha")

    // Conflict: they save first, then this device's save is refused. Load theirs takes their text; Keep mine keeps ours.
    await fields(theirs).idea.fill("Theirs")
    await expect(theirs.getByText("Saved")).toBeVisible()
    await fields(page.getByRole("dialog", nav("New Design"))).idea.fill("Mine")
    const banner = page.getByRole("dialog", nav("New Design")).getByRole("alert")
    await expect(banner).toContainText("This draft changed on another device.")
    await banner.getByRole("button", { name: "Load theirs" }).click()
    await expect(banner).toBeHidden()
    await expect(fields(page.getByRole("dialog", nav("New Design"))).idea).toHaveValue("Theirs")

    await fields(theirs).idea.fill("Theirs again")
    await expect(theirs.getByText("Saved")).toBeVisible()
    await fields(page.getByRole("dialog", nav("New Design"))).idea.fill("Mine again")
    await expect(banner).toContainText("This draft changed on another device.")
    await banner.getByRole("button", { name: "Keep mine" }).click()
    await expect(banner).toBeHidden()
    await expect(fields(page.getByRole("dialog", nav("New Design"))).idea).toHaveValue("Mine again")
    await expect.poll(() => fixture.cli("designs", "drafts", "show", id)).toContain("Mine again")
    await page2.keyboard.press("Escape")

    // A second draft: New draft is blank and titled "New draft"; the menu lists a row per draft.
    const form = page.getByRole("dialog", nav("New Design"))
    const opener = form.locator("button[aria-haspopup='menu']")
    const menu = form.getByRole("menu", nav("Drafts"))
    await opener.click()
    await menu.getByRole("menuitem", { name: /New draft/ }).click()
    await expect(opener).toHaveText(/^New draft/)
    await expect(fields(form).slug).toHaveValue("")
    await fields(form).slug.fill("beta")
    await fields(form).idea.fill("Second idea")
    await expect(form.getByText("Saved")).toBeVisible()
    await opener.click()
    const alphaRow = menu.getByRole("menuitem", { name: /alpha/ })
    await expect(alphaRow).toContainText("demo")
    await expect(alphaRow).toContainText(/edited/)
    await expect(menu.getByRole("menuitem", { name: /beta/ })).toContainText("editing now")

    // Switching shows each draft's own text.
    await alphaRow.click()
    await expect(fields(form).idea).toHaveValue("Mine again")
    await opener.click()
    await menu.getByRole("menuitem", { name: /beta/ }).click()
    await expect(fields(form).idea).toHaveValue("Second idea")

    // Name this draft: the title reads the name.
    await opener.click()
    await menu.getByRole("menuitem", { name: /Name this draft/ }).click()
    await form.getByRole("textbox", nav("Draft name")).fill("Launch plan")
    await form.getByRole("button", nav("Done")).click()
    await expect(opener).toHaveText(/^Launch plan/)
    await expect(form.getByText("Saved")).toBeVisible()

    // Closing and reopening shows the most recently edited draft.
    await page.keyboard.press("Escape")
    await expect(form).toBeHidden()
    await openDialog(page)
    await expect(opener).toHaveText(/^Launch plan/)

    // The trash on a row asks first, then removes the draft.
    await opener.click()
    await menu
      .locator("> div")
      .filter({ hasText: "alpha" })
      .getByRole("button", nav("Delete draft"))
      .click()
    await expect(menu).toContainText("Discard this draft and its images?")
    await menu.getByRole("button", nav("Discard")).click()
    await expect(menu.getByRole("menuitem", { name: /alpha/ })).toBeHidden()
    await expect.poll(() => fixture.cli("designs", "drafts", "list")).not.toContain("alpha")
    await fields(form).slug.click()

    // Discard with text asks, then removes the open draft.
    await expect(menu).toBeHidden()
    await form.getByRole("button", nav("Discard")).click()
    await expect(form.getByText("Discard this draft and its images?")).toBeVisible()
    await form.getByRole("button", nav("Discard")).click()
    await expect(form).toBeHidden()
    await expect.poll(() => fixture.cli("designs", "drafts", "list")).not.toContain("beta")

    // The form starts on the last project picked, here and on the other device.
    const blank = await openDialog(page)
    await fields(blank).project.selectOption("scratch")
    await page.keyboard.press("Escape")
    await expect(blank).toBeHidden()
    await page.reload()
    await expect(fields(await openDialog(page)).project).toHaveValue("scratch")
    await page2.reload()
    await expect(fields(await openDialog(page2)).project).toHaveValue("scratch")
    await page2.keyboard.press("Escape")

    // Images: a pasted PNG and an Add image file each get a thumbnail, and the line counts them.
    const imgs = page.getByRole("dialog", nav("New Design"))
    const { slug: imgSlug, idea: imgIdea, project: imgProject } = fields(imgs)
    const open = (where: Locator) => where.getByRole("button", { name: "Open image" })
    const start = imgs.getByRole("button", nav("Start design"))
    const uploads = await steerUploads(page)
    await imgProject.selectOption("demo")
    // An image needs a saved draft, so the epic name comes first.
    await imgSlug.fill("ready-epic")
    await expect(imgs.getByText("Saved")).toBeVisible()
    await expect(imgIdea).toBeVisible()
    await imgIdea.evaluate((el, b64) => {
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
    await expect(open(imgs)).toHaveCount(1)
    await addInput(imgs).setInputFiles([png(1)])
    await expect(open(imgs)).toHaveCount(2)
    await expect(imgs.getByText("2 of 10 images")).toBeVisible()

    // Start stays off with the reason for no slug, then for no idea.
    await expect(start).toBeDisabled()
    await expect(imgs.getByText("Write an idea to start.")).toBeVisible()
    await imgIdea.fill("Same name.")
    await imgSlug.fill("")
    await expect(start).toBeDisabled()
    await expect(imgs.getByText("Name the epic to start.")).toBeVisible()
    await imgSlug.fill("ready-epic")
    await expect(start).toBeEnabled()

    // A held upload shows the spinner and "1 uploading"; a failed one shows the red tile.
    uploads.set("hold")
    await addInput(imgs).setInputFiles([png(2)])
    await expect(imgs.getByLabel("Uploading")).toBeVisible()
    await expect(imgs.getByText("1 uploading")).toBeVisible()
    await expect(start).toBeDisabled()
    await expect(imgs.getByText("Wait for the upload to finish")).toBeVisible()
    await uploads.release("fail")
    const failed = imgs.locator('[data-state="failed"]')
    await expect(failed).toBeVisible()
    await expect(imgs.getByText("Remove the failed image first")).toBeVisible()
    await expect(start).toBeDisabled()
    await failed.getByRole("button", nav("Remove image")).click()
    await expect(failed).toHaveCount(0)
    await expect(start).toBeEnabled()
    uploads.set("pass")

    // The other device shows the same ready thumbnails, and a click opens the viewer.
    await page2.reload()
    const theirImgs = await openDialog(page2)
    await expect(fields(theirImgs).slug).toHaveValue("ready-epic")
    await expect(open(theirImgs)).toHaveCount(2)
    await page2.keyboard.press("Escape")
    // Closing the viewer returns to the sheet: no new epic, and no Start request.
    const starts: string[] = []
    page.on("request", (r) => r.url().includes("/v1/designs/open") && starts.push(r.url()))
    const epicsBefore = await fixture.cli("server", "epics")
    await open(imgs).first().click()
    await page.getByRole("button", { name: /^Close ✕/ }).click()
    await expect(page.getByRole("button", { name: /^Close ✕/ })).toBeHidden()
    await expect(imgs).toBeVisible()
    expect(await fixture.cli("server", "epics")).toBe(epicsBefore)
    expect(starts).toEqual([])

    // Discard with text and images asks, then removes the draft and its folder.
    const draftId = (await fixture.cli("designs", "drafts", "list")).split("\t")[0]!
    await expect
      .poll(() =>
        access(fixture.draftDir(draftId)).then(
          () => true,
          () => false,
        ),
      )
      .toBe(true)
    await imgs.getByRole("button", nav("Discard")).click()
    await expect(imgs.getByText("Discard this draft and its images?")).toBeVisible()
    await imgs.getByRole("button", nav("Discard")).click()
    await expect(imgs).toBeHidden()
    await expect.poll(() => fixture.cli("designs", "drafts", "list")).not.toContain("ready-epic")
    await expect
      .poll(() =>
        access(fixture.draftDir(draftId)).then(
          () => true,
          () => false,
        ),
      )
      .toBe(false)

    // Start with images: an existing epic is refused and the draft keeps its images; a good Start moves the files.
    const form2 = await openDialog(page)
    await fields(form2).project.selectOption("demo")
    await fields(form2).slug.fill("ready-epic")
    await fields(form2).idea.fill("With pictures.")
    await addInput(form2).setInputFiles([png(3), png(4)])
    await expect(open(form2)).toHaveCount(2)
    const keptId = (await fixture.cli("designs", "drafts", "list")).split("\t")[0]!
    await form2.getByRole("button", nav("Start design")).click()
    await expect(page.locator("[data-scope='toast'][data-part='root']").last()).toContainText(
      "already has an epic",
    )
    await expect(open(form2)).toHaveCount(2)
    await expect(access(fixture.draftDir(keptId))).resolves.toBeUndefined()
    // The refusal toast would intercept the next click while it is up.
    const toast = page.locator("[data-scope='toast'][data-part='root']")
    await expect(async () => {
      for (const closer of await toast.locator("[data-part='close-trigger']").all())
        await closer.click({ timeout: 1000 })
      await expect(toast).toHaveCount(0, { timeout: 1000 })
    }).toPass()

    const before = (await fixture.launches()).length
    await fields(form2).slug.fill("pictured")
    await form2.getByRole("button", nav("Start design")).click()
    await expect(page).toHaveURL(/#\/d\/demo:pictured$/)
    const shown2 = await fixture.cli("tickets", "show", "demo:pictured")
    const lines = shown2.slice(shown2.indexOf("## Initial prompt")).trimEnd().split("\n")
    const after = lines.slice(lines.lastIndexOf("Images:") + 1)
    const listed = after.slice(0, after.includes("") ? after.indexOf("") : undefined)
    expect(lines.lastIndexOf("Images:")).toBeGreaterThan(0)
    expect(listed).toHaveLength(2)
    for (const path of listed) {
      // `gx tickets root` needs a git repo, which the fixture's cwd is not; the store is under the fixture's data dir.
      expect(path).toMatch(/^\/.*\/gx\/tickets\/demo\/pictured\/images\/[^/]+$/)
      await expect(access(path)).resolves.toBeUndefined()
    }
    await expect
      .poll(() =>
        access(fixture.draftDir(keptId)).then(
          () => true,
          () => false,
        ),
      )
      .toBe(false)
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

    // The Design chat shows the strip read-only: no Remove, Add or upload states; a click opens the viewer.
    const strip = page.getByLabel("Images", { exact: true })
    // The Questions view (the first view with a round) shows no strip; the Chat view does.
    await expect(strip).toHaveCount(0)
    await page.getByRole("button", { name: "Chat", exact: true }).click()
    await expect(open(strip)).toHaveCount(2)
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
    await open(strip).first().click()
    await page.getByRole("button", { name: /^Close ✕/ }).click()
    await expect(page.getByRole("button", { name: /^Close ✕/ })).toBeHidden()
  } finally {
    await other.close()
  }
})

test("94. New Design launches at the strongest tier", async ({ page, start }) => {
  const fixture = await start("design-model-tiers")
  const before = (await fixture.launches()).length
  const dialog = await openDialog(page)
  await fields(dialog).slug.fill("tier-fresh")
  await fields(dialog).idea.fill("A rough idea.")
  await dialog.getByRole("button", nav("Start design")).click()
  await expect(page).toHaveURL(/#\/d\/demo:tier-fresh$/)

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
