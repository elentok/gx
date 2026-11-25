import { fireEvent } from "@testing-library/react"
import { describe, expect, it } from "vitest"
import { renderThemed } from "../ui/render"
import { DraftFooter } from "./DraftFooter"

const REASON = "Write a prompt first"

function setup() {
  const view = renderThemed(
    <DraftFooter
      canAttach={false}
      startDisabled
      startReason={REASON}
      pending={false}
      onAttach={() => {}}
      onRemove={() => {}}
    />,
  )
  const start = view.getByRole("button", { name: "Start design" })
  return { ...view, start }
}

// fireEvent ignores `pointer-events`, so the style is asserted: a real tap or hover on `none` never lands.
describe("DraftFooter blocked Start", () => {
  it("takes pointer events, so a hover or tap can reach it", () => {
    const { start } = setup()
    expect(getComputedStyle(start).pointerEvents).not.toBe("none")
  })

  it("opens its reason tooltip on a tap", async () => {
    const { start, findByRole } = setup()
    fireEvent.click(start)
    expect((await findByRole("tooltip")).textContent).toBe(REASON)
  })
})
