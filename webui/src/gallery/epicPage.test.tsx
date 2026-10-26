import { cleanup, screen } from "@testing-library/react";
import { afterEach, expect, test } from "vitest";
import { renderThemed } from "../ui/render";
import { galleryEntries } from "./registry";

afterEach(cleanup);

const cases = galleryEntries.find((e) => e.name === "EpicPage")?.cases ?? [];

test.each(cases.map((c) => [c.label, c.node] as const))("the %s epic page renders", async (_label, node) => {
  renderThemed(<>{node}</>);
  expect(await screen.findByRole("heading", { level: 1 })).toBeTruthy();
});

test("the needs-repair dialog case opens the fix dialog", async () => {
  renderThemed(<>{cases.find((c) => c.label === "needs-repair dialog")?.node}</>);
  expect(await screen.findByRole("dialog")).toBeTruthy();
});
