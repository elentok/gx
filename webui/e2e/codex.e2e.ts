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
  await expect(control(page).getByRole("button", { name: "Codex" })).toHaveAttribute("aria-pressed", "false");
  await expect(page.getByText("Codex runs on PTY")).toHaveCount(0);
  await expect(projectRow(page, "gx").getByText("Codex", { exact: true })).toBeVisible();
  await expect(projectRow(page, "gx").getByRole("button", { name: "Remove override" })).toBeVisible();
  for (const inheriting of ["demo", "dotfiles"]) {
    await expect(projectRow(page, inheriting)).toContainText("Inherits global (Claude)");
    await expect(projectRow(page, inheriting).getByRole("button", { name: "Override" })).toBeVisible();
  }

  // Switching reports how many waiting tickets follow and how many running agents keep Claude.
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

  // Every row names its harness, tier and model; a model switched by hand says so.
  const label = (s: { id: string }) => go(page, `#/agents/${s.id}`);
  const all = await sessions(fixture);
  const claude = all.find((s) => s.project === "demo" && !s.queue && s.model)!;
  const switched = all.find((s) => s.project === "dotfiles")!;
  await label(claude);
  await expect(page.getByText("Claude", { exact: true }).first()).toBeVisible();
  await expect(page.getByText("Worker", { exact: true }).first()).toBeVisible();
  await expect(page.getByText(`${claude.model} · ${claude.effort}`).first()).toBeVisible();
  await label(fresh);
  await expect(page.getByText("Codex", { exact: true }).first()).toBeVisible();
  await expect(page.getByText("Worker", { exact: true }).first()).toBeVisible();
  await expect(page.getByText(`${fresh.model} · ${fresh.effort}`).first()).toBeVisible();
  await label(switched);
  await expect(page.getByText("switched by hand").first()).toBeVisible();
});

test("117. Default harness setting, tier without a model", async ({ page, start }) => {
  const fixture = await start("default-harness", { config: { "agents.claude.model": "", "agents.claude.effort": "" } });
  const before = await sessions(fixture);
  await fixture.cli("agents", "create", "--project", "demo", "--harness", "claude", "--name", "silent");
  const silent = await created(fixture, before);
  await go(page, `#/agents/${silent.id}`);
  await expect(page.getByText("Claude", { exact: true }).first()).toBeVisible();
  await expect(page.getByText("Worker · CLI default")).toBeVisible();
});

test("117. Default harness setting, headless note", async ({ page, start }) => {
  await start("default-harness", { config: { "agent-runner": "headless" } });
  await go(page, "#/settings/agents");
  await expect(page.getByText("Codex runs on PTY")).toBeVisible();
});

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
    await expect(chips(page, "codex-running")).toContainText("Worker");
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
  await expect(chips(page, "follows-default")).toContainText("follows default");
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
