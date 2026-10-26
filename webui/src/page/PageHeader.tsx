import { Box, Heading } from "@chakra-ui/react";
import type { ReactNode } from "react";
import { BackLink } from "../nav/BackLink";

// Back link, condensed h1 and an optional line under it.
export function PageHeader({
  backTo,
  backLabel,
  title,
  heading,
  children,
}: {
  backTo: string;
  backLabel: string;
  title: string;
  /** Replaces the plain h1, for a title that is a control. It must render the h1 itself. */
  heading?: ReactNode;
  children?: ReactNode;
}) {
  return (
    <>
      <BackLink to={backTo} label={backLabel} />
      <Box mt="4">
        {heading ?? (
          <Heading as="h1" fontFamily="condensed" fontSize="2xl">
            {title}
          </Heading>
        )}
        {children}
      </Box>
    </>
  );
}
