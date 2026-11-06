import { cleanup, fireEvent, screen } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { renderThemed } from "../ui/render";
import { ImageViewer } from "./ImageViewer";

afterEach(cleanup);

const ids = (n: number) => Array.from({ length: n }, (_, i) => ({ id: `img-${i}` }));

function open(n: number, start = 0, onClose = () => {}) {
  renderThemed(<ImageViewer images={ids(n)} sessionId="s-a" start={start} onClose={onClose} />);
}

const shown = () => screen.getByRole("img").getAttribute("src");

test("opens at the tapped image with its count", () => {
  open(3, 1);
  expect(screen.getByText("2 of 3")).toBeTruthy();
  expect(shown()).toBe("/v1/sessions/images?session=s-a&id=img-1");
});

test("Next reaches the tenth of ten and then stops", () => {
  open(10);
  const next = screen.getByRole("button", { name: /Next/ });
  for (let i = 0; i < 9; i++) fireEvent.click(next);
  expect(screen.getByText("10 of 10")).toBeTruthy();
  expect(shown()).toBe("/v1/sessions/images?session=s-a&id=img-9");
  expect((next as HTMLButtonElement).disabled).toBe(true);
});

test("Prev steps back and stops at the first", () => {
  open(3, 1);
  const prev = screen.getByRole("button", { name: /Prev/ });
  fireEvent.click(prev);
  expect(screen.getByText("1 of 3")).toBeTruthy();
  expect((prev as HTMLButtonElement).disabled).toBe(true);
});

test("Close and Escape both close", () => {
  const onClose = vi.fn();
  open(2, 0, onClose);
  fireEvent.click(screen.getByRole("button", { name: /Close/ }));
  fireEvent.keyDown(window, { key: "Escape" });
  expect(onClose).toHaveBeenCalledTimes(2);
});

test("a placeholder image draws a plain box, not an img", () => {
  renderThemed(<ImageViewer images={[{ placeholder: true }]} sessionId="s-a" start={0} onClose={() => {}} />);
  expect(screen.queryByRole("img")).toBeNull();
  expect(screen.getByText("1 of 1")).toBeTruthy();
});
