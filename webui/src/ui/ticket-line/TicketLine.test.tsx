import { describe, expect, it } from "vitest";
import { renderThemed } from "../render";
import { TONES } from "../tone";
import { TicketLine } from "./TicketLine";

describe("TicketLine", () => {
  it.each(TONES)("renders id and title in tone %s", (tone) => {
    const { container, getByText } = renderThemed(<TicketLine id="gx:demo/01" title="First" tone={tone} />);
    expect(getByText("gx:demo/01")).toBeTruthy();
    expect(getByText("First")).toBeTruthy();
    expect(container.querySelector("[data-tone]")!.getAttribute("data-tone")).toBe(tone);
  });

  it("omits the reason when none is given", () => {
    const { container } = renderThemed(<TicketLine id="a" title="b" />);
    expect(container.querySelector("[data-reason]")).toBeNull();
  });

  it("clamps the reason to two lines with an ellipsis", () => {
    const { getByText } = renderThemed(<TicketLine id="a" title="b" reason={"long reason ".repeat(50)} />);
    const style = getComputedStyle(getByText(/long reason/));
    expect(style.getPropertyValue("-webkit-line-clamp")).toBe("2");
    expect(style.overflow).toBe("hidden");
  });
});
