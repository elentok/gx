import { act, cleanup, fireEvent, screen, waitFor } from "@testing-library/react"
import { afterEach, describe, expect, test, vi } from "vitest"
import { session } from "../agents/agentsTestKit"
import type { ChatDraftsClient } from "../design/chatDrafts"
import type { ChatDraft } from "../generated/server"
import { slugProblem } from "../design/draftSlug"
import type { DraftResult } from "../design/drafts"
import { renderThemed } from "../ui/render"
import { RenameHost } from "./RenameHost"
import { closeRename, openRename } from "./renameOpen"

// The real check, observed: the popup and the page field must share it.
const { slugProblemSpy } = vi.hoisted(() => ({ slugProblemSpy: vi.fn() }))
vi.mock("../design/draftSlug", async (importOriginal) => {
  const real = await importOriginal<typeof import("../design/draftSlug")>()
  slugProblemSpy.mockImplementation(real.slugProblem)
  return { ...real, slugProblem: slugProblemSpy }
})

afterEach(() => {
  closeRename()
  cleanup()
})

const chat = { ...session("s1", "gamma"), name: "talk" }
const field = () => screen.getByRole("textbox", { name: "Title" }) as HTMLInputElement
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
  await waitFor(() => expect(screen.queryByRole("textbox", { name: "Title" })).toBeNull())
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

test("a session request still opens Rename chat with the Title field", () => {
  open()
  expect(screen.getByRole("dialog", { name: "Rename chat" })).toBeTruthy()
  expect(field().placeholder).toBe("Title")
})

describe("a chat draft request", () => {
  const ok = (revision = 3) => ({
    kind: "ok" as const,
    draft: { id: "c1", brief: "hi", revision } as ChatDraft,
  })
  const openChatDraft = (drafts: Partial<ChatDraftsClient>) => {
    renderThemed(<RenameHost />)
    act(() =>
      openRename({
        chatDraft: { id: "c1", name: "old" },
        drafts: drafts as ChatDraftsClient,
      }),
    )
  }

  test("saves the new name through the client with the read brief and revision, then closes", async () => {
    const save = vi.fn(async () => ok(4))
    openChatDraft({ read: vi.fn(async () => ok()), save })
    expect(screen.getByRole("dialog", { name: "Rename draft" })).toBeTruthy()
    expect(field().value).toBe("old")
    expect(field().placeholder).toBe("Title")
    fireEvent.change(field(), { target: { value: " fresh " } })
    fireEvent.submit(field().closest("form")!)
    await waitFor(() => expect(screen.queryByRole("textbox", { name: "Title" })).toBeNull())
    expect(save).toHaveBeenCalledExactlyOnceWith({
      id: "c1",
      brief: "hi",
      revision: 3,
      name: "fresh",
    })
  })

  test("a conflict keeps the form open with the reason", async () => {
    openChatDraft({
      read: vi.fn(async () => ok()),
      save: vi.fn(async () => ({ kind: "conflict" as const })),
    })
    fireEvent.submit(field().closest("form")!)
    expect(await screen.findByText("The draft changed. Try again.")).toBeTruthy()
  })
})

describe("a draft request", () => {
  const draft = { id: "d1", project: "p", slug: "old-slug" }
  const slug = () => screen.getByRole("textbox", { name: "Slug" }) as HTMLInputElement
  const openDraft = (saveFields = vi.fn(async () => ({ kind: "deleted" }) as DraftResult)) => {
    renderThemed(<RenameHost />)
    act(() => openRename({ draft, saveFields }))
    return saveFields
  }
  const submit = () => fireEvent.submit(slug().closest("form")!)

  test("opens titled Rename draft with the current slug and no auto-name line", () => {
    openDraft()
    expect(screen.getByRole("dialog", { name: "Rename draft" })).toBeTruthy()
    expect(slug().value).toBe("old-slug")
    expect(screen.queryByText(/Automatic name/)).toBeNull()
  })

  test("a valid slug is written once with the draft id and no project, then closes", async () => {
    const save = openDraft()
    fireEvent.change(slug(), { target: { value: "new-slug" } })
    submit()
    await waitFor(() => expect(screen.queryByRole("textbox", { name: "Slug" })).toBeNull())
    expect(save).toHaveBeenCalledExactlyOnceWith({ id: "d1", slug: "new-slug" })
  })

  test("an empty submit is enabled and clears the slug", async () => {
    const save = openDraft()
    fireEvent.change(slug(), { target: { value: "" } })
    expect((screen.getByRole("button", { name: "Rename" }) as HTMLButtonElement).disabled).toBe(
      false,
    )
    submit()
    await waitFor(() => expect(save).toHaveBeenCalledExactlyOnceWith({ id: "d1", slug: "" }))
  })

  test("an invalid slug shows the page field's message, writes nothing and stays open", () => {
    const save = openDraft()
    fireEvent.change(slug(), { target: { value: "Bad Slug" } })
    submit()
    expect(screen.getByRole("alert").textContent).toBe(slugProblem("Bad Slug"))
    expect(save).not.toHaveBeenCalled()
    expect(slug().value).toBe("Bad Slug")
  })

  test("validates through slugProblem, the page field's one check, for valid, invalid and empty", async () => {
    openDraft()
    for (const value of ["new-slug", "Bad Slug", ""]) {
      slugProblemSpy.mockClear()
      fireEvent.change(slug(), { target: { value } })
      submit()
      expect(slugProblemSpy).toHaveBeenCalledWith(value)
    }
  })

  test("a gone draft shows a refusal and stays open", async () => {
    openDraft(vi.fn(async () => ({ kind: "gone" }) as DraftResult))
    submit()
    expect((await screen.findByRole("alert")).textContent).toContain("no longer exists")
  })
})
