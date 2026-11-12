import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react"
import { useState } from "react"
import { afterEach, describe, expect, test } from "vitest"
import type { SessionRoundCard } from "../generated/server"
import { NO_SIDEBAR_SWIPE_ATTR } from "../shell/useSidebarSwipe"
import { typeInLargeEditor } from "../ui/expandable-text/testing"
import { renderThemed } from "../ui/render"
import { Question } from "../ui/question-sheet/Question"
import { allAnswered, type Answers } from "../ui/question-sheet/round"
import { QuestionSheet } from "./QuestionSheet"

afterEach(cleanup)

test("an options row shows letter, label, muted id and the consequence line", () => {
  renderThemed(
    <Question
      card={card({
        kind: "options",
        options: [
          { id: "alpha", label: "First", consequence: "Ships today" },
          { id: "beta", label: "Second" },
        ],
      })}
      answer={{}}
      onChange={() => {}}
      onPicked={() => {}}
    />,
  )
  const rows = screen.getAllByRole("button", { pressed: false })
  expect(rows[0].textContent).toBe("AFirstalphaShips today")
  expect(rows[1].textContent).toBe("BSecondbeta")
})

test("yes-no buttons read Yes and No with the consequence under them", () => {
  renderThemed(
    <Question
      card={card({ kind: "yes-no", yes: "Go ahead", no: "Stop here" })}
      answer={{}}
      onChange={() => {}}
      onPicked={() => {}}
    />,
  )
  const rows = screen.getAllByRole("button", { pressed: false })
  expect(rows[0].textContent).toBe("YesGo ahead")
  expect(rows[1].textContent).toBe("NoStop here")
})

test("a text answer and a note turn AutoFill off and get distinct names", () => {
  renderThemed(
    <Question
      card={card({ kind: "text" })}
      answer={{ note: "n" }}
      onChange={() => {}}
      onPicked={() => {}}
    />,
  )
  const answer = screen.getByLabelText("Answer to question 1")
  const note = screen.getByLabelText("Note text for question 1")
  expect(answer.getAttribute("autocomplete")).toBe("off")
  expect(note.getAttribute("autocomplete")).toBe("off")
  expect(answer.getAttribute("name")).toBe("question-answer")
  expect(note.getAttribute("name")).toBe("question-note")
})

const card = (over: Partial<SessionRoundCard> = {}): SessionRoundCard => ({
  label: "1",
  title: "Pick",
  body: "",
  kind: "text",
  ...over,
})

test.each([
  [false, 2],
  [true, 5],
])("text answer rows (big=%s)", (big, rows) => {
  renderThemed(
    <Question card={card()} answer={{}} onChange={() => {}} onPicked={() => {}} big={big} />,
  )
  expect(screen.getByLabelText("Answer to question 1")).toHaveProperty("rows", rows)
})

test("the text answer's large editor is titled with the question and its text reaches onAnswers; choice questions have no answer field", async () => {
  const seen: Answers[] = []
  function Harness({ kind }: { kind: string }) {
    const [answers, setAnswers] = useState<Answers>({})
    return (
      <QuestionSheet
        round={[card({ label: "2", title: "Pick one", kind })]}
        answers={answers}
        onAnswers={(u) =>
          setAnswers((a) => {
            const next = u(a)
            seen.push(next)
            return next
          })
        }
      />
    )
  }
  renderThemed(<Harness kind="text" />)
  fireEvent.click(screen.getByLabelText("Open the note in a large editor"))
  expect(screen.getByRole("dialog").getAttribute("aria-label")).toBe("Answer to Q2 - Pick one")
  await typeInLargeEditor("Answer to Q2 - Pick one", "yes please")
  expect(seen.at(-1)?.["2"]?.text).toBe("yes please")
  cleanup()
  renderThemed(<Harness kind="yes-no" />)
  expect(screen.queryByLabelText("Answer to question 2")).toBeNull()
})

test.each([false, true])("note opens five rows tall (big=%s)", (big) => {
  renderThemed(
    <Question card={card()} answer={{}} onChange={() => {}} onPicked={() => {}} big={big} />,
  )
  fireEvent.click(screen.getByLabelText("Note on question 1"))
  expect(screen.getByLabelText("Note text for question 1")).toHaveProperty("rows", 5)
})

