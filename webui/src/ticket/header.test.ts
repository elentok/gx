import { expect, test } from "vitest";
import type { TicketDetail } from "../generated/server";
import { statValues } from "./header";

const detail = (stats: NonNullable<TicketDetail["stats"]>) => ({ type: "implement", stats }) as unknown as TicketDetail;

test("a Codex ticket shows a dash for its cost", () => {
  expect(statValues(detail({ elapsed: 180, cost: 0.4, had_codex: true }), 0)).toEqual(["implement", "3m", "Cost —"]);
});

test("a Claude ticket keeps its dollars", () => {
  expect(statValues(detail({ elapsed: 180, cost: 0.4 }), 0)).toEqual(["implement", "3m", "$0.40"]);
});
