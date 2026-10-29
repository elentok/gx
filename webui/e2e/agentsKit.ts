import type { Page } from "@playwright/test";
import type { SessionInfo } from "../src/generated/server";
import { expect, type Fixture } from "./fixture";

export const sessions = async (fixture: Fixture) => JSON.parse(await fixture.cli("agents", "list", "--json")) as SessionInfo[];

export const isActionable = (s: SessionInfo) => !!s.actionable.reason || s.status.state === "blocked";

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
  JSON.parse(await fixture.cli("agents", "chat", id, "--json")) as { messages: unknown[] };

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
