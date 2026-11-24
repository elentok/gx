import { useEffect } from "react"
import { useSearchParams } from "react-router"
import { SWITCHER_CSS } from "./PrototypeSwitcher.css"

export interface PrototypeVariant {
  key: string
  name: string
}

function inField(t: EventTarget | null): boolean {
  const el = t as HTMLElement | null
  return !!el && (/^(INPUT|TEXTAREA|SELECT)$/.test(el.tagName) || el.isContentEditable)
}

/** Dev-only variant switcher for gx prototype routes. The variant lives in `?variant=`. */
export function PrototypeSwitcher({ variants }: { variants: PrototypeVariant[] }) {
  if (!import.meta.env.DEV) return null
  return <Switcher variants={variants} />
}

function Switcher({ variants }: { variants: PrototypeVariant[] }) {
  const [params, setParams] = useSearchParams()
  const n = variants.length
  const found = variants.findIndex((v) => v.key === params.get("variant"))
  const i = Math.max(found, 0)

  const step = (d: number) => {
    if (n === 0) return
    const next = new URLSearchParams(params)
    next.set("variant", variants[(i + d + n) % n].key)
    setParams(next)
  }

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (inField(e.target)) return
      if (e.key === "ArrowLeft") step(-1)
      else if (e.key === "ArrowRight") step(1)
    }
    document.addEventListener("keydown", onKey)
    return () => document.removeEventListener("keydown", onKey)
  })

  if (n === 0) return null
  return (
    <>
      <style>{SWITCHER_CSS}</style>
      {/* biome-ignore lint/a11y/useSemanticElements: markup must match the snippet */}
      <div className="mk-sw" role="group" aria-label="Mock variants">
        <button
          type="button"
          className="prev"
          aria-label="Previous variant"
          onClick={() => step(-1)}
        >
          ‹
        </button>
        <div className="mk-label" aria-live="polite">
          <b>
            {variants[i].key} of {n}
          </b>
          <small>{variants[i].name || variants[i].key}</small>
        </div>
        <button type="button" className="next" aria-label="Next variant" onClick={() => step(1)}>
          ›
        </button>
        <span className="mk-tag">MOCK</span>
      </div>
    </>
  )
}
