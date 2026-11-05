import type { Page } from "@playwright/test";
import { created, createButton, go, newForm, promptBox, sessions } from "./agentsKit";
import { expect, nav, test } from "./fixture";

async function openAgentsSettings(page: Page) {
  await page.getByRole("button", nav("Menu")).click();
  await page.getByRole("menuitem", nav("Settings")).click();
  await expect(page.getByRole("heading", { name: "Settings" })).toBeVisible();
  const row = page.getByRole("link", { name: /^Agents Default:/ });
  await expect(row).toContainText("Default: Claude");
  await row.click();
  await expect(page.getByRole("heading", { name: "Agents", exact: true })).toBeVisible();
}

const control = (page: Page) => page.getByRole("group", { name: "Default harness" });
const projectRow = (page: Page, project: string) =>
  page.locator('[data-mock-part="project-list"] > div').filter({ has: page.getByText(project, { exact: true }) });

test("117. Default harness setting", async ({ page, start }) => {
  const fixture = await start("default-harness");
  await page.setViewportSize({ width: 640, height: 900 });
  await openAgentsSettings(page);

  await expect(control(page).getByRole("button", { name: "Claude" })).toHaveAttribute("aria-pressed", "true");

  // Switching reports how many waiting tickets follow and how many running agents keep Claude.
  // The running epic's ticket counts as waiting until its worker claims it.
  await expect
    .poll(async () => fixture.cli("tickets", "show", "demo:running-epic/01"), { timeout: 60_000 })
    .toMatch(/^status: claimed$/m);
  await control(page).getByRole("button", { name: "Codex" }).click();
  await expect(page.locator('[data-mock-part="switch-notice"]')).toHaveText(
    /^Default is now Codex\. 1 waiting ticket and the next design step will use it\. \d+ running agents? keeps? Claude\.$/,
  );
  await expect(control(page).getByRole("button", { name: "Codex" })).toHaveAttribute("aria-pressed", "true");
  const global = await fixture.globalConfig();
  expect(global.agents["default-harness"]).toBe("codex");
  expect(global["agent-runner"], "other keys stay").toBe("pty");
  expect(global.server, "other keys stay").toBeTruthy();

  // A project override appears, then goes; the project's other keys stay.
  await projectRow(page, "dotfiles").getByRole("button", { name: "Override" }).click();
  await expect(projectRow(page, "dotfiles").getByRole("button", { name: "Remove override" })).toBeVisible();
  await expect.poll(async () => (await fixture.projectConfig("dotfiles")).agents?.["default-harness"]).toBe("claude");
  const keys = Object.keys(await fixture.projectConfig("dotfiles")).filter((k) => k !== "agents");
  expect(keys.length, "other keys stay").toBeGreaterThan(0);
  await projectRow(page, "dotfiles").getByRole("button", { name: "Remove override" }).click();
  await expect(projectRow(page, "dotfiles").getByRole("button", { name: "Override" })).toBeVisible();
  await expect.poll(async () => (await fixture.projectConfig("dotfiles")).agents?.["default-harness"]).toBeUndefined();
  expect(Object.keys(await fixture.projectConfig("dotfiles"))).toEqual(expect.arrayContaining(keys));

  // The new session starts on the Default harness; the form has no harness picker.
  const before = await sessions(fixture);
  await go(page, "#/agents");
  await page.getByRole("button", { name: "+ New session", exact: true }).click();
  await expect(newForm(page).getByRole("combobox", { name: "Harness" })).toHaveCount(0);
  await promptBox(page).fill("hello codex");
  await createButton(page).click();
  const fresh = await created(fixture, before);
  expect(fresh.harness).toBe("codex");

  // On a phone the header reads the model name alone; the menu's Agent block holds the tier and effort, and a model switched by hand says so.
  const label = (s: { id: string }) => go(page, `#/agents/${s.id}`);
  const menu = async () => {
    await page.getByRole("button", nav("More actions")).click();
    return page.getByRole("menu", nav("More actions"));
  };
  const closeMenu = async () => {
    // Escape goes to the menu once focus is inside it.
    await expect(async () => {
      await page.keyboard.press("Escape");
      await expect(page.getByRole("menu", nav("More actions"))).toHaveCount(0, { timeout: 500 });
    }).toPass();
  };
  const all = await sessions(fixture);
  const claude = all.find((s) => s.project === "demo" && !s.queue && s.model)!;
  const switched = all.find((s) => s.project === "dotfiles")!;
  await label(claude);
  await expect(page.getByText(new RegExp(`^Claude ${claude.model}$`, "i"))).toBeVisible();
  await expect(await menu()).toContainText(new RegExp(`Worker.*Effort.*${claude.effort}`));
  await closeMenu();
  await label(fresh);
  await expect(page.getByText(new RegExp(`^Codex ${fresh.model}$`, "i"))).toBeVisible();
  await expect(await menu()).toContainText(new RegExp(`Worker.*Effort.*${fresh.effort}`));
  await closeMenu();
  await label(switched);
  await expect(await menu()).toContainText("switched by hand");
});

