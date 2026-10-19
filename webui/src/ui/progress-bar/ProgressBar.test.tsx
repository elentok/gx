import { describe, expect, it } from "vitest";
import { renderThemed } from "../render";
import { TONES } from "../tone";
import { ProgressBar } from "./ProgressBar";

describe("ProgressBar", () => {
  it.each(TONES)("renders in tone %s", (tone) => {
    const { getByRole } = renderThemed(<ProgressBar value={3} max={10} tone={tone} />);
    const bar = getByRole("progressbar");
    expect(bar.getAttribute("data-tone")).toBe(tone);
    expect(bar.getAttribute("aria-valuenow")).toBe("3");
    expect(bar.getAttribute("aria-valuemax")).toBe("10");
  });

  it.each([
    [5, 10, "50%"],
    [20, 10, "100%"],
    [-1, 10, "0%"],
    [1, 0, "0%"],
  ])("fills value %d of max %d to %s", (value, max, width) => {
    const { getByRole } = renderThemed(<ProgressBar value={value} max={max} />);
    const fill = getByRole("progressbar").querySelector<HTMLElement>("[data-fill]")!;
    expect(fill.style.width).toBe(width);
  });
});
