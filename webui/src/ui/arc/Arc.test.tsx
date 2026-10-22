import { describe, expect, it } from "vitest";
import { renderThemed } from "../render";
import { TONES } from "../tone";
import { Arc } from "./Arc";

describe("Arc", () => {
  it.each(TONES)("exposes its label in tone %s", (tone) => {
    const { getByRole } = renderThemed(<Arc tone={tone} label="Running" />);
    expect(getByRole("img", { name: "Running" }).getAttribute("data-tone")).toBe(tone);
  });

  it("defaults to running (green)", () => {
    const { getByRole } = renderThemed(<Arc label="Running" />);
    expect(getByRole("img").getAttribute("data-tone")).toBe("running");
  });
});
