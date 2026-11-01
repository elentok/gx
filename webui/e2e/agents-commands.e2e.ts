import { writeFile } from "node:fs/promises";
import { join } from "node:path";
import type { Page } from "@playwright/test";
import { expect, holdRoute, nav, test } from "./fixture";
import { chatOf, commandMenu, commandNames, commandRows, go, messageBox, newForm, promptBox, sendButton, sessions, setViMode, sidebar, worktreeBox } from "./agentsKit";

/** The nth idle Claude session; a sent slash command keeps its session busy, so a later step takes another. */
const claudeIdle = async (fixture: Parameters<typeof sessions>[0], n = 0) =>
  (await sessions(fixture)).filter((s) => s.status.state === "idle" && s.harness !== "codex" && !s.design && !s.queue)[n];

test("147. Command menu opens and filters", async ({ page, start }) => {
  const fixture = await start("agents-commands");
  const session = await claudeIdle(fixture);
  await page.setViewportSize({ width: 1280, height: 800 });
  await go(page, `#/agents/${session.id}`);
  const box = messageBox(page);
  const menu = commandMenu(page);

  // `/` opens the menu: name, hint, description and source on a row, and a footer on desktop.
  await box.fill("/");
  await expect(menu).toBeVisible();
  const clear = commandRows(page).filter({ hasText: "Clear the conversation" });
  await expect(clear).toContainText("clear");
  await expect(clear).toContainText("[name]");
  await expect(clear).toContainText("built-in");
  await expect(page.getByText("↑↓ select")).toBeVisible();
  await expect(page.getByText("Enter fills")).toBeVisible();
  await expect(page.getByText("Esc closes")).toBeVisible();

  // Prefix matches first, then substring matches, alphabetical in each group; the typed part is highlighted.
  await box.fill("/co");
  await expect(commandRows(page).filter({ hasText: "Show the session cost" })).toBeVisible();
  const sorted = (xs: string[]) => [...xs].sort((a, b) => a.localeCompare(b));
  const names = await commandNames(page);
  const prefix = names.filter((n) => n.startsWith("/co"));
  expect(prefix).toEqual(expect.arrayContaining(["/compact", "/cost"]));
  expect(names.slice(0, prefix.length)).toEqual(sorted(prefix));
  expect(names.slice(prefix.length)).toEqual(sorted(names.slice(prefix.length)));
  expect(names.slice(prefix.length).length).toBeGreaterThan(0);
  await expect(commandRows(page).first().locator("mark")).toHaveText("/co");

  // A space and Escape close it.
  await box.fill("/co");
  await expect(menu).toBeVisible();
  await box.press("Escape");
  await expect(menu).toHaveCount(0);
  await expect(box).toHaveValue("/co");
  await box.fill("/co ");
  await expect(menu).toHaveCount(0);

  // No match keeps the menu open with one line and keeps the text.
  await box.fill("/zzq");
  await expect(page.getByText("No command matches /zzq. Cmd/Ctrl+Enter sends it as typed.")).toBeVisible();
  await expect(commandRows(page)).toHaveCount(0);
  await expect(box).toHaveValue("/zzq");

  // A `/` after other text opens nothing.
  await box.fill("hello /co");
  await expect(menu).toHaveCount(0);
  await expect(page.getByText("No command matches")).toHaveCount(0);
});