test("the question note's large editor is titled with the question and its text reaches onAnswers", async () => {
  const seen: Answers[] = []
  function Harness() {
    const [answers, setAnswers] = useState<Answers>({})
    return (
      <QuestionSheet
        round={[card({ label: "2", title: "Pick one" })]}
        answers={answers}
        onAnswers={(u) =>
          setAnswers((a) => {
            const next = u(a)
            seen.push(next)
            return next
          })
        }
      />
    )
  }
  renderThemed(<Harness />)
  fireEvent.click(screen.getByLabelText("Note on question 2"))
  // The text answer has its own expand button above the note's.
  fireEvent.click(screen.getAllByLabelText("Open the note in a large editor")[1])
  expect(screen.getByRole("dialog").getAttribute("aria-label")).toBe("Note on Q2 - Pick one")
  await typeInLargeEditor("Note on Q2 - Pick one", "hello")
  expect(seen.at(-1)?.["2"]?.note).toBe("hello")
})

test.each(["text", "yes-no"])(
  "Huh? on a %s question marks it huh, and a pick replaces the mark",
  (kind) => {
    function Harness() {
      const [answers, setAnswers] = useState<Answers>({})
      return (
        <>
          <QuestionSheet
            round={[card({ kind })]}
            answers={answers}
            onAnswers={(u) => setAnswers(u)}
          />
          <output>{JSON.stringify(answers["1"])}</output>
        </>
      )
    }
    renderThemed(<Harness />)
    fireEvent.click(screen.getByText("Huh?"))
    expect(screen.getByText("1 of 1 answered")).toBeTruthy()
    expect(JSON.parse(screen.getByRole("status").textContent ?? "")).toMatchObject({ huh: true })
    if (kind === "yes-no") {
      fireEvent.click(screen.getByText("Yes"))
      const a = JSON.parse(screen.getByRole("status").textContent ?? "")
      expect(a.choice).toBe("yes")
      expect(a.huh).toBeFalsy()
    }
  },
)

test("the cards row opts out of the sidebar swipe", () => {
  renderThemed(
    <QuestionSheet
      round={[card({ label: "1", kind: "yes-no" }), card({ label: "2", kind: "yes-no" })]}
      answers={{}}
      onAnswers={() => {}}
    />,
  )
  const row = screen.getByRole("group", { name: "Question 1 of 2" }).parentElement
  expect(row?.hasAttribute(NO_SIDEBAR_SWIPE_ATTR)).toBe(true)
})

describe("a move scrolls the body to the top", () => {
  const round = [
    card({ label: "1", kind: "yes-no", title: "A" }),
    card({ label: "2", kind: "yes-no", title: "B" }),
    card({ label: "3", kind: "text", title: "C" }),
  ]
  function Harness({ focus }: { focus?: { index: number } }) {
    const [answers, setAnswers] = useState<Answers>({})
    return (
      <QuestionSheet
        round={round}
        answers={answers}
        onAnswers={(u) => setAnswers(u)}
        focus={focus}
      />
    )
  }
  const body = () => screen.getByTestId("question-body")
  const scrolled = () => {
    body().scrollTop = 120
  }

  test.each([
    ["a dot", () => fireEvent.click(screen.getByLabelText("Question 3"))],
    ["Next", () => fireEvent.click(screen.getByText("Next →"))],
    ["a pick", () => fireEvent.click(screen.getAllByText("Yes")[0])],
  ])("%s", (_, move) => {
    renderThemed(<Harness />)
    scrolled()
    move()
    expect(body().scrollTop).toBe(0)
  })

  test("Prev", () => {
    renderThemed(<Harness />)
    fireEvent.click(screen.getByText("Next →"))
    scrolled()
    fireEvent.click(screen.getByText("← Prev"))
    expect(body().scrollTop).toBe(0)
  })

  test("a new focus object, even for the same question", () => {
    function Refocus() {
      const [focus, setFocus] = useState<{ index: number }>({ index: 1 })
      return (
        <>
          <button onClick={() => setFocus({ index: 1 })}>refocus</button>
          <Harness focus={focus} />
        </>
      )
    }
    renderThemed(<Refocus />)
    scrolled()
    fireEvent.click(screen.getByText("refocus"))
    expect(body().scrollTop).toBe(0)
  })

  test("a note toggle or typing on the same question leaves it alone", () => {
    renderThemed(<Harness />)
    fireEvent.click(screen.getByLabelText("Question 3"))
    scrolled()
    fireEvent.click(screen.getByLabelText("Note on question 3"))
    fireEvent.change(screen.getByLabelText("Answer to question 3"), {
      target: { value: "hi" },
    })
    expect(body().scrollTop).toBe(120)
  })
})

