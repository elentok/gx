import { vi } from "vitest"

type State = { canGoBack: boolean; canGoForward: boolean }

/** Stubs `window.navigation`, which jsdom lacks. The caller runs `vi.unstubAllGlobals()` in `afterEach`. */
export function installFakeNavigation(initial: Partial<State> = {}) {
  const fake = Object.assign(new EventTarget(), {
    canGoBack: false,
    canGoForward: false,
    ...initial,
  })
  vi.stubGlobal("navigation", fake)
  return {
    set(patch: Partial<State>) {
      Object.assign(fake, patch)
      fake.dispatchEvent(new Event("currententrychange"))
    },
  }
}