test("148. Command menu keys", async ({ page, start }) => {
  const fixture = await start("agents-commands");
  const session = await claudeIdle(fixture);
  await page.setViewportSize({ width: 1280, height: 800 });
  await go(page, `#/agents/${session.id}`);
  const box = messageBox(page);
  const menu = commandMenu(page);
  const before = (await chatOf(fixture, session.id)).messages.length;
  const unsent = async () => expect((await chatOf(fixture, session.id)).messages).toHaveLength(before);

  // Down, Up, Enter and Tab fill `/name ` and send nothing.
  await box.fill("/comp");
  await expect.poll(() => commandNames(page)).toEqual(["/compact", "/autocompact"]);
  await expect(commandRows(page).first()).toHaveAttribute("aria-selected", "true");
  await box.press("ArrowDown");
  await expect(commandRows(page).nth(1)).toHaveAttribute("aria-selected", "true");
  await box.press("ArrowUp");
  await expect(commandRows(page).first()).toHaveAttribute("aria-selected", "true");
  await box.press("ArrowDown");
  await box.press("Enter");
  await expect(box).toHaveValue("/autocompact ");
  await expect(menu).toHaveCount(0);
  await unsent();
  await box.fill("/cost");
  await expect(menu).toBeVisible();
  await box.press("Tab");
  await expect(box).toHaveValue("/cost ");
  await unsent();

  // A click on a row fills.
  await box.fill("/mo");
  await commandRows(page).filter({ hasText: "Switch model" }).click();
  await expect(box).toHaveValue("/model ");
  await expect(box).toBeFocused();
  await unsent();

  // Enter with no match adds a newline.
  await box.fill("/zzq");
  await expect(page.getByText("No command matches")).toBeVisible();
  await box.press("Enter");
  await expect(box).toHaveValue("/zzq\n");
  await unsent();

  // Cmd/Ctrl+Enter sends the typed text with the menu closed...
  const posted: string[] = [];
  page.on("request", (r) => {
    if (r.method() === "POST" && r.url().includes("/v1/sessions/prompt")) posted.push(r.postData() ?? "");
  });
  await box.fill("/model fast");
  await expect(menu).toHaveCount(0);
  await box.press("ControlOrMeta+Enter");
  await expect(box).toHaveValue("");
  await expect.poll(() => posted.length).toBe(1);
  expect(posted[0]).toContain("/model fast");

  // ...and with the menu open, in another session since the first is busy with the command.
  await go(page, `#/agents/${(await claudeIdle(fixture, 2)).id}`);
  await box.fill("/cost");
  await expect(menu).toBeVisible();
  await box.press("ControlOrMeta+Enter");
  await expect(box).toHaveValue("");
  await expect.poll(() => posted.length).toBe(2);
  expect(posted[1]).toContain("/cost");

  // A disabled composer (the Exited session) shows no menu.
  const exited = (await sessions(fixture)).find((s) => s.status.state === "exited")!;
  await go(page, `#/agents/${exited.id}`);
  await expect(sendButton(page)).toBeDisabled();
  await messageBox(page).fill("/");
  await expect(menu).toHaveCount(0);

  // With vi mode on, typing in insert mode opens the menu and a normal-mode key does not.
  await go(page, `#/agents/${(await claudeIdle(fixture, 1)).id}`);
  await setViMode(fixture, page, true);
  await messageBox(page).focus();
  await page.keyboard.press("ControlOrMeta+g");
  const dialog = page.getByRole("dialog");
  const editor = dialog.getByRole("textbox");
  await expect(editor).toBeFocused();
  const mode = dialog.getByTestId("vi-mode-text");

  // In insert mode typing opens the menu.
  await expect(mode).toHaveText("INSERT");
  await page.keyboard.type("/co");
  await expect(commandMenu(page)).toBeVisible();

  // A normal-mode key opens nothing.
  await page.keyboard.press("Escape");
  await page.keyboard.press("Escape");
  await expect(mode).toHaveText("NORMAL");
  await expect(commandMenu(page)).toHaveCount(0);
  await page.keyboard.press("0");
  await expect(commandMenu(page)).toHaveCount(0);
  await page.keyboard.press("i");
  await expect(mode).toHaveText("INSERT");
});

const codexIdle = async (fixture: Parameters<typeof sessions>[0]) =>
  (await sessions(fixture)).find((s) => s.status.state === "idle" && s.harness === "codex" && !s.design && !s.queue)!;

/** Whether a command of that name is in the open menu. */
const hasName = (page: Parameters<typeof commandRows>[0], name: string) => commandNames(page).then((names) => names.includes(name));

test("149. Command list freshness and failure, loading", async ({ page, start }) => {
  const fixture = await start("agents-commands-slow");
  await page.setViewportSize({ width: 1280, height: 800 });
  await go(page, `#/agents/${(await claudeIdle(fixture)).id}`);
  await messageBox(page).fill("/");
  await expect(page.getByText("Loading commands…")).toBeVisible();
  await expect(commandRows(page)).toHaveCount(0);
});

test("149. Command list freshness and failure, failed list", async ({ page, start }) => {
  const fixture = await start("agents-commands-failed");
  await page.setViewportSize({ width: 1280, height: 800 });
  await go(page, `#/agents/${(await claudeIdle(fixture)).id}`);
  const box = messageBox(page);
  await box.fill("/co");
  await expect(page.getByText("Skills could not load")).toBeVisible();
  await expect.poll(() => commandNames(page)).toEqual(expect.arrayContaining(["/compact", "/cost"]));
  const posted: string[] = [];
  page.on("request", (r) => {
    if (r.method() === "POST" && r.url().includes("/v1/sessions/prompt")) posted.push(r.postData() ?? "");
  });
  await box.fill("/cost");
  await box.press("ControlOrMeta+Enter");
  await expect(box).toHaveValue("");
  await expect.poll(() => posted.length).toBe(1);
  expect(posted[0]).toContain("/cost");

});

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
  await start("agents-commands");
  await page.setViewportSize({ width: 390, height: 844 });
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
  const frame = (await menu.boundingBox())!;
  expect(frame.y).toBeGreaterThanOrEqual(0);
  expect(frame.y + frame.height).toBeLessThanOrEqual(420);

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
