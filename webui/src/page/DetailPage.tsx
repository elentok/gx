import { Box } from "@chakra-ui/react";
import { type ReactNode, useEffect } from "react";
import { useActionableCount } from "../agents/useActionableCount";
import { PageContainer } from "../ui/PageContainer";
import { formatTitle, type PageTitle } from "./title";

// The only place that writes `document.title`. Writes on mount and whenever the string changes;
// the next page sets its own, so nothing is restored.
function usePageTitle(title: PageTitle) {
  const text = formatTitle(title, useActionableCount());
  useEffect(() => {
    document.title = text;
  }, [text]);
}

// The frame every page shares: the page container under the shell's strips.
// `fill` makes the page exactly the window tall; its children then share that height.
export function DetailPage({
  fill,
  title,
  children,
}: {
  title: PageTitle;
  fill?: boolean;
  children: ReactNode;
}) {
  usePageTitle(title);
  return (
    <Box {...(fill && { h: "100dvh", display: "flex", flexDirection: "column" })}>
      <PageContainer fill={fill}>{children}</PageContainer>
    </Box>
  );
}
