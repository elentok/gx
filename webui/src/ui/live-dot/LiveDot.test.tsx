import { describe, expect, it } from "vitest";
import { renderThemed } from "../render";
import { TONES } from "../tone";
import { LiveDot } from "./LiveDot";

describe("LiveDot", () => {
  it.each(TONES)("exposes its label in tone %s", (tone) => {
    const { getByRole } = renderThemed(<LiveDot tone={tone} label="Connected" />);
    const dot = getByRole("status", { name: "Connected" });
    expect(dot.getAttribute("data-tone")).toBe(tone);
  });

  it("defaults to running (green)", () => {
    const { getByRole } = renderThemed(<LiveDot label="Connected" />);
    expect(getByRole("status").getAttribute("data-tone")).toBe("running");
  });
});
