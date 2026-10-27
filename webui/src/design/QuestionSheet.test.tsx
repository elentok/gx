import { cleanup, fireEvent, screen } from "@testing-library/react";
import { useState } from "react";
import { afterEach, describe, expect, test } from "vitest";
import type { SessionRoundCard } from "../generated/server";
import { renderThemed } from "../ui/render";
import { Question, QuestionSheet } from "./QuestionSheet";
import type { Answers } from "./round";

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

test.each([false, true])("note opens five rows tall (big=%s)", (big) => {
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
    5,
  );
});

test("the question note's large editor is titled with the question and its text reaches onAnswers", () => {
  const seen: Answers[] = [];
  function Harness() {
    const [answers, setAnswers] = useState<Answers>({});
    return (
      <QuestionSheet
        round={[card({ label: "2", title: "Pick one" })]}
        answers={answers}
        onAnswers={(u) =>
          setAnswers((a) => {
            const next = u(a);
            seen.push(next);
            return next;
          })
        }
      />
    );
  }
  renderThemed(<Harness />);
  fireEvent.click(screen.getByLabelText("Note on question 2"));
  fireEvent.click(screen.getByLabelText("Open the note in a large editor"));
  expect(screen.getByRole("dialog").getAttribute("aria-label")).toBe(
    "Note on Q2 - Pick one",
  );
  fireEvent.change(screen.getByLabelText("Note text in the large editor"), {
    target: { value: "hello" },
  });
  expect(seen.at(-1)?.["2"]?.note).toBe("hello");
});

describe("a move scrolls the body to the top", () => {
  const round = [
    card({ label: "1", kind: "yes-no", title: "A" }),
    card({ label: "2", kind: "yes-no", title: "B" }),
    card({ label: "3", kind: "text", title: "C" }),
  ];
  function Harness({ focus }: { focus?: { index: number } }) {
    const [answers, setAnswers] = useState<Answers>({});
    return (
      <QuestionSheet
        round={round}
        answers={answers}
        onAnswers={(u) => setAnswers(u)}
        focus={focus}
      />
    );
  }
  const body = () => screen.getByTestId("question-body");
  const scrolled = () => {
    body().scrollTop = 120;
  };

  test.each([
    ["a dot", () => fireEvent.click(screen.getByLabelText("Question 3"))],
    ["Next", () => fireEvent.click(screen.getByText("Next →"))],
    ["a pick", () => fireEvent.click(screen.getAllByText("Yes")[0])],
  ])("%s", (_, move) => {
    renderThemed(<Harness />);
    scrolled();
    move();
    expect(body().scrollTop).toBe(0);
  });

  test("Prev", () => {
    renderThemed(<Harness />);
    fireEvent.click(screen.getByText("Next →"));
    scrolled();
    fireEvent.click(screen.getByText("← Prev"));
    expect(body().scrollTop).toBe(0);
  });

  test("a new focus object, even for the same question", () => {
    function Refocus() {
      const [focus, setFocus] = useState<{ index: number }>({ index: 1 });
      return (
        <>
          <button onClick={() => setFocus({ index: 1 })}>refocus</button>
          <Harness focus={focus} />
        </>
      );
    }
    renderThemed(<Refocus />);
    scrolled();
    fireEvent.click(screen.getByText("refocus"));
    expect(body().scrollTop).toBe(0);
  });

  test("a note toggle or typing on the same question leaves it alone", () => {
    renderThemed(<Harness />);
    fireEvent.click(screen.getByLabelText("Question 3"));
    scrolled();
    fireEvent.click(screen.getByLabelText("Note on question 3"));
    fireEvent.change(screen.getByLabelText("Answer to question 3"), {
      target: { value: "hi" },
    });
    expect(body().scrollTop).toBe(120);
  });
});
