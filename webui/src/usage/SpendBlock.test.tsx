import { fireEvent, waitFor, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { UsageSpend, UsageSummary } from "../generated/server";
import { UsageModeSpending } from "../generated/server";
import { renderThemed } from "../ui/render";
import { SpendBlock, spendCaveat } from "./SpendBlock";

const show = (spend: UsageSpend) =>
  renderThemed(
    <SpendBlock summary={{ mode: UsageModeSpending, limits: {}, spend } as UsageSummary} />,
  );

const normal: UsageSpend = { today: 4.82, since_sunday: 21.5, since_first: 96.1, first_day: "2026-09-01" };
const short: UsageSpend = {
  today: 1.2,
  since_sunday: 3.4,
  since_sunday_short: true,
  since_first: 3.4,
  since_first_short: true,
  first_day: "2026-10-30",
};
const empty: UsageSpend = { today: 0, since_sunday: 0, since_first: 0 };

describe("SpendBlock", () => {
  it("normal: three rows in order with amounts and no notes", () => {
    const v = show(normal);
    const rows = ["Today", "Since Sunday", "Since the 1st"].map((n) => v.getByRole("group", { name: n }));
    expect(rows.map((r) => r.textContent)).toEqual(["Today$4.82", "Since Sunday$21.50", "Since the 1st$96.10"]);
    expect(v.queryByText(/^since [A-Z]/)).toBeNull();
  });

  it("short ledger: notes on flagged rows only, never Today", () => {
    const v = show(short);
    expect(within(v.getByRole("group", { name: "Today" })).queryByText(/since/)).toBeNull();
    expect(within(v.getByRole("group", { name: "Since Sunday" })).getByText("since Fri, Oct 30")).toBeTruthy();
    expect(within(v.getByRole("group", { name: "Since the 1st" })).getByText("since Fri, Oct 30")).toBeTruthy();
  });

  it("flags drive the notes row by row", () => {
    const v = show({ ...normal, since_first_short: true, first_day: "2026-10-30" });
    expect(within(v.getByRole("group", { name: "Since Sunday" })).queryByText(/since/)).toBeNull();
    expect(within(v.getByRole("group", { name: "Since the 1st" })).getByText("since Fri, Oct 30")).toBeTruthy();
  });

  it("empty: $0.00 three times and no notes", () => {
    const v = show(empty);
    expect(v.getAllByText("$0.00")).toHaveLength(3);
    expect(v.queryByText(/^since [A-Z]/)).toBeNull();
  });

  it("tooltip holds the approved sentence and opens on focus and click", async () => {
    expect(spendCaveat).toBe(
      "Estimated cost of Claude runs started by gx. Not the bill. Codex runs and Claude sessions outside gx are not counted.",
    );
    const v = show(normal);
    const marker = v.getByRole("button", { name: "About gx spend" });
    fireEvent.focus(marker);
    expect(await v.findByText(spendCaveat)).toBeTruthy();
    fireEvent.blur(marker);
    await waitFor(() => expect(v.queryByText(spendCaveat)).toBeNull());
    fireEvent.click(marker);
    expect(await v.findByText(spendCaveat)).toBeTruthy();
  });

  it("shows no budget limit words", () => {
    const v = show(short);
    expect(v.container.textContent).not.toMatch(/budget|limit|soft|hard|over|remaining/i);
  });
});
