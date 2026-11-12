import { ChakraProvider } from "@chakra-ui/react"
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react"
import { afterEach, expect, test, vi } from "vitest"
import canned from "../canned-detail.json"
import type { DetailRound, TicketDetail } from "../generated/server"
import { typeInLargeEditor } from "../ui/expandable-text/testing"
import { renderThemed } from "../ui/render"
import { system } from "../ui/theme"
import { AppToaster } from "../ui/toaster"
import type { AnswerBody, PostAnswer } from "./answer"
import { AnswerForm } from "./AnswerForm"

afterEach(cleanup)

const EXPAND = "Open the note in a large editor"

const base = canned as TicketDetail
const withLatest = (latest: Partial<DetailRound>): TicketDetail => ({
  ...base,
  latest: { ...base.latest!, ...latest },
})
const ok: PostAnswer = async () => ({ outcome: "answered" })
const seq = base.latest!.seq
const ANSWER_FIELD = `Answer to question ${seq}`
const NOTE_FIELD = `Note text for question ${seq}`
const openNote = () =>
  fireEvent.click(screen.getByRole("button", { name: `Note on question ${seq}` }))
const noteField = () => screen.getByLabelText(NOTE_FIELD) as HTMLTextAreaElement

test("a note alone enables Send and posts no choice", async () => {
  const post = vi.fn<(b: AnswerBody) => ReturnType<PostAnswer>>(ok)
  renderThemed(<AnswerForm detail={withLatest({ kind: "yes-no" })} post={post} />)
  const send = screen.getByRole("button", { name: "Send answer" }) as HTMLButtonElement
  expect(send.disabled).toBe(true)
  openNote()
  fireEvent.change(noteField(), { target: { value: "   " } })
  expect(send.disabled).toBe(true)
  fireEvent.change(noteField(), { target: { value: "neither" } })
  expect(send.disabled).toBe(false)
  fireEvent.click(send)
  await waitFor(() => expect(post).toHaveBeenCalled())
  expect(post.mock.calls[0][0]).toStrictEqual({
    address: "gx:demo/12",
    state: "tok-1",
    note: "neither",
  })
})

test("yes-no renders Yes and No with nothing selected, and posts the choice, state and note", async () => {
  const post = vi.fn<(b: AnswerBody) => ReturnType<PostAnswer>>(ok)
  renderThemed(<AnswerForm detail={withLatest({ kind: "yes-no" })} post={post} />)
  const yes = screen.getByRole("button", { name: "Yes" })
  expect(yes.getAttribute("aria-pressed")).toBe("false")
  expect(screen.getByRole("button", { name: "No" }).getAttribute("aria-pressed")).toBe("false")
  fireEvent.click(yes)
  openNote()
  fireEvent.change(noteField(), { target: { value: "go" } })
  fireEvent.click(screen.getByRole("button", { name: "Send answer" }))
  await waitFor(() => expect(post).toHaveBeenCalled())
  expect(post).toHaveBeenCalledWith({
    address: "gx:demo/12",
    state: "tok-1",
    choice: "yes",
    note: "go",
  })
})

test("options show the letter, muted id, consequence and the recommended mark on the suggested one only; Send sends nothing until picked", () => {
  const post = vi.fn(ok)
  const detail = withLatest({
    kind: "options",
    recommended: "b",
    options: [
      { id: "a", label: "Redis", consequence: "Survives restarts" },
      { id: "b", label: "Memory", consequence: "Loses data" },
    ],
  })
  renderThemed(<AnswerForm detail={detail} post={post} />)
  expect(screen.getAllByText("★ recommended")).toHaveLength(1)
  const memory = screen.getByRole("button", { name: "Memory" })
  expect(memory.textContent).toContain("★ recommended")
  expect(memory.textContent).toContain("B")
  expect(memory.textContent).toContain("b")
  expect(screen.getByRole("button", { name: "Redis" }).textContent).toContain("A")
  expect(screen.getByText("Survives restarts")).toBeTruthy()
  expect(screen.getByRole("button", { name: "Redis" }).getAttribute("aria-pressed")).toBe("false")
  expect(memory.getAttribute("aria-pressed")).toBe("false")
  const send = screen.getByRole("button", { name: "Send answer" }) as HTMLButtonElement
  expect(send.disabled).toBe(true)
  fireEvent.click(send)
  expect(post).not.toHaveBeenCalled()
})

