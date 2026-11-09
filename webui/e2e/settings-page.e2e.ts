import type { Page } from "@playwright/test";
import { expect, test, type Fixture } from "./fixture";

// What jsdom cannot see on the Settings page: the real server round trip into config.json and project.json, and phone layout.
// Per-control behavior lives in SettingsPage.test.tsx.

const globalHash = (section: string) => `#/settings/s/${section}`;
const projectHash = (project: string, section: string) => `#/settings/p/${project}/s/${section}`;

async function go(page: Page, hash: string, heading: string) {
  await page.evaluate((h) => (location.hash = h), hash);
  await expect(page.getByRole("heading", { name: heading, exact: true })).toBeVisible();
}

/** Only the changed row's Save is enabled. */
const save = (page: Page) => page.getByRole("button", { name: "Save", exact: true }).and(page.locator(":enabled"));

const text = (page: Page, label: string) => page.getByRole("textbox", { name: label, exact: true });

async function saveText(page: Page, label: string, value: string) {
  await text(page, label).fill(value);
  await save(page).click();
  await expect(page.getByRole("status").filter({ hasText: "Saved" })).toBeVisible();
}

const noSideScroll = async (page: Page) => {
  await expect.poll(() => page.evaluate(() => document.documentElement.scrollWidth <= document.documentElement.clientWidth)).toBe(true);
};

test("200. Settings page, global journey", async ({ page, start }) => {
  // The reload at the end re-reads config.json through the page.
  const fixture: Fixture = await start("settings", { reloads: 1 });
  await go(page, globalHash("execution-queue"), "Execution queue");
  const before = await fixture.globalConfig();

  // A number and a duration reach config.json with the other keys intact.
  await saveText(page, "Max released agents", "5");
  await saveText(page, "Spin window", "20s");
  const after = await fixture.globalConfig();
  expect(after["execution-queue"]["max-released-agents"]).toBe(5);
  expect(after["execution-queue"]["spin-window"]).toBe("20s");
  expect(after.budget).toEqual(before.budget);
  expect(after.notifications).toEqual(before.notifications);

  // A reopened page shows the saved values.
  await page.reload();
  await expect(text(page, "Spin window")).toHaveValue("20s");
  await expect(text(page, "Max released agents")).toHaveValue("5");

  // The server refuses a value that is not a duration; the draft stays and nothing is written.
  await text(page, "Spin window").fill("abc");
  await save(page).click();
  await expect(page.getByRole("alert").filter({ hasText: "Not saved:" })).toBeVisible();
  await expect(text(page, "Spin window")).toHaveValue("abc");
  expect((await fixture.globalConfig())["execution-queue"]["spin-window"]).toBe("20s");

  // Reset removes the key from the file.
  await page.getByRole("button", { name: /^Reset to default/ }).first().click();
  await expect(page.getByRole("status").filter({ hasText: "Saved" })).toBeVisible();
  const reset = await fixture.globalConfig();
  expect(reset["execution-queue"]["spin-window"]).toBeUndefined();
  expect(reset["execution-queue"]["max-released-agents"]).toBe(5);

  // Secrets: the read response and the page carry the state, never the value.
  await go(page, globalHash("notifications"), "Notifications");
  const body = await page.evaluate(async () => (await fetch("/v1/settings?target=global")).text());
  expect(body).toContain("notifications.slack.webhook-url");
  expect(body).not.toContain("fixture-bot-token");
  expect(await page.content()).not.toContain("fixture-bot-token");
  const setChips = page.locator("span[data-tone]").filter({ hasText: /^Set$/ });
  await expect(setChips).toHaveCount(1);
  await expect(page.getByText("Not set", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Set", exact: true }).click();
  await page.getByLabel("New Webhook url").fill("https://hooks.slack.example/secret-webhook");
  await save(page).click();
  await expect(page.getByText("Not set", { exact: true })).toHaveCount(0);
  expect(await page.content()).not.toContain("secret-webhook");
  expect(JSON.stringify(await fixture.globalConfig())).toContain("secret-webhook");
  const reread = await page.evaluate(async () => (await fetch("/v1/settings?target=global")).text());
  expect(reread).not.toContain("secret-webhook");

  // A startup-only key says it needs a restart.
  await go(page, globalHash("agents"), "Agents");
  await saveText(page, "Runner wait", "45s");
  await expect(page.getByText("Restart needed to apply").first()).toBeVisible();

  // The phone layout has no sideways scroll on the home, a section and the project list.
  await page.setViewportSize({ width: 390, height: 844 });
  await noSideScroll(page);
  await page.getByRole("link", { name: "‹ Settings" }).click();
  await expect(page.getByRole("heading", { name: "Settings", exact: true })).toBeVisible();
  await expect(page.getByRole("link", { name: /^Budget/ })).toBeVisible();
  await noSideScroll(page);
  await page.getByRole("link", { name: "demo", exact: true }).click();
  await expect(page.getByRole("heading", { name: "demo", exact: true })).toBeVisible();
  await noSideScroll(page);
  await page.getByRole("link", { name: /^Checks/ }).click();
  await expect(page.getByRole("heading", { name: "Checks", exact: true })).toBeVisible();
  await noSideScroll(page);
});

test("201. Settings page, project journey", async ({ page, start }) => {
  const fixture = await start("settings");
  await go(page, projectHash("demo", "execution-queue"), "Execution queue");

  // Override a cap for the project, then go back to the global value.
  await page.getByRole("button", { name: "Override for demo" }).first().click();
  await expect(page.getByRole("status").filter({ hasText: "Saved" })).toBeVisible();
  const overridden = await fixture.projectConfig("demo");
  expect(overridden["execution-queue"]).toBeDefined();
  await page.getByRole("button", { name: /^Use global value/ }).first().click();
  await expect(page.getByRole("button", { name: "Override for demo" }).first()).toBeVisible();

  // The per-project cap (demo starts with one) is removed and set again.
  expect((await fixture.projectConfig("demo"))["max-agents"]).toBe(1);
  await page.getByRole("button", { name: "Remove cap" }).click();
  await expect(page.getByRole("button", { name: "Set a cap for demo" })).toBeVisible();
  expect((await fixture.projectConfig("demo"))["max-agents"]).toBeUndefined();
  await page.getByRole("button", { name: "Set a cap for demo" }).click();
  await expect(page.getByRole("button", { name: "Remove cap" })).toBeVisible();
  expect((await fixture.projectConfig("demo"))["max-agents"]).toBeDefined();

  // A hand-written notifications block survives saving another key.
  await go(page, projectHash("notes", "execution-queue"), "Execution queue");
  await page.getByRole("button", { name: "Override for notes" }).first().click();
  await expect(page.getByRole("status").filter({ hasText: "Saved" })).toBeVisible();
  const notes = await fixture.projectConfig("notes");
  expect(notes.notifications.telegram["bot-token"]).toBe("notes-token");

  // A Checks card edit lands in project.json.
  await go(page, projectHash("demo", "checks"), "Checks");
  await page.getByRole("group", { name: "format mode" }).getByRole("button", { name: "off", exact: true }).click();
  await save(page).click();
  await expect.poll(async () => (await fixture.projectConfig("demo")).checks.format).toBeDefined();
  expect((await fixture.projectConfig("demo")).checks.test.full).toBe("true");
});
