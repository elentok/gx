import { cleanup, screen } from "@testing-library/react";
import { afterEach, expect, test } from "vitest";
import { galleryEntries } from "../gallery/registry";
import { renderThemed } from "../ui/render";

afterEach(cleanup);

test("the gallery renders the draft image row in each state, with its line", () => {
  const entry = galleryEntries.find((e) => e.name === "DraftImages")!;
  const lines = ["Paste or add images (PNG, JPEG, GIF, WebP, up to 20 MB, 10 per draft)", "1 of 10 images", "0 failed · 1 uploading", "2 failed · 1 uploading"];
  expect(entry.cases.map((c) => c.label)).toEqual(["empty", "ready", "uploading", "failed"]);
  entry.cases.forEach((c, i) => {
    renderThemed(<>{c.node}</>);
    expect(screen.getByText(lines[i]!)).toBeTruthy();
    expect(screen.getByRole("button", { name: "Add image" })).toBeTruthy();
    cleanup();
  });
});
