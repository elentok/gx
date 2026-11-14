import { act } from "@testing-library/react"
import { useState } from "react"
import { describe, expect, it } from "vitest"
import { renderThemed } from "../ui/render"
import { MarkdownView } from "./MarkdownView"
import { stubResolver } from "./stubResolver"

const settle = () => act(async () => {})
const FILE = "/repo/docs/guide.md"

async function show(text: string, found: string[] = [], source = false) {
  const stub = stubResolver(found)
  const view = renderThemed(
    <MarkdownView path={FILE} text={text} source={source} resolver={stub.resolver} />,
  )
  await settle()
  return { ...view, ...stub }
}

describe("MarkdownView", () => {
  it("renders by default and shows the raw text as Source, and back", async () => {
    const stub = stubResolver([])
    const text = "# Title\n\nsome *words*"
    function Toggle() {
      const [source, setSource] = useState(false)
      return (
        <>
          <button onClick={() => setSource((s) => !s)}>toggle</button>
          <MarkdownView path={FILE} text={text} source={source} resolver={stub.resolver} />
        </>
      )
    }
    const { container, getByText } = renderThemed(<Toggle />)
    expect(container.querySelector("h1")?.textContent).toBe("Title")
    await act(async () => getByText("toggle").click())
    expect(container.querySelector("h1")).toBeNull()
    expect(container.textContent).toContain("some *words*")
    await act(async () => getByText("toggle").click())
    expect(container.querySelector("h1")?.textContent).toBe("Title")
  })

  it("links a found relative file to its viewer URL", async () => {
    const { container } = await show("[other](other.md)", ["other.md"])
    expect(container.querySelector("a")?.getAttribute("href")).toBe("#/files/abs/other.md")
  })

  it("keeps a missing target, an outside-roots path and an unknown scheme as text", async () => {
    const { container } = await show(
      "[a](gone.md) [b](/etc/passwd.md) [c](mailto:x@y.com) [d](foo://bar)",
    )
    expect(container.querySelector("a")).toBeNull()
    expect(container.textContent).toContain("a b c d")
  })

  it("checks other.md without its fragment and links with heading", async () => {
    const { container, calls } = await show("[x](other.md#install)", ["other.md"])
    expect(calls.flatMap((c) => c.candidates)).toEqual(["other.md"])
    expect(container.querySelector("a")?.getAttribute("href")).toBe(
      "#/files/abs/other.md?heading=install",
    )
  })

  it("links a fragment-only link to the same file with heading", async () => {
    const { container } = await show("[x](#install)")
    expect(container.querySelector("a")?.getAttribute("href")).toBe(
      `#/files${FILE}?heading=install`,
    )
  })

  it("loads a relative image through the raw route", async () => {
    const { container } = await show("![shot](../img/my%20shot.png)")
    expect(container.querySelector("img")?.getAttribute("src")).toBe(
      "/v1/files/repo/img/my%20shot.png",
    )
  })

  it("leaves an https image alone", async () => {
    const { container } = await show("![x](https://example.com/a.png)")
    expect(container.querySelector("img")?.getAttribute("src")).toBe("https://example.com/a.png")
  })

  it("shows raw HTML as text, not elements", async () => {
    const { container } = await show("<b>bold</b> <script>x</script>")
    expect(container.querySelector("b")).toBeNull()
    expect(container.querySelector("script")).toBeNull()
  })
})
