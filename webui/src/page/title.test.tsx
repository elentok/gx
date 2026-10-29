import { expect, test } from "vitest";
import { DetailPage } from "./DetailPage";
import { formatTitle, type PageTitle } from "./title";

test.each<[PageTitle, string]>([
  [{ kind: "Epics" }, "Epics · gx"],
  [{ kind: "Stats" }, "Stats · gx"],
  [{ kind: "Epic", name: "my-epic" }, "my-epic · Epic · gx"],
  [{ kind: "Design", name: "my-epic" }, "my-epic · Design · gx"],
  [{ kind: "Ticket", name: "demo:mixed-epic/02" }, "demo:mixed-epic/02 · Ticket · gx"],
  [{ kind: "Ticket", name: "" }, "Ticket · gx"],
  [{ kind: "Epic", name: "" }, "Epic · gx"],
  [{ kind: "Design", name: "" }, "Design · gx"],
])("%j formats as %s", (title, expected) => {
  expect(formatTitle(title)).toBe(expected);
});

test("the separator is a space, U+00B7 and a space", () => {
  expect(formatTitle({ kind: "Epic", name: "a" })).toBe("a · Epic · gx");
});

test("a name holding the separator or a colon passes through unchanged", () => {
  expect(formatTitle({ kind: "Ticket", name: "a · b: c" })).toBe("a · b: c · Ticket · gx");
});

test.each(["Epic", "Design", "Ticket"] as const)("the name leads every %s title", (kind) => {
  expect(formatTitle({ kind, name: "the-name" }).startsWith("the-name · ")).toBe(true);
});

// Never rendered: tsc alone enforces that the page frame's title prop stays required.
export function MissingTitle() {
  // @ts-expect-error title is required
  return <DetailPage mode="wide">x</DetailPage>;
}
