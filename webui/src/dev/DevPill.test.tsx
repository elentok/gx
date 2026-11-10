import { cleanup, screen } from "@testing-library/react";
import { afterEach, expect, test } from "vitest";
import type { DevState } from "../generated/server";
import { renderThemed } from "../ui/render";
import { DevPill } from "./DevPill";

afterEach(cleanup);

const cases: [DevState, string, string, boolean][] = [
  ["installing", "Dev · installing", "muted", true],
  ["starting", "Dev · starting", "muted", true],
  ["running", "Dev", "done", false],
  ["down", "Dev · down", "deadlocked", false],
];

test.each(cases)("%s shows its text, tone and spinner", (state, text, tone, spinner) => {
  renderThemed(<DevPill state={state} />);
  const pill = screen.getByText(text, { exact: true });
  expect(pill.getAttribute("data-tone")).toBe(tone);
  expect(!!screen.queryByTestId("dev-pill-spinner")).toBe(spinner);
});

test("no pill when off", () => {
  const { container } = renderThemed(<DevPill state="off" />);
  expect(container.querySelector('[data-part="dev-pill"]')).toBeNull();
});
