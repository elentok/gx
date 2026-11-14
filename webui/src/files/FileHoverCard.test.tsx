import { act, fireEvent } from "@testing-library/react"
import { describe, expect, it, vi } from "vitest"
import { type HostedKind, type HostedResolved, HostedFound } from "../generated/ops"
import { renderThemed } from "../ui/render"
import { CannotHoverContext, CoarsePointerContext } from "../useWide"
import { FileLink } from "./FileLink"
import { FileLinkProvider, useFoundFile } from "./FileLinkContext"
import type { FileResolver } from "./resolve"

const settle = () => act(async () => {})

function resolverFor(kind: HostedKind, size: number, recheck: "found" | "missing" = "found") {
  const calls: { candidates: string[]; fresh?: boolean }[] = []
  const resolver: FileResolver = {
    async resolve(candidates, options) {
      calls.push({ candidates, fresh: options?.fresh })
      const out = new Map<string, HostedResolved>()
      for (const c of candidates) {
        const verdict = options?.fresh ? recheck : HostedFound
        out.set(c, { candidate: c, verdict, path: "/abs/docs/a.x", size, kind, modified: "t1" })
      }
      return out
    },
  }
  return { resolver, calls }
}

function Link({ line }: { line?: number }) {
  const found = useFoundFile("docs/a.x")
  if (!found?.path) return null
  return (
    <FileLink path={found.path} line={line}>
      a.x
    </FileLink>
  )
}

async function setup(
  kind: HostedKind,
  size = 100,
  recheck: "found" | "missing" = "found",
  line?: number,
) {
  const { resolver, calls } = resolverFor(kind, size, recheck)
  const fetchSpy = vi.spyOn(globalThis, "fetch")
  const view = renderThemed(
    <FileLinkProvider resolver={resolver}>
      <Link line={line} />
    </FileLinkProvider>,
  )
  await settle()
  const link = view.container.querySelector("a") as HTMLElement
  return { ...view, link, calls, fetchSpy }
}

const card = () => document.querySelector('[data-file-hover-card][data-state="open"]')

async function hover(link: HTMLElement) {
  await act(async () => {
    fireEvent.pointerMove(link)
    fireEvent.pointerEnter(link)
    fireEvent.mouseEnter(link)
    await new Promise((r) => setTimeout(r, 200))
  })
}

describe("FileHoverCard", () => {
  it("opens on hover with path, size and an Open link carrying the line", async () => {
    const { link } = await setup("code", 2048, "found", 7)
    expect(card()).toBeNull()
    await hover(link)
    const c = card()
    expect(c?.textContent).toContain("/abs/docs/a.x")
    expect(c?.textContent).toContain("2.0 KB")
    expect(c?.querySelector("[data-file-open]")?.getAttribute("href")).toBe(
      "#/files/abs/docs/a.x?line=7",
    )
  })

  it("opens on focus and closes on Escape", async () => {
    const { link } = await setup("code")
    await act(async () => {
      link.focus()
      fireEvent.focus(link)
      await new Promise((r) => setTimeout(r, 200))
    })
    expect(card()).not.toBeNull()
    await act(async () => {
      fireEvent.keyDown(link, { key: "Escape" })
    })
    await settle()
    expect(card()).toBeNull()
  })

  it("shows an HTML placeholder with the Sandboxed tag and no iframe", async () => {
    const { link } = await setup("html")
    await hover(link)
    expect(card()?.textContent).toContain("No live preview")
    expect(card()?.textContent).toContain("Sandboxed")
    expect(document.querySelector("iframe")).toBeNull()
  })

  it.each([
    ["a file over 2 MB", "text", 3 * 1024 * 1024],
    ["a PDF", "pdf", 100],
    ["a binary", "binary", 100],
  ] as const)("shows no preview for %s and fetches nothing", async (_n, kind, size) => {
    const { link, fetchSpy } = await setup(kind, size)
    await hover(link)
    expect(card()?.textContent).toContain("No preview")
    expect(fetchSpy).not.toHaveBeenCalled()
  })

  it("re-checks once on open; a missing verdict shows File not found and dims the link", async () => {
    const { link, calls } = await setup("code", 100, "missing")
    await hover(link)
    expect(calls.filter((c) => c.fresh)).toEqual([{ candidates: ["/abs/docs/a.x"], fresh: true }])
    expect(card()?.textContent).toContain("File not found")
    expect(link.hasAttribute("data-missing")).toBe(true)
  })

  it("renders no card on a touch device", async () => {
    const { resolver } = resolverFor("code", 100)
    const view = renderThemed(
      <CoarsePointerContext.Provider value={true}>
        <FileLinkProvider resolver={resolver}>
          <Link />
        </FileLinkProvider>
      </CoarsePointerContext.Provider>,
    )
    await settle()
    await hover(view.container.querySelector("a") as HTMLElement)
    expect(card()).toBeNull()
  })

  it("shows the sheet, not nothing, on a coarse pointer that can still hover", async () => {
    const { resolver } = resolverFor("code", 100)
    const view = renderThemed(
      <CoarsePointerContext.Provider value={true}>
        <CannotHoverContext.Provider value={false}>
          <FileLinkProvider resolver={resolver}>
            <Link />
          </FileLinkProvider>
        </CannotHoverContext.Provider>
      </CoarsePointerContext.Provider>,
    )
    await settle()
    await act(async () => {
      fireEvent.click(view.container.querySelector("a") as HTMLElement)
    })
    expect(document.querySelector("[data-file-sheet]")).not.toBeNull()
    expect(card()).toBeNull()
  })
})
