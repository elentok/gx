import { fireEvent } from "@testing-library/react"
import { useState } from "react"
import { describe, expect, it } from "vitest"
import { renderThemed } from "../ui/render"
import { HtmlView } from "./HtmlView"

const PATH = "/work/my site/index.html"
const SOURCE = "<h1>hi</h1><script>window.x = 1</script>"

function Harness() {
  const [source, setSource] = useState(false)
  return (
    <>
      <button type="button" onClick={() => setSource((s) => !s)}>
        toggle
      </button>
      <HtmlView path={PATH} text={SOURCE} source={source} />
    </>
  )
}

describe("HtmlView", () => {
  it("shows the note above one sandboxed frame on the stable raw link", () => {
    const view = renderThemed(<HtmlView path={PATH} source={false} />)
    expect(view.getByText("Sandboxed page, scripts allowed, no access to gx")).toBeTruthy()
    const frames = view.container.querySelectorAll("iframe")
    expect(frames).toHaveLength(1)
    const frame = frames[0]
    expect(frame.getAttribute("sandbox")).toBe("allow-scripts")
    expect(frame.getAttribute("src")).toBe("/v1/files/work/my%20site/index.html")
    expect(frame.title).toBe("index.html")
  })

  it("shows the source as text and brings the frame back", () => {
    const view = renderThemed(<Harness />)
    fireEvent.click(view.getByText("toggle"))
    expect(view.container.querySelector("iframe")).toBeNull()
    expect(view.container.textContent).toContain("<script>window.x = 1</script>")
    expect(view.container.querySelector("script")).toBeNull()
    expect(view.container.querySelector("h1")).toBeNull()
    fireEvent.click(view.getByText("toggle"))
    expect(view.container.querySelectorAll("iframe")).toHaveLength(1)
  })
})
