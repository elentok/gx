import type { Page } from "@playwright/test";
import type { SessionInfo } from "../src/generated/server";
import { expect, test, type Fixture } from "./fixture";
import { chatOf, claudeIdle, composer, go, sendButton, sessions } from "./agentsKit";

const conversationOf = async (fixture: Fixture, id: string) => (await chatOf(fixture, id)).conversation_id;
const stateOf = async (fixture: Fixture, id: string) => (await sessions(fixture)).find((s) => s.id === id)!.status.state;

const failure = (page: Page) => page.getByRole("region", { name: "Session failure" });
const screenDialog = (page: Page) => page.getByRole("dialog", { name: "Screen" });
const typeBox = (page: Page) => screenDialog(page).getByRole("textbox", { name: "Text to type" });
const typeButton = (page: Page) => screenDialog(page).getByRole("button", { name: "Type", exact: true });
const keyButton = (page: Page, name: string) => screenDialog(page).getByRole("group", { name: "Keys" }).getByRole("button", { name, exact: true });

// The sequences the keys route sends, in the key bar's order.
const KEYS: [name: string, cli: string, bytes: string][] = [
  ["Esc", "esc", "\x1b"],
  ["Enter", "enter", "\r"],
  ["Tab", "tab", "\t"],
  ["Up", "up", "\x1b[A"],
  ["Down", "down", "\x1b[B"],
  ["Left", "left", "\x1b[D"],
  ["Right", "right", "\x1b[C"],
  ["Ctrl+C", "ctrl+c", "\x03"],
];

/** What the fake read after `before`, the input log it had then. */
const readSince = async (fixture: Fixture, conv: string, before: string) => (await fixture.input(conv)).slice(before.length);

/** The text of a refused CLI call. A refusal exits non-zero. */
async function refusal(fixture: Fixture, ...args: string[]) {
  try {
    await fixture.cli("agents", "keys", ...args);
  } catch (e) {
    return String((e as Error).message);
  }
  throw new Error(`gx agents keys ${args.join(" ")} was not refused`);
}

/** Counts the screen reads the page makes. */
function countScreenReads(page: Page) {
  let reads = 0;
  page.on("request", (r) => {
    if (r.url().includes("/v1/sessions/screen")) reads++;
  });
  return () => reads;
}

async function openScreen(page: Page, id: string) {
  await go(page, `#/agents/${id}`);
  await failure(page).getByRole("button", { name: "Screen", exact: true }).click();
  await expect(screenDialog(page)).toBeVisible();
  await expect(screenDialog(page).getByLabel("Screen rows")).toBeVisible();
}

test("156. Key bar and Type field in the screen view", async ({ page, start }) => {
  const fixture = await start("agents-failures");
  await page.setViewportSize({ width: 1280, height: 800 });
  // The stalled session is a Working one whose notice offers Screen.
  await expect.poll(async () => (await sessions(fixture)).some((s) => s.failure?.kind === "stalled")).toBe(true);
  const stalled = (await sessions(fixture)).find((s) => s.failure?.kind === "stalled")!;
  const conv = await conversationOf(fixture, stalled.id);
  const reads = countScreenReads(page);
  await openScreen(page, stalled.id);

  // Type sends the text and no Enter; Enter is a key of its own. Each send reads the screen again.
  let before = await fixture.input(conv);
  let seen = reads();
  await typeBox(page).fill("hello");
  await typeButton(page).click();
  await expect.poll(() => readSince(fixture, conv, before)).toBe("hello");
  await expect(typeBox(page)).toHaveValue("");
  await expect.poll(reads).toBeGreaterThan(seen);
  expect(await readSince(fixture, conv, before)).not.toContain("\r");

  // Ctrl+C ends the stalled turn and closes the notice, so it goes last.
  for (const [name, , bytes] of KEYS) {
    before = await fixture.input(conv);
    seen = reads();
    await keyButton(page, name).click();
    await expect.poll(() => readSince(fixture, conv, before)).toBe(bytes);
    if (name !== "Ctrl+C") await expect.poll(reads).toBeGreaterThan(seen);
  }

  await expect.poll(() => stateOf(fixture, stalled.id)).toBe("idle");
});

