import { expect, test } from "vitest";
import type { EpicListRow } from "../generated/server";
import { waitingCount } from "./list";

const row = (name: string, section: string): EpicListRow =>
  ({ address: `demo:${name}`, addresses: [`demo:${name}`], section }) as EpicListRow;

test("waitingCount leaves out the epic on screen", () => {
  const rows = [
    row("question", "needs-you"),
    row("other-question", "needs-you"),
    row("current", "needs-you"),
    row("loop-epic", "implementing"),
  ];
  expect(waitingCount(rows, "demo:current")).toBe(2);
  expect(waitingCount(rows, "demo:loop-epic")).toBe(3);
  expect(waitingCount([], "demo:current")).toBe(0);
});
