import { act, cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { renderThemed } from "../ui/render";
import type { PostKeys } from "./keys";
import { ScreenModal } from "./ScreenModal";
import type { FetchScreen } from "./screen";

afterEach(cleanup);

const fetcher = (): FetchScreen =>
  vi.fn(async () => ({ kind: "ok", detail: { rows: ["row one"] } }) as Awaited<ReturnType<FetchScreen>>);

function open(opts: { keys?: boolean; poster?: PostKeys } = {}) {
  const f = fetcher();
  const poster = opts.poster ?? vi.fn<PostKeys>(async () => ({}));
  renderThemed(<ScreenModal target="s-a" fetcher={f} keys={opts.keys ?? true} poster={poster} onClose={() => {}} />);
  return { f, poster };
}

test.each([
  ["Esc", "esc"],
  ["Enter", "enter"],
  ["Tab", "tab"],
  ["Up", "up"],
  ["Down", "down"],
  ["Left", "left"],
  ["Right", "right"],
  ["Ctrl+C", "ctrl+c"],
])("the %s button posts %s with settle and refetches a response without rows", async (name, key) => {
  const { f, poster } = open();
  await waitFor(() => expect(f).toHaveBeenCalledTimes(1));
  fireEvent.click(screen.getByRole("button", { name }));
  await waitFor(() => expect(poster).toHaveBeenCalledWith({ id: "s-a", key, settle: true }));
  await waitFor(() => expect(f).toHaveBeenCalledTimes(2));
});

test("Type posts the text with settle and clears the field", async () => {
  const { f, poster } = open();
  const field = screen.getByLabelText("Text to type") as HTMLInputElement;
  fireEvent.change(field, { target: { value: "yes please" } });
  fireEvent.click(screen.getByRole("button", { name: "Type" }));
  await waitFor(() => expect(poster).toHaveBeenCalledWith({ id: "s-a", text: "yes please", settle: true }));
  await waitFor(() => expect(field.value).toBe(""));
  await waitFor(() => expect(f).toHaveBeenCalledTimes(2));
});

const rowsPoster = (rows: string[]) => vi.fn<PostKeys>(async () => ({ rows }));
const rowOf = (text: string) => screen.getByText(text) as HTMLElement;

test("settled rows show without a second read and the changed row is marked", async () => {
  const { f } = open({ poster: rowsPoster(["row one", "row two"]) });
  await screen.findByText("row one");
  fireEvent.click(screen.getByRole("button", { name: "Enter" }));
  await screen.findByText("row two");
  expect(f).toHaveBeenCalledTimes(1);
  expect(getComputedStyle(rowOf("row two")).fontWeight).toBe("700");
  expect(getComputedStyle(rowOf("row one")).fontWeight).not.toBe("700");
});

test("equal rows say the screen is unchanged, and the mark and the line fade", async () => {
  open({ poster: rowsPoster(["row one"]) });
  await screen.findByText("row one");
  vi.useFakeTimers();
  try {
    fireEvent.click(screen.getByRole("button", { name: "Enter" }));
    await act(async () => {});
    expect(screen.getByRole("status").textContent).toBe("Screen unchanged");
    await act(async () => {
      vi.advanceTimersByTime(5000);
    });
    expect(screen.getByRole("status").textContent).toBe("");
  } finally {
    vi.useRealTimers();
  }
});

test("marks fade", async () => {
  open({ poster: rowsPoster(["row one", "changed"]) });
  await screen.findByText("row one");
  vi.useFakeTimers();
  try {
    fireEvent.click(screen.getByRole("button", { name: "Enter" }));
    await act(async () => {});
    expect(getComputedStyle(rowOf("changed")).fontWeight).toBe("700");
    await act(async () => {
      vi.advanceTimersByTime(5000);
    });
    expect(getComputedStyle(rowOf("changed")).fontWeight).not.toBe("700");
  } finally {
    vi.useRealTimers();
  }
});

test("Refresh does one plain read and sends no key", async () => {
  const { f, poster } = open();
  await waitFor(() => expect(f).toHaveBeenCalledTimes(1));
  fireEvent.click(screen.getByRole("button", { name: "Refresh" }));
  await waitFor(() => expect(f).toHaveBeenCalledTimes(2));
  expect(poster).not.toHaveBeenCalled();
});

test("a refusal shows its message and keeps the text", async () => {
  const poster = vi.fn<PostKeys>(async () => ({ refused: true, message: "q-1 is a Queue agent" }));
  open({ poster });
  const field = screen.getByLabelText("Text to type") as HTMLInputElement;
  fireEvent.change(field, { target: { value: "hi" } });
  fireEvent.click(screen.getByRole("button", { name: "Type" }));
  expect(await screen.findByText("q-1 is a Queue agent")).toBeTruthy();
  expect(field.value).toBe("hi");
});

test("no key bar or Type field when keys are not allowed", async () => {
  const { f } = open({ keys: false });
  await waitFor(() => expect(f).toHaveBeenCalled());
  expect(screen.queryByRole("button", { name: "Esc" })).toBeNull();
  expect(screen.queryByLabelText("Text to type")).toBeNull();
  expect(screen.getByRole("button", { name: "Refresh" })).toBeTruthy();
});
