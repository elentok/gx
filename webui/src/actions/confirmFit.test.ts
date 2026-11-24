import { describe, expect, test } from "vitest"
import { confirmFit, type FitInput } from "./confirmFit"

const anchor = (top: number, bottom: number) => ({ top, bottom })

function input(over: Partial<FitInput> = {}): FitInput {
  return {
    anchor: anchor(100, 130),
    height: 100,
    viewport: { offsetTop: 0, height: 800 },
    ...over,
  }
}

describe("confirmFit", () => {
  test("a popover as tall as the larger side minus both insets fits; one pixel more does not", () => {
    // Space below: 800 - 130 = 670, so the fitting height is 654.
    expect(confirmFit(input({ height: 654 })).fits).toBe(true)
    expect(confirmFit(input({ height: 655 })).fits).toBe(false)
  })

  test("picks the larger side, below unless it is the smaller one", () => {
    expect(confirmFit(input()).side).toBe("below")
    expect(confirmFit(input({ anchor: anchor(700, 730) })).side).toBe("above")
    expect(confirmFit(input({ anchor: anchor(400, 400) })).side).toBe("below")
  })

  test("a keyboard (shorter viewport) shrinks the space below", () => {
    const open = input({ anchor: anchor(300, 330), height: 400 })
    expect(confirmFit(open)).toEqual({ fits: true, side: "below" })
    const keyboard = { ...open, viewport: { offsetTop: 0, height: 500 } }
    expect(confirmFit(keyboard)).toEqual({ fits: false, side: "above" })
  })

  test("a scrolled visual viewport moves both sides", () => {
    // Visual viewport spans 200..700; anchor 250..280 leaves 50 above and 420 below.
    const r = confirmFit(
      input({ anchor: anchor(250, 280), viewport: { offsetTop: 200, height: 500 } }),
    )
    expect(r.side).toBe("below")
    expect(
      confirmFit(
        input({ anchor: anchor(250, 280), height: 404, viewport: { offsetTop: 200, height: 500 } }),
      ).fits,
    ).toBe(true)
    expect(
      confirmFit(
        input({ anchor: anchor(250, 280), height: 405, viewport: { offsetTop: 200, height: 500 } }),
      ).fits,
    ).toBe(false)
  })
})
