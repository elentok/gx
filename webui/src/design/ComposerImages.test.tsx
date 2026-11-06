import { act, cleanup, fireEvent, renderHook, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { renderThemed } from "../ui/render";
import { ComposerImages } from "./ComposerImages";
import { MSG_TOO_LARGE } from "./imageLimits";
import { deleteSessionImage, uploadSessionImage } from "./sessionImages";
import { useComposerImages, type ComposerImage, type ComposerImages as State } from "./useComposerImages";

vi.mock("./sessionImages", () => ({
  uploadSessionImage: vi.fn(),
  deleteSessionImage: vi.fn(async () => ({})),
  sessionImageUrl: (s: string, id: string) => `/u/${s}/${id}`,
}));

afterEach(cleanup);

function state(images: ComposerImage[], over: Partial<State> = {}): State {
  return {
    images,
    readyIds: [],
    uploading: images.filter((i) => i.state === "uploading").length,
    failed: images.filter((i) => i.state === "failed").length,
    blocking: false,
    hasContent: () => false,
    canSend: () => false,
    payload: {},
    error: "",
    addFiles: vi.fn(async () => undefined),
    retry: vi.fn(),
    remove: vi.fn(),
    clear: vi.fn(),
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

test("a refused tile counts as failed in the line and turns it red", () => {
  const refused: ComposerImage = { key: "r", state: "failed", message: "Too large", canRetry: false };
  render(state([ready("a"), { key: "b", state: "uploading", canRetry: false }, refused], { failed: 0 }));
  const line = screen.getByText("1 failed · 1 uploading");
  expect(line).toBeTruthy();
  expect(getComputedStyle(line).color).not.toBe(getComputedStyle(document.body).color);
});

const realPng = () => new File([new Uint8Array([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a])], "a.png", { type: "image/png" });

test("removing an image while it uploads deletes the saved file once the upload completes", async () => {
  localStorage.clear();
  let finish: (id: string) => void = () => undefined;
  vi.mocked(uploadSessionImage).mockReturnValue(new Promise((r) => (finish = r)));
  const { result } = renderHook(() => useComposerImages("s1"));
  await act(() => result.current.addFiles([realPng()]));
  expect(result.current.uploading).toBe(1);
  act(() => result.current.remove(result.current.images[0].key));
  expect(deleteSessionImage).not.toHaveBeenCalled();
  await act(async () => finish("img-1"));
  await waitFor(() => expect(deleteSessionImage).toHaveBeenCalledExactlyOnceWith("s1", "img-1"));
  expect(result.current.images).toHaveLength(0);
});

test("each refused file keeps its own reason on its thumbnail while another image uploads", async () => {
  localStorage.clear();
  vi.mocked(uploadSessionImage).mockReturnValue(new Promise(() => undefined));
  const { result } = renderHook(() => useComposerImages("s2"));
  const big = new File([new Uint8Array([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a])], "big.png", { type: "image/png" });
  Object.defineProperty(big, "size", { value: 21 * 1024 * 1024 });
  const text = new File(["just text"], "n.png", { type: "image/png" });
  await act(() => result.current.addFiles([realPng(), big, text]));
  render(result.current);
  expect(screen.getByText(MSG_TOO_LARGE)).toBeTruthy();
  expect(screen.getByText("Not a supported image type")).toBeTruthy();
  expect(result.current.uploading).toBe(1);
  expect(result.current.failed).toBe(0);
});
