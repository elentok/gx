import type { Locator, Page } from "@playwright/test";
import { card, expect, flag, nav, section, test } from "./fixture";

test("25. New Design from an empty page", async ({ page, start }) => {
  await start("empty");
  await expect(page.getByText("No epics yet.")).toBeVisible();
  await expect(page.getByRole("button", nav("New Design"))).toBeVisible();
});

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

/** A second tab of the same browser context, on the Epics page. */
async function secondPage(page: Page) {
  const other = await page.context().newPage();
  await other.goto(new URL(page.url()).origin + "/#/epics");
  return other;
}

test("26. New Design form", async ({ page, start }) => {
  const fixture = await start("ready");

  // The button sits right of the filter and opens the form in a dialog.
  const filter = await page.getByRole("searchbox", nav("Filter epics")).boundingBox();
  const at = await page.getByRole("button", nav("New Design")).boundingBox();
  expect(at!.x).toBeGreaterThanOrEqual(filter!.x + filter!.width);
  const dialog = await openDialog(page);
  const { slug, idea, project } = fields(dialog);

  // One project: no pill. The idea box fills more than half the dialog.
  await expect(project).toHaveCount(0);
  await expect(slug).toHaveAttribute("placeholder", "my-epic");
  await expect(idea).toHaveAttribute("placeholder", "What is the problem? Who hits it? What does done look like?");
  const [box, sheet] = [await idea.boundingBox(), await dialog.boundingBox()];
  expect(box!.height).toBeGreaterThan(sheet!.height / 2);

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

test("34. New Design text is 16px", async ({ page, start }) => {
  await start("ready");
  const { slug, idea } = fields(await openDialog(page));
  for (const field of [slug, idea]) {
    await expect(field).toHaveCSS("font-size", "16px");
  }
});

test("35. New Design project select and its starting project", async ({ page, start }) => {
  await start("united");

  // The select lists both projects; with "All projects" and an empty draft it picks the first.
  const first = fields(await openDialog(page)).project;
  await expect(first.locator("option")).toHaveText(["demo", "auto"]);
  await expect(first).toHaveValue("demo");
  await page.keyboard.press("Escape");

  // The Epics filter picks the starting project.
  await page.getByRole("combobox", nav("Project")).selectOption("auto");
  await expect(fields(await openDialog(page)).project).toHaveValue("auto");
});

test("36. New Design draft keeps its project", async ({ page, start }) => {
  await start("united");
  await page.getByRole("combobox", nav("Project")).selectOption("demo");
  const dialog = await openDialog(page);
  await fields(dialog).project.selectOption("auto");
  await fields(dialog).slug.fill("kept-draft");
  await page.keyboard.press("Escape");

  // The filter says demo, but a draft with text wins.
  await expect(fields(await openDialog(page)).project).toHaveValue("auto");
});

test("37. New Design draft is shared and closing keeps it", async ({ page, start }) => {
  await start("ready");
  const dialog = await openDialog(page);
  const { slug, idea } = fields(dialog);
  await slug.fill("shared-draft");
  await idea.fill("Half an idea.");

  // The draft reaches a second page of the same context, which does not open the modal by itself.
  const other = await secondPage(page);
  await expect(other.getByRole("button", nav("New Design"))).toBeVisible();
  await expect(other.getByRole("dialog", nav("New Design"))).toHaveCount(0);
  const again = await openDialog(other);
  await expect(fields(again).slug).toHaveValue("shared-draft");
  await expect(fields(again).idea).toHaveValue("Half an idea.");
  await other.close();

  // ×, Escape and a click on the backdrop each close the modal and keep the draft.
  const closers: Array<() => Promise<void>> = [
    () => dialog.getByRole("button", nav("Close")).click(),
    () => page.keyboard.press("Escape"),
    () => page.mouse.click(2, 2),
  ];
  for (const close of closers) {
    await close();
    await expect(dialog).toHaveCount(0);
    const reopened = await openDialog(page);
    await expect(fields(reopened).slug).toHaveValue("shared-draft");
    await expect(fields(reopened).idea).toHaveValue("Half an idea.");
  }
});

test("38. New Design Discard", async ({ page, start }) => {
  await start("ready");
  const dialog = await openDialog(page);

  // Empty: Discard closes at once.
  await dialog.getByRole("button", nav("Discard")).click();
  await expect(dialog).toHaveCount(0);

  // With text: it asks first, and Keep returns to the text.
  const again = await openDialog(page);
  await fields(again).slug.fill("doomed");
  await fields(again).idea.fill("Never mind.");
  await again.getByRole("button", nav("Discard")).click();
  await expect(again.getByText("Discard this draft?")).toBeVisible();
  await again.getByRole("button", nav("Keep")).click();
  await expect(again.getByText("Discard this draft?")).toHaveCount(0);
  await expect(fields(again).idea).toHaveValue("Never mind.");

  // Confirmed: the dialog closes and the next one is empty.
  await again.getByRole("button", nav("Discard")).click();
  await again.getByRole("button", nav("Discard")).click();
  await expect(again).toHaveCount(0);
  const fresh = fields(await openDialog(page));
  await expect(fresh.slug).toHaveValue("");
  await expect(fresh.idea).toHaveValue("");
});

test("39. New Design refused for an existing epic", async ({ page, start }) => {
  await start("ready");
  const dialog = await openDialog(page);
  await fields(dialog).slug.fill("ready-epic");
  await fields(dialog).idea.fill("Same name.");
  await dialog.getByRole("button", nav("Start design")).click();

  await expect(page.locator("[data-scope='toast'][data-part='root']").last()).toContainText("already has an epic");
  await expect(fields(dialog).slug).toHaveValue("ready-epic");
  await expect(fields(dialog).idea).toHaveValue("Same name.");
  await expect(page).toHaveURL(/#\/epics$/);
});

test("40. gx designs open starts a New Design", async ({ start }) => {
  const fixture = await start("ready");
  const open = () => fixture.cli("designs", "open", "cli-design", "--idea", "x", "--project", "demo", "--json");
  await open();
  const shown = await fixture.cli("tickets", "show", "demo:cli-design");
  expect(shown).toContain("kind: map");

  // With --json a refusal is printed as JSON; the exit code may or may not be non-zero.
  const refused = await open().catch((err: { stdout?: string; stderr?: string }) => `${err.stdout}${err.stderr}`);
  expect(refused).toContain("epic-exists");
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
