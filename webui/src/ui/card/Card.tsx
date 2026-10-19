import { Box, Text } from "@chakra-ui/react";
import type { ReactNode } from "react";
import type { Tone } from "../tone";

export function Card({ title, tone = "muted", children }: { title?: string; tone?: Tone; children: ReactNode }) {
  return (
    <Box
      data-tone={tone}
      bg="surface.raised"
      borderWidth="1px"
      borderColor="border"
      borderLeftWidth="3px"
      borderLeftColor={`tone.${tone}.fg`}
      borderRadius="md"
      p="3"
    >
      {title && (
        <Text fontFamily="condensed" fontSize="lg" fontWeight="600" mb="2">
          {title}
        </Text>
      )}
      {children}
    </Box>
  );
}
