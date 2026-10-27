import { Box, type BoxProps } from "@chakra-ui/react";
import type { ReactNode } from "react";
import type { Tone } from "../tone";

export function Badge({ tone = "muted", children, ...rest }: { tone?: Tone; children: ReactNode } & Omit<BoxProps, "children">) {
  return (
    <Box
      as="span"
      data-tone={tone}
      display="inline-block"
      px="2"
      borderRadius="sm"
      fontFamily="condensed"
      fontSize="base"
      fontWeight="600"
      textTransform="uppercase"
      color={`tone.${tone}.fg`}
      bg={`tone.${tone}.bg`}
      {...rest}
    >
      {children}
    </Box>
  );
}
