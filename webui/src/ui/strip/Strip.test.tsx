import { describe, expect, it } from "vitest";
import { renderThemed } from "../render";
import { TONES } from "../tone";
import { Strip } from "./Strip";

describe("Strip", () => {
  it.each(TONES)("renders its text in tone %s", (tone) => {
    const { getByText } = renderThemed(<Strip tone={tone}>Offline</Strip>);
    expect(getByText("Offline").getAttribute("data-tone")).toBe(tone);
  });

  it("defaults to muted", () => {
    const { getByText } = renderThemed(<Strip>Offline</Strip>);
    expect(getByText("Offline").getAttribute("data-tone")).toBe("muted");
  });
});
