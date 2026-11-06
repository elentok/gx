import { Box } from "@chakra-ui/react";
import { type ReactNode, useEffect } from "react";
import { useActionableCount } from "../agents/useActionableCount";
import { ConnectedTopBar } from "../nav/ConnectedTopBar";
import { PageContainer } from "../ui/PageContainer";
import { StickyTopContainer } from "../ui/top-bar/StickyTopContainer";
import type { TopBarTab } from "../ui/top-bar/TopBar";
import { formatTitle, type PageTitle } from "./title";

// The only place that writes `document.title`. Writes on mount and whenever the string changes;
// the next page sets its own, so nothing is restored.
function usePageTitle(title: PageTitle) {
  const text = formatTitle(title, useActionableCount());
  useEffect(() => {
    document.title = text;
  }, [text]);
}

// The frame every page shares: the sticky banners and top bar, then the page container.
// `fill` makes the page exactly the window tall; its children then share the space below the top bar.
export function DetailPage({
  active,
  waiting,
  fill,
  title,
  children,
}: {
  title: PageTitle;
  active?: TopBarTab;
  /** Other epics in Needs you, counted on the Epics tab. */
  waiting?: number;
  fill?: boolean;
  children: ReactNode;
}) {
  usePageTitle(title);
  return (
    <Box {...(fill && { h: "100dvh", display: "flex", flexDirection: "column" })}>
      <PageContainer fill={fill}>
        <StickyTopContainer ruled={fill}>
          <ConnectedTopBar active={active} waiting={waiting} />
        </StickyTopContainer>
        {children}
      </PageContainer>
    </Box>
  );
}
