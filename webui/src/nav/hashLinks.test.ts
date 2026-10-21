import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { routeHashLinkClicks } from "./hashLinks";

const navigate = vi.fn();
let stop: () => void;

beforeEach(() => {
  navigate.mockReset();
  stop = routeHashLinkClicks({ navigate });
  // jsdom cannot navigate away; swallow the default after the router handler has run.
  document.addEventListener("click", swallow);
});

const swallow = (e: Event) => e.preventDefault();

afterEach(() => {
  stop();
  document.removeEventListener("click", swallow);
  document.body.innerHTML = "";
});

function link(attrs: string) {
  document.body.innerHTML = `<a id="l" ${attrs}><span id="inner">go</span></a>`;
  return document.getElementById("inner")!;
}

test("a hash link click goes through the router", () => {
  link('href="#/e/gx:foo"').click();
  expect(navigate).toHaveBeenCalledWith("/e/gx:foo");
});

test("a modified click keeps the browser default", () => {
  link('href="#/e/gx:foo"').dispatchEvent(new MouseEvent("click", { bubbles: true, cancelable: true, ctrlKey: true }));
  expect(navigate).not.toHaveBeenCalled();
});

test("other links keep the browser default", () => {
  link('href="https://example.com/"').click();
  link('href="#/e/gx:foo" target="_blank"').click();
  expect(navigate).not.toHaveBeenCalled();
});
