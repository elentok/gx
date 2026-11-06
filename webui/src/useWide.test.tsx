import { renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, expect, test, vi } from "vitest";
import { type Device, DeviceContext, useDevice, useWide, WideContext } from "./useWide";

afterEach(() => vi.unstubAllGlobals());

// Stubs matchMedia the way a browser answers at `px` wide (40em and 62em are 640px and 992px).
function stubViewport(px: number) {
  vi.stubGlobal("matchMedia", (q: string) => {
    const min = q === "(min-width: 40em)" ? 640 : q === "(min-width: 62em)" ? 992 : Infinity;
    return { matches: px >= min, addEventListener: () => {}, removeEventListener: () => {} };
  });
}

const forcing =
  (opts: { wide?: boolean; device?: Device }) =>
  ({ children }: { children: ReactNode }) => (
    <WideContext.Provider value={opts.wide}>
      <DeviceContext.Provider value={opts.device}>{children}</DeviceContext.Provider>
    </WideContext.Provider>
  );

test.each([
  [639, "phone", false],
  [640, "tablet", true],
  [991, "tablet", true],
  [992, "desktop", true],
] as const)("at %ipx the device is %s", (px, device, wide) => {
  stubViewport(px);
  expect(renderHook(useDevice).result.current).toBe(device);
  expect(renderHook(useWide).result.current).toBe(wide);
});

test("forced contexts win over the viewport", () => {
  stubViewport(700);
  expect(renderHook(useDevice, { wrapper: forcing({ device: "phone" }) }).result.current).toBe("phone");
  expect(renderHook(useWide, { wrapper: forcing({ device: "phone" }) }).result.current).toBe(false);
  expect(renderHook(useWide, { wrapper: forcing({ wide: false }) }).result.current).toBe(false);
  stubViewport(500);
  expect(renderHook(useWide, { wrapper: forcing({ wide: true }) }).result.current).toBe(true);
  expect(renderHook(useDevice, { wrapper: forcing({ device: "tablet" }) }).result.current).toBe("tablet");
});

test("without matchMedia the device is desktop", () => {
  expect(renderHook(useDevice).result.current).toBe("desktop");
  expect(renderHook(useWide).result.current).toBe(true);
});
