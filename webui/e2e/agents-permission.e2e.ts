import type { Page } from "@playwright/test";
import type { SessionInfo } from "../src/generated/server";
import { expect, test, type Fixture } from "./fixture";
import { chatOf, claudeIdle, go, sessions } from "./agentsKit";

const conversationOf = async (fixture: Fixture, id: string) => (await chatOf(fixture, id)).conversation_id;
const stateOf = async (fixture: Fixture, id: string) => (await sessions(fixture)).find((s) => s.id === id)!.status.state;
// An answer waits up to the prompt grace for the session to move on, and a loaded machine slows that.
test.describe.configure({ timeout: 180_000 });
const SLOW = { timeout: 45_000 };
const leavesBlocked = (fixture: Fixture, id: string) => expect.poll(() => stateOf(fixture, id), SLOW).not.toBe("blocked");

/** What the fake read after `before`, the input log it had then. */
const readSince = async (fixture: Fixture, conv: string, before: string) => (await fixture.input(conv)).slice(before.length);

const failure = (page: Page) => page.getByRole("region", { name: "Session failure" });
const noticeButton = (page: Page, name: string) => failure(page).getByRole("button", { name, exact: true });
const screenDialog = (page: Page) => page.getByRole("dialog", { name: "Screen" });

const BASH_DIALOG = (command: string) => ["────────────────────", "Bash command", `  ${command}`, "Do you want to proceed?", "❯ 1. Yes", "  2. No", "Esc to cancel"];
const UNREADABLE = ["▒▒▒▒ nothing gx can read ▒▒▒▒"];
const QUESTION = ["Select a thing", "❯ 1. One", "  2. Two", "Esc to cancel"];

const NOT_APPROVABLE = "This request needs a real answer. Use the key bar in Screen.";
const NOT_DELIVERED = "The answer did not reach the session. Check Screen.";

const plainBlocked = (s: SessionInfo) => !s.design && !s.queue && s.failure?.kind === "blocked";

/** The seeded Blocked session whose permission prompt ends in `text`; "" is a prompt gx cannot read. */
async function blockedAt(fixture: Fixture, harness: string, text: string) {
  for (const s of (await sessions(fixture)).filter((x) => plainBlocked(x) && x.failure?.why === "permission" && (x.harness || "claude") === harness)) {
    const perm = JSON.parse(await fixture.cli("agents", "permission", s.id, "--json")) as { text: string };
    if (perm.text.split("\n").at(-1) === text) return s;
  }
  throw new Error(`no ${harness} session blocked at ${JSON.stringify(text)}`);
}

/** Blocks the first idle Claude session at a dialog. */
async function blockIdle(fixture: Fixture, dialog: string[], waitingFor = "permission prompt") {
  const s = (await claudeIdle(fixture))!;
  const conv = await conversationOf(fixture, s.id);
  await fixture.controlClaude(conv, { action: "block", waiting_for: waitingFor, dialog });
  await expect.poll(() => stateOf(fixture, s.id)).toBe("blocked");
  return { s, conv };
}

const open = async (page: Page, hash: string) => {
  await go(page, hash);
  await expect(failure(page)).toBeVisible();
};

/** Stops the page from learning anything new, so its notice outlives the Blocked state. */
const freeze = (page: Page) => page.route("**/v1/snapshot*", () => new Promise(() => {}));

async function closeDialogTwice(page: Page, fixture: Fixture, hash: string, s: SessionInfo, conv: string) {
  const before = await fixture.input(conv);
  await open(page, hash);
  await freeze(page);
  await noticeButton(page, "Close dialog").click();
  await expect.poll(() => stateOf(fixture, s.id)).toBe("idle");
  await expect.poll(async () => JSON.stringify(await readSince(fixture, conv, before))).toBe('"\\u001b"');

  // The notice still shows, so the button is there; the guard refuses it.
  await noticeButton(page, "Close dialog").click();
  await expect(failure(page).getByRole("alert")).toBeVisible();
  expect(await readSince(fixture, conv, before)).toBe("\x1b");
  await page.unrouteAll({ behavior: "ignoreErrors" });
}

test("157. Close dialog, Agents page", async ({ page, start }) => {
  const fixture = await start("agents-blocked");
  await page.setViewportSize({ width: 1280, height: 800 });
  const { s, conv } = await blockIdle(fixture, QUESTION, "input needed");
  await closeDialogTwice(page, fixture, `#/agents/${s.id}`, s, conv);
});

