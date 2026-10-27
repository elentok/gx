import { epicHref, ticketHref } from "../hrefs";
import type { ToastSpec } from "../ui/toaster";
import { EpicActionButton } from "./EpicActionButton";
import { postEpicMerge, postEpicStashMerge, type EpicWriteResult } from "./queue";

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

interface MergeButtonProps {
  address: string;
  state: string;
  onMerged: () => void;
  label?: string;
  tooltip?: string;
}

export function MergeButton({ address, state, onMerged, label = "Merge", tooltip }: MergeButtonProps) {
  return (
    <EpicActionButton
      address={address}
      state={state}
      post={postEpicMerge}
      label={label}
      tone="done"
      pendingLabel="Merging…"
      tooltip={tooltip}
      success={(res) => mergeToast(address, res)}
      onDone={onMerged}
    />
  );
}

/** "Stash, merge and restore": its accepted outcomes show in place (the epic refetches), so only a refusal toasts. */
export function StashMergeButton({ address, state, onMerged, label = "Stash, merge and restore", tooltip }: MergeButtonProps) {
  return (
    <EpicActionButton
      address={address}
      state={state}
      post={postEpicStashMerge}
      label={label}
      pendingLabel="Merging…"
      tooltip={tooltip}
      onDone={onMerged}
    />
  );
}
