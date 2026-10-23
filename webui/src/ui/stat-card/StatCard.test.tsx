import { describe, expect, it } from "vitest";
import { renderThemed } from "../render";
import { StatCard } from "./StatCard";

const days = ["Mon", "Tue", "Wed", "Thu"];
const one = (values: (number | null)[]) => [{ key: "a", label: "Tickets", tone: "running" as const, values }];

describe("StatCard", () => {
  it("shows the sum and averages over days with data only", () => {
    const { getByText, queryByText } = renderThemed(<StatCard title="Merged" days={days} series={one([3, 5, 0, 0])} />);
    expect(getByText("Merged")).toBeTruthy();
    expect(getByText("8")).toBeTruthy();
    expect(getByText("2.0/day")).toBeTruthy();
    expect(queryByText("partial")).toBeNull();
  });

  it("marks the headline partial and skips the no-data day in the average", () => {
    const { getByText } = renderThemed(<StatCard title="Merged" days={days} series={one([3, null, 5, 0])} />);
    expect(getByText("partial")).toBeTruthy();
    expect(getByText("2.7/day")).toBeTruthy();
  });

  it("uses an overridden headline and small line", () => {
    const { getByText, queryByText } = renderThemed(<StatCard title="Cost" days={days} series={one([3, 5, 0, 0])} headline="$1.20" small="of $10 budget" />);
    expect(getByText("$1.20")).toBeTruthy();
    expect(getByText("of $10 budget")).toBeTruthy();
    expect(queryByText("8")).toBeNull();
  });

  it("lists each series in the legend", () => {
    const series = [...one([1, 1, 1, 1]), { key: "b", label: "Epics", tone: "done" as const, values: [1, 1, 1, 1] }];
    const { getByLabelText } = renderThemed(<StatCard title="Merged" days={days} series={series} chart="stacked" />);
    expect([...getByLabelText("Legend").querySelectorAll("li")].map((li) => li.textContent)).toEqual(["Tickets", "Epics"]);
  });

  it("draws a line chart with one point per day that has data", () => {
    const { container } = renderThemed(<StatCard title="Waiting" days={days} series={one([3, null, 5, 0])} chart="line" />);
    expect(container.querySelectorAll("[data-point]")).toHaveLength(3);
    expect(container.querySelector("polyline")).not.toBeNull();
  });
});
