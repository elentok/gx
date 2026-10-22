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

  it("shows a running arc only when live", () => {
    const live = renderThemed(<TicketLine id="a" title="b" live />);
    expect(live.getByRole("img", { name: "Running" })).toBeTruthy();
    live.unmount();
    expect(renderThemed(<TicketLine id="c" title="d" />).queryByRole("img")).toBeNull();
  });

  it("omits the reason when none is given", () => {
    const { container } = renderThemed(<TicketLine id="a" title="b" />);
    expect(container.querySelector("[data-reason]")).toBeNull();
  });

  it("renders a paused reason in the red paused token and others muted", () => {
    const paused = renderThemed(<TicketLine id="paused-id" title="b" reason="needs a person" tone="paused" />);
    const muted = renderThemed(<TicketLine id="muted-id" title="b" reason="other reason" tone="muted" />);
    const pausedColor = getComputedStyle(paused.getByText("needs a person")).color;
    expect(pausedColor).toBe(getComputedStyle(paused.getByText("paused-id")).color);
    expect(getComputedStyle(muted.getByText("other reason")).color).not.toBe(pausedColor);
  });

  it("clamps the reason to two lines with an ellipsis", () => {
    const { getByText } = renderThemed(<TicketLine id="a" title="b" reason={"long reason ".repeat(50)} />);
    const style = getComputedStyle(getByText(/long reason/));
    expect(style.getPropertyValue("-webkit-line-clamp")).toBe("2");
    expect(style.overflow).toBe("hidden");
  });

  it("puts the reason on the title line when inlineReason is set", () => {
    const { getByText } = renderThemed(<TicketLine id="a" title="the title" reason="1m · $1.37" inlineReason />);
    expect(getByText("1m · $1.37").parentElement).toBe(getByText("the title").parentElement);
  });

  it("puts the reason below the title row by default", () => {
    const { getByText } = renderThemed(<TicketLine id="a" title="the title" reason="1m · $1.37" />);
    expect(getByText("1m · $1.37").parentElement).not.toBe(getByText("the title").parentElement);
  });
});
