import { Box } from "@chakra-ui/react"
import type { ReactNode } from "react"

export function PageContainer({ children, fill }: { children: ReactNode; fill?: boolean }) {
  return (
    <Box
      p="4"
      pt="0"
      // A full-height page ends in its composer. On desktop its bottom row lines up with the docked sidebar's footer (8px of padding); phone and tablet keep the same small gap.
      {...(fill && { pb: "1.5" })}
      maxW="1366px"
      mx="auto"
      data-page-container
      {...(fill && { display: "flex", flexDirection: "column", flex: "1", minH: "0", w: "100%" })}
    >
      {children}
    </Box>
  )
}