test("156. Type refuses !x in the screen view", async ({ page, start }) => {
  const fixture = await start("agents-failures");
  await page.setViewportSize({ width: 1280, height: 800 });
  await expect.poll(async () => (await sessions(fixture)).some((s) => s.failure?.kind === "stalled")).toBe(true);
  const stalled = (await sessions(fixture)).find((s) => s.failure?.kind === "stalled")!;
  const conv = await conversationOf(fixture, stalled.id);
  await openScreen(page, stalled.id);

  const before = await fixture.input(conv);
  await typeBox(page).fill("!x");
  await typeButton(page).click();
  await expect(screenDialog(page).getByRole("alert")).toContainText("text may not start with !");
  expect(await readSince(fixture, conv, before)).toBe("");
});

const plain = (s: SessionInfo) => !s.design && !s.queue;

test("156. Keys by session state", async ({ start }) => {
  const fixture = await start("agents-blocked");
  const all = await sessions(fixture);
  const claude = (s: SessionInfo) => plain(s) && s.harness !== "codex";
  const byState: [string, SessionInfo][] = [
    ["working", all.filter((s) => claude(s) && s.status.state === "working").at(-1)!],
    ["idle", (await claudeIdle(fixture))!],
    ["blocked", all.find((s) => claude(s) && s.status.state === "blocked")!],
    ["rate-limited", all.find((s) => claude(s) && s.status.state === "rate-limited")!],
  ];

  // Every key reaches the fake of a session that is live in any of these states.
  for (const [state, session] of byState) {
    expect(session, state).toBeTruthy();
    const conv = await conversationOf(fixture, session.id);
    for (const [, cli, bytes] of KEYS.slice(3, 5)) {
      const before = await fixture.input(conv);
      await fixture.cli("agents", "keys", session.id, "--key", cli);
      await expect.poll(() => readSince(fixture, conv, before), state).toBe(bytes);
    }
    const before = await fixture.input(conv);
    await fixture.cli("agents", "keys", session.id, "--text", "typed");
    await expect.poll(() => readSince(fixture, conv, before), state).toBe("typed");
  }

  // An Exited session refuses with a message; so do text with `!` first, text with a newline, and a Queue agent.
  const exited = all.find((s) => plain(s) && s.status.state === "exited")!;
  expect(await refusal(fixture, exited.id, "--key", "down")).toMatch(/is exited/);
  const idle = byState[1][1];
  const conv = await conversationOf(fixture, idle.id);
  const before = await fixture.input(conv);
  expect(await refusal(fixture, idle.id, "--text", "!x")).toMatch(/may not start with !/);
  expect(await refusal(fixture, idle.id, "--text", "a\nb")).toMatch(/control character/);
  const queue = all.find((s) => s.queue)!;
  expect(await refusal(fixture, queue.id, "--key", "down")).toMatch(/session-read-only|Queue agent/);
  expect(await readSince(fixture, conv, before)).toBe("");
});

test("156. Queue agent session shows no Screen", async ({ page, start }) => {
  const fixture = await start("agents-blocked");
  await page.setViewportSize({ width: 1280, height: 800 });
  const queue = (await sessions(fixture)).find((s) => s.queue)!;
  await go(page, `#/agents/${queue.id}`);
  await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
  await expect(page.getByRole("button", { name: /Screen/ })).toHaveCount(0);
  await expect(page.getByRole("textbox", { name: "Text to type" })).toHaveCount(0);
});

