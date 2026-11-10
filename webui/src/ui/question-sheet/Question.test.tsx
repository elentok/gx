import { cleanup, fireEvent, screen } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { renderThemed as render } from "../render";
import { Question } from "./Question";
import type { QuestionCard } from "./round";

afterEach(cleanup);

const options = (extra: Partial<QuestionCard> = {}): QuestionCard => ({
  label: "1",
  title: "Pick",
  kind: "options",
  options: [
    { id: "a", label: "Alpha" },
    { id: "b", label: "Beta" },
    { id: "other", label: "Other", text: true },
  ],
  ...extra,
});

const show = (card: QuestionCard, props: Partial<Parameters<typeof Question>[0]> = {}) => {
  const onChange = vi.fn();
  render(<Question card={card} answer={{}} onChange={onChange} onPicked={vi.fn()} {...props} />);
  return onChange;
};

test("a card's links show as new-tab links", () => {
  show({
    label: "1",
    title: "Pick a variant?",
    kind: "yes-no",
    links: [{ label: "Open the mock", url: "https://claude.ai/artifact/abc" }],
  });
  const link = screen.getByRole("link", { name: /Open the mock/ });
  expect(link.getAttribute("href")).toBe("https://claude.ai/artifact/abc");
  expect(link.getAttribute("target")).toBe("_blank");
});

test("multi-select toggles options and answers with the chosen labels", () => {
  const onChange = show(options({ kind: "multi-select" }), { answer: { choices: ["Alpha"] } });
  expect(screen.getByRole("button", { name: "Alpha" }).getAttribute("aria-pressed")).toBe("true");
  fireEvent.click(screen.getByRole("button", { name: "Beta" }));
  expect(onChange).toHaveBeenLastCalledWith({ choices: ["Alpha", "Beta"], huh: false });
  fireEvent.click(screen.getByRole("button", { name: "Alpha" }));
  expect(onChange).toHaveBeenLastCalledWith({ choices: [], huh: false });
});

test("the note field shows only when the card takes notes", () => {
  show(options());
  expect(screen.getByRole("button", { name: /Note on question 1/ })).toBeTruthy();
  cleanup();
  show(options(), { notes: false });
  expect(screen.queryByRole("button", { name: /Note on question 1/ })).toBeNull();
});

test("a picked text row shows its text field", () => {
  show(options(), { answer: { choice: "other" } });
  expect(screen.getByRole("textbox", { name: /Other \(question 1\)/ })).toBeTruthy();
  cleanup();
  show(options(), { answer: { choice: "a" } });
  expect(screen.queryByRole("textbox")).toBeNull();
});

test("a fresh card has no pressed option and no recommended badge", () => {
  show(options());
  for (const name of ["Alpha", "Beta", "Other"]) {
    expect(screen.getByRole("button", { name }).getAttribute("aria-pressed")).toBe("false");
  }
  expect(screen.queryByText(/recommended/)).toBeNull();
});

test("a recommended option shows its badge", () => {
  show(options({ recommended: "a" }));
  expect(screen.getByText(/recommended/)).toBeTruthy();
});

test("Huh? shows only when the card is a printed round", () => {
  show(options());
  expect(screen.getByRole("button", { name: "Huh?" })).toBeTruthy();
  cleanup();
  show(options(), { huh: false });
  expect(screen.queryByRole("button", { name: "Huh?" })).toBeNull();
});
