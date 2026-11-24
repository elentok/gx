// @vitest-environment node
import { readFileSync } from "node:fs"
import { join } from "node:path"
import { createElement } from "react"
import { renderToStaticMarkup } from "react-dom/server"
import { MemoryRouter } from "react-router"
import { expect, test } from "vitest"
import { PrototypeSwitcher, SWITCHER_CSS } from "./PrototypeSwitcher"

const snippet = readFileSync(
  join(__dirname, "../../../skills/gx-prototype/switcher-snippet.html"),
  "utf8",
)

const attrs = (html: string, name: string) =>
  [...html.matchAll(new RegExp(`${name}="([^"]*)"`, "g"))].map((m) => m[1])

test("the component CSS is the snippet CSS", () => {
  const css = /<style>([\s\S]*?)<\/style>/.exec(snippet)?.[1] ?? ""
  expect(SWITCHER_CSS.trim()).toBe(css.trim())
})

test("the component markup has the snippet's classes, roles and labels", () => {
  const markup = renderToStaticMarkup(
    createElement(
      MemoryRouter,
      null,
      createElement(PrototypeSwitcher, { variants: [{ key: "A", name: "One" }] }),
    ),
  )
  const html = /<div class="mk-sw"[\s\S]*?<\/div>\s*<script>/.exec(snippet)?.[0] ?? ""
  expect(html).not.toBe("")
  for (const name of ["class", "role", "aria-label", "aria-live"]) {
    expect(attrs(markup, name), name).toEqual(attrs(html, name))
  }
  expect(markup).toContain(">MOCK</span>")
})