test("157. Close dialog, design page", async ({ page, start }) => {
  const fixture = await start("agents-blocked", { autoAdvanceDelay: "1h" });
  await page.setViewportSize({ width: 1280, height: 800 });
  const design = (await sessions(fixture)).find((s) => s.ticket === "demo:design-auto")!;
  const conv = await conversationOf(fixture, design.id);
  await fixture.controlClaude(conv, { action: "block", waiting_for: "input needed", dialog: QUESTION });
  await expect.poll(() => stateOf(fixture, design.id)).toBe("blocked");
  await closeDialogTwice(page, fixture, "#/d/demo:design-auto", design, conv);
});

test("158. Allow once and Deny, readable Claude prompt", async ({ page, start }) => {
  const fixture = await start("agents-blocked");
  await page.setViewportSize({ width: 1280, height: 800 });

  const allow = await blockIdle(fixture, BASH_DIALOG("rg allow-me"));
  let before = await fixture.input(allow.conv);
  await open(page, `#/agents/${allow.s.id}`);
  await expect(failure(page)).toContainText("rg allow-me");
  await expect(failure(page).getByRole("button")).toHaveText(["Screen", "Deny", "Allow once"]);
  await noticeButton(page, "Allow once").click();
  await leavesBlocked(fixture, allow.s.id);
  await expect.poll(() => readSince(fixture, allow.conv, before)).toBe("\r");

  const deny = await blockIdle(fixture, BASH_DIALOG("rg deny-me"));
  before = await fixture.input(deny.conv);
  await open(page, `#/agents/${deny.s.id}`);
  await expect(failure(page)).toContainText("rg deny-me");
  await noticeButton(page, "Deny").click();
  await leavesBlocked(fixture, deny.s.id);
  await expect.poll(() => readSince(fixture, deny.conv, before)).toBe("\x1b");
});

test("158. Allow once refused for Claude prompts gx cannot approve", async ({ page, start }) => {
  const fixture = await start("agents-blocked");
  await page.setViewportSize({ width: 1280, height: 800 });
  const refused = async (text: string, notice: string) => {
    const s = await blockedAt(fixture, "claude", text);
    const conv = await conversationOf(fixture, s.id);
    const before = await fixture.input(conv);
    await open(page, `#/agents/${s.id}`);
    await noticeButton(page, "Allow once").click();
    await expect(failure(page).getByRole("alert")).toHaveText(notice);
    expect(await stateOf(fixture, s.id)).toBe("blocked");
    return { s, conv, before };
  };

  // Unreadable screen: no prompt text; Allow is refused and sends nothing; Deny still works.
  const unreadable = await refused("", NOT_APPROVABLE);
  expect(await readSince(fixture, unreadable.conv, unreadable.before)).toBe("");
  await noticeButton(page, "Deny").click();
  await leavesBlocked(fixture, unreadable.s.id);
  await expect.poll(() => readSince(fixture, unreadable.conv, unreadable.before)).toBe("\x1b");

  // A first option that is not a plain Yes would pick a standing permission, so no Enter is sent.
  const standing = await refused("rg --files", NOT_APPROVABLE);
  await expect(failure(page)).toContainText("rg --files");
  expect(await readSince(fixture, standing.conv, standing.before)).toBe("");

  // Down in the key bar reaches the fake. The fake draws no cursor, so the redrawn dialog, cursor on option 2, is armed by hand.
  const moved = await blockedAt(fixture, "claude", "rg readable-command");
  const movedConv = await conversationOf(fixture, moved.id);
  const movedBefore = await fixture.input(movedConv);
  await open(page, `#/agents/${moved.id}`);
  // Any key leaves the session Blocked, and the notice (with the Screen dialog it owns) goes with it unless the page stops learning.
  await freeze(page);
  await noticeButton(page, "Screen").click();
  await screenDialog(page).getByRole("group", { name: "Keys" }).getByRole("button", { name: "Down", exact: true }).click();
  await expect.poll(() => readSince(fixture, movedConv, movedBefore)).toBe("\x1b[B");
  await screenDialog(page).getByRole("button", { name: "Close", exact: true }).click();
  await page.unrouteAll({ behavior: "ignoreErrors" });
  const redrawn = BASH_DIALOG("rg readable-command");
  redrawn[4] = "  1. Yes";
  redrawn[5] = "❯ 2. No";
  await fixture.controlClaude(movedConv, { action: "block", waiting_for: "permission prompt", dialog: redrawn });
  await go(page, "#/agents");
  await open(page, `#/agents/${moved.id}`);
  await noticeButton(page, "Allow once").click();
  await expect(failure(page).getByRole("alert")).toHaveText(NOT_APPROVABLE);
  expect(await readSince(fixture, movedConv, movedBefore)).toBe("\x1b[B");
  expect(await stateOf(fixture, moved.id)).toBe("blocked");
});

