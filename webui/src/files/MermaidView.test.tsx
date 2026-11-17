import { act } from "@testing-library/react"
import { useState } from "react"
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import { renderThemed } from "../ui/render"
import { MarkdownView } from "./MarkdownView"
import { MermaidView } from "./MermaidView"
import { stubResolver } from "./stubResolver.testutil"

const mocks = vi.hoisted(() => ({
  factory: vi.fn(),
  renderAsync: vi.fn(),
}))

vi.mock("pele/lazy", () => {
  mocks.factory()
  return { renderAsync: mocks.renderAsync }
})

const settle = () => act(async () => {})
const scrollIntoView = vi.fn()

beforeEach(() => {
  mocks.renderAsync.mockImplementation(async () => ({ svg: "<svg data-testid='dia'></svg>" }))
  Element.prototype.scrollIntoView = scrollIntoView
})
afterEach(() => vi.clearAllMocks())

async function markdown(text: string, heading?: string) {
  const view = renderThemed(
    <MarkdownView
      path="/r/doc.md"
      text={text}
      source={false}
      heading={heading}
      resolver={stubResolver([]).resolver}
    />,
  )
  await settle()
  return view
}

describe("Mermaid", () => {
  // First on purpose: the loader keeps the module once any later test has imported it.
  it("never imports pele for a Markdown file without a fence", async () => {
    await markdown("# Title\n\n```js\nconst a = 1\n```\n")
    expect(mocks.factory).not.toHaveBeenCalled()
    expect(mocks.renderAsync).not.toHaveBeenCalled()
  })

  it("renders a Mermaid file as a diagram with strict security, and Source shows the text", async () => {
    function Toggle() {
      const [source, setSource] = useState(false)
      return (
        <>
          <button onClick={() => setSource(true)}>source</button>
          <MermaidView text="graph TD; A-->B" source={source} />
        </>
      )
    }
    const { container, getByText } = renderThemed(<Toggle />)
    await settle()
    expect(mocks.renderAsync).toHaveBeenCalledWith("graph TD; A-->B", expect.objectContaining({ responsive: false }))
    expect(container.querySelector("svg")).not.toBeNull()
    await act(async () => getByText("source").click())
    expect(container.querySelector("svg")).toBeNull()
    expect(container.textContent).toContain("graph TD; A-->B")
  })

  it("renders one diagram per mermaid fence", async () => {
    const text = "```mermaid\ngraph TD; A-->B\n```\n\ntext\n\n```mermaid\ngraph LR; C-->D\n```\n"
    const { container } = await markdown(text)
    await settle()
    expect(mocks.renderAsync).toHaveBeenCalledTimes(2)
    expect(container.querySelectorAll("svg")).toHaveLength(2)
  })

  it("shows the error text and the source when a render fails", async () => {
    mocks.renderAsync.mockRejectedValue(new Error("Parse error on line 1"))
    const { container, getByRole } = renderThemed(
      <MermaidView text="not a diagram" source={false} />,
    )
    await settle()
    expect(getByRole("alert").textContent).toBe("Parse error on line 1")
    expect(container.textContent).toContain("not a diagram")
  })
})

describe("heading scroll", () => {
  const doc = "# Intro\n\ntext\n\n## Usage\n\n## Intro\n"

  it("scrolls to the heading with that slug, counting repeats", async () => {
    await markdown(doc, "intro-1")
    expect(scrollIntoView).toHaveBeenCalledTimes(1)
    const target = scrollIntoView.mock.contexts[0] as HTMLElement
    expect(target.textContent).toBe("Intro")
    expect(target.tagName).toBe("H2")
  })

  it("scrolls nowhere and shows no error for an unknown slug", async () => {
    const { container } = await markdown(doc, "nope")
    expect(scrollIntoView).not.toHaveBeenCalled()
    expect(container.querySelector("[role=alert]")).toBeNull()
  })

  it("adds no id to the headings", async () => {
    const { container } = await markdown(doc, "intro")
    expect(container.querySelector("[id]")).toBeNull()
  })

  it("waits for diagrams to render before scrolling", async () => {
    let finish!: (v: { svg: string }) => void
    mocks.renderAsync.mockReturnValue(new Promise((r) => (finish = r)))
    await markdown("```mermaid\ngraph TD; A-->B\n```\n\n## Usage\n", "usage")
    expect(scrollIntoView).not.toHaveBeenCalled()
    await act(async () => finish({ svg: "<svg></svg>" }))
    expect(scrollIntoView).toHaveBeenCalledTimes(1)
  })
})
