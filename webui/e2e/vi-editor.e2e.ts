import type { Page } from "@playwright/test";
import { composer, go, messageBox, sessions } from "./agentsKit";
import { card, expect, type Fixture, nav, test, touch } from "./fixture";

// Most vi-editor behaviour (open keys, closers, Done, backdrop, mode text, bar, vim motions and
// edits) is covered in vitest under src/ui/expandable-text. These flows keep what needs a real
// server or a real browser layout.

const TOLERANCE = 1;

// `gx vimode` changes the server; the page follows, which the menu switch shows.
const setViMode = async (fixture: Fixture, page: Page, on: boolean) => {
  await fixture.cli("vimode", on ? "on" : "off");
  await page.getByRole("button", nav("Menu")).click();
  const item = page.getByRole("menu", nav("Menu")).getByRole("menuitemcheckbox", nav("Vi mode"));
  await expect(item).toHaveAttribute("aria-checked", String(on));
  await page.getByRole("button", nav("Menu")).click();
  await expect(page.getByRole("menu", nav("Menu"))).toHaveCount(0);
};

type Locator = ReturnType<Page["getByRole"]>;

const MESSAGE = "Message to the agent";

// Opens the design-working message box in the large editor.
const openEditor = async (page: Page, width: number, height = 900) => {
  await page.setViewportSize({ width, height });
  await expect(card(page, "design-epic")).toBeVisible();
  await page.evaluate(() => (location.hash = "#/d/demo:design-epic"));
  await page.getByRole("button", nav("Start design session")).click();
  await page.getByRole("button", nav("Note")).click();
  await page.getByRole("button", nav("Open the note in a large editor")).click();
  const dialog = page.getByRole("dialog", nav(MESSAGE));
  await expect(dialog).toBeVisible();
  await expect(dialog.getByRole("textbox", nav(`${MESSAGE} text`))).toBeVisible();
  return dialog;
};

const expectDoneCloses = async (page: Page, dialog: Locator) => {
  const done = dialog.getByRole("button", nav("Done"));
  await expect(done).toBeVisible();
  await done.click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
};

// Opens the design-working editor with `text` in the field and the caret after `caret` characters.
const openWith = async (page: Page, text: string, caret: number) => {
  await expect(card(page, "design-epic")).toBeVisible();
  await page.evaluate(() => (location.hash = "#/d/demo:design-epic"));
  await page.getByRole("button", nav("Start design session")).click();
  // The session is idle for a moment before its first turn starts, and that turn disables the field.
  // Wait the turn out so the field stays enabled for the whole test.
  const working = page.getByRole("status").filter({ hasText: "Working:" });
  await expect(working).toBeVisible();
  await expect(working).toBeHidden({ timeout: 45_000 });
  await page.getByRole("button", nav("Note")).click();
  const field = page.getByRole("textbox", nav("Message"));
  await expect(field).toBeEnabled({ timeout: 45_000 });
  await field.fill(text);
  await field.evaluate((el: HTMLTextAreaElement, at) => el.setSelectionRange(at, at), caret);
  await page.keyboard.press("ControlOrMeta+g");
  const dialog = page.getByRole("dialog", nav(MESSAGE));
  await expect(dialog.getByRole("textbox", nav(`${MESSAGE} text`))).toBeFocused();
  return { field, dialog };
};

test.describe("124. Vi editor: open, close and submit", () => {
  test.use({ viewport: { width: 1280, height: 900 } });

  test("Cmd-Enter in the agent composer's editor closes it and sends", async ({ page, start }) => {
    const fixture = await start("agents");
    const idle = (await sessions(fixture))[0];
    await setViMode(fixture, page, true);
    await go(page, `#/agents/${idle.id}`);
    await messageBox(page).fill("sent from the editor");
    await composer(page).getByRole("button", nav("Open the note in a large editor")).click();
    const dialog = page.getByRole("dialog", { name: /^Message to / });
    await expect(dialog.getByRole("textbox", { name: /text$/ })).toBeFocused();
    const sent = page.waitForRequest((r) => r.method() === "POST" && r.url().includes("/v1/sessions/prompt"));
    await page.keyboard.press("ControlOrMeta+Enter");
    await sent;
    await expect(page.getByRole("dialog")).toHaveCount(0);
    await expect.poll(() => fixture.cli("agents", "chat", idle.id)).toContain("sent from the editor");
  });
});

