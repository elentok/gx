import { cleanup, screen, within } from "@testing-library/react";
import { afterEach, expect, test } from "vitest";
import { renderThemed } from "../ui/render";
import { galleryEntries } from "./registry";

afterEach(cleanup);

const cases = galleryEntries.find((e) => e.name === "EpicPage")?.cases ?? [];

test.each(cases.map((c) => [c.label, c.node] as const))("the %s epic page renders", async (_label, node) => {
  renderThemed(<>{node}</>);
  expect(await screen.findByRole("heading", { level: 1 })).toBeTruthy();
});

const renderCase = (label: string) => renderThemed(<>{cases.find((c) => c.label === label)?.node}</>);

test("the cancelled card case reads cancelled", async () => {
  renderCase("cancelled card");
  const card = (await screen.findByText("03")).closest("[data-indent]") as HTMLElement;
  expect(card.textContent).toContain("cancelled");
});

test("the running fork case indents the fork's card", async () => {
  renderCase("running fork");
  const card = (await screen.findByText("04")).closest("[data-indent]") as HTMLElement;
  expect(card.getAttribute("data-indent")).toBe("1");
});

test("the dirty-trunk case lists its files and both buttons", async () => {
  renderCase("dirty-trunk");
  const notice = await screen.findByTestId("land-block-notice");
  const files = within(notice).getByTestId("land-block-files");
  expect(Array.from(files.querySelectorAll("li")).map((li) => li.textContent)).toEqual(["a.go", "b/c.go", "d.go"]);
  expect(within(notice).getAllByRole("button").map((b) => b.textContent)).toEqual(["Stash, merge and restore", "Merge again"]);
});

test("the needs-repair dialog case opens the fix dialog", async () => {
  renderThemed(<>{cases.find((c) => c.label === "needs-repair dialog")?.node}</>);
  expect(await screen.findByRole("dialog")).toBeTruthy();
});
