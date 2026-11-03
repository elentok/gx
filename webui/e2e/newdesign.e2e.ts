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
  const fixture = await start("ready");

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
