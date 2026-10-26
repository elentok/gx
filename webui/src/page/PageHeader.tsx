import { Box, Heading } from "@chakra-ui/react";
import type { ReactNode } from "react";
import { BackLink } from "../nav/BackLink";

// Back link, condensed h1 and an optional line under it.
export function PageHeader({
  backTo,
  backLabel,
  waiting,
  title,
  heading,
  lead,
  children,
}: {
  backTo: string;
  backLabel: string;
  /** Other epics in Needs you, shown on the back link. */
  waiting?: number;
  title: string;
  /** Replaces the plain h1, for a title that is a control. It must render the h1 itself. */
  heading?: ReactNode;
  /** Sits left of the title, such as a ProgressRing. */
  lead?: ReactNode;
  children?: ReactNode;
}) {
  const h1 = heading ?? (
    <Heading as="h1" fontFamily="condensed" fontSize="2xl">
      {title}
    </Heading>
  );
  return (
    <>
      <BackLink to={backTo} label={backLabel} waiting={waiting} />
      <Box mt="4">
        {lead ? (
          <Box display="flex" alignItems="flex-start" gap="4">
            {lead}
            <Box minW="0" flex="1">
              {h1}
            </Box>
          </Box>
        ) : (
          h1
        )}
        {children}
      </Box>
    </>
  );
}
