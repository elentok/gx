import { describe, expect, it } from "vitest";
import { renderThemed } from "../render";
import { TONES } from "../tone";
import { Chip } from "./Chip";

describe("Chip", () => {
  it.each(TONES)("renders its text in tone %s", (tone) => {
    const { getByText } = renderThemed(<Chip tone={tone}>Paused</Chip>);
    expect(getByText("Paused").getAttribute("data-tone")).toBe(tone);
  });

  it("defaults to muted", () => {
    const { getByText } = renderThemed(<Chip>Draining</Chip>);
    expect(getByText("Draining").getAttribute("data-tone")).toBe("muted");
  });
});
