import { Box } from "@chakra-ui/react";
import type { ReactNode } from "react";
import { useConnectionContext, useConnectionState } from "../connection";
import { ConnectedTopBar } from "../nav/ConnectedTopBar";
import { PageContainer } from "../ui/PageContainer";
import { Strip } from "../ui/strip/Strip";
import type { TopBarTab } from "../ui/top-bar/TopBar";

// The frame every page shares: offline strip, page container and the connected top bar.
export function DetailPage({ active, children }: { active?: TopBarTab; children: ReactNode }) {
  const conn = useConnectionContext();
  const { connected } = useConnectionState(conn);
  return (
    <Box>
      {!connected && <Strip tone="waiting">Offline, reconnecting…</Strip>}
      <PageContainer>
        <ConnectedTopBar active={active} />
        {children}
      </PageContainer>
    </Box>
  );
}
