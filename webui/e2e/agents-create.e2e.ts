import { expect, test } from "./fixture";
import {
  composer,
  created,
  createButton,
  freshVisit,
  go,
  messageBox,
  newForm,
  projectSelect,
  promptBox,
  REPLY,
  row,
  sendButton,
  sessions,
  sidebar,
  switcher,
  switcherFilter,
  titleButton,
  worktreeBox,
} from "./agentsKit";

test("85. Create session", async ({ page, start }) => {
  const fixture = await start("agents");
  const before = await sessions(fixture);
  await page.setViewportSize({ width: 1280, height: 800 });
  await go(page, "#/agents");

  await sidebar(page).getByRole("button", { name: "New session in demo" }).click();
  await expect(projectSelect(page)).toHaveValue("demo");
  await expect(worktreeBox(page)).toHaveValue("Project root");
  await promptBox(page).fill("first words");
  await createButton(page).click();

  const made = await created(fixture, before);
  expect(made.project).toBe("demo");
  await expect(page).toHaveURL(new RegExp(`#/agents/${made.id}$`));
  await expect(newForm(page)).toHaveCount(0);
  await expect(row(page, made)).toHaveAttribute("aria-current", "page");
  await expect(page.getByText("first words", { exact: true })).toBeVisible();
  expect(await fixture.cli("agents", "list")).toContain(made.id);
});

test("86. Create in a new worktree", async ({ page, start }) => {
  const fixture = await start("agents");
  const before = await sessions(fixture);
  await page.setViewportSize({ width: 1280, height: 800 });
  await go(page, "#/agents");

  await sidebar(page).getByRole("button", { name: "New session in demo" }).click();
  await expect(newForm(page)).toBeVisible();

  // A bad name shows the refusal in the form and keeps the input.
  await worktreeBox(page).fill("bad name");
  await page.getByRole("option", { name: /Create worktree/ }).click();
  await createButton(page).click();
  await expect(newForm(page).getByRole("alert")).toBeVisible();
  await expect(worktreeBox(page)).toHaveValue("bad name");
  expect(await sessions(fixture)).toHaveLength(before.length);

  // A first prompt that starts with / is refused too.
  await worktreeBox(page).fill("fresh-tree");
  await page.getByRole("option", { name: /Create worktree/ }).click();
  await promptBox(page).fill("/clear");
  await createButton(page).click();
  await expect(newForm(page).getByRole("alert")).toBeVisible();
  await expect(promptBox(page)).toHaveValue("/clear");
  await expect(worktreeBox(page)).toHaveValue("fresh-tree");
  expect(await sessions(fixture)).toHaveLength(before.length);

  // A good one creates the worktree and runs the session in it.
  await promptBox(page).fill("hello");
  await createButton(page).click();
  const made = await created(fixture, before);
  expect(made.dir).toMatch(/fresh-tree$/);
  await expect(page).toHaveURL(new RegExp(`#/agents/${made.id}$`));
  await expect(newForm(page)).toHaveCount(0);
});

test("87. Create entry points", async ({ page, start }) => {
  const fixture = await start("agents");
  const list = await sessions(fixture);
  const inSite = list.find((s) => s.project === "site" && !s.design)!;
  await page.setViewportSize({ width: 1280, height: 800 });
  const closeForm = async () => {
    await newForm(page).getByRole("button", { name: "Close" }).click();
    await expect(newForm(page)).toHaveCount(0);
  };

  // No session selected: a group's "+" prefills its project.
  await go(page, "#/agents");
  await sidebar(page).getByRole("button", { name: "New session in other" }).click();
  await expect(projectSelect(page)).toHaveValue("other");
  await closeForm();

  // With a session selected, the sidebar button and the switcher row prefill its project.
  await go(page, `#/agents/${inSite.id}`);
  await sidebar(page).getByRole("button", { name: "+ New session" }).click();
  await expect(projectSelect(page)).toHaveValue("site");
  await closeForm();
  await titleButton(page).click();
  await switcher(page).getByRole("option", { name: /\+ New session/ }).click();
  await expect(switcher(page)).toHaveCount(0);
  await expect(projectSelect(page)).toHaveValue("site");
  await closeForm();

  // Cmd-K, then the keyboard to the last row.
  await page.keyboard.press("Control+k");
  await switcherFilter(page).press("ArrowUp");
  await switcherFilter(page).press("Enter");
  await expect(projectSelect(page)).toHaveValue("site");

  // The draft survives closing and a fresh page of the same browser.
  await promptBox(page).fill("a draft to keep");
  await closeForm();
  const second = await freshVisit(page, "/#/agents");
  await second.getByRole("button", { name: "+ New session" }).click();
  await expect(promptBox(second)).toHaveValue("a draft to keep");
  await expect(second.getByRole("dialog", { name: "New session" }).getByRole("combobox", { name: "Project" })).toHaveValue("site");
  await second.close();
  expect(await sessions(fixture)).toHaveLength(list.length);
});

