import { expect, test } from "vitest";
import { session } from "./agentsTestKit";
import { actionableCount, statusMark } from "./rules";

test("actionableCount ignores Design and Queue sessions and counts a Fix agent", () => {
  const asks = { actionable: { reason: "Pick one" } };
  const sessions = [
    session("own", "p", asks),
    session("fix", "p", { ...asks, ticket: "gx:m/07" }),
    session("design", "p", { ...asks, design: true }),
    session("queue", "p", { ...asks, queue: true, status: { state: "blocked" } }),
  ];
  expect(actionableCount(sessions)).toBe(2);
});

test("statusMark gives Rate-limited its own glyph and tone", () => {
  const limited = statusMark(session("1", "p", { status: { state: "rate-limited" } }));
  const starting = statusMark(session("2", "p", { status: { state: "starting" } }));
  expect(limited.glyph).not.toBe(starting.glyph);
  expect(limited.tone).toBe("paused");
});
