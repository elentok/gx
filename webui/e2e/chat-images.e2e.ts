import type { Page } from "@playwright/test";
import { expect, nav, test } from "./fixture";
import { addInput, chatOf, claudeIdle, codexIdle, composer, freshVisit, go, messageBox, png, sendButton, sessions, thumbs } from "./agentsKit";

/** A chat message's thumbnails, and the "Image N of M" buttons that open them. */
const chatThumbs = (page: Page) => page.getByRole("button", { name: /^Image \d+ of \d+/ });
const viewer = (page: Page) => page.getByRole("dialog", { name: "Image" });

const pngs = (n: number) => Array.from({ length: n }, (_, i) => png(i));

async function open(page: Page, id: string) {
  await page.setViewportSize({ width: 1280, height: 800 });
  await go(page, `#/agents/${id}`);
  await expect(composer(page)).toBeVisible();
}

/** The transcript message texts of a chat, as `gx agents chat` lists them. */
const messagesOf = async (fixture: Parameters<typeof chatOf>[0], id: string) =>
  (await chatOf(fixture, id)).messages as { kind: string; text?: string; images?: unknown[] }[];

test("189. Send images", async ({ page, start }) => {
  const fixture = await start("agents");
  const claude = (await claudeIdle(fixture))!;
  await open(page, claude.id);
  const before = (await messagesOf(fixture, claude.id)).length;

  // Images only: the button reads Send and is on, and the chat shows a grid with no text.
  await addInput(page).setInputFiles([png(1)]);
  await expect(thumbs(page)).toHaveCount(1);
  await expect(sendButton(page)).toBeEnabled();
  await sendButton(page).click();
  await expect(thumbs(page)).toHaveCount(0);
  await expect(chatThumbs(page)).toHaveCount(1);
  await expect.poll(async () => (await messagesOf(fixture, claude.id)).length).toBeGreaterThan(before);
  await expect.poll(async () => (await sessions(fixture)).find((s) => s.id === claude.id)!.status.state).toBe("idle");

  // Three images and text: Cmd+Enter at 1280 px sends.
  await addInput(page).setInputFiles(pngs(3));
  await expect(thumbs(page)).toHaveCount(3);
  await messageBox(page).fill("three for you");
  // Send stays off while an upload runs, so enabled means all three are ready.
  await expect(sendButton(page)).toBeEnabled();
  await messageBox(page).press("Meta+Enter");
  await expect(thumbs(page)).toHaveCount(0);
  await expect(page.getByText("three for you")).toBeVisible();
  await expect(chatThumbs(page)).toHaveCount(4);
  await expect.poll(async () => (await messagesOf(fixture, claude.id)).some((m) => m.text?.includes("three for you"))).toBe(true);

  // The chat shows no chip, path or wrapper text.
  await expect(page.getByText("[Image #")).toHaveCount(0);
  await expect(page.getByText("images/")).toHaveCount(0);
  const texts = (await messagesOf(fixture, claude.id)).map((m) => m.text ?? "").join("\n");
  expect(texts).not.toContain("[Image");
  expect(texts).not.toContain("/images/");

  // Codex: the same send.
  const codex = await codexIdle(fixture);
  await open(page, codex.id);
  await addInput(page).setInputFiles(pngs(2));
  await expect(thumbs(page)).toHaveCount(2);
  await messageBox(page).fill("for codex");
  await sendButton(page).click();
  await expect(page.getByText("for codex")).toBeVisible();
  await expect(chatThumbs(page)).toHaveCount(2);
  await expect(page.getByText("[Image #")).toHaveCount(0);
  await expect.poll(async () => (await messagesOf(fixture, codex.id)).some((m) => m.text?.includes("for codex"))).toBe(true);

  // A slash command: the images land in the input before the command.
  const conv = (await chatOf(fixture, claude.id)).conversation_id;
  await open(page, claude.id);
  await addInput(page).setInputFiles(pngs(2));
  await expect(thumbs(page)).toHaveCount(2);
  await messageBox(page).fill("/compact foo");
  await sendButton(page).click();
  await expect(thumbs(page)).toHaveCount(0);
  await expect.poll(async () => (await sessions(fixture)).find((s) => s.id === claude.id)!.status.state).toBe("working");
  const input = await fixture.input(conv);
  const command = input.lastIndexOf("/compact foo");
  expect(command).toBeGreaterThan(0);
  expect(input.lastIndexOf("/images/", command)).toBeGreaterThan(input.lastIndexOf("three for you"));
  expect(input.lastIndexOf("/images/")).toBeLessThan(command);
});

