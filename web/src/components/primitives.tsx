import type { ComponentChildren } from "preact"
import { useState } from "preact/hooks"
import { Icon, type IconName } from "./Icon.tsx"

/* Faux-TUI primitive components. Each wraps a class from primitives.css so the
   markup stays declarative and the styling stays in hand-written CSS. */

interface PanelProps {
  title?: ComponentChildren
  rightTitle?: ComponentChildren
  inset?: boolean
  id?: string
  class?: string
  children: ComponentChildren
}

export function Panel({ title, rightTitle, inset, id, class: cls, children }: PanelProps) {
  const classes = ["panel"]
  if (inset) classes.push("panel--inset")
  if (cls) classes.push(cls)
  return (
    <section id={id} class={classes.join(" ")}>
      {title != null && <span class="panel-title">{title}</span>}
      {rightTitle != null && (
        <span class="panel-title panel-title--right">{rightTitle}</span>
      )}
      {children}
    </section>
  )
}

export type BadgeVariant =
  | "default"
  | "deepbg"
  | "blue"
  | "green"
  | "yellow"
  | "orange"
  | "mauve"
  | "lime"

export function Badge({
  variant = "default",
  children,
}: {
  variant?: BadgeVariant
  children: ComponentChildren
}) {
  const cls = variant === "default" ? "badge" : `badge badge--${variant}`
  return (
    <span class={cls}>
      <span class="badge-body">{children}</span>
    </span>
  )
}

export function Button({
  href,
  primary,
  icon,
  trailingIcon,
  children,
}: {
  href: string
  primary?: boolean
  icon?: IconName
  trailingIcon?: IconName
  children: ComponentChildren
}) {
  return (
    <a class={primary ? "btn btn--primary" : "btn"} href={href}>
      {icon && <Icon name={icon} />}
      {children}
      {trailingIcon && <Icon name={trailingIcon} />}
    </a>
  )
}

export function Cmd({
  children,
  copy: showCopy,
}: {
  children: ComponentChildren
  copy?: boolean
}) {
  const [copied, setCopied] = useState(false)

  function handleCopy() {
    const text = typeof children === "string" ? children : ""
    navigator.clipboard?.writeText(text).then(() => {
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    })
  }

  return (
    <span class="cmd">
      <code>{children}</code>
      {showCopy && (
        <button
          class="cmd-copy"
          type="button"
          aria-label="copy to clipboard"
          title="copy to clipboard"
          onClick={handleCopy}
        >
          <Icon name={copied ? "check" : "copy"} />
        </button>
      )}
    </span>
  )
}

export type SyncState = "synced" | "ahead" | "behind" | "diverged"

export function SyncStatus({
  state,
  ahead = 0,
  behind = 0,
}: {
  state: SyncState
  ahead?: number
  behind?: number
}) {
  return (
    <span class="sync">
      {state === "synced" && <span class="sync-item sync-synced">synced</span>}
      {(state === "ahead" || state === "diverged") && (
        <span class="sync-item sync-ahead">{ahead}</span>
      )}
      {(state === "behind" || state === "diverged") && (
        <span class="sync-item sync-behind">{behind}</span>
      )}
    </span>
  )
}
