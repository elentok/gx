import { cleanup, fireEvent, screen } from "@testing-library/react";
import { afterEach, expect, test } from "vitest";
import type { SessionRoundCard } from "../generated/server";
import { renderThemed } from "../ui/render";
import { Question } from "./QuestionSheet";

afterEach(cleanup);

const card = (over: Partial<SessionRoundCard> = {}): SessionRoundCard => ({
  label: "1",
  title: "Pick",
  body: "",
  kind: "text",
  ...over,
});

test.each([
  [false, 2],
  [true, 5],
])("text answer rows (big=%s)", (big, rows) => {
  renderThemed(
    <Question
      card={card()}
      answer={{}}
      onChange={() => {}}
      onPicked={() => {}}
      big={big}
    />,
  );
  expect(screen.getByLabelText("Answer to question 1")).toHaveProperty(
    "rows",
    rows,
  );
});

test.each([false, true])("note opens three rows tall (big=%s)", (big) => {
  renderThemed(
    <Question
      card={card()}
      answer={{}}
      onChange={() => {}}
      onPicked={() => {}}
      big={big}
    />,
  );
  fireEvent.click(screen.getByLabelText("Note on question 1"));
  expect(screen.getByLabelText("Note text for question 1")).toHaveProperty(
    "rows",
    3,
  );
});
