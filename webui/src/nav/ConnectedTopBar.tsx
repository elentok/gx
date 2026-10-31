import { useState } from "react";
import { useActionableCount } from "../agents/useActionableCount";
import { toggleAfk } from "../afk";
import { useConnectionContext, useConnectionState } from "../connection";
import { NewVersionStrip, useBuildChanged } from "../restart/NewVersionStrip";
import { RestartDialog } from "../restart/RestartDialog";
import { TopBar, type TopBarTab } from "../ui/top-bar/TopBar";
import { UsageMenuBlock } from "../usage/UsageMenuBlock";
import { toggleViMode } from "../vimode";

// The top bar for pages that do not receive the connection state as a prop.
export function ConnectedTopBar({ active, waiting }: { active?: TopBarTab; waiting?: number }) {
  const conn = useConnectionContext();
  const { snapshot, connected } = useConnectionState(conn);
  const actionable = useActionableCount();
  const [usageShown, setUsageShown] = useState(false);
  const [restartOpen, setRestartOpen] = useState(false);
  const buildChanged = useBuildChanged(snapshot?.build);
  return (
    <>
      {buildChanged && <NewVersionStrip />}
      <TopBar
        active={active}
        build={snapshot?.build}
        mode={snapshot?.mode}
        connected={connected}
        waiting={waiting}
        actionable={actionable}
        afk={snapshot?.afk}
        health={snapshot?.health}
        onToggleAfk={() => toggleAfk(snapshot?.afk)}
        viMode={snapshot?.vi_mode}
        onToggleViMode={() => toggleViMode(snapshot?.vi_mode)}
        onRestart={() => setRestartOpen(true)}
        menuBlock={<UsageMenuBlock conn={conn} onShownChange={setUsageShown} />}
        menuBlockShown={usageShown}
      />
      {restartOpen && <RestartDialog liveInstance={snapshot?.instance} onClose={() => setRestartOpen(false)} />}
    </>
  );
}
