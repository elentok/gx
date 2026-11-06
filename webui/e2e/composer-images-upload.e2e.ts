import type { Page, Route } from "@playwright/test";
import { expect, test } from "./fixture";
import { addInput, composer, file, freshVisit, idleSession, messageBox, png, sendButton, thumbs } from "./agentsKit";

type Mode = "pass" | "fail" | "hold";

/** Steers every image upload the page makes: let it through, refuse it, or hold it until `release`. */
async function steerUploads(page: Page) {
  let mode: Mode = "pass";
  const held: Route[] = [];
  await page.route(/\/v1\/sessions\/images\?id=/, async (route) => {
    if (route.request().method() !== "POST" || mode === "pass") return route.fallback();
    if (mode === "fail") return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ refused: true }) });
    held.push(route);
  });
  return {
    set: (m: Mode) => {
      mode = m;
    },
    /** Lets held uploads through, or refuses them when `as` is "fail". */
    release: async (as: "pass" | "fail" = "pass") => {
      for (const route of held.splice(0)) {
        if (as === "pass") await route.continue();
        else await route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify({ refused: true }) });
      }
    },
  };
}

const retryThumb = (page: Page) => composer(page).getByRole("button", { name: "Upload failed, tap to retry" });
const readyThumbs = (page: Page) => composer(page).locator('[data-state="ready"]');

test("195. Upload states", async ({ page, start }) => {
  const fixture = await start("agents");
  await idleSession(page, fixture);
  const uploads = await steerUploads(page);
  await messageBox(page).fill("look at these");
  await expect(sendButton(page)).toBeEnabled();

  uploads.set("fail");
  await addInput(page).setInputFiles([png(1)]);
  await expect(retryThumb(page)).toBeVisible();
  await expect(sendButton(page)).toBeDisabled();

  uploads.set("hold");
  await addInput(page).setInputFiles([png(2)]);
  await expect(composer(page).getByLabel("Uploading")).toBeVisible();
  await expect(composer(page).getByText("1 failed · 1 uploading")).toBeVisible();
  await expect(sendButton(page)).toBeDisabled();

  uploads.set("pass");
  await uploads.release();
  await expect(composer(page).getByText("1 failed · 0 uploading")).toBeVisible();
  await expect(sendButton(page)).toBeDisabled();

  await retryThumb(page).click();
  await expect(readyThumbs(page)).toHaveCount(2);
  await expect(composer(page).getByText("2 of 10 images")).toBeVisible();
  await expect(sendButton(page)).toBeEnabled();
});

test("186. Upload failures", async ({ page, start }) => {
  const fixture = await start("agents");
  await idleSession(page, fixture);
  const uploads = await steerUploads(page);
  const refusal = async (f: ReturnType<typeof file>, text: string) => {
    await addInput(page).setInputFiles([f]);
    // The reason sits on the refused file's own thumbnail; removing it leaves nothing behind.
    await expect(composer(page).locator('[data-state="failed"]').getByText(text, { exact: true })).toBeVisible();
    await thumbs(page).click();
    await expect(thumbs(page)).toHaveCount(0);
  };

  await refusal(file("big.png", "image/png", Buffer.alloc(21 * 1024 * 1024)), "Too large, max 20 MB");
  await refusal(file("logo.svg", "image/svg+xml", Buffer.from('<svg xmlns="http://www.w3.org/2000/svg"/>')), "Not a supported image type");
  await refusal(file("shot.heic", "image/heic", Buffer.from("\0\0\0\x18ftypheic\0\0\0\0mif1heic", "latin1")), "Not a supported image type");
  await refusal(file("notes.png", "image/png", Buffer.from("just some text, not an image")), "Not a supported image type");

  // A good image, then one whose upload fails: the good one stays and sends once the failed one is gone.
  await addInput(page).setInputFiles([png(1)]);
  await expect(readyThumbs(page)).toHaveCount(1);
  uploads.set("fail");
  await addInput(page).setInputFiles([png(2)]);
  await expect(retryThumb(page)).toBeVisible();

  // Tapping the failed upload retries it.
  uploads.set("pass");
  await retryThumb(page).click();
  await expect(readyThumbs(page)).toHaveCount(2);
  await expect(retryThumb(page)).toHaveCount(0);

  // A held upload that then fails.
  uploads.set("hold");
  await addInput(page).setInputFiles([png(3)]);
  await expect(composer(page).getByLabel("Uploading")).toBeVisible();
  await uploads.release("fail");
  await expect(retryThumb(page)).toBeVisible();
  await messageBox(page).fill("two good ones");
  await expect(sendButton(page)).toBeDisabled();

  await composer(page).locator('[data-state="failed"]').getByRole("button", { name: "Remove image" }).click();
  await expect(thumbs(page)).toHaveCount(2);
  await expect(sendButton(page)).toBeEnabled();
  await sendButton(page).click();
  await expect(thumbs(page)).toHaveCount(0);
  await expect(messageBox(page)).toHaveValue("");
});

test("192. Image draft", async ({ page, start }) => {
  const fixture = await start("agents");
  const idle = await idleSession(page, fixture);
  const uploads = await steerUploads(page);

  await addInput(page).setInputFiles([png(1), png(2)]);
  await expect(readyThumbs(page)).toHaveCount(2);

  // A second tab shows the same draft.
  const second = await freshVisit(page, `#/agents/${idle.id}`);
  await expect(readyThumbs(second)).toHaveCount(2);

  // An upload still running in the first tab reads as cut off in a new tab, with remove only.
  uploads.set("hold");
  await addInput(page).setInputFiles([png(3)]);
  await expect(composer(page).getByLabel("Uploading")).toBeVisible();
  const third = await freshVisit(page, `#/agents/${idle.id}`);
  await expect(readyThumbs(third)).toHaveCount(2);
  await expect(composer(third).getByText("Upload cut off")).toBeVisible();
  await expect(retryThumb(third)).toHaveCount(0);

  // Finishing it in the first tab updates the entry in the other tab.
  uploads.set("pass");
  await uploads.release();
  await expect(readyThumbs(page)).toHaveCount(3);
  await expect(readyThumbs(third)).toHaveCount(3);
  await expect(composer(third).getByText("Upload cut off")).toHaveCount(0);
});