test("189. Send images, design composer round", async ({ page, start }) => {
  const fixture = await start("design", { autoAdvanceDelay: "1h" });
  await page.setViewportSize({ width: 1280, height: 800 });
  await go(page, "#/d/demo:design-epic");
  await page.getByRole("button", nav("Start design session")).click();
  const questions = page.getByRole("group", nav("Questions"));
  await expect(questions.getByText("Q8 - Which store?")).toBeVisible();
  const design = (await sessions(fixture)).find((s) => s.design)!;

  await addInput(page).setInputFiles(pngs(2));
  await expect(thumbs(page)).toHaveCount(2);
  await questions.getByRole("button", { name: /SQLite/ }).click();
  await expect(questions.getByText("Q8b - Migrate at startup?")).toBeVisible();
  await questions.getByRole("button", { name: /Yes/ }).click();
  await expect(questions.getByText("2 of 2 answered")).toBeVisible();
  await page.getByRole("button", nav("Send answers")).click();
  await expect(thumbs(page)).toHaveCount(0);

  // The round's answers reach the agent with both images pasted before them.
  const conv = (await chatOf(fixture, design.id)).conversation_id;
  await expect.poll(async () => (await fixture.input(conv)).split("/images/").length - 1).toBe(2);
  expect(await fixture.input(conv)).toContain("Q8 - Which store?");
});

test("191. Chat images on the agents-images scenario", async ({ page, start }) => {
  const fixture = await start("agents-images");
  const claude = (await sessions(fixture)).find((s) => s.harness !== "codex" && !s.design && !s.queue && s.status.state === "idle")!;
  await open(page, claude.id);

  // The ten-image message shows three thumbnails and a "+7" tile.
  const ten = page.getByRole("button", { name: "7 more" });
  await expect(ten).toBeVisible();
  await expect(ten).toContainText("+7");
  await expect(page.getByRole("button", { name: /^Image \d of 10/ })).toHaveCount(3);

  // The three-image message: tap the second thumbnail.
  await page.getByRole("button", { name: "Image 2 of 3", exact: true }).click();
  await expect(viewer(page)).toBeVisible();
  await expect(viewer(page).getByText("2 of 3")).toBeVisible();
  await expect(viewer(page).getByRole("button", { name: /Prev/ })).toBeEnabled();
  await expect(viewer(page).getByRole("button", { name: /Next/ })).toBeEnabled();
  await viewer(page).getByRole("button", { name: /Close/ }).click();
  await expect(viewer(page)).toHaveCount(0);

  // With ten images, Next reaches the tenth.
  await ten.click();
  await expect(viewer(page).getByText("4 of 10")).toBeVisible();
  for (let i = 5; i <= 10; i++) {
    await viewer(page).getByRole("button", { name: /Next/ }).click();
    await expect(viewer(page).getByText(`${i} of 10`)).toBeVisible();
  }
  await expect(viewer(page).getByRole("button", { name: /Next/ })).toBeDisabled();
  await viewer(page).getByRole("button", { name: /Close/ }).click();

  // The line with a path outside the session folder shows a plain placeholder.
  await expect(page.getByTestId("chat-image-placeholder")).toHaveCount(1);

  // A second tab shows the thumbnails.
  const other = await freshVisit(page, `#/agents/${claude.id}`);
  await expect(other.getByRole("button", { name: "7 more" })).toBeVisible();
  await other.close();

  // Stop and resume the session: they still show.
  await fixture.cli("agents", "stop", claude.id);
  await expect.poll(async () => (await sessions(fixture)).find((s) => s.id === claude.id)!.status.state).toBe("exited");
  const resumed = (await fixture.cli("agents", "resume", claude.id)).trim();
  await expect.poll(async () => (await sessions(fixture)).some((s) => s.id === resumed)).toBe(true);
  await go(page, `#/agents/${resumed}`);
  await expect(page.getByRole("button", { name: "7 more" })).toBeVisible();
});
