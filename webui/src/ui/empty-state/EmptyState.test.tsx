import { describe, expect, it } from "vitest";
import { renderThemed } from "../render";
import { TONES } from "../tone";
import { EmptyState } from "./EmptyState";

describe("EmptyState", () => {
  it.each(TONES)("renders its title in tone %s", (tone) => {
    const { getByText } = renderThemed(<EmptyState tone={tone} title="All quiet" />);
    expect(getByText("All quiet").parentElement?.getAttribute("data-tone")).toBe(tone);
  });

  it("renders the body when given", () => {
    const { getByText } = renderThemed(<EmptyState title="All quiet">Nothing is running.</EmptyState>);
    expect(getByText("Nothing is running.")).toBeTruthy();
  });

  it("renders no body when none is given", () => {
    const { getByText } = renderThemed(<EmptyState title="All quiet" />);
    expect(getByText("All quiet").parentElement?.children).toHaveLength(1);
  });
});
