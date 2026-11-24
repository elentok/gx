/** The gap a confirm popover keeps from every screen edge, and from the other end of the side it sits on. */
export const CONFIRM_INSET = 8

export interface FitInput {
  anchor: { top: number; bottom: number }
  /** The popover's measured height. */
  height: number
  /** The visual viewport, so an on-screen keyboard counts. */
  viewport: { offsetTop: number; height: number }
}

export interface Fit {
  fits: boolean
  /** The larger side of the anchor: below, unless above is larger. */
  side: "above" | "below"
}

/** Whether a confirm popover fits on either side of its anchor, and which side is larger. */
export function confirmFit({ anchor, height, viewport }: FitInput): Fit {
  const above = anchor.top - viewport.offsetTop
  const below = viewport.offsetTop + viewport.height - anchor.bottom
  const side = below >= above ? "below" : "above"
  return { fits: height <= Math.max(above, below) - 2 * CONFIRM_INSET, side }
}
