import { Box } from "@chakra-ui/react";
import type { ReactNode } from "react";
import { useConnectionContext, useConnectionState } from "../connection";
import { type PageMode, PageModeContext } from "../useWide";
import { ConnectedTopBar } from "../nav/ConnectedTopBar";
import { PageContainer } from "../ui/PageContainer";
import { Strip } from "../ui/strip/Strip";
import type { TopBarTab } from "../ui/top-bar/TopBar";
import { DiskStrips } from "./DiskStrips";
import { type PageTitle, usePageTitle } from "./title";

// The frame every page shares: offline strip, page container and the connected top bar.
// `fill` makes the page exactly the window tall; its children then share the space below the top bar.
// `mode` sets the breakpoint `useWide` answers by for everything below.
export function DetailPage({
  active,
  waiting,
  fill,
  mode,
  title,
  children,
}: {
  title: PageTitle;
  active?: TopBarTab;
  /** Other epics in Needs you, counted on the Epics tab. */
  waiting?: number;
  fill?: boolean;
  mode: PageMode;
  children: ReactNode;
}) {
  usePageTitle(title);
  const conn = useConnectionContext();
  const { connected } = useConnectionState(conn);
  return (
    <PageModeContext.Provider value={mode}>
      <Box {...(fill && { h: "100dvh", display: "flex", flexDirection: "column" })}>
        {!connected && <Strip tone="waiting">Offline, reconnecting…</Strip>}
        <DiskStrips />
        <PageContainer fill={fill}>
          <ConnectedTopBar active={active} waiting={waiting} />
          {children}
        </PageContainer>
      </Box>
    </PageModeContext.Provider>
  );
}
