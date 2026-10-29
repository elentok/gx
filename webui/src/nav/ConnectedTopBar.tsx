import { actionableCount } from "../agents/rules";
import { toggleAfk } from "../afk";
import { useConnectionContext, useConnectionState } from "../connection";
import { TopBar, type TopBarTab } from "../ui/top-bar/TopBar";

// The top bar for pages that do not receive the connection state as a prop.
export function ConnectedTopBar({ active, waiting }: { active?: TopBarTab; waiting?: number }) {
  const { snapshot, connected } = useConnectionState(useConnectionContext());
  return (
    <TopBar
      active={active}
      build={snapshot?.build}
      mode={snapshot?.mode}
      connected={connected}
      waiting={waiting}
      actionable={actionableCount(snapshot?.sessions ?? [])}
      afk={snapshot?.afk}
      health={snapshot?.health}
      onToggleAfk={() => toggleAfk(snapshot?.afk)}
    />
  );
}
