import { cleanup, render } from "@testing-library/react";
import { afterEach, expect, test } from "vitest";
import { useIsTopLayer } from "../ui/modal/layers";
import { escOpensInterrupt } from "./escOpensInterrupt";

afterEach(() => {
  cleanup();
  document.body.innerHTML = "";
});

function Layer() {
  useIsTopLayer();
  return null;
}

function add(html: string): HTMLElement {
  const host = document.createElement("div");
  host.innerHTML = html;
  document.body.append(host);
  return host;
}

test("nothing on top: opens", () => {
  const host = add("<textarea></textarea>");
  expect(escOpensInterrupt(host.firstElementChild)).toBe(true);
  expect(escOpensInterrupt(null)).toBe(true);
});

test.each(["dialog", "alertdialog", "menu", "listbox"])("an open %s takes the Esc", (role) => {
  add(`<div role="${role}"></div>`);
  expect(escOpensInterrupt(document.body)).toBe(false);
});

test("the Questions view takes the Esc", () => {
  add("<div data-questions-view></div>");
  expect(escOpensInterrupt(document.body)).toBe(false);
});

test("a mounted layer takes the Esc", () => {
  render(<Layer />);
  expect(escOpensInterrupt(document.body)).toBe(false);
});

test("Vi NORMAL lets the Esc through, INSERT and VISUAL do not", () => {
  const host = add('<div data-vi-mode="NORMAL"><textarea></textarea></div>');
  const ta = host.querySelector("textarea");
  expect(escOpensInterrupt(ta)).toBe(true);
  host.firstElementChild!.setAttribute("data-vi-mode", "INSERT");
  expect(escOpensInterrupt(ta)).toBe(false);
  host.firstElementChild!.setAttribute("data-vi-mode", "VISUAL");
  expect(escOpensInterrupt(ta)).toBe(false);
});

test("the Vi editor's own dialog and its layer do not count as on top", () => {
  render(<Layer />);
  const host = add('<div role="dialog"><div data-vi-mode="NORMAL"><textarea></textarea></div></div>');
  expect(escOpensInterrupt(host.querySelector("textarea"))).toBe(true);
});

test("another dialog still takes the Esc in Vi NORMAL", () => {
  add('<div role="dialog"></div>');
  const host = add('<div role="dialog"><div data-vi-mode="NORMAL"><textarea></textarea></div></div>');
  expect(escOpensInterrupt(host.querySelector("textarea"))).toBe(false);
});
