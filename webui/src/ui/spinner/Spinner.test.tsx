import { describe, expect, it } from "vitest";
import { renderThemed } from "../render";
import { TONES } from "../tone";
import { Spinner } from "./Spinner";

describe("Spinner", () => {
  it.each(TONES)("announces its label in tone %s", (tone) => {
    const { getByRole } = renderThemed(<Spinner label="Loading things" tone={tone} />);
    expect(getByRole("status", { name: "Loading things" }).getAttribute("data-tone")).toBe(tone);
  });

  it("defaults to muted", () => {
    const { getByRole } = renderThemed(<Spinner label="Loading things" />);
    expect(getByRole("status").getAttribute("data-tone")).toBe("muted");
  });
});
