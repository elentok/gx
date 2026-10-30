import { Box, chakra } from "@chakra-ui/react";
import type { ReactNode } from "react";
import { SETTINGS_HREF } from "../hrefs";

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
