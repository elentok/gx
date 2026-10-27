import { expect, test } from "vitest";
import { formatAge } from "./cardTime";

const S = 1000;
const M = 60 * S;
const H = 60 * M;
const D = 24 * H;

test.each([
  [59 * S, "<1m"],
  [M, "1m"],
  [59 * M, "59m"],
  [H, "1h"],
  [2 * H, "2h"],
  [2 * H + 5 * M, "2h 5m"],
  [23 * H + 59 * M, "23h 59m"],
  [D, "1d"],
  [6 * D + 23 * H, "6d 23h"],
  [7 * D, "1w"],
  [10 * D, "1w 3d"],
  [29 * D, "4w 1d"],
  [30 * D, "4w"],
  [35 * D, "5w"],
  [-5 * M, "<1m"],
])("%i ms reads %s", (ms, words) => {
  expect(formatAge(ms)).toBe(words);
});
