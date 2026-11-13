import { act, cleanup, fireEvent, screen, waitFor } from "@testing-library/react"
import { afterEach, expect, test, vi } from "vitest"
import { session } from "../agents/agentsTestKit"
import { renderThemed } from "../ui/render"
import { RenameHost } from "./RenameHost"
import { closeRename, openRename } from "./renameOpen"

afterEach(() => {
  closeRename()
  cleanup()
})

const chat = { ...session("s1", "gamma"), name: "talk" }
const field = () => screen.getByRole("textbox", { name: "Name" }) as HTMLInputElement
const open = (rename = vi.fn(async () => ({})), s = chat) => {
  renderThemed(<RenameHost />)
  act(() => openRename({ session: s, rename }))
  return rename
}

test("opens with the current name selected", () => {
  open()
  expect(field().value).toBe("talk")
  expect(field().selectionStart).toBe(0)
  expect(field().selectionEnd).toBe(4)
})

test("Enter renames with the trimmed name and closes", async () => {
  const rename = open()
  fireEvent.change(field(), { target: { value: "  new name  " } })
  fireEvent.submit(field().closest("form")!)
  await waitFor(() => expect(screen.queryByRole("textbox", { name: "Name" })).toBeNull())
  expect(rename).toHaveBeenCalledExactlyOnceWith({ id: "s1", name: "new name" })
})

test("Esc closes with no call", () => {
  const rename = open()
  fireEvent.keyDown(field(), { key: "Escape" })
  expect(screen.queryByRole("textbox", { name: "Name" })).toBeNull()
  expect(rename).not.toHaveBeenCalled()
})

// A text input strips line breaks from its value, so only the blank rule can be driven through the DOM.
test("a blank name disables Rename and sends nothing", () => {
  const rename = open()
  fireEvent.change(field(), { target: { value: "  " } })
  expect((screen.getByRole("button", { name: "Rename" }) as HTMLButtonElement).disabled).toBe(true)
  fireEvent.submit(field().closest("form")!)
  expect(rename).not.toHaveBeenCalled()
})

test("a refusal shows its reason, keeps the text and stays open", async () => {
  const rename = vi.fn(async () => ({ refused: true, reason: "name-taken", message: "Taken" }))
  open(rename)
  fireEvent.change(field(), { target: { value: "dup" } })
  fireEvent.submit(field().closest("form")!)
  expect(await screen.findByText("Taken")).toBeTruthy()
  expect(field().value).toBe("dup")
})

test("the auto-name line shows only when auto_name is set", () => {
  open(undefined, { ...chat, auto_name: true })
  expect(screen.getByText("Automatic name from the terminal title")).toBeTruthy()
  cleanup()
  closeRename()
  open()
  expect(screen.queryByText("Automatic name from the terminal title")).toBeNull()
})
