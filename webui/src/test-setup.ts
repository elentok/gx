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

// jsdom has no layout; CodeMirror measures with these.
const emptyRects = () => ({ length: 0, item: () => null, [Symbol.iterator]: [][Symbol.iterator] }) as unknown as DOMRectList;
const zeroRect = () => ({ x: 0, y: 0, top: 0, left: 0, right: 0, bottom: 0, width: 0, height: 0, toJSON() {} }) as DOMRect;
// Some test files run in the node environment, which has no Range.
if (typeof Range !== "undefined") {
  Range.prototype.getClientRects ??= emptyRects;
  Range.prototype.getBoundingClientRect ??= zeroRect;
}

// A page cache entry or a stored filter must never leak into the next test.
afterEach(() => {
  globalThis.sessionStorage?.clear();
  globalThis.localStorage?.clear();
});
