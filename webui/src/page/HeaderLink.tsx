import { Link } from "@chakra-ui/react";
import type { ReactNode } from "react";

/** A bordered link styled like the header buttons beside it. */
export function HeaderLink({ href, children }: { href: string; children: ReactNode }) {
  return (
    <Link
      href={href}
      display="inline-flex"
      alignItems="center"
      h="40px"
      px="4"
      flexShrink="0"
      whiteSpace="nowrap"
      fontWeight="600"
      color="text"
      bg="surface.raised"
      borderWidth="1px"
      borderColor="border"
      borderRadius="8px"
      textDecoration="none"
      _hover={{ borderColor: "text.dim", textDecoration: "none" }}
    >
      {children}
    </Link>
  );
}
