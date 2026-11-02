import { writeFile } from "node:fs/promises";
import { join } from "node:path";
import type { Page } from "@playwright/test";
import { expect, holdRoute, nav, test, touch } from "./fixture";
import { claudeIdle, codexIdle, commandMenu, commandNames, commandRows, go, messageBox, newForm, promptBox, sidebar, worktreeBox } from "./agentsKit";

/** Whether a command of that name is in the open menu. */
const hasName = (page: Parameters<typeof commandRows>[0], name: string) => commandNames(page).then((names) => names.includes(name));

test("149. Command list freshness and failure, refresh", async ({ page, start }) => {
  // The server serves a list from its cache for a second; the page keeps its own for a minute.
  const fixture = await start("agents-commands", { commandListAge: "1s" });
  const session = await claudeIdle(fixture);
  const other = await claudeIdle(fixture, 1);
  await page.setViewportSize({ width: 1280, height: 800 });
  await page.clock.install();
  await go(page, `#/agents/${session.id}`);
  const box = messageBox(page);
  await box.fill("/");
  await expect.poll(() => hasName(page, "/html-skill")).toBe(true);

  // Escaped text shows plain, and a skill only a person can run shows. The fake never lists a
  // user-invocable:false skill, so its absence has no row to check.
  await expect(commandRows(page).filter({ hasText: "Turn a <template> into a page" })).toHaveCount(1);
  expect(await hasName(page, "/deploy-notes")).toBe(true);

  // A skill added to the directory shows after the refresh; the cached rows show at once.
  await writeFile(
    join(session.dir, ".agentfake-commands.json"),
    JSON.stringify({ entries: [{ name: "html-skill", description: "Turn a <template> into a page" }, { name: "late-skill", description: "Added after the first read" }] }),
  );
  await go(page, `#/agents/${other.id}`);
  await new Promise((resolve) => setTimeout(resolve, 1500));
  await page.clock.fastForward(61_000);
  const release = await holdRoute(page, "**/v1/sessions/commands*");
  await go(page, `#/agents/${session.id}`);
  await box.fill("/");
  await expect.poll(() => hasName(page, "/deploy-notes")).toBe(true);
  expect(await hasName(page, "/late-skill")).toBe(false);
  release();
  await expect.poll(() => hasName(page, "/late-skill")).toBe(true);
  expect(await hasName(page, "/deploy-notes")).toBe(false);
});

test("150. Codex command menu", async ({ page, start }) => {
  const fixture = await start("agents-commands");
  const codex = await codexIdle(fixture);
  await page.setViewportSize({ width: 1280, height: 800 });
  await go(page, `#/agents/${codex.id}`);
  const box = messageBox(page);
  const menu = commandMenu(page);

  // `/` lists only built-ins, no skills.
  await box.fill("/");
  await expect(menu).toBeVisible();
  await expect.poll(() => hasName(page, "/status")).toBe(true);
  expect(await hasName(page, "/review-skill")).toBe(false);
  expect(await hasName(page, "/html-skill")).toBe(false);

  // `$` at the start lists skills, a doubled name once; filling writes `$name `.
  await box.fill("$review-s");
  await expect.poll(() => commandNames(page)).toEqual(["$review-skill"]);
  await box.press("Enter");
  await expect(box).toHaveValue("$review-skill ");

  // `$` in the second word lists them too and fills at that word.
  await box.fill("use $htm");
  await expect.poll(() => commandNames(page)).toEqual(["$html-skill"]);
  await box.press("Tab");
  await expect(box).toHaveValue("use $html-skill ");
  await expect(menu).toHaveCount(0);

  // `$` in a Claude session opens nothing.
  await go(page, `#/agents/${(await claudeIdle(fixture)).id}`);
  await box.fill("$re");
  await expect(menu).toHaveCount(0);
  await expect(page.getByText("No command matches")).toHaveCount(0);

  // A Codex draft that starts with `/gx-` shows the hint.
  await go(page, `#/agents/${codex.id}`);
  await box.fill("/gx-triage");
  await expect(page.getByText("Codex skills start with $. Type $gx-... instead.")).toBeVisible();
});

/** The design page composer's box, opened with the Note button. */
const openNote = async (page: Page) => {
  await page.getByRole("button", nav("Note")).click();
  const box = messageBox(page);
  await expect(box).toBeEnabled();
  return box;
};

const openNewForm = async (page: Page) => {
  await go(page, "#/agents");
  const phone = (page.viewportSize()?.width ?? 0) < 600;
  if (phone) await page.getByRole("button", { name: "+ New session", exact: true }).click();
  else await sidebar(page).getByRole("button", { name: "New session in demo" }).click();
  await expect(newForm(page)).toBeVisible();
};

