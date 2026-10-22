import { EpicActionButton } from "./EpicActionButton";
import { postEpicMerge } from "./queue";

/** The states a person can merge from: a done epic whose merge has not started, or whose merge paused. */
export const isMergeable = (state: string | undefined, mergeBlocked: boolean) =>
  state === "awaiting-merge" || (state === "paused" && mergeBlocked);

export function MergeButton({ address, state, onMerged }: { address: string; state: string; onMerged: () => void }) {
  return (
    <EpicActionButton
      address={address}
      state={state}
      post={postEpicMerge}
      label="Merge"
      pendingLabel="Merging…"
      failure={(err) => `Merge failed: ${err instanceof Error ? err.message : String(err)}`}
      onDone={onMerged}
    />
  );
}
