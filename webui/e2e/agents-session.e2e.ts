import type { Page } from "@playwright/test";
import { expect, nav, test } from "./fixture";
import { ask, go, messageBox, moreActions, REPLY, sessions, titleHeading } from "./agentsKit";

/** The chat's scroll box, found from a message in it. */
const scrollBox = (page: Page, text: string) => page.getByText(text, { exact: true }).first();
const scrollTo = async (page: Page, text: string, top: boolean) => {
  const message = scrollBox(page, text);
  await expect(message).toBeVisible();
  await message.evaluate((el, toTop) => {
    let box = el.parentElement;
    while (box && getComputedStyle(box).overflowY !== "auto") box = box.parentElement;
    box!.scrollTop = toTop ? 0 : box!.scrollHeight;
  }, top);
};

test("75. Read a session", async ({ page, start }) => {
  const fixture = await start("agents");
  const list = await sessions(fixture);
  const idle = list[0];
  const working = list[1];
  const design = list[2];
  const unread = list.find((s) => s.unread.unread && s.harness === "claude")!;
  await page.setViewportSize({ width: 1280, height: 560 });

  // Header fields and the chat.
  await go(page, `#/agents/${idle.id}`);
  const header = page.getByRole("banner").or(page.locator("header")).last();
  await expect(header.getByRole("img", { name: idle.status.state, exact: true })).toBeVisible();
  await expect(header.getByText(new RegExp(`^${idle.status.state}$`, "i"))).toBeVisible();
  await expect(header.getByText(idle.dir)).toBeVisible();
  await expect(header.getByText(idle.harness, { exact: true })).toHaveCount(0);
  await expect(titleHeading(page)).toContainText(idle.name || idle.id);
  // Promote to Design lives in the "..." menu, not as a header button.
  await expect(header.getByRole("button", { name: "Promote to Design", exact: true })).toHaveCount(0);
  await moreActions(page).click();
  await expect(page.getByRole("menu", nav("More actions")).getByRole("menuitem", { name: "Promote to Design", exact: true })).toBeVisible();
  await expect(async () => {
    await page.keyboard.press("Escape");
    await expect(page.getByRole("menu", nav("More actions"))).toHaveCount(0, { timeout: 500 });
  }).toPass();
  const ticketed = list.find((s) => s.ticket && s.harness === "claude")!;
  await go(page, `#/agents/${ticketed.id}`);
  await expect(header.getByText(ticketed.ticket!, { exact: true })).toBeVisible();
  await go(page, `#/agents/${idle.id}`);
  await expect(page.getByText("hello", { exact: true })).toBeVisible();
  await expect(page.getByText(REPLY)).toBeVisible();
  await go(page, `#/agents/${design.id}`);
  await expect(page.getByRole("link", { name: "Open design page →" })).toBeVisible();

  // Unread clears when the session is viewed.
  expect(unread.unread.unread).toBe(true);
  await go(page, `#/agents/${unread.id}`);
  await expect.poll(async () => !!(await sessions(fixture)).find((s) => s.id === unread.id)!.unread.unread).toBe(false);

  // A Working session shows a plain Working row.
  await go(page, `#/agents/${working.id}`);
  await expect(page.getByRole("status").filter({ hasText: /^Working$/ })).toBeVisible();

  // The chat opens at the newest message and follows new ones until the reader scrolls up.
  await go(page, `#/agents/${idle.id}`);
  for (const n of [1, 2, 3, 4, 5, 6, 7, 8]) await ask(fixture, idle.id, `turn ${n}`);
  const newest = page.getByText(REPLY).last();
  await expect(page.getByText(REPLY)).toHaveCount(9);
  await expect(newest).toBeInViewport();
  await expect(page.getByText("hello", { exact: true })).not.toBeInViewport();
  await ask(fixture, idle.id, "turn 9");
  await expect(page.getByText(REPLY)).toHaveCount(10);
  await expect(newest).toBeInViewport();
  await scrollTo(page, "hello", true);
  await expect(page.getByText("hello", { exact: true })).toBeInViewport();
  await ask(fixture, idle.id, "turn 10");
  await expect(page.getByText(REPLY)).toHaveCount(11);
  await expect(newest).not.toBeInViewport();
  await scrollTo(page, "hello", false);

  // A failed chat read keeps the chat; the next event refetches it.
  let failing = true;
  await page.route("**/v1/sessions/chat*", (route) => (failing ? route.fulfill({ status: 500, body: "boom" }) : route.continue()));
  await ask(fixture, idle.id, "while failing");
  await expect(page.getByText("turn 10", { exact: true })).toBeVisible();
  await expect(page.getByText("while failing", { exact: true })).toHaveCount(0);
  failing = false;
  await ask(fixture, idle.id, "after recovery");
  await expect(page.getByText("while failing", { exact: true })).toHaveCount(1);
  await expect(page.getByText("after recovery", { exact: true })).toBeVisible();

  // Tools in flight: the Working row names the newest call and counts the rest.
  const toolsFixture = await start("agents-tools");
  const tools = await sessions(toolsFixture);
  const row = page.getByRole("status").filter({ hasText: /^Working/ });
  await go(page, `#/agents/${tools[0].id}`);
  await expect(row).toHaveText("Working · running shell: make test");
  await expect(row.getByRole("button")).toHaveCount(0);
  await go(page, `#/agents/${tools[1].id}`);
  await expect(row).toContainText("searching: InFlightTool");
  const more = row.getByRole("button", { name: "(+2 more)" });
  await expect(more).toBeVisible();
  const popover = page.getByLabel("Other tools in flight");
  await more.hover();
  await expect(popover.getByRole("listitem")).toHaveText([/sub-agent\s*Survey the sessions code/, /shell\s*make test/]);
  await page.mouse.move(0, 0);
  await expect(popover).toHaveCount(0);
  // Tab to the button opens it; Escape closes it.
  await more.focus();
  await expect(popover).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(popover).toHaveCount(0);
  const confirm = page.getByRole("alertdialog", nav("Interrupt the agent?"));
  const interrupt = page.getByRole("button", { name: "Interrupt", exact: true });
  const isWorking = async (id: string) => (await sessions(toolsFixture)).find((s) => s.id === id)!.status.state === "working";
  await expect(confirm).toHaveCount(0);
  // The tooltip names the key on a wide screen.
  await interrupt.hover();
  await expect(interrupt).toHaveAttribute("title", /Esc/);

  // Esc closes the command palette first; no confirm opens.
  await page.keyboard.press("ControlOrMeta+e");
  const palette = page.getByRole("dialog", nav("Commands"));
  await expect(palette).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(palette).toHaveCount(0);
  await expect(confirm).toHaveCount(0);

  // Esc from the Message box opens the confirm; Esc closes it and nothing is interrupted.
  await messageBox(page).focus();
  await page.keyboard.press("Escape");
  await expect(confirm).toBeVisible();
  await expect(confirm.getByRole("button", { name: "Interrupt", exact: true })).toBeFocused();
  await page.keyboard.press("Escape");
  await expect(confirm).toHaveCount(0);
  await expect(page.getByRole("status").filter({ hasText: /^Working/ })).toBeVisible();
  expect(await isWorking(tools[1].id)).toBe(true);

  // Esc on the page opens the confirm with Interrupt focused; Enter confirms.
  await messageBox(page).blur();
  await page.keyboard.press("Escape");
  await expect(confirm).toBeVisible();
  await expect(confirm.getByRole("button", { name: "Interrupt", exact: true })).toBeFocused();
  await page.keyboard.press("Enter");
  await expect(confirm).toHaveCount(0);
  await expect(page.getByRole("status").filter({ hasText: /^Working/ })).toHaveCount(0);
  await expect.poll(() => isWorking(tools[1].id)).toBe(false);

  // The Design session: Esc opens the confirm, Esc closes it and the session stays Working.
  await go(page, `#/agents/${tools[2].id}`);
  await expect(page.getByRole("status").filter({ hasText: /^Working/ })).toBeVisible();
  // gx drives this session, so Interrupt is off until the composer is turned on.
  await expect(interrupt).toBeDisabled();
  await page.getByRole("button", { name: "Turn on composer" }).click();
  await page.getByRole("button", { name: "Turn on composer" }).last().click();
  await expect(interrupt).toBeEnabled();
  await page.keyboard.press("Escape");
  await expect(confirm).toBeVisible();
  await expect(confirm.getByRole("button", { name: "Interrupt", exact: true })).toBeFocused();
  await page.keyboard.press("Escape");
  await expect(confirm).toHaveCount(0);
  expect(await isWorking(tools[2].id)).toBe(true);
  // The turn ends under the open confirm: the confirm and the button leave.
  await page.keyboard.press("Escape");
  await expect(confirm).toBeVisible();
  await toolsFixture.cli("agents", "interrupt", tools[2].id);
  await expect(confirm).toHaveCount(0);
  await expect(interrupt).toHaveCount(0);
  await expect.poll(() => isWorking(tools[2].id)).toBe(false);

  // Click Interrupt on a Working Chat session and confirm.
  await go(page, `#/agents/${tools[0].id}`);
  await expect(page.getByRole("status").filter({ hasText: /^Working/ })).toBeVisible();
  await interrupt.click();
  await expect(confirm).toBeVisible();
  await confirm.getByRole("button", { name: "Interrupt", exact: true }).click();
  await expect(confirm).toHaveCount(0);
  await expect(page.getByRole("status").filter({ hasText: /^Working/ })).toHaveCount(0);
  await expect.poll(() => isWorking(tools[0].id)).toBe(false);

  // On the idle session the CLI refuses and Esc does nothing.
  const refused = await toolsFixture.cli("agents", "interrupt", tools[0].id).catch((e: { stdout?: string; stderr?: string }) => `${e.stdout}${e.stderr}`);
  expect(refused).toContain("not working");
  await page.keyboard.press("Escape");
  await expect(confirm).toHaveCount(0);
  await expect(interrupt).toHaveCount(0);

  // The interrupts above ended the turns; the phone steps need fresh Working sessions.
  const fresh = await sessions(await start("agents-tools"));
  await go(page, `#/agents/${fresh[1].id}`);
  await expect(more).toBeVisible();
  // A tap opens it at the phone width.
  await page.setViewportSize({ width: 390, height: 800 });
  await expect(more).toBeVisible();
  await more.dispatchEvent("click");
  await expect(popover).toBeVisible();
  await expect(popover.getByRole("listitem")).toHaveCount(2);
  // A phone has no key: the tooltip does not name Esc and Esc opens nothing.
  await expect(interrupt).toBeVisible();
  await expect(interrupt).not.toHaveAttribute("title", /Esc/);
  await page.keyboard.press("Escape");
  await expect(confirm).toHaveCount(0);

  // An interrupted turn is one dim separator in Claude and Codex, Chat and Design sessions.
  const interrupted = await sessions(await start("agents-interrupted"));
  expect(interrupted).toHaveLength(4);
  await page.setViewportSize({ width: 1280, height: 560 });
  for (const s of interrupted) {
    await go(page, `#/agents/${s.id}`);
    await expect(page.getByText("hello", { exact: true })).toBeVisible();
    await expect(page.getByRole("separator").filter({ hasText: /^Turn interrupted$/ })).toHaveCount(1);
    await expect(page.getByText("[Request interrupted by user]")).toHaveCount(0);
  }
});