// 117b (tier without a model) and 117c (headless note) are vitest cases:
// AgentsPage.sessionView.test.tsx and AgentsSettingsPage.test.tsx.

const chips = (page: Page, epic: string) =>
  page.locator(`a[href$="/t/demo:${epic}/01"]`).filter({ hasNotText: "→" }).locator("[data-chips]");
const verdict = (page: Page, epic: string) =>
  page.locator(`a[href$="/t/demo:${epic}/01"]`).filter({ hasNotText: "→" }).locator("[data-verdict]");

test("118. Codex queue rows", async ({ page, start }) => {
  test.setTimeout(240_000);
  const fixture = await start("codex-queue");
  await page.setViewportSize({ width: 640, height: 900 });

  const expectRows = async () => {
    await go(page, "#/e/demo:codex-running");
    await expect(chips(page, "codex-running")).toContainText("Codex");
    // The row shows its pinned note until the worker claims it, which is slow under load.
    await expect(chips(page, "codex-running")).toContainText("Worker", { timeout: 45_000 });
    await go(page, "#/e/demo:claude-running");
    await expect(chips(page, "claude-running")).toContainText("Claude");
    await go(page, "#/e/demo:pinned");
    await expect(chips(page, "pinned")).toContainText("Codex");
    await expect(chips(page, "pinned")).toContainText("pinned");
    await go(page, "#/e/demo:ran-on-claude");
    await expect(chips(page, "ran-on-claude")).toContainText("Claude");
    await expect(chips(page, "ran-on-claude")).toContainText("keeps Claude (ran before)");
  };

  await go(page, "#/e/demo:follows-default");
  // "follows default" is the common case, so the row shows no chip for it.
  await expect(page.locator(`a[href$="/t/demo:follows-default/01"]`).filter({ hasNotText: "→" })).toBeVisible();
  await expect(chips(page, "follows-default")).toHaveCount(0);
  await expectRows();

  await openAgentsSettings(page);
  await control(page).getByRole("button", { name: "Codex" }).click();
  await expect(control(page).getByRole("button", { name: "Codex" })).toHaveAttribute("aria-pressed", "true");

  // The scenario's cap holds all three waiting tickets. One free slot goes to the first in queue order.
  await fixture.patchProjectConfig("demo", { "max-agents": 3 });
  await expect
    .poll(async () => (await sessions(fixture)).filter((s) => (s.ticket ?? "").includes("follows-default")).map((s) => s.harness), {
      timeout: 90_000,
    })
    .toEqual(["codex"]);
  const all = await sessions(fixture);
  const other = all.filter((s) => /pinned|ran-on-claude/.test(s.ticket ?? ""));
  expect(other.map((s) => s.harness)).not.toContain("codex");
  await expectRows();
});

test("118. Codex queue rows, codex missing from PATH", async ({ page, start }) => {
  const fixture = await start("codex-queue", { fakecodex: false });
  // The global cap is 4: take the other waiting tickets off the queue so the pinned one is judged on its harness.
  await fixture.patchProjectConfig("demo", { "max-agents": 5 });
  await fixture.cli("server", "queue", "remove", "demo:follows-default");
  await fixture.cli("server", "queue", "remove", "demo:ran-on-claude");
  await go(page, "#/e/demo:pinned");
  await expect(verdict(page, "pinned")).toContainText("codex is not on PATH");
  const text = (await verdict(page, "pinned").innerText()).trim();
  const explained = await fixture.cli("server", "tickets", "explain", "demo:pinned/01");
  expect(explained).toContain("codex is not on PATH");
  expect(explained).toContain(text.replace(/^.*?(codex is not on PATH)/s, "$1"));
});

test("120. Codex rate-limit wait", async ({ page, start }) => {
  await start("codex-rate-limited");
  const reason = /Codex usage: 99% of the (5h|5-hour|weekly) limit, resets /;

  await go(page, "#/e/demo:codex-waiting-epic");
  await expect(verdict(page, "codex-waiting-epic")).toContainText(reason, { timeout: 30_000 });

  const meta = (epic: string) => page.locator(`a[href$="/t/demo:${epic}/01"] [data-meta]`).first();

  await go(page, "#/e/demo:codex-running-epic");
  await expect(meta("codex-running-epic")).toHaveText(/^running · /);
  await expect(page.getByText("Paused")).toHaveCount(0);
  await go(page, "#/e/demo:claude-running-epic");
  await expect(meta("claude-running-epic")).toHaveText(/^running · /);
  await expect(verdict(page, "claude-running-epic")).toHaveCount(0);

  await go(page, "#/e/demo:codex-waiting-epic");
  await verdict(page, "codex-waiting-epic").locator("..").getByRole("button", { name: "Override" }).click();
  await expect(verdict(page, "codex-waiting-epic")).toHaveCount(0);
  // The click also opens the ticket page, where the run shows as it starts.
  await expect(page.getByRole("img", { name: "Running" })).toBeVisible({ timeout: 30_000 });
  await go(page, "#/e/demo:codex-waiting-epic");
  await expect(meta("codex-waiting-epic")).toHaveText(/^running · /);

  await go(page, "#/e/demo:claude-running-epic");
  await expect(meta("claude-running-epic")).toHaveText(/^running · /);
});

