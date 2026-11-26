import { describe, expect, it } from "vitest"
import { renderThemed } from "../render"
import { TONES } from "../tone"
import { Card } from "./Card"

describe("Card", () => {
  it.each(TONES)("renders title and children in tone %s", (tone) => {
    const { container, getByText } = renderThemed(
      <Card title="Heading" tone={tone}>
        body
      </Card>,
    )
    expect(getByText("Heading")).toBeTruthy()
    expect(getByText("body")).toBeTruthy()
    expect(container.querySelector("[data-tone]")!.getAttribute("data-tone")).toBe(tone)
  })

  it("marks a live card and keeps its tone", () => {
    const { container, getByText } = renderThemed(
      <Card tone="running" live>
        body
      </Card>,
    )
    expect(getByText("body")).toBeTruthy()
    const card = container.querySelector("[data-live]")!
    expect(card.getAttribute("data-tone")).toBe("running")
  })

  it("is not live by default", () => {
    const { container } = renderThemed(<Card>body</Card>)
    expect(container.querySelector("[data-live]")).toBeNull()
  })

  it.each([true, false])("caps its height only when maxHeight is set (live=%s)", (live) => {
    const { container } = renderThemed(
      <>
        <Card live={live} maxHeight="100px" data-mock-part="capped">
          body
        </Card>
        <Card live={live} data-mock-part="free">
          body
        </Card>
      </>,
    )
    const part = (name: string) =>
      getComputedStyle(container.querySelector(`[data-mock-part='${name}']`)!)
    expect(part("capped").maxHeight).toBe("100px")
    expect(part("capped").flexDirection).toBe("column")
    expect(part("free").maxHeight).toBe("")
    expect(part("free").flexDirection).toBe("")
  })

  it("renders without a title", () => {
    const { getByText } = renderThemed(<Card>only body</Card>)
    expect(getByText("only body")).toBeTruthy()
  })
})
