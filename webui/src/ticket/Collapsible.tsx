import { Box } from "@chakra-ui/react"
import { useState, type ReactNode } from "react"
import { Button } from "../ui/button/Button"

// Collapsed by default; a button instead of <details> so the state is testable and themable.
export function Collapsible({
  label,
  children,
  muted,
}: {
  label: string
  children: ReactNode
  /** Mutes the toggle; the default keeps the accent colour. */
  muted?: boolean
}) {
  const [open, setOpen] = useState(false)
  return (
    <Box css={muted ? { "& > button": { color: "var(--chakra-colors-text-muted)" } } : undefined}>
      <Button emphasis="ghost" size="sm" aria-expanded={open} onClick={() => setOpen(!open)}>
        {open ? "▾" : "▸"} {label}
      </Button>
      {open && <Box mt="2">{children}</Box>}
    </Box>
  )
}
