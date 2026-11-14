import { act, fireEvent } from "@testing-library/react"
import { afterEach, describe, expect, it, vi } from "vitest"
import { type HostedKind, type HostedResolved, HostedFound } from "../generated/ops"
import { renderThemed } from "../ui/render"
import { CannotHoverContext } from "../useWide"
import { FileLink } from "./FileLink"
import { FileLinkProvider, useFoundFile } from "./FileLinkContext"
import type { FileResolver } from "./resolve"

afterEach(() => vi.restoreAllMocks())

const settle = () => act(async () => {})

function resolverFor(kind: HostedKind, recheck: "found" | "missing" = "found"): FileResolver {
  return {
    async resolve(candidates, options) {
      const out = new Map<string, HostedResolved>()
      for (const c of candidates) {
        const verdict = options?.fresh ? recheck : HostedFound
        out.set(c, {
          candidate: c,
          verdict,
          path: "/abs/docs/a.x",
          size: 100,
          kind,
          modified: "t1",
        })
      }
      return out
    },
  }
}

function Link() {
  const found = useFoundFile("docs/a.x")
  if (!found?.path) return null
  return <FileLink path={found.path}>a.x</FileLink>
}

async function setup(cannotHover: boolean, recheck: "found" | "missing" = "found") {
  const view = renderThemed(
    <CannotHoverContext.Provider value={cannotHover}>
      <FileLinkProvider resolver={resolverFor("code", recheck)}>
        <Link />
      </FileLinkProvider>
    </CannotHoverContext.Provider>,
  )
  await settle()
  return { ...view, link: view.container.querySelector("a") as HTMLElement }
}

const sheet = () => document.querySelector("[data-file-sheet]")
const card = () => document.querySelector('[data-file-hover-card][data-state="open"]')

describe("FileLinkSheet", () => {
  it("opens on tap without navigating, with path, preview, Open and Copy link", async () => {
    const { link } = await setup(true)
    const before = window.location.hash
    const notPrevented = fireEvent.click(link)
    await settle()
    expect(notPrevented).toBe(false)
    expect(window.location.hash).toBe(before)
    const s = sheet()
    expect(s?.textContent).toContain("/abs/docs/a.x")
    expect(s?.querySelector("[data-preview-body]")).not.toBeNull()
    expect(s?.querySelector("[data-file-open]")?.getAttribute("href")).toBe("#/files/abs/docs/a.x")
    expect(s?.textContent).toContain("Copy link")
  })

  it("on a hover device a hover opens the card and a click opens no sheet", async () => {
    const { link } = await setup(false)
    await act(async () => {
      fireEvent.pointerMove(link)
      fireEvent.pointerEnter(link)
      fireEvent.mouseEnter(link)
      await new Promise((r) => setTimeout(r, 200))
    })
    expect(card()).not.toBeNull()
    fireEvent.click(link)
    await settle()
    expect(sheet()).toBeNull()
  })

  it("Copy link copies the stable viewer link, with no raw route or token", async () => {
    const write = vi.fn().mockResolvedValue(undefined)
    vi.stubGlobal("navigator", { ...navigator, clipboard: { writeText: write } })
    const { link } = await setup(true)
    fireEvent.click(link)
    await settle()
    const copy = [...sheet()!.querySelectorAll("button")].find((b) =>
      b.textContent?.includes("Copy link"),
    )!
    await act(async () => {
      fireEvent.click(copy)
    })
    expect(write).toHaveBeenCalledWith(`${window.location.origin}/#/files/abs/docs/a.x`)
    expect(write.mock.calls[0][0]).not.toMatch(/\/v1\/files|token/)
    vi.unstubAllGlobals()
  })

  it("closes on Escape and on a tap outside", async () => {
    const { link } = await setup(true)
    fireEvent.click(link)
    await settle()
    await act(async () => {
      fireEvent.keyDown(document, { key: "Escape" })
    })
    expect(sheet()).toBeNull()
    fireEvent.click(link)
    await settle()
    const panel = sheet() as HTMLElement
    await act(async () => {
      fireEvent.mouseDown(panel.parentElement as HTMLElement)
      fireEvent.click(panel.parentElement as HTMLElement)
    })
    expect(sheet()).toBeNull()
  })

  it("shows File not found for a missing file, with Open still going to the viewer", async () => {
    const { link } = await setup(true, "missing")
    fireEvent.click(link)
    await settle()
    expect(sheet()?.textContent).toContain("File not found")
    expect(sheet()?.querySelector("[data-file-open]")?.getAttribute("href")).toBe(
      "#/files/abs/docs/a.x",
    )
  })
})
