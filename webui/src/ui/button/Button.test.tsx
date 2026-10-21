import { describe, expect, it } from "vitest";
import { renderThemed } from "../render";
import { TONES } from "../tone";
import { Button } from "./Button";

describe("Button", () => {
  it.each(TONES)("renders its text in tone %s", (tone) => {
    const { getByRole } = renderThemed(<Button tone={tone}>Queue</Button>);
    expect(getByRole("button", { name: "Queue" }).getAttribute("data-tone")).toBe(tone);
  });

  it.each(["solid", "tinted", "outline", "ghost"] as const)("marks the %s variant", (variant) => {
    const { getByRole } = renderThemed(<Button variant={variant}>Go</Button>);
    expect(getByRole("button").getAttribute("data-variant")).toBe(variant);
  });

  it("defaults to a tinted done button", () => {
    const { getByRole } = renderThemed(<Button>Go</Button>);
    expect(getByRole("button").getAttribute("data-variant")).toBe("tinted");
    expect(getByRole("button").getAttribute("data-tone")).toBe("done");
  });
});
