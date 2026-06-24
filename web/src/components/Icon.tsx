import type { JSX } from "preact"

// Nerd Font icon names mirror the keys in tokens.css (--i-*), which in turn
// mirror ui/icons.go. The glyph is injected via the --glyph custom property
// consumed by the .nf class in base.css.
export type IconName =
  | "check"
  | "close"
  | "branch"
  | "worktree"
  | "folder-closed"
  | "folder-open"
  | "file-modified"
  | "file-added"
  | "file-deleted"
  | "file-renamed"
  | "ahead"
  | "behind"
  | "search"
  | "staged"
  | "warning"
  | "info"
  | "star"
  | "copy"
  | "github"

interface IconProps {
  name: IconName
  color?: string
  class?: string
  style?: JSX.CSSProperties
}

export function Icon({ name, color, class: cls, style }: IconProps) {
  return (
    <i
      class={cls ? `nf ${cls}` : "nf"}
      aria-hidden="true"
      style={{ "--glyph": `var(--i-${name})`, color, ...style }}
    />
  )
}
