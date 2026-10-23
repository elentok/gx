import { Box, chakra, Text } from "@chakra-ui/react";
import { useState, type ReactNode } from "react";
import type { Tone } from "../tone";

interface SidebarSectionProps {
  title: string;
  count?: number;
  tone?: Tone;
  defaultCollapsed?: boolean;
  children: ReactNode;
}

export function SidebarSection({ title, count, tone = "muted", defaultCollapsed = false, children }: SidebarSectionProps) {
  const [collapsed, setCollapsed] = useState(defaultCollapsed);
  return (
    <Box as="section" data-tone={tone} mb="4">
      <chakra.button
        display="flex"
        w="100%"
        alignItems="center"
        gap="1.5"
        mb="1"
        px="2"
        fontSize="xs"
        color="text.dim"
        textTransform="uppercase"
        letterSpacing="wide"
        cursor="pointer"
        aria-expanded={!collapsed}
        onClick={() => setCollapsed((c) => !c)}
      >
        <Text as="span" aria-hidden>
          {collapsed ? "▸" : "▾"}
        </Text>
        <Text as="span">{title}</Text>
        {count !== undefined && (
          <Text as="span" color={`tone.${tone}.fg`}>
            {count}
          </Text>
        )}
      </chakra.button>
      {!collapsed && children}
    </Box>
  );
}