test("a round of one shows no dots, count, Prev/Next or fold button, but Huh? and Note", () => {
  renderThemed(<AnswerForm detail={withLatest({ kind: "yes-no" })} post={vi.fn(ok)} />)
  expect(screen.getByRole("button", { name: "Huh?" })).toBeTruthy()
  expect(screen.getByRole("button", { name: `Note on question ${seq}` })).toBeTruthy()
  expect(screen.queryByRole("button", { name: `Question ${seq}` })).toBeNull()
  expect(screen.queryByText(/answered$/)).toBeNull()
  expect(screen.queryByRole("button", { name: /Prev|Next/ })).toBeNull()
  expect(screen.queryByRole("button", { name: /Fold questions|Open questions/ })).toBeNull()
})

test("Huh? posts the huh flag, not the message, and posts nothing before Send", async () => {
  const post = vi.fn<(b: AnswerBody) => ReturnType<PostAnswer>>(ok)
  renderThemed(<AnswerForm detail={withLatest({ kind: "text" })} post={post} />)
  fireEvent.click(screen.getByRole("button", { name: "Huh?" }))
  expect(post).not.toHaveBeenCalled()
  fireEvent.click(screen.getByRole("button", { name: "Send answer" }))
  await waitFor(() => expect(post).toHaveBeenCalled())
  expect(post).toHaveBeenCalledWith({ address: "gx:demo/12", state: "tok-1", huh: true })
})

test("text posts the typed text", async () => {
  const post = vi.fn<(b: AnswerBody) => ReturnType<PostAnswer>>(ok)
  renderThemed(<AnswerForm detail={withLatest({ kind: "text" })} post={post} />)
  fireEvent.change(screen.getByLabelText(ANSWER_FIELD), { target: { value: "Redis" } })
  fireEvent.click(screen.getByRole("button", { name: "Send answer" }))
  await waitFor(() => expect(post).toHaveBeenCalled())
  expect(post).toHaveBeenCalledWith({ address: "gx:demo/12", state: "tok-1", text: "Redis" })
})

test("the answer field has an expand button; its editor shares the text and the sent body", async () => {
  const post = vi.fn<(b: AnswerBody) => ReturnType<PostAnswer>>(ok)
  const summary = "Which database?"
  renderThemed(<AnswerForm detail={withLatest({ kind: "text", summary })} post={post} />)
  const field = screen.getByLabelText(ANSWER_FIELD) as HTMLTextAreaElement
  expect(field.placeholder).toBe("Your answer")
  fireEvent.click(screen.getByRole("button", { name: EXPAND }))
  await typeInLargeEditor(`Answer to Q${seq} - ${summary}`, "from editor")
  expect(field.value).toBe("from editor")
  fireEvent.click(screen.getByRole("button", { name: "Send answer" }))
  await waitFor(() => expect(post).toHaveBeenCalled())
  expect(post).toHaveBeenCalledWith({ address: "gx:demo/12", state: "tok-1", text: "from editor" })
})

