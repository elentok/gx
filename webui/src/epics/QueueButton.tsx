import { EpicActionButton } from "./EpicActionButton";
import { postQueueAdd } from "./queue";

export function QueueButton({ address, state, onQueued }: { address: string; state: string; onQueued: () => void }) {
  return (
    <EpicActionButton
      address={address}
      state={state}
      post={postQueueAdd}
      label="Queue"
      pendingLabel="Queueing…"
      onDone={onQueued}
    />
  );
}
