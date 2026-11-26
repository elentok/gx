import { act, cleanup, fireEvent, screen, within } from "@testing-library/react"
import { afterEach, expect, test, vi } from "vitest"
import { FileLinkProvider } from "../../files/FileLinkContext"
import { stubResolver } from "../../files/stubResolver.testutil"
import { renderThemed as render } from "../render"
import { Question } from "./Question"
import type { QuestionCard } from "./round"

afterEach(cleanup)

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
})

const show = (card: QuestionCard, props: Partial<Parameters<typeof Question>[0]> = {}) => {
  const onChange = vi.fn()
  render(<Question card={card} answer={{}} onChange={onChange} onPicked={vi.fn()} {...props} />)
  return onChange
}

test("a card's links show as new-tab links", () => {
  show({
    label: "1",
    title: "Pick a variant?",
    kind: "yes-no",
    links: [{ label: "Open the mock", url: "https://claude.ai/artifact/abc" }],
  })
  const link = screen.getByRole("link", { name: /Open the mock/ })
  expect(link.getAttribute("href")).toBe("https://claude.ai/artifact/abc")
  expect(link.getAttribute("target")).toBe("_blank")
})

test("multi-select toggles options and answers with the chosen option ids", () => {
  const onChange = show(options({ kind: "multi-select" }), {
    answer: { choices: ["a"] },
  })
  expect(screen.getByRole("button", { name: "Alpha" }).getAttribute("aria-pressed")).toBe("true")
  fireEvent.click(screen.getByRole("button", { name: "Beta" }))
  expect(onChange).toHaveBeenLastCalledWith({
    choices: ["a", "b"],
    huh: false,
  })
  fireEvent.click(screen.getByRole("button", { name: "Alpha" }))
  expect(onChange).toHaveBeenLastCalledWith({ choices: [], huh: false })
})

test("multi-select's Other row answers with the id other and asks for its text", () => {
  const onChange = show(options({ kind: "multi-select" }))
  fireEvent.click(screen.getByRole("button", { name: "Other" }))
  expect(onChange).toHaveBeenLastCalledWith({ choices: ["other"], huh: false })
  cleanup()
  show(options({ kind: "multi-select" }), { answer: { choices: ["other"] } })
  expect(screen.getByRole("textbox", { name: "Answer to question 1" })).toBeTruthy()
})

test("the note field shows only when the card takes notes", () => {
  show(options())
  expect(screen.getByRole("button", { name: /Note on question 1/ })).toBeTruthy()
  cleanup()
  show(options(), { mode: "plain" })
  expect(screen.queryByRole("button", { name: /Note on question 1/ })).toBeNull()
  cleanup()
  show(options(), { mode: "notes" })
  expect(screen.getByRole("button", { name: /Note on question 1/ })).toBeTruthy()
})

test("a picked text row shows its text field", () => {
  show(options(), { answer: { choice: "other" } })
  const box = screen.getByRole("textbox", { name: "Answer to question 1" })
  expect(box.getAttribute("placeholder")).toBe("Your answer")
  expect(screen.getByRole("button", { name: "Other" }).getAttribute("aria-pressed")).toBe("true")
  cleanup()
  show(options(), { answer: { choice: "a" } })
  expect(screen.queryByRole("textbox")).toBeNull()
})

test("a text row's large editor keeps the inline box's name, under the same dialog title", async () => {
  show(options(), { answer: { choice: "other" } })
  fireEvent.click(screen.getByRole("button", { name: "Open the note in a large editor" }))
  const dialog = screen.getByRole("dialog", { name: "Other - Q1" })
  const editor = await within(dialog).findByRole("textbox", { name: "Answer to question 1" })
  expect(dialog.contains(editor)).toBe(true)
})

test("a fresh card has no pressed option and no recommended badge", () => {
  show(options())
  for (const name of ["Alpha", "Beta", "Other"]) {
    expect(screen.getByRole("button", { name }).getAttribute("aria-pressed")).toBe("false")
  }
  expect(screen.queryByText(/recommended/)).toBeNull()
})

test("a recommended option shows its badge", () => {
  show(options({ recommended: "a" }))
  expect(screen.getByText(/recommended/)).toBeTruthy()
})

test("a found path in the title, body and an option label links; clicking it does not select the option", async () => {
  const { resolver } = stubResolver(["a/t.go", "a/b.go", "a/o.go"])
  const onChange = vi.fn()
  render(
    <FileLinkProvider resolver={resolver}>
      <Question
        card={options({
          title: "Check a/t.go",
          body: "See a/b.go",
          options: [{ id: "a", label: "Use a/o.go" }],
        })}
        answer={{}}
        onChange={onChange}
        onPicked={vi.fn()}
      />
    </FileLinkProvider>,
  )
  await act(async () => {})
  const links = screen.getAllByRole("link")
  expect(links.map((l) => l.textContent)).toEqual(["a/t.go", "a/b.go", "a/o.go"])
  fireEvent.click(links[2])
  expect(onChange).not.toHaveBeenCalled()
})

test("Huh? shows only when the card is a printed round", () => {
  show(options())
  expect(screen.getByRole("button", { name: "Huh?" })).toBeTruthy()
  cleanup()
  show(options(), { mode: "notes" })
  expect(screen.queryByRole("button", { name: "Huh?" })).toBeNull()
})
