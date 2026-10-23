import { fireEvent } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { renderThemed } from "../render";
import { DayChart } from "./DayChart";

const days = ["Mon", "Tue", "Wed"];

describe("DayChart", () => {
  it("stacks two series on one day", () => {
    const { container } = renderThemed(
      <DayChart
        days={days}
        kind="stacked"
        series={[
          { key: "a", label: "A", tone: "running", values: [2, 1, 1] },
          { key: "b", label: "B", tone: "waiting", values: [2, 1, 1] },
        ]}
      />,
    );
    const first = [...container.querySelectorAll<SVGRectElement>('[data-bar="0"]')];
    expect(first.map((r) => r.dataset.series)).toEqual(["a", "b"]);
    const [a, b] = first;
    expect(Number(b.getAttribute("y")) + Number(b.getAttribute("height"))).toBe(Number(a.getAttribute("y")));
  });

  it("hatches a no-data day and leaves a zero day empty", () => {
    const { container } = renderThemed(<DayChart days={days} series={[{ key: "a", label: "A", tone: "running", values: [3, null, 0] }]} />);
    const hatched = container.querySelector<SVGRectElement>("[data-nodata='1']")!;
    expect(hatched.getAttribute("height")).toBe("110");
    expect(hatched.getAttribute("fill")).toMatch(/^url\(#/);
    expect(container.querySelector('[data-bar="1"]')).toBeNull();
    expect(container.querySelector('[data-bar="2"]')).toBeNull();
    expect(container.querySelector("[data-nodata='2']")).toBeNull();
    expect(container.querySelector('[data-bar="0"]')).not.toBeNull();
  });

  it("draws removed bars below the baseline", () => {
    const { container } = renderThemed(
      <DayChart
        days={days}
        kind="diverging"
        series={[
          { key: "added", label: "Added", tone: "done", values: [4, 1, 1] },
          { key: "removed", label: "Removed", tone: "paused", values: [2, 1, 1] },
        ]}
      />,
    );
    const baseline = Number(container.querySelector("[data-baseline]")!.getAttribute("y1"));
    const removed = container.querySelector<SVGRectElement>('[data-bar="0"][data-series="removed"]')!;
    const added = container.querySelector<SVGRectElement>('[data-bar="0"][data-series="added"]')!;
    expect(Number(removed.getAttribute("y"))).toBe(baseline);
    expect(Number(added.getAttribute("y")) + Number(added.getAttribute("height"))).toBe(baseline);
  });

  it("puts side-by-side series next to each other", () => {
    const { container } = renderThemed(
      <DayChart
        days={days}
        kind="side-by-side"
        series={[
          { key: "a", label: "A", tone: "running", values: [1, 1, 1] },
          { key: "b", label: "B", tone: "waiting", values: [1, 1, 1] },
        ]}
      />,
    );
    const [a, b] = [...container.querySelectorAll<SVGRectElement>('[data-bar="0"]')];
    expect(Number(b.getAttribute("x"))).toBeGreaterThan(Number(a.getAttribute("x")));
    expect(a.getAttribute("y")).toBe(b.getAttribute("y"));
  });

  it("shows the date, each value and folded series on hover, and no data for a gap", () => {
    const { container, getByRole, queryByRole } = renderThemed(
      <DayChart
        days={days}
        series={[{ key: "a", label: "Tickets", tone: "running", values: [3, null, 0] }]}
        folded={[{ key: "x", label: "Other: docs", values: [1, null, 0] }]}
      />,
    );
    expect(queryByRole("tooltip")).toBeNull();
    fireEvent.mouseEnter(container.querySelector("[data-hit='0']")!);
    expect(getByRole("tooltip").textContent).toBe("MonTickets: 3Other: docs: 1");
    fireEvent.mouseEnter(container.querySelector("[data-hit='1']")!);
    expect(getByRole("tooltip").textContent).toBe("Tueno data");
  });
});
