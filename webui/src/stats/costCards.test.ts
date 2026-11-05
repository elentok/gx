import { expect, test } from "vitest";
import { compact, costCardProps } from "./costCards";
import type { Stats } from "./stats";

const amount = (value: number, no_data = false) => ({ value, no_data });
const statsOf = (cost: unknown, days = ["a", "b", "c"]) => ({ days, cost }) as unknown as Stats;
const tokens = (stats: Stats) =>
  costCardProps(stats, "Tokens", (g) => g.tokens, (h) => h.tokens, compact, (stamps) => (stamps ? " · stamps" : ""));

test("one no-data group nulls the whole day and the card is partial", () => {
  const cost = {
    groups: [
      { group: "Queue", tokens: [amount(1000), amount(0, true), amount(2000)], spend: [], harness: [] },
      { group: "Other", tokens: [amount(5), amount(0), amount(0)], spend: [], harness: [] },
    ],
    sources: [],
    partial: false,
  };
  const props = tokens(statsOf(cost));
  expect(props.series.map((s) => s.values)).toEqual([
    [1000, null, 2000],
    [5, null, 0],
  ]);
  expect(props.headline).toBe("3K");
  expect(props.partial).toBe(true);
});

test("sources dim rebuilt and stamp days and note them; live and missing stay plain", () => {
  const cost = { groups: [], sources: ["live", "rebuilt", "stamp"], partial: false };
  const props = tokens(statsOf(cost, ["a", "b", "c", "d"]));
  expect(props.dayStyles).toEqual([undefined, "dim", "dim", undefined]);
  expect(props.dayNotes).toEqual([undefined, ["rebuilt from transcripts"], ["ticket stamps only"], undefined]);
  expect(props.small).toBe("0/day · stamps");
});

test("the harness split sums per day and is omitted when both are zero", () => {
  const cost = {
    groups: [{ group: "Queue", tokens: [amount(1), amount(0)], spend: [], harness: [{ claude: { tokens: 700, dollars: 2 }, codex: { tokens: 300, dollars: 0 } }, {}] }],
    sources: [],
    partial: true,
  };
  const props = tokens(statsOf(cost, ["a", "b"]));
  expect(props.extraRows).toEqual([
    [
      { label: "Claude", value: "700" },
      { label: "Codex", value: "300" },
    ],
    undefined,
  ]);
  expect(props.partial).toBe(true);
});
