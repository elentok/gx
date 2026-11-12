import { Box, chakra, Image } from "@chakra-ui/react"
import { useDevice } from "../useWide"
import { useConnectionContext, useConnectionState } from "../connection"
import { type Shell, useShell } from "./ShellContext"

// The bare mascot that opens the sidebar. On desktop it shows only while the sidebar is collapsed.
export function MascotToggle() {
  const shell = useShell()
  const device = useDevice()
  if (!shell || (device === "desktop" && shell.sidebarOpen)) return null
  return <Toggle shell={shell} />
}

function Toggle({ shell }: { shell: Shell }) {
  const { snapshot } = useConnectionState(useConnectionContext())
  const needsYou = !shell.sidebarOpen && (snapshot?.needs_you_total ?? 0) > 0
  return (
    <chakra.button
      type="button"
      aria-label="Show sidebar"
      aria-expanded={shell.sidebarOpen}
      onClick={shell.toggle}
      position="relative"
      flex="none"
      boxSize="40px"
      borderRadius="8px"
      display="grid"
      placeItems="center"
      cursor="pointer"
      _hover={{ bg: "surface.raised" }}
    >
      <Image src="/gx-menu-mascot.png" alt="" draggable={false} boxSize="32px" />
      {needsYou && (
        <Box
          data-testid="mascot-dot"
          position="absolute"
          top="1px"
          right="1px"
          boxSize="10px"
          borderRadius="full"
          bg="tone.waiting.fg"
          borderWidth="2px"
          borderColor="surface"
        />
      )}
    </chakra.button>
  )
}
