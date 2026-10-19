import { describe, expect, it } from "vitest";
import { renderThemed } from "../render";
import { TONES } from "../tone";
import { Badge } from "./Badge";

describe("Badge", () => {
  it.each(TONES)("renders its text in tone %s", (tone) => {
    const { getByText } = renderThemed(<Badge tone={tone}>hello</Badge>);
    expect(getByText("hello").getAttribute("data-tone")).toBe(tone);
  });

  it("defaults to muted", () => {
    const { getByText } = renderThemed(<Badge>x</Badge>);
    expect(getByText("x").getAttribute("data-tone")).toBe("muted");
  });
});