describe("the Submit rule", () => {
  const cards = [
    { label: "1", title: "A", kind: "yes-no" },
    {
      label: "2",
      title: "B",
      kind: "options",
      options: [
        { id: "go", label: "Go" },
        { id: "change", label: "Tell Claude what to change", text: true },
      ],
    },
    { label: "3", title: "C", kind: "multi-select", options: [{ id: "x", label: "X" }] },
  ]

  test("a card is answered only when every question is", () => {
    expect(allAnswered(cards, { "1": { choice: "yes" }, "2": { choice: "go" } })).toBe(false)
    expect(
      allAnswered(cards, {
        "1": { choice: "yes" },
        "2": { choice: "go" },
        "3": { choices: ["X"] },
      }),
    ).toBe(true)
    expect(
      allAnswered(cards, { "1": { huh: true }, "2": { choice: "go" }, "3": { choices: [] } }),
    ).toBe(false)
  })

  test("a picked text row stays unanswered while its text is empty", () => {
    const base = { "1": { choice: "yes" }, "3": { choices: ["X"] } }
    expect(allAnswered(cards, { ...base, "2": { choice: "change" } })).toBe(false)
    expect(allAnswered(cards, { ...base, "2": { choice: "change", text: "  " } })).toBe(false)
    expect(allAnswered(cards, { ...base, "2": { choice: "change", text: "shorter" } })).toBe(true)
  })
})

describe("keyboard", () => {
  const round = [
    card({ label: "1", kind: "yes-no", title: "A" }),
    card({ label: "2", kind: "yes-no", title: "B" }),
  ]
  function Harness({ initial = {} }: { initial?: Answers }) {
    const [answers, setAnswers] = useState<Answers>(initial)
    return (
      <>
        <QuestionSheet round={round} answers={answers} onAnswers={(u) => setAnswers(u)} />
        <button data-send-answers="" onClick={() => document.body.setAttribute("data-sent", "1")}>
          Send answers
        </button>
      </>
    )
  }
  const q = (k: number) => document.querySelectorAll<HTMLElement>(`[role=group][aria-label$=" of 2"]`)[k]
  const answer = (k: number, i: number) => q(k).querySelectorAll<HTMLElement>("[data-answer]")[i]
  const key = (k: string, init: KeyboardEventInit = {}) =>
    fireEvent.keyDown(document.activeElement ?? document.body, { key: k, ...init })

  test("n goes to the next question's first answer; p returns to the selected answer", async () => {
    renderThemed(<Harness initial={{ "1": { choice: "no" } }} />)
    answer(0, 0).focus()
    key("n")
    await waitFor(() => expect(document.activeElement).toBe(answer(1, 0)))
    key("p")
    await waitFor(() => expect(document.activeElement).toBe(answer(0, 1)))
  })

  test("p lands on the first answer when none is selected", async () => {
    renderThemed(<Harness />)
    answer(0, 0).focus()
    key("n")
    await waitFor(() => expect(document.activeElement).toBe(answer(1, 0)))
    key("p")
    await waitFor(() => expect(document.activeElement).toBe(answer(0, 0)))
  })

  test.each([
    ["j", "k"],
    ["ArrowDown", "ArrowUp"],
  ])("%s / %s move between answers and stop at the ends", (down, up) => {
    renderThemed(<Harness />)
    answer(0, 0).focus()
    // Yes, No and Huh?.
    key(down)
    key(down)
    expect(document.activeElement).toBe(answer(0, 2))
    key(down)
    expect(document.activeElement).toBe(answer(0, 2))
    key(up)
    expect(document.activeElement).toBe(answer(0, 1))
  })

  test("N opens the note and focuses it; Esc leaves the box so keys work again", async () => {
    renderThemed(<Harness />)
    answer(0, 0).focus()
    key("N")
    const note = await screen.findByLabelText("Note text for question 1")
    await waitFor(() => expect(document.activeElement).toBe(note))
    key("n")
    expect(document.activeElement).toBe(note)
    key("Escape")
    expect(document.activeElement).toBe(answer(0, 0))
    key("N")
    await waitFor(() => expect(document.activeElement).toBe(note))
  })

  test("Cmd+Enter presses Send answers, also from a text box", async () => {
    renderThemed(<Harness />)
    answer(0, 0).focus()
    key("N")
    await waitFor(() => expect(document.activeElement?.tagName).toBe("TEXTAREA"))
    key("Enter", { metaKey: true })
    expect(document.body.getAttribute("data-sent")).toBe("1")
    document.body.removeAttribute("data-sent")
  })
})
