import type { Page } from "@playwright/test";
import { card, expect, nav, test, type Fixture } from "./fixture";
import { ask, composer, created, freshVisit, go, messageBox, REPLY, sendButton, sessions } from "./agentsKit";

/** Counts the prompts the page posts, so a key that does nothing can be told from one that sent. */
function countPrompts(page: Page) {
  const seen = { count: 0 };
  page.on("request", (r) => {
    if (r.method() === "POST" && r.url().includes("/v1/sessions/prompt")) seen.count++;
  });
  return seen;
}

const waitIdle = (fixture: Fixture, id: string) =>
  expect.poll(async () => (await sessions(fixture)).find((s) => s.id === id)!.status.state).toBe("idle");

test("76. Send and draft", async ({ page, start }) => {
  const fixture = await start("agents");
  const idle = (await sessions(fixture))[0];
  await page.setViewportSize({ width: 1280, height: 800 });
  await go(page, `#/agents/${idle.id}`);

  // The draft survives a new tab of the same browser.
  await messageBox(page).fill("a draft to keep");
  await expect(composer(page).getByText("Draft saved in this browser")).toBeVisible();
  const second = await freshVisit(page, `/#/agents/${idle.id}`);
  await expect(messageBox(second)).toHaveValue("a draft to keep");
  await second.close();

  await sendButton(page).click();
  await expect(messageBox(page)).toHaveValue("");
  await expect(page.getByText("a draft to keep", { exact: true })).toBeVisible();
  await expect.poll(() => fixture.cli("agents", "chat", idle.id)).toContain("a draft to keep");
  await expect(composer(page).getByText("Draft saved in this browser")).toHaveCount(0);
});

test("78. Send refused", async ({ page, start }) => {
  const fixture = await start("agents");
  const idle = (await sessions(fixture))[0];
  await page.setViewportSize({ width: 1280, height: 800 });
  await go(page, `#/agents/${idle.id}`);
  await expect(page.getByText(REPLY)).toBeVisible();

  // The page keeps its old chat while the chat read fails, so the CLI prompt moves the chat under it.
  let failing = true;
  await page.route("**/v1/sessions/chat*", (route) => (failing ? route.fulfill({ status: 500, body: "boom" }) : route.continue()));
  await ask(fixture, idle.id, "moved from the CLI");
  await expect(page.getByText("moved from the CLI", { exact: true })).toHaveCount(0);

  await messageBox(page).fill("my draft");
  const dim = await composer(page).getByText("Draft saved in this browser").evaluate((el) => getComputedStyle(el).color);
  failing = false;
  await sendButton(page).click();
  const reason = composer(page).getByText("the conversation moved; review it first");
  await expect(reason).toBeVisible();
  expect(await reason.evaluate((el) => getComputedStyle(el).color)).not.toBe(dim);
  await expect(page.getByText("moved from the CLI", { exact: true })).toBeVisible();
  await expect(messageBox(page)).toHaveValue("my draft");
  expect(await fixture.cli("agents", "chat", idle.id)).not.toContain("my draft");

  // A send that starts with / is refused by prefix and keeps the draft.
  await messageBox(page).fill("/clear");
  await sendButton(page).click();
  await expect(composer(page).getByText("text may not start with / or !")).toBeVisible();
  await expect(messageBox(page)).toHaveValue("/clear");
});

test("79. Keyboard send", async ({ page, start }) => {
  const fixture = await start("agents");
  const idle = (await sessions(fixture))[0];
  const prompts = countPrompts(page);

  // At 390px only the button sends.
  await page.setViewportSize({ width: 390, height: 800 });
  await go(page, `#/agents/${idle.id}`);
  await messageBox(page).fill("phone text");
  await messageBox(page).press("Control+Enter");
  await expect(messageBox(page)).toHaveValue("phone text");
  expect(prompts.count).toBe(0);
  await sendButton(page).click();
  await expect(messageBox(page)).toHaveValue("");
  await expect(page.getByText("phone text", { exact: true })).toBeVisible();
  await waitIdle(fixture, idle.id);

  // Wide, Enter adds a newline and Ctrl+Enter sends.
  await page.setViewportSize({ width: 1280, height: 800 });
  await messageBox(page).fill("line one");
  await messageBox(page).press("Enter");
  await messageBox(page).pressSequentially("line two");
  await expect(messageBox(page)).toHaveValue("line one\nline two");
  expect(prompts.count).toBe(1);
  await messageBox(page).press("Control+Enter");
  await expect(messageBox(page)).toHaveValue("");
  expect(prompts.count).toBe(2);
  await expect.poll(() => fixture.cli("agents", "chat", idle.id)).toContain("line two");
});

