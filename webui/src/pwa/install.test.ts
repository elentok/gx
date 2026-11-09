import { act, renderHook } from "@testing-library/react";
import { afterEach,beforeEach, describe, expect, test, vi } from "vitest";

const UA = {
  macChrome: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0 Safari/537.36",
  winEdge: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0 Safari/537.36 Edg/130.0",
  android: "Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0 Mobile Safari/537.36",
  macSafari: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Safari/605.1.15",
  iphone: "Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1",
  iphoneChrome: "Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) CriOS/130.0 Mobile/15E148 Safari/604.1",
};

function fakeNavigator(ua: string, platform: string, maxTouchPoints = 0) {
  Object.defineProperty(navigator, "userAgent", { value: ua, configurable: true });
  Object.defineProperty(navigator, "platform", { value: platform, configurable: true });
  Object.defineProperty(navigator, "maxTouchPoints", { value: maxTouchPoints, configurable: true });
}

function promptEvent(outcome: "accepted" | "dismissed") {
  const e = new Event("beforeinstallprompt", { cancelable: true }) as Event & Record<string, unknown>;
  e.prompt = vi.fn().mockResolvedValue(undefined);
  e.userChoice = Promise.resolve({ outcome });
  return e;
}

// The store listens from import time, so each test gets a fresh module after faking the environment.
async function load() {
  vi.resetModules();
  return import("./install");
}

beforeEach(() => {
  vi.stubGlobal("matchMedia", () => ({ matches: false }));
  fakeNavigator(UA.winEdge, "Win32");
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("with a saved beforeinstallprompt event", () => {
  test.each([
    ["Windows Edge", UA.winEdge, "Win32", true],
    ["Mac Chrome", UA.macChrome, "MacIntel", true],
    ["Android Chrome", UA.android, "Linux armv81", false],
  ])("%s: mode prompt, desktop caption %s", async (_, ua, platform, caption) => {
    fakeNavigator(ua, platform);
    const { getInstallState } = await load();
    expect(getInstallState().mode).toBe("none");
    window.dispatchEvent(promptEvent("accepted"));
    expect(getInstallState()).toEqual({ mode: "prompt", desktopCaption: caption });
  });

  test("accepting replays the event and ends in none", async () => {
    const { getInstallState, promptInstall } = await load();
    const e = promptEvent("accepted");
    window.dispatchEvent(e);
    await promptInstall();
    expect(e.prompt).toHaveBeenCalledOnce();
    expect(getInstallState().mode).toBe("none");
    // a later event does not bring the row back: the app is installed
    window.dispatchEvent(promptEvent("accepted"));
    expect(getInstallState().mode).toBe("none");
  });

  test("dismissing ends in none until the browser fires a new event", async () => {
    const { getInstallState, promptInstall } = await load();
    window.dispatchEvent(promptEvent("dismissed"));
    await promptInstall();
    expect(getInstallState().mode).toBe("none");
    window.dispatchEvent(promptEvent("dismissed"));
    expect(getInstallState().mode).toBe("prompt");
  });

  test("appinstalled ends in none", async () => {
    const { getInstallState } = await load();
    window.dispatchEvent(promptEvent("accepted"));
    window.dispatchEvent(new Event("appinstalled"));
    expect(getInstallState().mode).toBe("none");
  });

  test("standalone gives none", async () => {
    vi.stubGlobal("matchMedia", (q: string) => ({ matches: q === "(display-mode: standalone)" }));
    const { getInstallState } = await load();
    window.dispatchEvent(promptEvent("accepted"));
    expect(getInstallState().mode).toBe("none");
  });
});

describe("with no saved event", () => {
  test.each([
    ["an iPhone", UA.iphone, "iPhone", 5, "ios"],
    ["an iPad sending a Mac user agent", UA.macSafari, "MacIntel", 5, "ios"],
    ["a non-Safari iOS browser", UA.iphoneChrome, "iPhone", 5, "ios"],
    ["macOS Safari without touch", UA.macSafari, "MacIntel", 0, "mac-safari"],
    ["Mac Chrome", UA.macChrome, "MacIntel", 0, "none"],
    ["Windows Edge", UA.winEdge, "Win32", 0, "none"],
  ])("%s gives %s", async (_, ua, platform, touch, mode) => {
    fakeNavigator(ua, platform, touch);
    const { getInstallState } = await load();
    expect(getInstallState()).toEqual({ mode, desktopCaption: false });
  });

  test("an installed iOS home-screen app gives none", async () => {
    fakeNavigator(UA.iphone, "iPhone", 5);
    Object.defineProperty(navigator, "standalone", { value: true, configurable: true });
    try {
      const { getInstallState } = await load();
      expect(getInstallState().mode).toBe("none");
    } finally {
      Reflect.deleteProperty(navigator, "standalone");
    }
  });
});

test("the hook re-renders without a reload when the mode changes", async () => {
  const { useInstall } = await load();
  const { result } = renderHook(() => useInstall());
  expect(result.current.mode).toBe("none");
  act(() => {
    window.dispatchEvent(promptEvent("accepted"));
  });
  expect(result.current.mode).toBe("prompt");
  await act(() => result.current.promptInstall());
  expect(result.current.mode).toBe("none");
});