test("156. Error-blocked session, Type and Enter get out", async ({ page, start }) => {
  const fixture = await start("agents-blocked");
  await page.setViewportSize({ width: 1280, height: 800 });
  const errored = (await sessions(fixture)).find((s) => s.failure?.label === "Agent hit an error")!;
  await openScreen(page, errored.id);

  // Screen is the notice's only button, and the composer stays off while the session is Blocked.
  await screenDialog(page).getByRole("button", { name: "Close", exact: true }).click();
  await expect(failure(page).getByRole("button")).toHaveText(["Screen"]);
  await expect(sendButton(page)).toBeDisabled();
  await expect(composer(page).getByText(/^Blocked\./)).toBeVisible();

  await page.getByRole("button", { name: "Screen", exact: true }).click();
  await typeBox(page).fill("go on");
  await typeButton(page).click();
  await keyButton(page, "Enter").click();
  await expect.poll(() => stateOf(fixture, errored.id)).not.toBe("blocked");
});

const BASH_DIALOG = ["────────────────────", "Bash command", "  rg phone-notice", "Do you want to proceed?", "❯ 1. Yes", "  2. No", "Esc to cancel"];
const LONG_ROW = `Select a thing ${"long-row ".repeat(30)}`;

/** Every button of the notice sits inside the viewport, and the notice itself does not overflow. */
async function expectNoticeFits(page: Page, buttons: string[]) {
  const notice = failure(page);
  await expect(notice.getByRole("button")).toHaveText(buttons);
  expect(await notice.evaluate((el) => el.scrollWidth <= el.clientWidth)).toBe(true);
  for (const button of await notice.getByRole("button").all()) {
    const box = (await button.boundingBox())!;
    expect(box.x).toBeGreaterThanOrEqual(0);
    expect(box.x + box.width).toBeLessThanOrEqual(390);
  }
}

test("160. Phone, key input and Blocked notices", async ({ page, start }) => {
  const fixture = await start("agents-blocked");
  await page.setViewportSize({ width: 390, height: 844 });
  const pageFits = () => page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth);

  // A dialog with a long row: the notice offers Close dialog, and the screen rows scroll sideways.
  const dialogSession = (await claudeIdle(fixture, 0))!;
  await fixture.controlClaude(await conversationOf(fixture, dialogSession.id), {
    action: "block", waiting_for: "input needed", dialog: [LONG_ROW, "❯ 1. One", "  2. Two", "Esc to cancel"],
  });
  await go(page, `#/agents/${dialogSession.id}`);
  await expectNoticeFits(page, ["Screen", "Close dialog"]);
  expect(await pageFits()).toBe(true);

  await failure(page).getByRole("button", { name: "Screen", exact: true }).click();
  const rows = screenDialog(page).getByLabel("Screen rows");
  await expect(rows).toContainText("long-row");
  expect(await rows.evaluate((el) => el.scrollWidth > el.clientWidth && getComputedStyle(el).overflowX !== "visible")).toBe(true);
  expect(await pageFits()).toBe(true);

  // The keys are at least 44px tall and wrap onto more than one line.
  const ys = new Set<number>();
  for (const [name] of KEYS) {
    const box = (await keyButton(page, name).boundingBox())!;
    expect(box.height, name).toBeGreaterThanOrEqual(44);
    expect(box.x + box.width, name).toBeLessThanOrEqual(390);
    ys.add(Math.round(box.y));
  }
  expect(ys.size).toBeGreaterThan(1);
  const type = (await typeButton(page).boundingBox())!;
  expect(type.height).toBeGreaterThanOrEqual(44);
  await screenDialog(page).getByRole("button", { name: "Close", exact: true }).click();

  // A permission prompt: Deny and Allow once fit beside Screen.
  const permissionSession = (await claudeIdle(fixture, 1))!;
  await fixture.controlClaude(await conversationOf(fixture, permissionSession.id), { action: "block", waiting_for: "permission prompt", dialog: BASH_DIALOG });
  await go(page, `#/agents/${permissionSession.id}`);
  await expectNoticeFits(page, ["Screen", "Deny", "Allow once"]);
  expect(await pageFits()).toBe(true);
});
