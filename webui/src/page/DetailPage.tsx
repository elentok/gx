import { Box } from "@chakra-ui/react";
import type { ReactNode } from "react";
import { useConnectionContext, useConnectionState } from "../connection";
import { ConnectedTopBar } from "../nav/ConnectedTopBar";
import { PageContainer } from "../ui/PageContainer";
import { Strip } from "../ui/strip/Strip";
import type { TopBarTab } from "../ui/top-bar/TopBar";

// The frame every page shares: offline strip, page container and the connected top bar.
// `fill` makes the page exactly the window tall; its children then share the space below the top bar.
export function DetailPage({
  active,
  fill,
  children,
}: {
  active?: TopBarTab;
  fill?: boolean;
  children: ReactNode;
}) {
  const conn = useConnectionContext();
  const { connected } = useConnectionState(conn);
  return (
    <Box {...(fill && { h: "100dvh", display: "flex", flexDirection: "column" })}>
      {!connected && <Strip tone="waiting">Offline, reconnecting…</Strip>}
      <PageContainer fill={fill}>
        <ConnectedTopBar active={active} />
        {children}
      </PageContainer>
    </Box>
  );
}