test("79. Keyboard send on the design page", async ({ page, start }) => {
  await start("design", { autoAdvanceDelay: "1h" });
  await page.setViewportSize({ width: 1280, height: 800 });
  await expect(card(page, "design-epic")).toBeVisible();
  await go(page, "#/d/demo:design-epic");
  await page.getByRole("button", nav("Start design session")).click();
  const prompts = countPrompts(page);
  const questions = page.getByRole("group", nav("Questions"));
  const popover = page.getByRole("alertdialog", nav("Unanswered questions"));
  const box = page.getByRole("textbox", { name: "Message" });
  await expect(questions.getByText("Q8 - Which store?")).toBeVisible();

  // Send answers with gaps opens the popover and posts nothing.
  await page.getByRole("button", nav("Note")).click();
  await expect(box).toBeEnabled();
  await box.press("Control+Enter");
  await expect(popover.getByText("2 questions are unanswered")).toBeVisible();
  expect(prompts.count).toBe(0);
  await page.keyboard.press("Escape");
  await expect(popover).toHaveCount(0);

  // With every question answered the key sends the round.
  await questions.getByRole("button", { name: /SQLite/ }).click();
  await questions.getByRole("button", { name: /Yes/ }).click();
  await box.press("Control+Enter");
  await expect.poll(() => prompts.count).toBe(1);
  const cont = page.getByRole("button", { name: /^Continue/ });
  await expect(cont).toBeVisible();

  // The main button is Continue: the key does nothing. The sent round closed the Note box, so it opens again.
  await page.getByRole("button", nav("Note")).click();
  await box.press("Control+Enter");
  await expect(popover).toHaveCount(0);
  await expect(cont).toBeVisible();
  expect(prompts.count).toBe(1);

  // With text the main button is Send, and the key sends it.
  await box.fill("one more thing");
  await expect(page.getByRole("button", nav("Send"))).toBeVisible();
  await box.press("Control+Enter");
  await expect.poll(() => prompts.count).toBe(2);
  await expect(box).toHaveValue("");
});

test("79. Keyboard send while the design works", async ({ page, start }) => {
  await start("design-working");
  await page.setViewportSize({ width: 1280, height: 800 });
  await expect(card(page, "design-epic")).toBeVisible();
  await go(page, "#/d/demo:design-epic");
  await page.getByRole("button", nav("Start design session")).click();
  const prompts = countPrompts(page);
  await expect(page.getByRole("button", nav("Interrupt"))).toBeVisible();

  // The box is off while the agent works, so the key is sent to it directly.
  await page.getByRole("button", nav("Note")).click();
  const box = page.getByRole("textbox", { name: "Message" });
  await expect(box).toBeDisabled();
  await box.dispatchEvent("keydown", { key: "Enter", ctrlKey: true, bubbles: true });
  await expect(page.getByRole("button", nav("Interrupt"))).toBeVisible();
  await expect(page.getByRole("alertdialog")).toHaveCount(0);
  expect(prompts.count).toBe(0);
});

test("91. Removed session draft", async ({ page, start }) => {
  const fixture = await start("agents");
  const exited = (await sessions(fixture)).find((s) => s.status.state === "exited")!;
  await page.setViewportSize({ width: 1280, height: 800 });
  const draftKey = `gx.draft.composer.${exited.id}`;
  const stored = () => page.evaluate((k) => localStorage.getItem(k), draftKey);

  // The draft is written in a tab that is then closed.
  const tab = await freshVisit(page, `/#/agents/${exited.id}`);
  await messageBox(tab).fill("a draft for a dead session");
  await expect(composer(tab).getByText("Exited. Restore it in gx, Agents tab.")).toBeVisible();
  await expect.poll(stored).toBe("a draft for a dead session");
  await tab.close();

  // Removing the session and opening the Epics page drops the draft.
  await fixture.cli("agents", "remove", exited.id);
  await go(page, "#/");
  await expect.poll(stored).toBeNull();

  // A new session starts empty. Ids are random, so it cannot reuse the removed one's label.
  const before = await sessions(fixture);
  await fixture.cli("agents", "create", "--project", "demo", "--name", exited.id, "--prompt", "hello");
  const made = await created(fixture, before);
  await go(page, `#/agents/${made.id}`);
  await expect(messageBox(page)).toHaveValue("");
  await expect(composer(page).getByText("Draft saved in this browser")).toHaveCount(0);
});

