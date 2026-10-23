import { Box, Text } from "@chakra-ui/react";
import type { ReactNode } from "react";
import type { Tone } from "../tone";

interface SidebarProps {
  title?: string;
  tone?: Tone;
  // Slot in the title row, e.g. a ghost "+ New" Button.
  action?: ReactNode;
  children: ReactNode;
}

// The raised background sets the sidebar apart, so it needs no separator line.
export function Sidebar({ title, tone = "muted", action, children }: SidebarProps) {
  return (
    <Box as="aside" data-tone={tone} bg="surface.raised" h="100%" overflowY="auto" px="3" py="4">
      {(title || action) && (
        <Box display="flex" alignItems="center" mb="3" px="2">
          <Text fontFamily="condensed" fontWeight="600" textTransform="uppercase" color={`tone.${tone}.fg`} flex="1">
            {title}
          </Text>
          {action}
        </Box>
      )}
      {children}
    </Box>
  );
}
