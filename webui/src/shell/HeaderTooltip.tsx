import { Portal, Tooltip } from "@chakra-ui/react"
import type { ReactNode } from "react"
import { useShell } from "./ShellContext"

/**
 * A sidebar header button's tooltip. Uncontrolled, unlike `TipPopup`, so a tap never opens it. A
 * greyed-out button keeps it. It is disabled with the sidebar closed, because it lives in a portal
 * and would stay on screen.
 */
export function HeaderTooltip({ content, children }: { content: string; children: ReactNode }) {
  const closed = useShell()?.sidebarOpen === false
  return (
    <Tooltip.Root disabled={closed} lazyMount unmountOnExit>
      <Tooltip.Trigger asChild>{children}</Tooltip.Trigger>
      <Portal>
        <Tooltip.Positioner>
          <Tooltip.Content
            bg="surface.raised"
            color="text"
            borderWidth="1px"
            borderColor="border"
            borderRadius="md"
            px="2"
            py="1"
            fontSize="xs"
          >
            {content}
          </Tooltip.Content>
        </Tooltip.Positioner>
      </Portal>
    </Tooltip.Root>
  )
}