test("the note has an expand button; its editor shares the text and the sent body", async () => {
  const post = vi.fn<(b: AnswerBody) => ReturnType<PostAnswer>>(ok)
  const summary = "Retry?"
  renderThemed(<AnswerForm detail={withLatest({ kind: "yes-no", summary })} post={post} />)
  openNote()
  const field = noteField()
  fireEvent.click(screen.getByRole("button", { name: EXPAND }))
  await typeInLargeEditor(`Note on Q${seq} - ${summary}`, "from editor")
  expect(field.value).toBe("from editor")
  fireEvent.click(screen.getByRole("button", { name: "Yes" }))
  fireEvent.click(screen.getByRole("button", { name: "Send answer" }))
  await waitFor(() => expect(post).toHaveBeenCalled())
  expect(post).toHaveBeenCalledWith({
    address: "gx:demo/12",
    state: "tok-1",
    choice: "yes",
    note: "from editor",
  })
})

test("choice questions show no answer field", () => {
  renderThemed(<AnswerForm detail={withLatest({ kind: "yes-no" })} post={vi.fn(ok)} />)
  expect(screen.queryByLabelText(ANSWER_FIELD)).toBeNull()
})

test("Send stays disabled while the request runs, and the form goes once it is sent", async () => {
  let release: () => void = () => {}
  const post: PostAnswer = () => new Promise((r) => (release = () => r({ outcome: "answered" })))
  renderThemed(<AnswerForm detail={withLatest({ kind: "yes-no" })} post={post} />)
  fireEvent.click(screen.getByRole("button", { name: "Yes" }))
  fireEvent.click(screen.getByRole("button", { name: "Send answer" }))
  await waitFor(() =>
    expect(
      (screen.getByRole("button", { name: "Send answer" }) as HTMLButtonElement).disabled,
    ).toBe(true),
  )
  release()
  await waitFor(() => expect(screen.queryByRole("button", { name: "Send answer" })).toBeNull())
})

const refuse =
  (reason: string, message: string): PostAnswer =>
  async () => ({ refused: true, reason, message })

async function sendYes(post: PostAnswer, onStale?: () => void) {
  renderThemed(<AnswerForm detail={withLatest({ kind: "yes-no" })} post={post} onStale={onStale} />)
  fireEvent.click(screen.getByRole("button", { name: "Yes" }))
  openNote()
  fireEvent.change(noteField(), { target: { value: "keep me" } })
  fireEvent.click(screen.getByRole("button", { name: "Send answer" }))
}

test("a refusal shows its message inline and keeps the note", async () => {
  await sendYes(refuse("not-answerable", "Already answered"))
  expect(await screen.findByText("Already answered")).toBeTruthy()
  expect(noteField().value).toBe("keep me")
})

test("agent-not-ready tells the person to approve the prompt first", async () => {
  await sendYes(refuse("agent-not-ready", "The agent is not ready."))
  expect((await screen.findByRole("alert")).textContent).toContain(
    "Approve the agent's prompt first",
  )
})

test("stale refetches and keeps the note", async () => {
  const onStale = vi.fn()
  await sendYes(refuse("stale", "the question changed; answer the current one"), onStale)
  await waitFor(() => expect(onStale).toHaveBeenCalled())
  expect(noteField().value).toBe("keep me")
})

test("a new question swaps in with the note kept and the pick cleared", async () => {
  const post = refuse("stale", "changed")
  const { rerender } = renderThemed(
    <AnswerForm detail={withLatest({ kind: "yes-no" })} post={post} />,
  )
  fireEvent.click(screen.getByRole("button", { name: "Yes" }))
  openNote()
  fireEvent.change(noteField(), { target: { value: "keep me" } })
  rerender(
    <ChakraProvider value={system}>
      <AnswerForm
        detail={{ ...withLatest({ kind: "yes-no", summary: "New question" }), state: "tok-2" }}
        post={post}
      />
      <AppToaster />
    </ChakraProvider>,
  )
  expect(screen.getByRole("button", { name: "Yes" }).getAttribute("aria-pressed")).toBe("false")
  expect(noteField().value).toBe("keep me")
})

test("no answer action renders nothing", () => {
  renderThemed(<AnswerForm detail={{ ...withLatest({ kind: "text" }), actions: [] }} post={ok} />)
  expect(screen.queryByRole("textbox")).toBeNull()
})