test.describe("123. Vi editor: layouts and touch bar", () => {
  // Also covers 126 "Cmd-K does not open the Jump menu over the editor".
  test("1280px mouse: a 860 by 680 modal; mode text left of the footer; Cmd-K over it; 1280px touch is at most 680px high", async ({
    page,
    start,
  }) => {
    const fixture = await start("design-working");
    await setViMode(fixture, page, true);
    const dialog = await openEditor(page, 1280, 900);
    const box = (await dialog.boundingBox())!;
    expect(Math.abs(box.width - 860)).toBeLessThanOrEqual(TOLERANCE);
    expect(Math.abs(box.height - 680)).toBeLessThanOrEqual(TOLERANCE);
    await expect(dialog.getByText(MESSAGE, { exact: true })).toBeVisible();
    await expect(dialog.getByText("Saved as you type")).toBeVisible();
    await expect(dialog.getByRole("button", nav("Done"))).toBeVisible();
    await expect(dialog.getByRole("button")).toHaveCount(1);

    const text = dialog.getByRole("status");
    await expect(text).toHaveText("INSERT");
    const saved = (await dialog.getByText("Saved as you type").boundingBox())!;
    expect((await text.boundingBox())!.x).toBeLessThan(saved.x);

    const jump = page.getByRole("dialog", nav("Jump to epic"));
    await page.keyboard.press("ControlOrMeta+k");
    await expect(jump).toHaveCount(0);
    await expect(dialog).toBeVisible();

    // The same open editor on a touch device: the bar shows and the modal is shorter.
    const phone = await page.context().browser()!.newContext({
      ...touch,
      ignoreHTTPSErrors: true,
      viewport: { width: 1280, height: 900 },
      storageState: await page.context().storageState(),
    });
    const second = await phone.newPage();
    await second.goto(page.url());
    await second.getByRole("button", nav("Note")).click();
    await second.getByRole("button", nav("Open the note in a large editor")).click();
    const touched = second.getByRole("dialog", nav(MESSAGE));
    await expect(touched.getByTestId("editor-bar")).toBeVisible();
    expect((await touched.boundingBox())!.height).toBeLessThanOrEqual(680 + TOLERANCE);
    await phone.close();

    await expectDoneCloses(page, dialog);
    await page.keyboard.press("ControlOrMeta+k");
    await expect(jump).toBeVisible();
  });
});

// The editor text, one line per `.cm-line`.
const editorText = async (editable: Locator) => {
  await expect(editable).toBeVisible();
  return editable.evaluate((el) => [...el.querySelectorAll(".cm-line")].map((l) => l.textContent).join("\n"));
};

// [anchor, head] of the editor's selection as character offsets in the text; `\n` counts as one.
const editorSelection = async (editable: Locator) => {
  await expect(editable).toBeVisible();
  return editable.evaluate((el) => {
    const lines = [...el.querySelectorAll(".cm-line")];
    const offset = (node: Node, at: number) => {
      const line = lines.findIndex((l) => l.contains(node));
      let n = 0;
      for (let i = 0; i < line; i++) n += (lines[i].textContent ?? "").length + 1;
      const range = document.createRange();
      range.setStart(lines[line], 0);
      range.setEnd(node, at);
      return n + range.toString().length;
    };
    const s = document.getSelection()!;
    return [offset(s.anchorNode!, s.anchorOffset), offset(s.focusNode!, s.focusOffset)];
  });
};

const head = async (editable: Locator) => (await editorSelection(editable))[1];

test.describe("125. Vi editor: commands", () => {
  // Also covers "a click resets the column that j and k keep".
  test("390px: j moves one screen line inside a wrapped paragraph; a click resets the column j keeps", async ({ page, start }) => {
    const fixture = await start("design-working");
    await page.setViewportSize({ width: 390, height: 800 });
    await setViMode(fixture, page, true);
    const paragraph = "word ".repeat(100).trim();
    const text = `${paragraph}\naaaaaaaaaa\nbbbbbbbbbb\ncccccccccc\ndddddddddd`;
    const { dialog } = await openWith(page, text, 0);
    const editable = dialog.getByRole("textbox", nav(`${MESSAGE} text`));
    const mode = dialog.locator('[data-testid^="vi-mode-"]');
    await page.keyboard.press("Escape");
    await expect(mode).toHaveText("NORMAL");
    await page.keyboard.type("gg");

    const cursor = dialog.locator(".cm-cursor, .cm-fat-cursor").first();
    const before = (await cursor.boundingBox())!;
    await page.keyboard.press("j");
    await expect.poll(() => head(editable)).toBeGreaterThan(0);
    expect(await head(editable)).toBeLessThan(paragraph.length / 2);
    const lineHeight = await editable.locator(".cm-line").first().evaluate((l) => parseFloat(getComputedStyle(l).lineHeight));
    // The cursor element repaints after the selection changes, so wait for it to move.
    await expect
      .poll(async () => Math.abs((await cursor.boundingBox())!.y - before.y - lineHeight))
      .toBeLessThanOrEqual(2);

    // Lines a to d start at paragraph.length + 1 and are 11 characters apart.
    const a = paragraph.length + 1;
    await editable.locator(".cm-line").nth(1).click({ position: { x: 1, y: 5 } });
    await expect.poll(() => head(editable)).toBe(a);
    await page.keyboard.type("5lj");
    await expect.poll(() => head(editable)).toBe(a + 11 + 5);
    await editable.locator(".cm-line").nth(3).click({ position: { x: 1, y: 5 } });
    await expect.poll(() => head(editable)).toBe(a + 22);
    await page.keyboard.press("j");
    await expect.poll(() => head(editable)).toBe(a + 33);
  });
});