// Moved to vitest: 77 and 81 (AgentsPage.composer.test.tsx), 82 (AgentsPage.sessionView.test.tsx),
// 80 and 80 crashed (FailureBanner.test.tsx, AgentsPage.composer.test.tsx failureCases),
// 83 (AgentsPage.composer.test.tsx).

// The hold lasts until the flow releases it; the server then waits out its 1 s buffer.
const holdEnds = { timeout: 60_000 };

test("103. Rate-limit hold", async ({ page, start }) => {
  const fixture = await start("rate-limited");
  const verdict = (epic: string) => page.locator(`a[href$="/t/demo:${epic}/01"]`).filter({ hasNotText: "→" }).locator("[data-verdict]");
  // The pages learn of a verdict change on the connection's 60 s tick, so each try moves the page clock by one.
  await page.clock.install();
  const afterTick = (assertion: () => Promise<void>) =>
    expect(async () => {
      await page.clock.fastForward(60_000);
      await assertion();
    }).toPass({ timeout: 40_000, intervals: [500] });

  await page.getByRole("link", { name: "limited-epic", exact: true }).click();
  await afterTick(() => expect(page.getByText(/rate-limited: resets /).first()).toBeVisible({ timeout: 1_000 }));
  await expect(page.getByText("Paused", { exact: false })).toHaveCount(0);

  await go(page, "#/e/demo:waiting-epic");
  await expect(verdict("waiting-epic")).toContainText("rate-limited");

  // The Agents page has no session list, so the queue agent is opened by its address.
  const [queueAgent] = (await sessions(fixture)).filter((s) => s.id.startsWith("queue-limited-epic"));
  await go(page, `#/agents/${queueAgent.id}`);
  const mark = page.getByTestId("page-header-actions").getByRole("img", { name: "rate-limited" });
  await expect(mark).toBeVisible();

  await fixture.releaseRateLimit();
  await expect(mark).toHaveCount(0, holdEnds);
  await go(page, "#/e/demo:waiting-epic");
  await afterTick(() => expect(verdict("waiting-epic")).toHaveCount(0, { timeout: 1_000 }));
});

test("104. Usage gate and override", async ({ page, start }) => {
  await start("usage-gated");
  const verdict = page.locator('a[href$="/t/demo:gated-epic/01"]').filter({ hasNotText: "→" }).locator("[data-verdict]");

  await go(page, "#/e/demo:gated-epic");
  await expect(verdict).toContainText(/Claude usage: 99% of the weekly limit, resets /);
  await verdict.locator("..").getByRole("button", { name: "Override" }).click();
  await expect(verdict).toHaveCount(0);
});
