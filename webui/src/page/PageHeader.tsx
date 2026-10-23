import { Box, Heading } from "@chakra-ui/react";
import type { ReactNode } from "react";
import { BackLink } from "../nav/BackLink";

// Back link, condensed h1 and an optional line under it.
export function PageHeader({
  backTo,
  backLabel,
  title,
  children,
}: {
  backTo: string;
  backLabel: string;
  title: string;
  children?: ReactNode;
}) {
  return (
    <>
      <BackLink to={backTo} label={backLabel} />
      <Box mt="4">
        <Heading as="h1" fontFamily="condensed" fontSize="2xl">
          {title}
        </Heading>
        {children}
      </Box>
    </>
  );
}
