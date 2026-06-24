import { hydrate, prerender as ssr } from "preact-iso"

import "./styles/tokens.css"
import "./styles/fonts.css"
import "./styles/base.css"
import "./styles/primitives.css"
import "./styles/layout.css"

import { App } from "./app.tsx"

// Client: hydrate the prerendered markup in #app.
if (typeof window !== "undefined") {
  const root = document.getElementById("app")
  if (root) hydrate(<App />, root)
}

// Build: @preact/preset-vite calls this to render static HTML into #app.
export async function prerender() {
  return await ssr(<App />)
}
