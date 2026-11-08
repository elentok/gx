import { expect, test } from "vitest";
import { changedRows, sameScreen } from "./screenDiff";

test("changed rows are found by index", () => {
  expect(changedRows(["a", "b", "c"], ["a", "x", "c"])).toEqual([1]);
});

test("added rows count as changed", () => {
  expect(changedRows(["a"], ["a", "b", "c"])).toEqual([1, 2]);
});

test("removed rows leave nothing to mark but are not the same screen", () => {
  expect(changedRows(["a", "b"], ["a"])).toEqual([]);
  expect(sameScreen(["a", "b"], ["a"])).toBe(false);
});

test("identical rows have no changes and are the same screen", () => {
  expect(changedRows(["a", "b"], ["a", "b"])).toEqual([]);
  expect(sameScreen(["a", "b"], ["a", "b"])).toBe(true);
});
