import { renderHook } from "@testing-library/react";
import { beforeEach, expect, test } from "vitest";
import { resetBuildBaseline, useBuildChanged } from "./NewVersionStrip";

beforeEach(resetBuildBaseline);

test("the baseline survives a remount, so a navigation keeps the strip", () => {
  const first = renderHook(() => useBuildChanged("old"));
  expect(first.result.current).toBe(false);
  first.unmount();
  const second = renderHook(() => useBuildChanged("new"));
  expect(second.result.current).toBe(true);
});

test("an unchanged build never shows the strip", () => {
  renderHook(() => useBuildChanged("old")).unmount();
  expect(renderHook(() => useBuildChanged("old")).result.current).toBe(false);
});
