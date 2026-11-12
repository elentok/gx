import { cleanup, render } from "@testing-library/react"
import { afterEach, describe, expect, it } from "vitest"
import { focusTopDialog, useIsTopLayer } from "./layers"

afterEach(cleanup)

function Layer({ name, tops }: { name: string; tops: Record<string, () => boolean> }) {
  tops[name] = useIsTopLayer()
  return null
}

describe("useIsTopLayer", () => {
  it("reports only the last mounted layer as topmost, and the one beneath again after it unmounts", () => {
    const tops: Record<string, () => boolean> = {}
    const { rerender } = render(<Layer name="a" tops={tops} />)
    expect(tops.a()).toBe(true)
    rerender(
      <>
        <Layer name="a" tops={tops} />
        <Layer name="b" tops={tops} />
      </>,
    )
    expect(tops.a()).toBe(false)
    expect(tops.b()).toBe(true)
    rerender(<Layer name="a" tops={tops} />)
    expect(tops.a()).toBe(true)
  })

  it("returns the same function across re-renders", () => {
    const tops: Record<string, () => boolean> = {}
    const seen: (() => boolean)[] = []
    const { rerender } = render(<Layer name="a" tops={tops} />)
    seen.push(tops.a)
    rerender(<Layer name="a" tops={tops} />)
    seen.push(tops.a)
    expect(seen[1]).toBe(seen[0])
  })
})

describe("focusTopDialog", () => {
  it("focuses the topmost dialog's marked first field, else its panel, and leaves focus already inside alone", () => {
    render(
      <>
        <button>outside</button>
        <div aria-modal="true" tabIndex={-1} data-testid="lower" />
        <div aria-modal="true" tabIndex={-1} data-testid="top">
          <input data-first-field="" aria-label="first" />
          <input aria-label="other" />
        </div>
      </>,
    )
    const outside = document.querySelector("button")!
    focusTopDialog(outside)
    expect(document.activeElement).toBe(document.querySelector('[aria-label="first"]'))

    const other = document.querySelector<HTMLElement>('[aria-label="other"]')!
    other.focus()
    focusTopDialog(other)
    expect(document.activeElement).toBe(other)
  })

  it("falls back to the panel when no field is marked", () => {
    render(<div aria-modal="true" tabIndex={-1} data-testid="panel" />)
    focusTopDialog(document.body)
    expect(document.activeElement).toBe(document.querySelector('[data-testid="panel"]'))
  })
})
