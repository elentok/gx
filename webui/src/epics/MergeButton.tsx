import { epicHref, ticketHref } from "../hrefs";
import type { ToastSpec } from "../ui/toaster";
import { EpicActionButton } from "./EpicActionButton";
import { postEpicMerge, type EpicWriteResult } from "./queue";

/** The states a person can merge from: a done epic whose merge has not started, or whose merge paused. */
export const isMergeable = (state: string | undefined, mergeBlocked: boolean) =>
  state === "awaiting-merge" || (state === "paused" && mergeBlocked);

/** The merge button's toast: the epic does not change in place, so every accepted merge says what happened. */
export function mergeToast(address: string, res: EpicWriteResult): ToastSpec {
  if (res.refused) return { type: "error", title: res.message || res.reason };
  if (res.parked) {
    return {
      type: "warning",
      title: res.message || "The merge paused.",
      link: { href: epicHref(address), label: "Open the paused epic" },
    };
  }
  if (res.merge_ticket) {
    return {
      type: "info",
      title: `Merge ticket ${res.merge_ticket} created`,
      link: { href: ticketHref(res.merge_ticket), label: "Open the merge ticket" },
    };
  }
  return { type: "success", title: "Epic merged" };
}

export function MergeButton({ address, state, onMerged }: { address: string; state: string; onMerged: () => void }) {
  return (
    <EpicActionButton
      address={address}
      state={state}
      post={postEpicMerge}
      label="Merge"
      tone="done"
      pendingLabel="Merging…"
      success={(res) => mergeToast(address, res)}
      onDone={onMerged}
    />
  );
}
