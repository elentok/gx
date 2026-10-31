import { Box, chakra, Text } from "@chakra-ui/react";
import type { ReactNode } from "react";
import { SETTINGS_HREF } from "../hrefs";

export const Group = ({ label, children }: { label?: string; children: ReactNode }) => (
  <Box mt="6">
    {label && (
      <Text fontSize="xs" letterSpacing="0.08em" textTransform="uppercase" color="text.muted" mb="2" px="3">
        {label}
      </Text>
    )}
    <Box borderWidth="1px" borderColor="border" borderRadius="lg" bg="surface.raised" overflow="hidden">
      {children}
    </Box>
  </Box>
);

export const rowProps = {
  display: "flex",
  alignItems: "center",
  gap: "3",
  px: "4",
  py: "3",
  minH: "52px",
  color: "text",
  textAlign: "left",
  width: "100%",
  cursor: "pointer",
  bg: "transparent",
  _notFirst: { borderTopWidth: "1px", borderTopColor: "hairline" },
  _hover: { bg: "surface" },
} as const;

export const Frame = ({ children }: { children: ReactNode }) => <Box maxW="560px">{children}</Box>;

export const BackLink = () => (
  <chakra.a
    href={SETTINGS_HREF}
    display="inline-flex"
    alignItems="center"
    h="8"
    px="3"
    fontSize="sm"
    fontWeight="600"
    color="text.muted"
    _hover={{ color: "text" }}
  >
    ‹ Settings
  </chakra.a>
);
