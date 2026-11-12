import { act, fireEvent } from "@testing-library/react"
import { describe, expect, it, vi } from "vitest"
import type { SettingsEntry, SettingsWriteResult } from "../generated/server"
import { renderThemed } from "../ui/render"
import { ChecksEditor } from "./ChecksEditor"

const entry = (value?: unknown): SettingsEntry => ({
  key: "checks",
  path: ["checks"],
  type: "json",
  section: "Checks",
  description: "Commands gx check runs.",
  set: value !== undefined,
  value,
})

const setup = (value?: unknown, result: SettingsWriteResult = {}, repoChecksFile = false) => {
  const onWrite = vi.fn(async () => result)
  const view = renderThemed(
    <ChecksEditor entry={entry(value)} repoChecksFile={repoChecksFile} onWrite={onWrite} />,
  )
  return { ...view, onWrite }
}

const flush = () => act(async () => {})
const modeIn = (v: ReturnType<typeof setup>, kind: string, label: string) => {
  const group = v.getByRole("group", { name: `${kind} mode` })
  fireEvent.click(
    Array.from(group.querySelectorAll("button")).find((b) => b.textContent === label)!,
  )
}
const type = (v: ReturnType<typeof setup>, label: string, value: string) =>
  fireEvent.change(v.getByLabelText(label), { target: { value } })
const save = async (v: ReturnType<typeof setup>) => {
  fireEvent.click(v.getByRole("button", { name: "Save" }))
  await flush()
}

describe("ChecksEditor", () => {
  it.each([
    ["off", undefined, { format: false }],
    ["one command", "go fmt", { format: "go fmt" }],
    ["changed + full", "x", { format: { full: "x" } }],
  ])("format card: %s saves the matching value", async (label, text, want) => {
    const v = setup({ lint: "l" })
    modeIn(v, "format", label)
    if (label === "one command") type(v, "format command", text!)
    if (label === "changed + full") type(v, "format full command", text!)
    await save(v)
    expect(v.onWrite).toHaveBeenCalledWith([{ key: "checks", value: { lint: "l", ...want } }])
  })

  it("back to default removes the kind, and the last kind removes the key", async () => {
    const v = setup({ format: false })
    modeIn(v, "format", "default")
    await save(v)
    expect(v.onWrite).toHaveBeenCalledWith([{ key: "checks", remove: true }])
  })

  it("shows a dot only on a set kind and blocks saving an empty command", () => {
    const v = setup({ lint: "l" })
    expect(v.queryByTestId("check-dot-lint")).not.toBeNull()
    expect(v.queryByTestId("check-dot-format")).toBeNull()
    modeIn(v, "format", "one command")
    expect(v.getByText("Enter a command.")).toBeTruthy()
    expect((v.getByRole("button", { name: "Save" }) as HTMLButtonElement).disabled).toBe(true)
  })

  it("the test card sets suite, hang and slow; clearing a field removes the limit", async () => {
    const v = setup({ test: { changed: "c", full: "f", limits: { suite: "4m", hang: "20s" } } })
    type(v, "suite limit", "")
    type(v, "slow limit", "5s")
    await save(v)
    expect(v.onWrite).toHaveBeenCalledWith([
      {
        key: "checks",
        value: { test: { changed: "c", full: "f", limits: { hang: "20s", slow: "5s" } } },
      },
    ])
  })

  it("a slow map shows three fields and saves as a map", async () => {
    const v = setup({ test: { full: "f", limits: { slow: { go: "5s", vitest: "2s" } } } })
    expect(v.queryByLabelText("slow limit")).toBeNull()
    type(v, "slow playwright limit", "9s")
    await save(v)
    expect(v.onWrite).toHaveBeenCalledWith([
      {
        key: "checks",
        value: {
          test: { full: "f", limits: { slow: { go: "5s", vitest: "2s", playwright: "9s" } } },
        },
      },
    ])
  })

  it("adds and removes an always command", async () => {
    const v = setup({ test: { full: "f" } })
    type(v, "Add always command for test", "go vet")
    fireEvent.keyDown(v.getByLabelText("Add always command for test"), { key: "Enter" })
    await save(v)
    expect(v.onWrite).toHaveBeenCalledWith([
      { key: "checks", value: { test: { full: "f", always: ["go vet"] } } },
    ])
  })

  it("one command with a limit reloads as one command", async () => {
    const v = setup({
      test: { changed: "go test ./...", full: "go test ./...", limits: { suite: "4m" } },
    })
    const group = v.getByRole("group", { name: "test mode" })
    const pressed = Array.from(group.querySelectorAll("button")).filter(
      (b) => b.getAttribute("aria-pressed") === "true",
    )
    expect(pressed.map((b) => b.textContent)).toEqual(["one command"])
    expect((v.getByLabelText("test command") as HTMLInputElement).value).toBe("go test ./...")
    expect((v.getByLabelText("suite limit") as HTMLInputElement).value).toBe("4m")
  })

  it("shows the repo-file notice with the flag", () => {
    expect(setup(undefined, {}, true).queryByTestId("repo-checks-notice")).not.toBeNull()
  })

  it("shows no repo-file notice without the flag", () => {
    expect(setup().queryByTestId("repo-checks-notice")).toBeNull()
  })

  it("a refused save shows the reason", async () => {
    const v = setup({ lint: "l" }, { refused: true, reason: "bad command" })
    modeIn(v, "format", "off")
    await save(v)
    expect(v.getByText("Not saved: bad command")).toBeTruthy()
  })
})
