import { Box } from "@chakra-ui/react";
import type { ReactNode } from "react";
import type { Tone } from "../tone";

export function EmptyState({ tone = "muted", title, children }: { tone?: Tone; title: string; children?: ReactNode }) {
  return (
    <Box data-tone={tone} py="6" textAlign="center">
      <Box fontFamily="condensed" fontSize="xl" fontWeight="600" color={`tone.${tone}.fg`}>
        {title}
      </Box>
      {children && (
        <Box mt="1" fontSize="md" color="text.muted">
          {children}
        </Box>
      )}
    </Box>
  );
}
