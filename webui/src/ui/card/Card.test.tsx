import { describe, expect, it } from "vitest";
import { renderThemed } from "../render";
import { TONES } from "../tone";
import { Card } from "./Card";

describe("Card", () => {
  it.each(TONES)("renders title and children in tone %s", (tone) => {
    const { container, getByText } = renderThemed(
      <Card title="Heading" tone={tone}>
        body
      </Card>,
    );
    expect(getByText("Heading")).toBeTruthy();
    expect(getByText("body")).toBeTruthy();
    expect(container.querySelector("[data-tone]")!.getAttribute("data-tone")).toBe(tone);
  });

  it("renders without a title", () => {
    const { getByText } = renderThemed(<Card>only body</Card>);
    expect(getByText("only body")).toBeTruthy();
  });
});
