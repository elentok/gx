import { Box, Link, Portal, Toast, Toaster, createToaster } from "@chakra-ui/react"
import type { ComponentProps } from "react"

import { Button } from "./button/Button"

export const toaster = createToaster({ placement: "bottom-end", pauseOnPageIdle: true })
// Chakra fixes a toast's placement to its toaster, so a top-right toast needs its own toaster.
export const topToaster = createToaster({ placement: "top-end", pauseOnPageIdle: true })

export interface ToastSpec {
  type: "success" | "error" | "warning" | "info"
  title: string
  link?: { href: string; label: string }
  description?: string
  /** Showing the same id again updates that toast instead of adding one. */
  id?: string
  /** Stays until the close button or code removes it. */
  persistent?: boolean
  /** Buttons under the text; each click also dismisses the toast. */
  actions?: { label: string; onClick: () => void }[]
  /** A raised card with an amber left edge instead of a filled alert; the close button stays. */
  card?: boolean
  /** Shows the toast top-right instead of bottom-right. Remove it with `topToaster`. */
  placement?: "top-end"
  /** Runs when the toast starts closing, from the close button or a dismiss. */
  onClose?: () => void
}

export function showToast({
  type,
  title,
  link,
  description,
  id,
  persistent,
  actions,
  card,
  placement,
  onClose,
}: ToastSpec) {
  const target = placement === "top-end" ? topToaster : toaster
  // Chakra spreads these over its defaults, so an undefined id or duration would erase them.
  target.create({
    ...(id && { id }),
    ...(persistent && { duration: Infinity }),
    ...((actions || card) && { meta: { actions, card } }),
    ...(onClose && {
      onStatusChange: ({ status }: { status: string }) => {
        if (status === "unmounted") onClose()
      },
    }),
    type,
    title,
    description: link ? <Link href={link.href}>{link.label}</Link> : description,
  })
}

type ToastOf = Parameters<NonNullable<ComponentProps<typeof Toaster>["children"]>>[0]

function renderToast(toast: ToastOf, owner: typeof toaster) {
  const actions = toast.meta?.actions as ToastSpec["actions"]
  const card = toast.meta?.card as boolean | undefined
  const raised = !!actions || !!card
  // A toast with buttons is a raised card like the Dev popover, not a filled alert.
  return (
    <Toast.Root
      width={{ md: actions ? "320px" : "sm" }}
      {...(raised && {
        bg: "surface.raised",
        color: "text",
        borderWidth: "1px",
        borderColor: "border",
      })}
      {...(card && { borderLeftWidth: "3px", borderLeftColor: "tone.waiting.fg" })}
    >
      {!raised && <Toast.Indicator />}
      <Box minW="0" flex="1">
        {toast.title && (
          <Toast.Title {...(raised && { fontWeight: "700", fontSize: "14px" })}>
            {toast.title}
          </Toast.Title>
        )}
        {toast.description && (
          // A long body scrolls inside the toast so the buttons stay on screen.
          <Toast.Description maxH="40vh" overflowY="auto" overflowWrap="anywhere">
            {toast.description}
          </Toast.Description>
        )}
        {actions && (
          <Box display="flex" gap="1.5" flexWrap="wrap" mt="2">
            {actions.map((a, i) => (
              <Button
                key={a.label}
                size="sm"
                emphasis="secondary"
                borderColor={i === actions.length - 1 ? "tone.running.fg" : undefined}
                onClick={() => {
                  a.onClick()
                  owner.dismiss(toast.id)
                }}
              >
                {a.label}
              </Button>
            ))}
          </Box>
        )}
      </Box>
      <Toast.CloseTrigger />
    </Toast.Root>
  )
}

/** Mount once under the ChakraProvider. */
export function AppToaster() {
  return (
    <Portal>
      <Toaster toaster={toaster} insetInline={{ mdDown: "4" }}>
        {(toast) => renderToast(toast, toaster)}
      </Toaster>
      <Toaster toaster={topToaster} insetInline={{ mdDown: "4" }}>
        {(toast) => renderToast(toast, topToaster)}
      </Toaster>
    </Portal>
  )
}