test("93. Shared draft", async ({ page, start }) => {
  const fixture = await start("design", { autoAdvanceDelay: "1h" });
  await page.setViewportSize({ width: 1280, height: 800 });
  await go(page, "#/d/demo:design-epic");
  await page.getByRole("button", nav("Start design session")).click();
  const questions = page.getByRole("group", nav("Questions"));
  await expect(questions.getByText("Q8 - Which store?")).toBeVisible();
  const design = (await sessions(fixture)).find((s) => s.design)!;

  // A half-answered round survives opening the design page in a new tab.
  await questions.getByRole("button", { name: /SQLite/ }).click();
  const answered = (p: Page) => p.getByRole("button", { name: /^Question .*, answered$/ });
  await expect(answered(page)).toHaveCount(1);
  const fresh = await freshVisit(page, "/#/d/demo:design-epic");
  await expect(fresh.getByRole("group", nav("Questions"))).toBeVisible();
  await expect(answered(fresh)).toHaveCount(1);
  await fresh.close();

  // Text typed on either page shows live on the other.
  const agentsPage = await freshVisit(page, `/#/agents/${design.id}`);
  await page.getByRole("button", nav("Note")).click();
  const box = page.getByRole("textbox", { name: "Message" });
  await expect(box).toBeEnabled();
  await messageBox(agentsPage).fill("typed on Agents");
  await expect(box).toHaveValue("typed on Agents");
  await box.fill("typed on Design");
  await expect(messageBox(agentsPage)).toHaveValue("typed on Design");

  // Send answers sends it as the note, and both boxes are empty after.
  await questions.getByRole("button", { name: /Yes/ }).click();
  await page.getByRole("button", nav("Send answers")).click();
  await expect.poll(() => fixture.cli("agents", "chat", design.id)).toContain("typed on Design");
  await expect(messageBox(agentsPage)).toHaveValue("");
  await page.getByRole("button", nav("Note")).click();
  await expect(box).toHaveValue("");
  await agentsPage.close();
});

test("114. Read-only Queue agent", async ({ page, start }) => {
  const fixture = await start("queue-agents");
  await expect.poll(async () => (await sessions(fixture)).filter((s) => s.queue).length).toBe(1);
  const queue = (await sessions(fixture)).find((s) => s.queue)!;
  await page.setViewportSize({ width: 1280, height: 800 });
  await go(page, `#/agents/${queue.id}`);

  const line = page.getByText("gx is driving this agent, so the composer is off.");
  const turnOn = page.getByRole("button", { name: "Turn on composer" });
  const dialog = page.getByRole("dialog", { name: "Turn on composer for Queue agent" });
  await expect(line).toBeVisible();
  await expect(sendButton(page)).toBeDisabled();

  // Cancel keeps it off.
  await turnOn.click();
  await expect(dialog.getByRole("button", { name: "Cancel" })).toBeFocused();
  await dialog.getByRole("button", { name: "Cancel" }).click();
  await expect(dialog).toHaveCount(0);
  await expect(line).toBeVisible();
  await expect(sendButton(page)).toBeDisabled();

  // The confirm turns it on.
  await turnOn.click();
  await dialog.getByRole("button", { name: "Turn on composer" }).click();
  await expect(line).toHaveCount(0);
  await expect(turnOn).toHaveCount(0);
  await messageBox(page).fill("hello");
  await expect(messageBox(page)).toHaveValue("hello");

  // Reopening is read-only again.
  const second = await freshVisit(page, `/#/agents/${queue.id}`);
  await expect(second.getByText("gx is driving this agent, so the composer is off.")).toBeVisible();
  await expect(sendButton(second)).toBeDisabled();
  await second.close();
});
