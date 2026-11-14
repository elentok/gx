import { act } from "@testing-library/react"
import { useState } from "react"
import { describe, expect, it } from "vitest"
import { FileLinkProvider } from "../files/FileLinkContext"
import { stubResolver } from "../files/stubResolver"
import { renderThemed } from "../ui/render"
import { Markdown } from "./Markdown"

it("renders emphasis, code, lists and links as elements", () => {
  const { container } = renderThemed(
    <Markdown>
      {"A **bold** word with `code`.\n\n- one\n- two\n\n[docs](https://example.com)"}
    </Markdown>,
  )
  expect(container.querySelector("strong")?.textContent).toBe("bold")
  expect(container.querySelector("code")?.textContent).toBe("code")
  expect(container.querySelectorAll("li")).toHaveLength(2)
  expect(container.querySelector("a")?.getAttribute("href")).toBe("https://example.com")
})

it("renders GFM task lists as read-only checkboxes, tables and strikethrough", () => {
  const { container } = renderThemed(
    <Markdown>{"- [x] done\n- [ ] todo\n\n| a | b |\n|---|---|\n| 1 | 2 |\n\n~~gone~~"}</Markdown>,
  )
  const boxes = container.querySelectorAll<HTMLInputElement>("input[type=checkbox]")
  expect(boxes).toHaveLength(2)
  expect(boxes[0].checked).toBe(true)
  expect(boxes[0].disabled).toBe(true)
  expect(container.querySelector("table td")?.textContent).toBe("1")
  expect(container.querySelector("del")?.textContent).toBe("gone")
})

it("links sibling ticket files to the ticket page and leaves other relative links as text", () => {
  const { container } = renderThemed(
    <Markdown epic="gx:my-epic">
      {"[a](issues/03-foo.md) [b](04-bar.md) [c](../README.md) [d](https://example.com)"}
    </Markdown>,
  )
  const links = Array.from(container.querySelectorAll("a"))
  expect(links.map((a) => a.getAttribute("href"))).toEqual([
    "#/t/gx:my-epic/03",
    "#/t/gx:my-epic/04",
    "https://example.com",
  ])
  expect(links[0].hasAttribute("target")).toBe(false)
  expect(links[2].getAttribute("target")).toBe("_blank")
  expect(links[2].getAttribute("rel")).toContain("noopener")
  expect(container.textContent).toContain("c")
})

it("renders relative links as plain text without an epic address", () => {
  const { container } = renderThemed(<Markdown>{"[a](issues/03-foo.md)"}</Markdown>)
  expect(container.querySelector("a")).toBeNull()
  expect(container.textContent).toBe("a")
})

it("renders its h2 headings smaller when asked", () => {
  const { container } = renderThemed(<Markdown smallHeadings>{"## Title"}</Markdown>)
  const h2 = container.querySelector("h2")
  expect(h2?.textContent).toBe("Title")
  expect(h2?.parentElement?.getAttribute("data-small-headings")).toBe("")
})

describe("file links", () => {
  const settle = () => act(async () => {})
  const hrefs = (c: HTMLElement) =>
    Array.from(c.querySelectorAll("a")).map((a) => a.getAttribute("href"))

  it("links found paths in prose, inline code and link targets", async () => {
    const { resolver } = stubResolver(["src/a.ts", "my dir/b.md", "c.md"])
    const { container } = renderThemed(
      <FileLinkProvider resolver={resolver}>
        <Markdown>{"See src/a.ts and `my dir/b.md` and [the c](c.md)."}</Markdown>
      </FileLinkProvider>,
    )
    await settle()
    expect(hrefs(container)).toEqual([
      "#/files/abs/src/a.ts",
      "#/files/abs/my%20dir/b.md",
      "#/files/abs/c.md",
    ])
    expect(container.querySelector("code a")?.textContent).toBe("my dir/b.md")
    expect(container.querySelectorAll("a")[2].textContent).toBe("the c")
  })

  it("leaves fenced blocks, missing paths and https links alone", async () => {
    const { resolver } = stubResolver(["src/a.ts"])
    const { container } = renderThemed(
      <FileLinkProvider resolver={resolver}>
        <Markdown>
          {"```\nsrc/a.ts\n```\n\nnope/x.ts [w](https://example.com/a/b) [m](gone.md)"}
        </Markdown>
      </FileLinkProvider>,
    )
    await settle()
    expect(hrefs(container)).toEqual(["https://example.com/a/b"])
    expect(container.querySelector("a")?.getAttribute("target")).toBe("_blank")
    expect(container.textContent).toContain("nope/x.ts")
    expect(container.textContent).toContain("m")
  })

  it("renders the same output without a provider", () => {
    const { container } = renderThemed(<Markdown>{"See src/a.ts and `b/c.md`."}</Markdown>)
    expect(container.querySelector("a")).toBeNull()
    expect(container.querySelector("span[data-file-candidate]")).toBeNull()
  })

  it("checks a fragment link as its file and links with the heading", async () => {
    const { resolver, calls } = stubResolver(["other.md"])
    const { container } = renderThemed(
      <FileLinkProvider resolver={resolver}>
        <Markdown>{"[i](other.md#install)"}</Markdown>
      </FileLinkProvider>,
    )
    await settle()
    expect(calls.flatMap((c) => c.candidates)).toEqual(["other.md"])
    expect(hrefs(container)).toEqual(["#/files/abs/other.md?heading=install"])
  })

  it("links a fragment-only link to the current file, or keeps it plain", async () => {
    const { resolver } = stubResolver([])
    const withFile = renderThemed(
      <FileLinkProvider resolver={resolver} file="/abs/doc.md">
        <Markdown>{"[i](#install)"}</Markdown>
      </FileLinkProvider>,
    )
    await settle()
    expect(hrefs(withFile.container)).toEqual(["#/files/abs/doc.md?heading=install"])
    withFile.unmount()
    const without = renderThemed(
      <FileLinkProvider resolver={resolver}>
        <Markdown>{"[i](#install)"}</Markdown>
      </FileLinkProvider>,
    )
    await settle()
    expect(without.container.querySelector("a")).toBeNull()
    expect(without.container.textContent).toBe("i")
  })

  it("batches blocks of one pass into one call and rechecks only missing paths on a new key", async () => {
    const { resolver, calls } = stubResolver(["a.md"])
    let setKey: (key: string) => void = () => {}
    function Page() {
      const [key, set] = useState("m1")
      setKey = set
      return (
        <FileLinkProvider resolver={resolver} recheckKey={key}>
          <Markdown>{"see a.md"}</Markdown>
          <Markdown>{"and b.md"}</Markdown>
          <Markdown>{"and `c/d.md`"}</Markdown>
        </FileLinkProvider>
      )
    }
    renderThemed(<Page />)
    await settle()
    expect(calls).toHaveLength(1)
    expect(calls[0].candidates.sort()).toEqual(["a.md", "b.md", "c/d.md"])
    act(() => setKey("m2"))
    await settle()
    expect(calls).toHaveLength(2)
    expect([...(calls[1].recheckMissing ?? [])].sort()).toEqual(["b.md", "c/d.md"])
    expect(calls[1].candidates.sort()).toEqual(["b.md", "c/d.md"])
  })
})

it("does not turn raw HTML into elements", () => {
  const { container } = renderThemed(<Markdown>{'<img src=x onerror="alert(1)"> hi'}</Markdown>)
  expect(container.querySelector("img")).toBeNull()
})