test("151. Command menu on the design page and First prompt", async ({ page, start }) => {
  await start("agents-commands");
  await page.setViewportSize({ width: 1280, height: 800 });

  // The design composer opens the menu for a Claude design session...
  await go(page, "#/d/demo:design-auto");
  let box = await openNote(page);
  await box.fill("/co");
  await expect(commandMenu(page)).toBeVisible();
  await expect.poll(() => commandNames(page)).toEqual(expect.arrayContaining(["/compact", "/cost"]));
  await box.fill("/");
  await expect.poll(() => hasName(page, "/html-skill")).toBe(true);
  await box.press("Escape");

  // ...and a Codex design session lists built-ins and `$` skills.
  await go(page, "#/d/gx:design-codex");
  box = await openNote(page);
  await box.fill("/");
  await expect.poll(() => hasName(page, "/status")).toBe(true);
  expect(await hasName(page, "/html-skill")).toBe(false);
  await box.fill("$html");
  await expect.poll(() => commandNames(page)).toEqual(["$html-skill"]);

  // First prompt lists the chosen worktree's commands: `feature` only in the feature worktree.
  await openNewForm(page);
  const prompt = promptBox(page);
  await prompt.fill("/");
  await expect.poll(() => hasName(page, "/html-skill")).toBe(true);
  expect(await hasName(page, "/feature-only-skill")).toBe(false);
  await prompt.fill("");
  await worktreeBox(page).fill("feature");
  await page.getByRole("option", { name: /feature/ }).first().click();
  await prompt.fill("/");
  await expect.poll(() => hasName(page, "/feature-only-skill")).toBe(true);

  // A pick fills the field and creates nothing.
  await prompt.fill("/feature-o");
  await prompt.press("Enter");
  await expect(prompt).toHaveValue("/feature-only-skill ");
  await expect(newForm(page)).toBeVisible();

  // "+ Create worktree" lists the project's base worktree commands.
  await prompt.fill("");
  await worktreeBox(page).fill("fresh-tree");
  await page.getByRole("option", { name: /Create worktree/ }).click();
  await prompt.fill("/");
  await expect.poll(() => hasName(page, "/html-skill")).toBe(true);
  expect(await hasName(page, "/feature-only-skill")).toBe(false);
});

test("152. Phone, command menu", async ({ page, start }) => {
  const fixture = await start("agents-commands");
  await page.setViewportSize({ width: 390, height: 844 });

  // The very first `/` on a fresh phone Agents page opens the menu, with no prior typing.
  const session = await claudeIdle(fixture);
  const phone = await page.context().browser()!.newContext({ ...touch, viewport: { width: 390, height: 844 }, storageState: await page.context().storageState() });
  const fresh = await phone.newPage();
  await fresh.goto(new URL(page.url()).origin + `/#/agents/${session.id}`);
  await messageBox(fresh).tap();
  await fresh.keyboard.type("/");
  await expect(commandMenu(fresh)).toBeVisible();
  await expect(commandRows(fresh).filter({ hasText: "Clear the conversation" })).toBeVisible();
  await phone.close();

  await go(page, "#/d/demo:design-auto");
  const box = await openNote(page);
  const menu = commandMenu(page);

  // Two-line rows, no footer.
  await box.fill("/");
  await expect(menu).toBeVisible();
  const clear = commandRows(page).filter({ hasText: "Clear the conversation" });
  await expect(clear).toBeVisible();
  await expect(page.getByText("↑↓ select")).toHaveCount(0);
  await expect(page.getByText("Esc closes")).toHaveCount(0);
  const rowBox = (await clear.boundingBox())!;
  expect(rowBox.height).toBeGreaterThan(36);
  expect(rowBox.x + rowBox.width).toBeLessThanOrEqual(390);

  // A tap fills.
  await box.fill("/mo");
  await commandRows(page).filter({ hasText: "Switch model" }).click();
  await expect(box).toHaveValue("/model ");
  await expect(menu).toHaveCount(0);

  // The short viewport stands in for the on-screen keyboard: the menu scrolls inside the room left.
  await page.setViewportSize({ width: 390, height: 420 });
  await box.fill("/");
  await expect(menu).toBeVisible();
  // The menu places itself after the resize, so poll its frame.
  await expect.poll(async () => {
    const frame = (await menu.boundingBox())!;
    return frame.y >= 0 && frame.y + frame.height <= 420;
  }).toBe(true);

  // First prompt opens below the field.
  await openNewForm(page);
  const prompt = promptBox(page);
  await prompt.fill("/");
  await expect(menu).toBeVisible();
  const field = (await prompt.boundingBox())!;
  const below = (await menu.boundingBox())!;
  expect(below.y).toBeGreaterThanOrEqual(field.y + field.height - 1);
  await commandRows(page).first().click();
  await expect(prompt).toHaveValue(/^\/\S+ $/);
});
