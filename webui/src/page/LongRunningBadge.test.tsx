import { expect, test } from "vitest";
import { renderThemed } from "../ui/render";
import { LongRunningBadge } from "./LongRunningBadge";

test("renders nothing for an unmarked ticket", () => {
  const { queryByText } = renderThemed(<LongRunningBadge of={{}} />);
  expect(queryByText("Long-running")).toBeNull();
});

test("shows the turn count and the threshold in the tooltip", () => {
  const { getByText } = renderThemed(<LongRunningBadge of={{ long_ticket_turns: 120, long_ticket_threshold: 100 }} />);
  expect(getByText("Long-running").getAttribute("title")).toBe("120 assistant turns (threshold 100)");
});
