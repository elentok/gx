import type { Locator, Page } from "@playwright/test";
import { card, expect, flag, nav, section, test } from "./fixture";

const openDialog = async (page: Page) => {
  await page.getByRole("button", nav("New Design")).click();
  const dialog = page.getByRole("dialog", nav("New Design"));
  await expect(dialog).toBeVisible();
  return dialog;
};

const fields = (dialog: Locator) => ({
  slug: dialog.getByRole("textbox", nav("Epic slug")),
  idea: dialog.getByRole("textbox", nav("Rough idea")),
  project: dialog.getByRole("combobox", nav("Project")),
});

test("26. New Design form", async ({ page, start }) => {
  const fixture = await start("ready", { reloads: 2 });

  // The button sits right of the filter and opens the form in a dialog.
  const filter = await page.getByRole("searchbox", nav("Filter epics")).boundingBox();
  const at = await page.getByRole("button", nav("New Design")).boundingBox();
  expect(at!.x).toBeGreaterThanOrEqual(filter!.x + filter!.width);
  const dialog = await openDialog(page);
  const { slug, idea, project } = fields(dialog);

  // The select lists every project, even one with no epics ("scratch"); the one-project rule is in NewDesignForm.test.
  // The idea box fills more than half the dialog.
  await expect(project.locator("option")).toHaveText(["demo", "scratch"]);
  await expect(slug).toHaveAttribute("placeholder", "my-epic");
  await expect(idea).toHaveAttribute("placeholder", "What is the problem? Who hits it? What does done look like?");
  const [box, sheet] = [await idea.boundingBox(), await dialog.boundingBox()];
  expect(box!.height).toBeGreaterThan(sheet!.height / 2);

  // An existing epic is refused by the server, and the dialog keeps what was typed.
  await slug.fill("ready-epic");
  await idea.fill("Same name.");
  await dialog.getByRole("button", nav("Start design")).click();
  await expect(page.locator("[data-scope='toast'][data-part='root']").last()).toContainText("already has an epic");
  await expect(slug).toHaveValue("ready-epic");
  await expect(idea).toHaveValue("Same name.");
  await expect(page).toHaveURL(/#\/epics$/);

  await slug.fill("fresh-design");
  await idea.fill("A rough idea.");
  await dialog.getByRole("button", nav("Start design")).click();

  await expect(page).toHaveURL(/#\/d\/demo:fresh-design$/);
  await expect.poll(() => fixture.cli("server", "epics")).toContain("fresh-design");
  const shown = await fixture.cli("tickets", "show", "demo:fresh-design");
  expect(shown).toContain("kind: map");
  expect(shown).toContain("design_step: chart");
  expect(shown).toContain("## Initial prompt");
  expect(shown).not.toContain("Destination");

  // A reopened modal is empty. The card of the Designing epic opens its epic page.
  await page.getByRole("link", { name: /^Epics/ }).click();
  const reopened = fields(await openDialog(page));
  await expect(reopened.slug).toHaveValue("");
  await expect(reopened.idea).toHaveValue("");
  await page.keyboard.press("Escape");
  await card(page, "fresh-design").getByRole("link", nav("fresh-design")).click();
  await expect(page).toHaveURL(/#\/e\/demo:fresh-design$/);

  // Autosave: typed text shows "Saved", survives a reload, and a closed form reopens on the same draft.
  await page.getByRole("link", { name: /^Epics/ }).click();
  const first = await openDialog(page);
  await fields(first).slug.fill("alpha");
  await fields(first).idea.fill("First idea");
  await expect(first.getByText("Saved")).toBeVisible();
  await page.reload();
  const again = await openDialog(page);
  await expect(fields(again).slug).toHaveValue("alpha");
  await expect(fields(again).idea).toHaveValue("First idea");
  await page.keyboard.press("Escape");
  await expect(again).toBeHidden();
  const reopenedDraft = fields(await openDialog(page));
  await expect(reopenedDraft.slug).toHaveValue("alpha");

  // The server holds the draft, and it is not an epic.
  await expect.poll(() => fixture.cli("designs", "drafts", "list")).toContain("alpha");
  const id = (await fixture.cli("designs", "drafts", "list")).split("\t")[0]!;
  expect(await fixture.cli("designs", "drafts", "show", id)).toContain("First idea");
  expect(await fixture.cli("server", "epics")).not.toContain("alpha");

  // A second browser context on the same server sees the draft.
  const other = await page.context().browser()!.newContext({ ignoreHTTPSErrors: true, storageState: await page.context().storageState() });
  try {
    const page2 = await other.newPage();
    await page2.goto(page.url());
    const theirs = await openDialog(page2);
    await expect(fields(theirs).slug).toHaveValue("alpha");

    // Conflict: they save first, then this device's save is refused. Load theirs takes their text; Keep mine keeps ours.
    await fields(theirs).idea.fill("Theirs");
    await expect(theirs.getByText("Saved")).toBeVisible();
    await fields(page.getByRole("dialog", nav("New Design"))).idea.fill("Mine");
    const banner = page.getByRole("dialog", nav("New Design")).getByRole("alert");
    await expect(banner).toContainText("This draft changed on another device.");
    await banner.getByRole("button", { name: "Load theirs" }).click();
    await expect(banner).toBeHidden();
    await expect(fields(page.getByRole("dialog", nav("New Design"))).idea).toHaveValue("Theirs");

    await fields(theirs).idea.fill("Theirs again");
    await expect(theirs.getByText("Saved")).toBeVisible();
    await fields(page.getByRole("dialog", nav("New Design"))).idea.fill("Mine again");
    await expect(banner).toContainText("This draft changed on another device.");
    await banner.getByRole("button", { name: "Keep mine" }).click();
    await expect(banner).toBeHidden();
    await expect(fields(page.getByRole("dialog", nav("New Design"))).idea).toHaveValue("Mine again");
    await expect.poll(() => fixture.cli("designs", "drafts", "show", id)).toContain("Mine again");
    await page2.keyboard.press("Escape");

    // A second draft: New draft is blank and titled "New draft"; the menu lists a row per draft.
    const form = page.getByRole("dialog", nav("New Design"));
    const opener = form.locator("button[aria-haspopup='menu']");
    const menu = form.getByRole("menu", nav("Drafts"));
    await opener.click();
    await menu.getByRole("menuitem", { name: /New draft/ }).click();
    await expect(opener).toHaveText(/^New draft/);
    await expect(fields(form).slug).toHaveValue("");
    await fields(form).slug.fill("beta");
    await fields(form).idea.fill("Second idea");
    await expect(form.getByText("Saved")).toBeVisible();
    await opener.click();
    const alphaRow = menu.getByRole("menuitem", { name: /alpha/ });
    await expect(alphaRow).toContainText("demo");
    await expect(alphaRow).toContainText(/edited/);
    await expect(menu.getByRole("menuitem", { name: /beta/ })).toContainText("editing now");

    // Switching shows each draft's own text.
    await alphaRow.click();
    await expect(fields(form).idea).toHaveValue("Mine again");
    await opener.click();
    await menu.getByRole("menuitem", { name: /beta/ }).click();
    await expect(fields(form).idea).toHaveValue("Second idea");

    // Name this draft: the title reads the name.
    await opener.click();
    await menu.getByRole("menuitem", { name: /Name this draft/ }).click();
    await form.getByRole("textbox", nav("Draft name")).fill("Launch plan");
    await form.getByRole("button", nav("Done")).click();
    await expect(opener).toHaveText(/^Launch plan/);
    await expect(form.getByText("Saved")).toBeVisible();

    // Closing and reopening shows the most recently edited draft.
    await page.keyboard.press("Escape");
    await expect(form).toBeHidden();
    await openDialog(page);
    await expect(opener).toHaveText(/^Launch plan/);

    // The trash on a row asks first, then removes the draft.
    await opener.click();
    await menu.locator("> div").filter({ hasText: "alpha" }).getByRole("button", nav("Delete draft")).click();
    await expect(menu).toContainText("Discard this draft and its images?");
    await menu.getByRole("button", nav("Discard")).click();
    await expect(menu.getByRole("menuitem", { name: /alpha/ })).toBeHidden();
    await expect.poll(() => fixture.cli("designs", "drafts", "list")).not.toContain("alpha");
    await fields(form).slug.click();

    // Discard with text asks, then removes the open draft.
    await expect(menu).toBeHidden();
    await form.getByRole("button", nav("Discard")).click();
    await expect(form.getByText("Discard this draft and its images?")).toBeVisible();
    await form.getByRole("button", nav("Discard")).click();
    await expect(form).toBeHidden();
    await expect.poll(() => fixture.cli("designs", "drafts", "list")).not.toContain("beta");

    // The form starts on the last project picked, here and on the other device.
    const blank = await openDialog(page);
    await fields(blank).project.selectOption("scratch");
    await page.keyboard.press("Escape");
    await expect(blank).toBeHidden();
    await page.reload();
    await expect(fields(await openDialog(page)).project).toHaveValue("scratch");
    await page2.reload();
    await expect(fields(await openDialog(page2)).project).toHaveValue("scratch");
  } finally {
    await other.close();
  }
});

test("94. New Design launches at the strongest tier", async ({ page, start }) => {
  const fixture = await start("design-model-tiers");
  const before = (await fixture.launches()).length;
  const dialog = await openDialog(page);
  await fields(dialog).slug.fill("tier-fresh");
  await fields(dialog).idea.fill("A rough idea.");
  await dialog.getByRole("button", nav("Start design")).click();
  await expect(page).toHaveURL(/#\/d\/demo:tier-fresh$/);

  await expect.poll(async () => (await fixture.launches()).length).toBe(before + 1);
  const args = (await fixture.launches()).at(-1)!;
  expect(flag(args, "model")).toBe("opus");
  expect(flag(args, "effort")).toBe("max");
});

test("27. Design a not-started Design", async ({ page, start }) => {
  const fixture = await start("united");
  const ready = await section(page, "Ready to design");
  await ready.locator("[data-kind]").filter({ has: page.getByRole("link", nav("design-new")) }).getByRole("button", nav("Design")).click();

  await expect(page).toHaveURL(/#\/d\/demo:design-new$/);
  await expect.poll(() => fixture.cli("agents", "list")).toContain("design-new");
});