test("158. Ignored Allow key shows the not-delivered text", async ({ page, start }) => {
  const fixture = await start("agents-blocked");
  await page.setViewportSize({ width: 1280, height: 800 });
  const s = await blockedAt(fixture, "claude", "rg ignores-allow");
  const conv = await conversationOf(fixture, s.id);
  const before = await fixture.input(conv);
  await open(page, `#/agents/${s.id}`);
  await noticeButton(page, "Allow once").click();
  // The server waits its whole prompt grace (45s) for the session to move on.
  await expect(failure(page).getByRole("alert")).toHaveText(NOT_DELIVERED, { timeout: 90_000 });
  // The Enter went out; the fake ignored it and no other option was picked.
  await expect.poll(() => readSince(fixture, conv, before)).toBe("\r");
  expect(await stateOf(fixture, s.id)).toBe("blocked");
});

test("158. Allow once and Deny, Codex hook and screen", async ({ page, start }) => {
  const fixture = await start("agents-blocked");
  await page.setViewportSize({ width: 1280, height: 800 });

  const hook = await blockedAt(fixture, "codex", "rg codex-hook");
  await open(page, `#/agents/${hook.id}`);
  await expect(failure(page)).toContainText("rg codex-hook");
  await noticeButton(page, "Allow once").click();
  await leavesBlocked(fixture, hook.id);
  await expect.poll(() => fixture.codexDecisions()).toEqual([{ behavior: "allow", message: "" }]);

  const screen = await blockedAt(fixture, "codex", "rg codex-screen");
  await open(page, `#/agents/${screen.id}`);
  await expect(failure(page)).toContainText("rg codex-screen");
  await noticeButton(page, "Deny").click();
  await leavesBlocked(fixture, screen.id);

  // No hook and a screen gx cannot read: Allow is refused, Deny sends Esc.
  const unreadable = await blockedAt(fixture, "codex", "");
  await open(page, `#/agents/${unreadable.id}`);
  await noticeButton(page, "Allow once").click();
  await expect(failure(page).getByRole("alert")).toHaveText(NOT_APPROVABLE);
  expect(await stateOf(fixture, unreadable.id)).toBe("blocked");
  await noticeButton(page, "Deny").click();
  await leavesBlocked(fixture, unreadable.id);
});

test("158. Allow once and Deny, design page", async ({ page, start }) => {
  const fixture = await start("agents-blocked", { autoAdvanceDelay: "1h" });
  await page.setViewportSize({ width: 1280, height: 800 });
  const design = (await sessions(fixture)).find((s) => s.ticket === "demo:design-auto")!;
  const conv = await conversationOf(fixture, design.id);
  const block = async (dialog: string[]) => {
    await fixture.controlClaude(conv, { action: "block", waiting_for: "permission prompt", dialog });
    await expect.poll(() => stateOf(fixture, design.id)).toBe("blocked");
  };

  await block(BASH_DIALOG("rg design-allow"));
  let before = await fixture.input(conv);
  await open(page, "#/d/demo:design-auto");
  await expect(failure(page)).toContainText("rg design-allow");
  await noticeButton(page, "Allow once").click();
  await leavesBlocked(fixture, design.id);
  await expect.poll(() => readSince(fixture, conv, before)).toBe("\r");

  // A prompt gx cannot read: Allow is refused, Deny works.
  await block(UNREADABLE);
  before = await fixture.input(conv);
  // A fresh visit: the notice of the first prompt is still being replaced.
  await go(page, "#/epics");
  await open(page, "#/d/demo:design-auto");
  await expect(failure(page)).not.toContainText("rg design-allow");
  await expect(noticeButton(page, "Allow once")).toBeVisible();
  await noticeButton(page, "Allow once").click();
  await expect(failure(page).getByRole("alert")).toHaveText(NOT_APPROVABLE);
  expect(await readSince(fixture, conv, before)).toBe("");
  await noticeButton(page, "Deny").click();
  await leavesBlocked(fixture, design.id);
  await expect.poll(() => readSince(fixture, conv, before)).toBe("\x1b");
});
