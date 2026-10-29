import { Box, Text } from "@chakra-ui/react";
import type { ReactNode } from "react";

// Centered card under the `gx` wordmark: the frame of the login, setup and crash screens.
export function AuthCard({ children, below }: { children: ReactNode; below?: ReactNode }) {
  return (
    <Box minH="100dvh" display="flex" flexDirection="column" alignItems="center" px="4" py={{ base: "8", md: "16" }}>
      <Box w="100%" maxW="400px">
        <Box textAlign="center" mb="6" data-mock-part="header">
          <Text fontFamily="condensed" fontWeight="700" fontSize="2xl" letterSpacing="0.02em" textTransform="uppercase">
            gx
          </Text>
          <Text color="text.dim" fontSize="sm">
            {window.location.hostname}
          </Text>
        </Box>
        <Box
          data-mock-part="card"
          bg="surface.raised"
          borderWidth="1px"
          borderColor="border"
          borderRadius="lg"
          p="6"
          display="flex"
          flexDirection="column"
          gap="4"
        >
          {children}
        </Box>
        {below}
      </Box>
    </Box>
  );
}
