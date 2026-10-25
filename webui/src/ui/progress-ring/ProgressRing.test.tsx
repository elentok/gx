import { describe, expect, it } from "vitest";
import { renderThemed } from "../render";
import { KINDS } from "../tone";
import { KIND_LABELS, ProgressRing } from "./ProgressRing";

describe("ProgressRing", () => {
  it.each(["60%", "3/5", "…"])("shows the label %s", (label) => {
    const { getByText } = renderThemed(<ProgressRing progress={0.6} kind="design" label={label} />);
    expect(getByText(label)).toBeTruthy();
  });

  it("shows no label text when empty", () => {
    const { container } = renderThemed(<ProgressRing progress={0} kind="design" />);
    expect(container.textContent).toBe("");
  });

  it.each(KINDS)("exposes the %s icon by name", (kind) => {
    const { getByRole } = renderThemed(<ProgressRing progress={0.5} kind={kind} />);
    expect(getByRole("img", { name: KIND_LABELS[kind] })).toBeTruthy();
  });

  it("draws the comet only when live", () => {
    const still = renderThemed(<ProgressRing progress={0.5} kind="design" />);
    expect(still.queryByTestId("comet")).toBeNull();
    const live = renderThemed(<ProgressRing progress={0.5} kind="design" live />);
    expect(live.getByTestId("comet").querySelectorAll("circle")).toHaveLength(10);
  });

  it("drops the icon badge in the compact size", () => {
    const { queryByRole } = renderThemed(<ProgressRing progress={0.5} kind="design" size="compact" />);
    expect(queryByRole("img")).toBeNull();
  });
});
