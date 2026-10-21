import { useConnectionContext, useConnectionState } from "../connection";
import { TopBar, type TopBarTab } from "../ui/top-bar/TopBar";

// The top bar for pages that do not receive the connection state as a prop.
export function ConnectedTopBar({ active }: { active?: TopBarTab }) {
  const { snapshot, connected } = useConnectionState(useConnectionContext());
  return <TopBar active={active} build={snapshot?.build} mode={snapshot?.mode} connected={connected} />;
}
