import { renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, expect, test, vi } from "vitest";
import { type PageMode, PageModeContext, useWide, WideContext } from "./useWide";

afterEach(() => vi.unstubAllGlobals());

// Only the 40em query matches, as at a 700px viewport.
function stubViewport700() {
  const queries: string[] = [];
  vi.stubGlobal("matchMedia", (q: string) => {
    queries.push(q);
    return { matches: q === "(min-width: 40em)", addEventListener: () => {}, removeEventListener: () => {} };
  });
  return queries;
}

const inMode =
  (mode: PageMode, forced?: boolean) =>
  ({ children }: { children: ReactNode }) => (
    <WideContext.Provider value={forced}>
      <PageModeContext.Provider value={mode}>{children}</PageModeContext.Provider>
    </WideContext.Provider>
  );

test("a narrow page answers by the 40em query", () => {
  const queries = stubViewport700();
  const { result } = renderHook(useWide, { wrapper: inMode("narrow") });
  expect(result.current).toBe(true);
  expect(new Set(queries)).toEqual(new Set(["(min-width: 40em)"]));
});

test("a wide page answers by the 62em query", () => {
  const queries = stubViewport700();
  const { result } = renderHook(useWide, { wrapper: inMode("wide") });
  expect(result.current).toBe(false);
  expect(new Set(queries)).toEqual(new Set(["(min-width: 62em)"]));
});

test("the gallery override wins over the page mode", () => {
  stubViewport700();
  expect(renderHook(useWide, { wrapper: inMode("narrow", false) }).result.current).toBe(false);
  expect(renderHook(useWide, { wrapper: inMode("wide", true) }).result.current).toBe(true);
});

test("without matchMedia the page is wide", () => {
  expect(renderHook(useWide, { wrapper: inMode("wide") }).result.current).toBe(true);
});
