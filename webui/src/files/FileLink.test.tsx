import { act } from "@testing-library/react"
import { describe, expect, it } from "vitest"
import { renderThemed } from "../ui/render"
import { FileLink } from "./FileLink"
import { FileLinkProvider, useFileLinks } from "./FileLinkContext"
import { LinkedText } from "./LinkedText"
import { stubResolver } from "./stubResolver.testutil"

const settle = () => act(async () => {})

describe("FileLink", () => {
  it("is a same-tab link with the file icon and the dotted mono look", () => {
    const { container } = renderThemed(<FileLink path="/abs/a.md">a.md</FileLink>)
    const a = container.querySelector("a")
    expect(a?.getAttribute("href")).toBe("#/files/abs/a.md")
    expect(a?.hasAttribute("target")).toBe(false)
    expect(a?.querySelector("svg")).not.toBeNull()
    const style = Array.from(document.querySelectorAll("style"))
      .map((s) => s.textContent)
      .join("")
    expect(style).toMatch(/underline dotted/)
    expect(style).toMatch(/monospace|mono/i)
  })

  it("dims and drops the underline once the context marks the path missing", async () => {
    function Mark() {
      const ctx = useFileLinks()
      return <button onClick={() => ctx?.markMissing("/abs/a.md")}>mark</button>
    }
    const { container, getByText } = renderThemed(
      <FileLinkProvider resolver={stubResolver([]).resolver}>
        <FileLink path="/abs/a.md">a.md</FileLink>
        <Mark />
      </FileLinkProvider>,
    )
    const a = container.querySelector("a")
    expect(a?.hasAttribute("data-missing")).toBe(false)
    await act(async () => getByText("mark").click())
    expect(container.querySelector("a")?.hasAttribute("data-missing")).toBe(true)
    const style = Array.from(document.querySelectorAll("style"))
      .map((s) => s.textContent)
      .join("")
    expect(style).toMatch(/\[data-missing\][^}]*opacity:\s*0\.55/)
    expect(style).toMatch(/\[data-missing\][^}]*text-decoration:\s*none/)
  })
})

describe("LinkedText", () => {
  it("links found paths and leaves the rest as text", async () => {
    const { resolver } = stubResolver(["src/a.ts"])
    const { container } = renderThemed(
      <FileLinkProvider resolver={resolver}>
        <LinkedText>{"Read src/a.ts:12 and gone/b.ts now"}</LinkedText>
      </FileLinkProvider>,
    )
    await settle()
    const links = container.querySelectorAll("a")
    expect(links).toHaveLength(1)
    expect(links[0].getAttribute("href")).toBe("#/files/abs/src/a.ts?line=12")
    expect(links[0].textContent).toBe("src/a.ts:12")
    expect(container.textContent).toBe("Read src/a.ts:12 and gone/b.ts now")
  })

  it("renders plain text without a provider", () => {
    const { container } = renderThemed(<LinkedText>{"see src/a.ts"}</LinkedText>)
    expect(container.querySelector("a")).toBeNull()
    expect(container.textContent).toBe("see src/a.ts")
  })
})
