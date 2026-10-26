import { Box } from "@chakra-ui/react";
import type { ReactNode } from "react";

export function PageContainer({ children, fill }: { children: ReactNode; fill?: boolean }) {
  return (
    <Box
      p="4"
      maxW="1366px"
      mx="auto"
      data-page-container
      {...(fill && { display: "flex", flexDirection: "column", flex: "1", minH: "0", w: "100%" })}
    >
      {children}
    </Box>
  );
}
