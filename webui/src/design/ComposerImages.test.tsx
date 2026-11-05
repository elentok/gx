import { cleanup, fireEvent, screen } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { renderThemed } from "../ui/render";
import { ComposerImages } from "./ComposerImages";
import type { ComposerImage, ComposerImages as State } from "./useComposerImages";

afterEach(cleanup);

function state(images: ComposerImage[], over: Partial<State> = {}): State {
  return {
    images,
    readyIds: [],
    uploading: images.filter((i) => i.state === "uploading").length,
    failed: images.filter((i) => i.state === "failed").length,
    error: "",
    addFiles: vi.fn(async () => undefined),
    retry: vi.fn(),
    remove: vi.fn(),
    ...over,
  };
}

const ready = (key: string): ComposerImage => ({ key, state: "ready", url: `/u/${key}`, canRetry: false });

function render(s: State) {
  return renderThemed(
    <ComposerImages state={s} send={<button>Send</button>}>
      <textarea aria-label="Message" />
    </ComposerImages>,
  );
}

const png = () => new File([new Uint8Array([0x89, 0x50, 0x4e, 0x47])], "a.png", { type: "image/png" });

test("paste with image files adds them and skips plain text pastes", () => {
  const s = state([]);
  render(s);
  const file = png();
  fireEvent.paste(screen.getByLabelText("Message"), { clipboardData: { files: [file] } });
  expect(s.addFiles).toHaveBeenCalledWith([file]);
  fireEvent.paste(screen.getByLabelText("Message"), { clipboardData: { files: [] } });
  expect(s.addFiles).toHaveBeenCalledTimes(1);
});

test("drop adds the dropped files", () => {
  const s = state([]);
  render(s);
  const file = png();
  fireEvent.drop(screen.getByLabelText("Message"), { dataTransfer: { files: [file], types: ["Files"] } });
  expect(s.addFiles).toHaveBeenCalledWith([file]);
});

test("the hidden input feeds the same attach function and has no capture", () => {
  const s = state([]);
  render(s);
  expect(screen.getByRole("button", { name: "Add image" }).textContent).toContain("Add");
  const input = screen.getByTestId("image-input") as HTMLInputElement;
  expect(input.accept).toBe("image/*");
  expect(input.multiple).toBe(true);
  expect(input.hasAttribute("capture")).toBe(false);
  const file = png();
  fireEvent.change(input, { target: { files: [file] } });
  expect(s.addFiles).toHaveBeenCalledWith([file]);
});

test("thumbnails show uploading, failed with retry, and remove", () => {
  const s = state([
    ready("a"),
    { key: "b", state: "uploading", canRetry: false },
    { key: "c", state: "failed", message: "Upload failed", canRetry: true },
    { key: "d", state: "failed", message: "Upload cut off", canRetry: false },
  ]);
  render(s);
  expect(screen.getByLabelText("Uploading")).toBeTruthy();
  fireEvent.click(screen.getByText("Upload failed, tap to retry"));
  expect(s.retry).toHaveBeenCalledWith("c");
  const cut = screen.getByText("Upload cut off");
  fireEvent.click(cut);
  expect(s.retry).toHaveBeenCalledTimes(1);
  fireEvent.click(screen.getAllByRole("button", { name: "Remove image" })[3]);
  expect(s.remove).toHaveBeenCalledWith("d");
});

test("the line reads failed and uploading, N of 10, or the max message", () => {
  const mixed = state([
    ready("a"),
    { key: "b", state: "uploading", canRetry: false },
    { key: "c", state: "failed", message: "x", canRetry: true },
  ]);
  render(mixed);
  expect(screen.getByText("1 failed · 1 uploading")).toBeTruthy();
  cleanup();
  render(state([ready("a"), ready("b"), ready("c")]));
  expect(screen.getByText("3 of 10 images")).toBeTruthy();
  cleanup();
  render(state([ready("a")], { error: "Max 10 images" }));
  expect(screen.getByText("Max 10 images")).toBeTruthy();
});
