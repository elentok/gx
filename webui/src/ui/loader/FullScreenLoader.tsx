import { Box, Image, Spinner, Text } from "@chakra-ui/react"

// A centered loader that fills the content area: mascot, spinner and a label. Use it when a whole view waits on something.
export function FullScreenLoader({ label }: { label: string }) {
  return (
    <Box
      role="status"
      aria-label={label}
      h="100%"
      minH="70vh"
      display="flex"
      flexDirection="column"
      alignItems="center"
      justifyContent="center"
      gap="4"
    >
      <Image src="/gx-menu-mascot.png" alt="" draggable={false} boxSize="64px" />
      <Spinner size="lg" borderWidth="3px" color="tone.muted.fg" />
      <Text color="text.muted">{label}</Text>
    </Box>
  )
}