test("88. Phone, Agents page", async ({ page, start }) => {
  const fixture = await start("agents");
  const list = await sessions(fixture);
  const target = list[1];
  await page.setViewportSize({ width: 375, height: 812 });
  const { width, height } = page.viewportSize()!;

  // The list alone, with no session panel beside it.
  await go(page, "#/agents");
  await expect(row(page, target)).toBeVisible();
  await expect(page.getByRole("heading", { name: "Pick a session" })).toHaveCount(0);

  // A row opens the session on its own; "‹" returns to the list.
  await row(page, target).click();
  await expect(page).toHaveURL(new RegExp(`#/agents/${target.id}$`));
  await expect(row(page, target)).toHaveCount(0);
  await expect(composer(page)).toBeVisible();
  await page.getByRole("link", { name: "All sessions" }).click();
  await expect(page).toHaveURL(/#\/agents$/);
  await expect(row(page, target)).toBeVisible();

  // The switcher covers the screen and Close closes it.
  await row(page, target).click();
  await titleButton(page).click();
  await expect(switcher(page)).toBeVisible();
  await expect
    .poll(async () => {
      const box = (await switcher(page).boundingBox())!;
      return [box.x, box.y, box.width, box.height];
    })
    .toEqual([0, 0, width, height]);
  await switcher(page).getByRole("button", { name: "Close" }).click();
  await expect(switcher(page)).toHaveCount(0);

  // Create opens as a sheet at the bottom of the screen.
  await page.getByRole("link", { name: "All sessions" }).click();
  await page.getByRole("button", { name: "+ New session", exact: true }).click();
  await expect(newForm(page)).toBeVisible();
  await expect(page.getByTestId("grab-handle")).toBeVisible();
  await expect
    .poll(async () => {
      const box = (await newForm(page).boundingBox())!;
      return [box.x, Math.round(box.y + box.height), box.width];
    })
    .toEqual([0, height, width]);
  await newForm(page).getByRole("button", { name: "Close" }).click();
  await expect(newForm(page)).toHaveCount(0);
});

test("90. Agents offline", async ({ page, start }) => {
  const fixture = await start("agents");
  const idle = (await sessions(fixture))[0];
  await page.setViewportSize({ width: 1280, height: 800 });
  await go(page, `#/agents/${idle.id}`);
  await expect(page.getByText(REPLY)).toBeVisible();
  await messageBox(page).fill("typed before the drop");
  await sidebar(page).getByRole("button", { name: "New session in demo" }).click();
  await promptBox(page).fill("a first prompt");
  await expect(createButton(page)).toBeEnabled();

  await fixture.stopServer();
  await expect(page.getByText("Offline, reconnecting…")).toBeVisible({ timeout: 15_000 });
  await expect(createButton(page)).toBeDisabled();
  await expect(promptBox(page)).toHaveValue("a first prompt");
  await newForm(page).getByRole("button", { name: "Close" }).click();

  // The last data stays; Send is off with its reason and the draft is kept.
  await expect(row(page, idle)).toBeVisible();
  await expect(page.getByText(REPLY)).toBeVisible();
  await expect(composer(page).getByText("Offline. Send turns on when gx reconnects.")).toBeVisible();
  await expect(sendButton(page)).toBeDisabled();
  await expect(messageBox(page)).toHaveValue("typed before the drop");
});
