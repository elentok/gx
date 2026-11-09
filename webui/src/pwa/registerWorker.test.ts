import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { getWorkerRegistration, registerWorker, resetWorkerRegistrationForTest } from "./registerWorker";

function fakeServiceWorker(register: () => Promise<unknown>) {
  const spy = vi.fn(register);
  Object.defineProperty(navigator, "serviceWorker", { value: { register: spy }, configurable: true });
  return spy;
}

describe("registerWorker", () => {
  beforeEach(() => {
    vi.stubEnv("DEV", false);
    vi.stubGlobal("isSecureContext", true);
    resetWorkerRegistrationForTest();
  });
  afterEach(() => {
    vi.unstubAllEnvs();
    vi.unstubAllGlobals();
    Reflect.deleteProperty(navigator, "serviceWorker");
  });

  it("registers /sw.js in a secure production page", async () => {
    const register = fakeServiceWorker(async () => ({}));
    await registerWorker();
    expect(register).toHaveBeenCalledWith("/sw.js");
    expect(getWorkerRegistration()).toEqual({ state: "registered" });
  });

  it("skips registration in dev", async () => {
    vi.stubEnv("DEV", true);
    const register = fakeServiceWorker(async () => ({}));
    await registerWorker();
    expect(register).not.toHaveBeenCalled();
    expect(getWorkerRegistration()).toEqual({ state: "idle" });
  });

  it("skips registration in an insecure context", async () => {
    vi.stubGlobal("isSecureContext", false);
    const register = fakeServiceWorker(async () => ({}));
    await registerWorker();
    expect(register).not.toHaveBeenCalled();
  });

  it("swallows a rejection and records a SecurityError by name", async () => {
    fakeServiceWorker(async () => {
      throw new DOMException("untrusted certificate", "SecurityError");
    });
    await expect(registerWorker()).resolves.toBeUndefined();
    expect(getWorkerRegistration()).toEqual({ state: "failed", error: "SecurityError" });
  });
});