const dwell = { timeout: 30_000 };
const stuck = (page: Page) => page.getByText("Paused · Agent stuck", { exact: true });

async function openBlockedTicket(page: Page, epic: string) {
  await page.getByRole("link", { name: epic }).click();
  await expect(page.getByText("needs answer").first()).toBeVisible(dwell); // oldword:ok the kept legacy rendered status
  await page.locator(`a[href$="/t/demo:${epic}/01"]`).filter({ hasNotText: "→" }).click();
  await expect(stuck(page)).toBeVisible();
  await expect(page.getByRole("group", { name: "Permission request" })).toBeVisible();
}

test("121. Codex error pause", async ({ page, start }) => {
  await start("codex-error");
  await page.getByRole("link", { name: "codex-error-epic" }).click();
  await expect(page.getByText("needs answer").first()).toBeVisible(dwell); // oldword:ok the kept legacy rendered status
  await page.locator('a[href$="/t/demo:codex-error-epic/01"]').filter({ hasNotText: "→" }).click();

  await expect(stuck(page)).toBeVisible();
  await expect(page.getByText(/The 'gx-no-such-model' model is not supported/).first()).toBeVisible();
  await expect(page.getByRole("button", nav("Allow once"))).toHaveCount(0);
  await page.getByRole("form", { name: "Recovery actions" }).getByRole("button", nav("Retry")).click();
  await expect(stuck(page)).toHaveCount(0);
  await expect(page.getByText(/^(Paused|needs-answer)/)).toHaveCount(0); // oldword:ok the kept legacy rendered status
});

test("122. Codex design run", async ({ page, start }) => {
  const fixture = await start("codex-design", { autoAdvanceDelay: "1h" });
  await page.evaluate(() => (location.hash = "#/d/demo:design-epic"));

  // The header names the harness and the model, and never the tier; the tier is in the menu.
  const meta = page.getByTestId("page-header-meta");
  await expect(meta).toHaveText(/^Codex \S+ · \S+$/);
  await expect(meta).not.toContainText(/Light|Worker|Strongest/);
  await page.getByRole("button", nav("More actions")).click();
  await expect(page.getByRole("menu", nav("More actions")).getByText(/^(Light|Worker|Strongest)$/)).toBeVisible();
  await page.keyboard.press("Escape");

  const questions = page.getByRole("group", nav("Questions"));
  await expect(questions.getByText("Q8 - Which store?")).toBeVisible();
  await questions.getByRole("button", { name: /SQLite/ }).click();
  await questions.getByRole("button", { name: /Yes/ }).click();
  await page.getByRole("button", nav("Send answers")).click();

  const cont = page.getByRole("button", { name: /^Continue/ });
  await expect(cont).toBeVisible();
  await expect(async () => {
    await cont.click({ timeout: 2000 });
    await expect(questions.getByText("Q9 - Keep going?")).toBeVisible({ timeout: 2000 });
  }).toPass();

  await page.getByRole("group", nav("View")).getByRole("button", nav("Chat")).click();
  await expect(page.getByText(/^\$gx-design /).first()).toBeVisible();

  const list = JSON.parse(await fixture.cli("agents", "list", "--json")) as { id: string; design?: boolean; harness: string }[];
  const designs = list.filter((s) => s.design);
  expect(designs).toHaveLength(1);
  expect(designs[0].harness).toBe("codex");

  // Rounds are counted when the conversation ends, so end it.
  await fixture.cli("agents", "stop", designs[0].id);
  await expect
    .poll(async () => {
      const stats = JSON.parse(await fixture.cli("server", "stats", "--json")) as { grilling: { questions: { value: number }[] } };
      return stats.grilling.questions.reduce((n, d) => n + d.value, 0);
    })
    .toBeGreaterThan(0);
});

// 119a and 119b: one server, two blocked tickets (Allow once, then Deny with a note).
test("119. Codex approval card", async ({ page, start }) => {
  test.setTimeout(180_000);
  const fixture = await start("codex-blocked");

  await openBlockedTicket(page, "codex-blocked-epic");
  await page.getByRole("button", nav("Allow once")).click();
  await expect(stuck(page)).toHaveCount(0);
  await expect.poll(() => fixture.codexDecisions()).toEqual([{ behavior: "allow", message: "" }]);

  await go(page, "#/");
  await openBlockedTicket(page, "codex-denied-epic");
  await page.getByRole("textbox", nav("Note for Deny (optional)")).fill("not on my machine");
  await page.getByRole("button", nav("Deny")).click();
  await expect(stuck(page)).toHaveCount(0);
  await expect
    .poll(() => fixture.codexDecisions())
    .toEqual([
      { behavior: "allow", message: "" },
      { behavior: "deny", message: "not on my machine" },
    ]);
});
