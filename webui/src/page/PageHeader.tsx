import { Box, Heading } from "@chakra-ui/react";
import type { ReactNode } from "react";
import { useWide } from "../useWide";
import { titleSize } from "../nav/titleSize";

/** Ring box sides: the height of the title plus one line, so a header with no line keeps the same height. */
export const ringSize = (wide: boolean) => (wide ? 56 : 44);

// Condensed h1 and an optional line under it. The shared Design and Epic header uses
// `ring`, `line`, `meta` and `right`; `lead` and `children` are the older form.
export function PageHeader({
  title,
  heading,
  lead,
  ring,
  line,
  meta,
  right,
  children,
}: {
  title: string;
  /** Replaces the plain h1, for a title that is a control. It must render the h1 itself. */
  heading?: ReactNode;
  /** Sits left of the title, such as a ProgressRing. */
  lead?: ReactNode;
  /** The progress ring, boxed to the shared ring size beside the title. */
  ring?: ReactNode;
  /** One truncated line of text under the title. Nothing renders when it is empty. */
  line?: ReactNode;
  /** The meta bar under a hairline. Nothing renders when it is empty. */
  meta?: ReactNode;
  /** Page controls at the right edge, such as the menu button. */
  right?: ReactNode;
  children?: ReactNode;
}) {
  const wide = useWide();
  const h1 = heading ?? (
    <Heading as="h1" fontFamily="condensed" {...titleSize(wide)}>
      {title}
    </Heading>
  );
  const gap = wide ? "18px" : "14px";
  const side = ring ?? lead;
  const sideBox = ring ? (
    <Box data-testid="page-header-ring" flex="none" boxSize={`${ringSize(wide)}px`}>
      {ring}
    </Box>
  ) : (
    lead
  );
  const text = (
    <Box minW="0" flex="1">
      {h1}
      {line ? (
        <Box
          data-testid="page-header-line"
          mt="2px"
          color="text.dim"
          fontSize={wide ? "15px" : "14px"}
          whiteSpace="nowrap"
          overflow="hidden"
          textOverflow="ellipsis"
        >
          {line}
        </Box>
      ) : null}
    </Box>
  );
  return (
    <Box>
      {side || right || line ? (
        <Box display="flex" alignItems="center" gap={gap}>
          {sideBox}
          {text}
          {right ? <Box flex="none">{right}</Box> : null}
        </Box>
      ) : (
        h1
      )}
      {meta ? (
        <Box data-testid="page-header-meta" mt="10px" pt="8px" borderTopWidth="1px" borderColor="hairline">
          {meta}
        </Box>
      ) : null}
      {children}
    </Box>
  );
}
