import { useEffect } from "react"
import { useSearchParams } from "react-router"

export interface PrototypeVariant {
  key: string
  name: string
}

// Kept byte-equal to the <style> in skills/gx-prototype/switcher-snippet.html (see the sync test).
// Fixed neutral colors on purpose: a prototype route must not depend on app tokens.
export const SWITCHER_CSS = `
.mk-sw{position:fixed;z-index:2147483000;left:50%;transform:translateX(-50%);
  bottom:calc(16px + env(safe-area-inset-bottom));display:flex;align-items:stretch;gap:2px;
  background:#f4f4f5;color:#18181b;border:2px solid #18181b;border-radius:999px;padding:4px;
  box-shadow:0 4px 16px rgba(0,0,0,.5);font:600 14px/1 ui-sans-serif,system-ui,sans-serif;
  -webkit-user-select:none;user-select:none;max-width:calc(100vw - 16px)}
.mk-sw button{all:unset;box-sizing:border-box;cursor:pointer;min-width:48px;min-height:48px;display:grid;place-items:center;
  border-radius:999px;font-size:20px;touch-action:manipulation}
.mk-sw button:active{background:#d4d4d8}
.mk-sw button:focus-visible{outline:3px solid #2563eb;outline-offset:1px}
.mk-sw .mk-label{display:flex;flex-direction:column;justify-content:center;align-items:center;gap:3px;
  padding:0 12px;min-width:112px;max-width:220px;text-align:center}
.mk-sw .mk-label b{font-size:15px}
.mk-sw .mk-label small{font:500 11px/1 ui-sans-serif,system-ui,sans-serif;color:#52525b;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;max-width:100%}
.mk-sw .mk-tag{align-self:center;background:#18181b;color:#f4f4f5;border-radius:999px;padding:5px 8px;font-size:10px;letter-spacing:.08em;margin:0 4px 0 8px}
`

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
