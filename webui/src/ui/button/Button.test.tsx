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

  it.each(["main", "secondary", "ghost"] as const)("renders the %s role at both sizes", (emphasis) => {
    for (const size of ["sm", "md"] as const) {
      const { getByRole, unmount } = renderThemed(
        <Button emphasis={emphasis} size={size}>
          Go
        </Button>,
      );
      const button = getByRole("button", { name: "Go" });
      expect(button.getAttribute("data-emphasis")).toBe(emphasis);
      expect(button.getAttribute("data-size")).toBe(size);
      unmount();
    }
  });

  it("is md by default", () => {
    const { getByRole } = renderThemed(<Button>Go</Button>);
    expect(getByRole("button").getAttribute("data-size")).toBe("md");
  });

  it.each([
    ["solid", "main"],
    ["tinted", "main"],
    ["outline", "secondary"],
    ["ghost", "ghost"],
  ] as const)("renders the old %s variant as %s", (variant, emphasis) => {
    const { getByRole } = renderThemed(<Button variant={variant}>Go</Button>);
    expect(getByRole("button").getAttribute("data-emphasis")).toBe(emphasis);
  });
});
