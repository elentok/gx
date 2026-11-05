import type { Page } from "@playwright/test";
import type { SessionInfo } from "../src/generated/server";
import { expect, type Fixture } from "./fixture";

export const sessions = async (fixture: Fixture) => JSON.parse(await fixture.cli("agents", "list", "--json")) as SessionInfo[];

/** Design and Queue agents never count: gx drives them. */
export const isActionable = (s: SessionInfo) => !s.design && !s.queue && (!!s.actionable.reason || s.status.state === "blocked");

/** The nth idle Claude session; a sent slash command keeps its session busy, so a later step takes another. */
export const claudeIdle = async (fixture: Fixture, n = 0) =>
  (await sessions(fixture)).filter((s) => s.status.state === "idle" && s.harness !== "codex" && !s.design && !s.queue)[n];

export const codexIdle = async (fixture: Fixture) =>
  (await sessions(fixture)).find((s) => s.status.state === "idle" && s.harness === "codex" && !s.design && !s.queue)!;

export const row = (page: Page, s: SessionInfo) => page.locator(`a[href$="/agents/${s.id}"]`);

export const sidebar = (page: Page) => page.getByRole("complementary");

// A hash change keeps the page, since the fixture fails a test whose page reloaded.
export const go = (page: Page, hash: string) => page.evaluate((h) => (location.hash = h), hash);

/** A second visit is a fresh page on the same server and context, since a reload would fail the test. */
export async function freshVisit(page: Page, hash: string) {
  const other = await page.context().newPage();
  await other.goto(new URL(page.url()).origin + hash);
  return other;
}

export const REPLY = "Sure, here is a short answer.";

export const chatOf = async (fixture: Fixture, id: string) =>
  JSON.parse(await fixture.cli("agents", "chat", id, "--json")) as { messages: unknown[]; conversation_id: string };

/** Prompts a session and waits for the reply. A prompt is refused until the last turn ends, so it retries. */
export async function ask(fixture: Fixture, id: string, text: string) {
  const before = (await chatOf(fixture, id)).messages.length;
  await expect
    .poll(() => fixture.cli("agents", "prompt", id, text).then(() => true, () => false))
    .toBe(true);
  await expect.poll(async () => (await chatOf(fixture, id)).messages.length).toBe(before + 2);
  // The server's state follows the fake a moment late, so a prompt sent now could still land on the old turn.
  await expect.poll(async () => (await sessions(fixture)).find((s) => s.id === id)!.status.state).toBe("working");
  await expect.poll(async () => (await sessions(fixture)).find((s) => s.id === id)!.status.state).toBe("idle");
}

export const composer = (page: Page) => page.getByRole("form", { name: "Composer" });
export const messageBox = (page: Page) => composer(page).getByRole("textbox", { name: "Message" });
export const sendButton = (page: Page) => composer(page).getByRole("button", { name: "Send", exact: true });

export const PNG = Buffer.from("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==", "base64");
export const file = (name: string, mimeType: string, buffer: Buffer) => ({ name, mimeType, buffer });
export const png = (n: number) => file(`pic-${n}.png`, "image/png", PNG);

export const thumbs = (page: Page) => page.getByRole("button", { name: "Remove image" });
export const addInput = (page: Page) => page.getByTestId("image-input");

/** Opens the first idle plain session at desktop width with its composer showing. */
export async function idleSession(page: Page, fixture: Fixture) {
  const idle = (await sessions(fixture)).find((s) => s.status.state === "idle" && !s.design && !s.queue)!;
  await page.setViewportSize({ width: 1280, height: 800 });
  await go(page, `#/agents/${idle.id}`);
  await expect(composer(page)).toBeVisible();
  return idle;
}

/** The completion menu under the composer and its rows. */
export const commandMenu = (page: Page) => page.getByRole("listbox", { name: "Commands" });
export const commandRows = (page: Page) => commandMenu(page).getByRole("option");
/** Row names in menu order, without the argument hint. */
export const commandNames = async (page: Page) => {
  const rows = commandRows(page);
  await expect(rows.first()).toBeVisible();
  return rows.evaluateAll((els) =>
    els.map((r) => {
      const name = r.firstElementChild!.cloneNode(true) as HTMLElement;
      name.querySelector("i")?.remove();
      return name.textContent!.trim();
    }),
  );
};

/** Turns the Vi mode setting on or off and checks the Menu shows it. */
export async function setViMode(fixture: Fixture, page: Page, on: boolean) {
  await fixture.cli("vimode", on ? "on" : "off");
  await page.getByRole("button", { name: "Menu", exact: true }).click();
  const menu = page.getByRole("menu", { name: "Menu", exact: true });
  await expect(menu.getByRole("menuitemcheckbox", { name: "Vi mode", exact: true })).toHaveAttribute("aria-checked", String(on));
  await page.getByRole("button", { name: "Menu", exact: true }).click();
  await expect(menu).toHaveCount(0);
}

export const titleButton = (page: Page) => page.getByRole("heading", { level: 1 }).getByRole("button");

export const switcher = (page: Page) => page.getByRole("dialog", { name: "Switch session" });
export const switcherFilter = (page: Page) => switcher(page).getByRole("combobox");

export const newForm = (page: Page) => page.getByRole("dialog", { name: "New session" });
export const projectSelect = (page: Page) => newForm(page).getByRole("combobox", { name: "Project" });
export const worktreeBox = (page: Page) => newForm(page).getByRole("combobox", { name: "Worktree" });
export const promptBox = (page: Page) => newForm(page).getByRole("textbox", { name: "First prompt" });
export const createButton = (page: Page) => newForm(page).getByRole("button", { name: "Create session" });

export const created = async (fixture: Fixture, before: SessionInfo[]) => {
  const known = new Set(before.map((s) => s.id));
  await expect.poll(async () => (await sessions(fixture)).filter((s) => !known.has(s.id)).length).toBe(1);
  return (await sessions(fixture)).find((s) => !known.has(s.id))!;
};
