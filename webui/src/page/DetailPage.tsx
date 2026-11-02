import { Box } from "@chakra-ui/react";
import { type ReactNode, useEffect } from "react";
import { useActionableCount } from "../agents/useActionableCount";
import { useConnectionContext, useConnectionState } from "../connection";
import { type PageMode, PageModeContext } from "../useWide";
import { ConnectedTopBar } from "../nav/ConnectedTopBar";
import { FailedMigrationStrip } from "../restart/FailedMigrationStrip";
import { MigratingStrip } from "../restart/MigratingStrip";
import { PageContainer } from "../ui/PageContainer";
import { TOP_BAR_HEIGHT_VAR } from "../ui/modal/useVisualViewportHeight";
import { Strip } from "../ui/strip/Strip";
import type { TopBarTab } from "../ui/top-bar/TopBar";
import { DiskStrips } from "./DiskStrips";
import { formatTitle, type PageTitle } from "./title";

// The only place that writes `document.title`. Writes on mount and whenever the string changes;
// the next page sets its own, so nothing is restored.
function usePageTitle(title: PageTitle) {
  const text = formatTitle(title, useActionableCount());
  useEffect(() => {
    document.title = text;
  }, [text]);
}

// The frame every page shares: the connected top bar, the strips under it and the page container.
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
        <PageContainer fill={fill}>
          <ConnectedTopBar active={active} waiting={waiting} ruled={fill} />
          {/* Under the bar, not above it: a banner above would push the sticky bar down and sit beneath the iOS status bar. They stick below the bar. */}
          <Box mx="-4" position="sticky" top={`var(${TOP_BAR_HEIGHT_VAR}, 0px)`} zIndex="sticky">
            {!connected && <Strip tone="waiting">Offline, reconnecting…</Strip>}
            <MigratingStrip />
            <DiskStrips />
          </Box>
          <FailedMigrationStrip />
          {children}
        </PageContainer>
      </Box>
    </PageModeContext.Provider>
  );
}
