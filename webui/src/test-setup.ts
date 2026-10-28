import { configure } from "@testing-library/react";
import { afterEach } from "vitest";

// findBy and waitFor give up after a second by default, which a machine under load outlasts.
configure({ asyncUtilTimeout: 10_000 });

// jsdom's AbortSignal is not an instance of the AbortSignal that Node's Request checks for, so the
// router's own navigations (a redirect, a click) throw. Dropping the signal only loses cancellation.
const NodeRequest = globalThis.Request;

globalThis.Request = class extends NodeRequest {
  constructor(input: RequestInfo | URL, init?: RequestInit) {
    super(input, init && { ...init, signal: undefined });
  }
};

// jsdom has no ResizeObserver; Chakra's popper (Menu) observes its anchor with one.
globalThis.ResizeObserver ??= class {
  observe() {}
  unobserve() {}
  disconnect() {}
};

// A page cache entry or a stored filter must never leak into the next test.
afterEach(() => {
  globalThis.sessionStorage?.clear();
  globalThis.localStorage?.clear();
});
