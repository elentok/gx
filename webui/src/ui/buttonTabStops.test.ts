import { afterEach, expect, test } from "vitest"
import { installButtonTabStops } from "./buttonTabStops"

afterEach(() => {
  document.body.innerHTML = ""
})

test("makes present and later buttons tab stops, and keeps a tabindex a button has", async () => {
  document.body.innerHTML = `<button id="a">A</button><button id="b" tabindex="-1">B</button>`
  const stop = installButtonTabStops()
  expect(document.getElementById("a")!.getAttribute("tabindex")).toBe("0")
  expect(document.getElementById("b")!.getAttribute("tabindex")).toBe("-1")
  const wrap = document.createElement("div")
  wrap.innerHTML = `<button id="c">C</button>`
  document.body.append(wrap)
  await Promise.resolve()
  expect(document.getElementById("c")!.getAttribute("tabindex")).toBe("0")
  stop()
})
