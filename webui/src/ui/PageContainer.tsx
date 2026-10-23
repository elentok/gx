import { Box } from "@chakra-ui/react";
import type { ReactNode } from "react";

export function PageContainer({ children }: { children: ReactNode }) {
  return (
    <Box p="4" maxW="1366px" mx="auto" data-page-container>
      {children}
    </Box>
  );
}
