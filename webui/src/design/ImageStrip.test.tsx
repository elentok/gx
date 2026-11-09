import { cleanup, fireEvent, screen } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { renderThemed } from "../ui/render";
import { ImageStrip } from "./ImageStrip";
import type { ComposerImage } from "./useComposerImages";

afterEach(cleanup);

const ready: ComposerImage = { key: "r", state: "ready", url: "/u/r", canRetry: false };
const uploading: ComposerImage = { key: "u", state: "uploading", canRetry: false };
const failed: ComposerImage = { key: "f", state: "failed", message: "Too large", canRetry: true };
const all = [ready, uploading, failed];

test("editable strip draws each tile state with Remove", () => {
  const { container } = renderThemed(<ImageStrip images={all} />);
  expect(Array.from(container.querySelectorAll("[data-state]")).map((n) => n.getAttribute("data-state"))).toEqual(["ready", "uploading", "failed"]);
  expect(screen.getByLabelText("Uploading")).toBeTruthy();
  expect(screen.getByText("Too large, tap to retry")).toBeTruthy();
  expect(screen.getAllByRole("button", { name: "Remove image" })).toHaveLength(3);
});

test("readOnly hides Remove, retry and the upload states", () => {
  renderThemed(<ImageStrip images={all} readOnly />);
  expect(screen.queryByRole("button", { name: "Remove image" })).toBeNull();
  expect(screen.queryByLabelText("Uploading")).toBeNull();
  expect(screen.queryByText(/tap to retry/)).toBeNull();
});

test("open, retry and remove inside a form never submit it", () => {
  const onSubmit = vi.fn((e: { preventDefault: () => void }) => e.preventDefault());
  renderThemed(
    <form onSubmit={onSubmit}>
      <ImageStrip images={all} onOpen={() => {}} onRetry={() => {}} />
    </form>,
  );
  const buttons = screen.getAllByRole("button");
  expect(buttons.length).toBeGreaterThanOrEqual(5);
  for (const b of buttons) {
    expect(b.getAttribute("type")).toBe("button");
    fireEvent.click(b);
  }
  expect(onSubmit).not.toHaveBeenCalled();
});

test("clicking a ready thumbnail opens it by index; other tiles are not openable", () => {
  const onOpen = vi.fn();
  renderThemed(<ImageStrip images={[uploading, ready]} onOpen={onOpen} />);
  const open = screen.getAllByRole("button", { name: "Open image" });
  expect(open).toHaveLength(1);
  fireEvent.click(open[0]!);
  expect(onOpen).toHaveBeenCalledWith(1);
});
