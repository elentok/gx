import type { Locator, Page } from "@playwright/test";
import { expect, nav, test } from "./fixture";
import { composer, go, messageBox, sendButton, sessions } from "./agentsKit";

const PNG = Buffer.from("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==", "base64");
// Only the leading bytes matter: the server and the page sniff the type from them.
const JPEG = Buffer.from([0xff, 0xd8, 0xff, 0xe0, 0, 0x10, 0x4a, 0x46, 0x49, 0x46, 0]);
const GIF = Buffer.from("GIF89a\x01\x00\x01\x00\x00\x00\x00;", "latin1");
const WEBP = Buffer.concat([Buffer.from("RIFF"), Buffer.from([0x1a, 0, 0, 0]), Buffer.from("WEBPVP8L")]);

const file = (name: string, mimeType: string, buffer: Buffer) => ({ name, mimeType, buffer });
const png = (n: number) => file(`pic-${n}.png`, "image/png", PNG);

const thumbs = (page: Page) => page.getByRole("button", { name: "Remove image" });
const addInput = (page: Page) => page.getByTestId("image-input");

/** Dispatches a paste or drop carrying `files` on `target`, as the browser does. */
async function dispatchFiles(target: Locator, type: "paste" | "drop", files: ReturnType<typeof file>[]) {
  await expect(target).toBeVisible();
  await target.evaluate(
    (el, { type, files }) => {
      const data = new DataTransfer();
      for (const f of files) data.items.add(new File([Uint8Array.from(atob(f.b64), (c) => c.charCodeAt(0))], f.name, { type: f.mimeType }));
      const init = { bubbles: true, cancelable: true };
      el.dispatchEvent(
        type === "paste" ? new ClipboardEvent("paste", { ...init, clipboardData: data }) : new DragEvent("drop", { ...init, dataTransfer: data }),
      );
    },
    { type, files: files.map((f) => ({ name: f.name, mimeType: f.mimeType, b64: f.buffer.toString("base64") })) },
  );
}

const idleSession = async (page: Page, fixture: Parameters<typeof sessions>[0]) => {
  const idle = (await sessions(fixture)).find((s) => s.status.state === "idle" && !s.design && !s.queue)!;
  await page.setViewportSize({ width: 1280, height: 800 });
  await go(page, `#/agents/${idle.id}`);
  await expect(composer(page)).toBeVisible();
  return idle;
};

test("182. Paste images", async ({ page, start }) => {
  const fixture = await start("agents");
  await idleSession(page, fixture);

  await dispatchFiles(messageBox(page), "paste", [png(1)]);
  await expect(thumbs(page)).toHaveCount(1);

  await dispatchFiles(messageBox(page), "paste", [file("b.jpg", "image/jpeg", JPEG), file("c.gif", "image/gif", GIF), file("d.webp", "image/webp", WEBP)]);
  await expect(thumbs(page)).toHaveCount(4);
  await expect(composer(page).getByText("4 of 10 images")).toBeVisible();
});

test("183. Drop images", async ({ page, start }) => {
  const fixture = await start("agents");
  await idleSession(page, fixture);

  await dispatchFiles(messageBox(page), "drop", [png(1), file("b.jpg", "image/jpeg", JPEG)]);
  await expect(thumbs(page)).toHaveCount(2);
  await expect(composer(page).getByText("2 of 10 images")).toBeVisible();
});

test("184. Add image button", async ({ page, start }) => {
  const fixture = await start("agents");
  const idle = (await sessions(fixture)).find((s) => s.status.state === "idle" && !s.design && !s.queue)!;
  await page.setViewportSize({ width: 390, height: 800 });
  await go(page, `#/agents/${idle.id}`);
  await expect(composer(page).getByRole("button", { name: "Add image" })).toBeVisible();
  await expect(thumbs(page)).toHaveCount(0);

  await addInput(page).setInputFiles([png(1), file("b.jpg", "image/jpeg", JPEG), file("c.gif", "image/gif", GIF)]);
  await expect(thumbs(page)).toHaveCount(3);

  // Busy: Add and the row keep working.
  await fixture.cli("agents", "prompt", idle.id, "keep busy");
  await expect.poll(async () => (await sessions(fixture)).find((s) => s.id === idle.id)!.status.state).toBe("working");
  await addInput(page).setInputFiles([png(2)]);
  await expect(thumbs(page)).toHaveCount(4);
});

test("184. Add image button, design composer", async ({ page, start }) => {
  const fixture = await start("design", { autoAdvanceDelay: "1h" });
  await page.setViewportSize({ width: 1280, height: 800 });
  await go(page, "#/d/demo:design-epic");
  await page.getByRole("button", nav("Start design session")).click();
  await expect(page.getByRole("group", nav("Questions")).getByText("Q8 - Which store?")).toBeVisible();
  const design = (await sessions(fixture)).find((s) => s.design)!;

  await addInput(page).setInputFiles([png(1), png(2)]);
  await expect(thumbs(page)).toHaveCount(2);

  // The same session on the Agents page shows the same thumbnails.
  await go(page, `#/agents/${design.id}`);
  await expect(composer(page)).toBeVisible();
  await expect(thumbs(page)).toHaveCount(2);
});

test("187. Remove image", async ({ page, start }) => {
  const fixture = await start("agents");
  const idle = await idleSession(page, fixture);

  await addInput(page).setInputFiles([png(1), png(2)]);
  await expect(thumbs(page)).toHaveCount(2);
  await expect(composer(page).getByText("2 of 10 images")).toBeVisible();
  await expect.poll(() => fixture.sessionImages(idle.id)).toHaveLength(2);

  await thumbs(page).first().click();
  await expect(thumbs(page)).toHaveCount(1);
  await expect.poll(() => fixture.sessionImages(idle.id)).toHaveLength(1);
});

test("188. Image limits", async ({ page, start }) => {
  const fixture = await start("agents");
  await idleSession(page, fixture);

  await addInput(page).setInputFiles(Array.from({ length: 11 }, (_, i) => png(i)));
  await expect(thumbs(page)).toHaveCount(10);
  await expect(composer(page).getByText("Max 10 images")).toBeVisible();
  await expect(sendButton(page)).toBeVisible();
});