// Presses `key` `times` times; after each press focus must sit inside `layer`.
const expectFocusStays = async (page: Page, layer: Locator, key: string, times: number) => {
  for (let i = 0; i < times; i++) {
    await page.keyboard.press(key);
    await expect.poll(() => layer.evaluate((el) => el.contains(document.activeElement))).toBe(true);
  }
};

const PAUSED_TICKET = "#/t/demo:paused-epic/08";
const START_OVER = "Start over reason";

// Opens the start-over dialog on the paused ticket; returns it.
const openStartOver = async (page: Page) => {
  await page.evaluate((hash) => (location.hash = hash), PAUSED_TICKET);
  await page.getByRole("form", { name: "Recovery actions" }).getByRole("button", nav("Start over")).click();
  const dialog = page.getByRole("dialog", nav("Start over"));
  await expect(dialog).toBeVisible();
  return dialog;
};

// Opens the large editor over the Reason field of the start-over dialog.
const openReasonEditor = async (page: Page, dialog: Locator) => {
  await dialog.getByRole("button", nav("Open the note in a large editor")).click();
  const editor = page.getByRole("dialog", nav(START_OVER));
  await expect(editor.getByRole("textbox", nav(`${START_OVER} text`))).toBeFocused();
  return editor;
};

test.describe("126. Vi editor: layers and focus", () => {
  test.use({ viewport: { width: 1280, height: 900 } });

  test("Escape in the editor over the question sheet closes neither", async ({ page, start }) => {
    // Kept as e2e: the Escape routing between the editor and the page's Questions view needs the
    // whole design page, which a component test would only imitate.
    const fixture = await start("design", { autoAdvanceDelay: "1h" });
    await setViMode(fixture, page, true);
    await expect(card(page, "design-epic")).toBeVisible();
    await page.evaluate(() => (location.hash = "#/d/demo:design-epic"));
    await page.getByRole("button", nav("Start design session")).click();
    const questions = page.getByRole("group", nav("Questions"));
    // Answering Q8 moves the sheet on to Q8b.
    await questions.getByRole("button", { name: /SQLite/ }).click();
    await questions.getByRole("button", nav("Note on question 8b")).click();
    await questions.getByRole("button", nav("Open the note in a large editor")).click();
    const editor = page.getByRole("dialog", nav("Note on Q8b - Migrate at startup?"));
    await expect(editor.getByRole("textbox", nav("Note on Q8b - Migrate at startup? text"))).toBeFocused();
    await page.keyboard.press("Escape");
    await expect(editor).toBeVisible();
    await expect(questions).toBeVisible();
    // Escape in the Questions view would show the chat; it did not.
    await expect(page.getByRole("group", nav("View")).getByRole("button", { name: /^Questions/ })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
  });

  // Also covers the touch Tab flows: Tab cycles editor, Esc and Done; Shift-Tab wraps to Done.
  test("Tab and Shift-Tab never leave the editor; Tab leaves no tab character; touch cycles editor, Esc, Done", async ({
    page,
    start,
  }) => {
    const fixture = await start("design-working");
    await setViMode(fixture, page, true);
    const dialog = await openEditor(page, 1280, 900);
    const editable = dialog.getByRole("textbox", nav(`${MESSAGE} text`));
    await expect(editable).toBeFocused();
    await page.keyboard.type("ab");
    await expectFocusStays(page, dialog, "Tab", 6);
    await expectFocusStays(page, dialog, "Shift+Tab", 6);
    await editable.focus();
    await page.keyboard.press("Tab");
    expect(await editable.evaluate((el) => el.textContent)).not.toContain("\t");
    await expect(editable).toHaveText("ab");

    // The same open editor on a touch device, where the bar adds Esc and Done to the cycle.
    const phone = await page.context().browser()!.newContext({
      ...touch,
      ignoreHTTPSErrors: true,
      viewport: { width: 1280, height: 900 },
      storageState: await page.context().storageState(),
    });
    const second = await phone.newPage();
    await second.goto(page.url());
    // The "ab" draft is shared across tabs, so the Note box is already open here.
    await second.getByRole("button", nav("Open the note in a large editor")).click();
    const touched = second.getByRole("dialog", nav(MESSAGE));
    const touchedEditable = touched.getByRole("textbox", nav(`${MESSAGE} text`));
    await expect(touchedEditable).toBeFocused();
    const seen = new Set<string>();
    for (let i = 0; i < 6; i++) {
      await second.keyboard.press("Tab");
      await expect.poll(() => touched.evaluate((el) => el.contains(document.activeElement))).toBe(true);
      seen.add(await second.evaluate(() => document.activeElement?.textContent || document.activeElement?.getAttribute("aria-label") || "editor"));
    }
    expect([...seen]).toEqual(expect.arrayContaining(["Done", "Esc"]));
    await expectFocusStays(second, touched, "Shift+Tab", 6);
    await touchedEditable.focus();
    await second.keyboard.press("Shift+Tab");
    await expect(touched.getByRole("button", { name: "Done" })).toBeFocused();
    await second.keyboard.press("Tab");
    await expect(touchedEditable).toBeFocused();
    await phone.close();
  });

  // Also covers "a backdrop press closes only the editor over the start-over dialog" and "Tab never
  // leaves a plain modal".
  test("Escape closes the start-over dialog, Tab stays in it, and the editor over it keeps both open", async ({
    page,
    start,
  }) => {
    const fixture = await start("paused");
    await setViMode(fixture, page, true);
    const dialog = await openStartOver(page);
    await expectFocusStays(page, dialog, "Tab", 8);
    await expectFocusStays(page, dialog, "Shift+Tab", 8);
    await page.keyboard.press("Escape");
    await expect(dialog).toHaveCount(0);

    const again = await openStartOver(page);
    const editor = await openReasonEditor(page, again);
    await page.keyboard.press("Escape");
    await expect(editor).toBeVisible();
    await expect(again).toBeVisible();

    // Last, so a failure here hides nothing above. The editor's backdrop starts below the top bar,
    // so the press lands beside the 860px panel, mid-height, not in the corner.
    await page.mouse.click(5, 450);
    await expect(editor).toHaveCount(0);
    await expect(again).toBeVisible();
  });
});

const menuSwitch = (page: Page) => page.getByRole("menu", nav("Menu")).getByRole("menuitemcheckbox", nav("Vi mode"));

test.describe("127. Vi editor: setting", () => {
  test.use({ viewport: { width: 1280, height: 900 } });

  // Also covers "the menu has a Vi mode switch that turns on" and "a second page of the same login
  // shows the switch on".
  test("flipping the setting with the editor open shows and hides vi at once", async ({ page, start }) => {
    const fixture = await start("design-working");
    await expect(card(page, "design-epic")).toBeVisible();
    await page.getByRole("button", nav("Menu")).click();
    const item = menuSwitch(page);
    await expect(item).toHaveAttribute("aria-checked", "false");
    await item.click();
    await expect(item).toHaveAttribute("aria-checked", "true");
    const second = await page.context().newPage();
    await second.goto(page.url());
    await expect(card(second, "design-epic")).toBeVisible();
    await second.getByRole("button", nav("Menu")).click();
    await expect(menuSwitch(second)).toHaveAttribute("aria-checked", "true");
    await second.close();
    await page.getByRole("button", nav("Menu")).click();
    await expect(page.getByRole("menu", nav("Menu"))).toHaveCount(0);

    const { dialog } = await openWith(page, "hello", 2);
    const editable = dialog.getByRole("textbox", nav(`${MESSAGE} text`));
    const mode = dialog.getByTestId("vi-mode-text");
    const same = async () => {
      expect(await editorText(editable)).toBe("hello");
      expect(await editorSelection(editable)).toEqual([2, 2]);
    };
    await expect(mode).toHaveText("INSERT");
    await same();

    await fixture.cli("vimode", "off");
    await expect(mode).toHaveCount(0);
    await same();
    // Without vi, Escape does nothing and `x` types a letter.
    await page.keyboard.press("Escape");
    await page.keyboard.press("x");
    await expect(dialog).toBeVisible();
    expect(await editorText(editable)).toBe("hexllo");
    await page.keyboard.press("Backspace");
    await same();

    await fixture.cli("vimode", "on");
    await expect(mode).toHaveText("INSERT");
    await same();
    await page.keyboard.press("Escape");
    await expect(mode).toHaveText("NORMAL");
    await page.keyboard.press("x");
    // Vi `x` deletes the character under the caret instead of typing.
    expect(await editorText(editable)).toBe("hllo");
  });
});
