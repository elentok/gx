import { Text } from "@chakra-ui/react";
import type { PendingRow } from "./generated/server";

// The one verdict that means the ticket is starting or running, so it shows nothing.
const CLAIMED = "claimed";

// "<verdict>: <reason>" for a queued, unclaimed ticket, else undefined. The words come from the server as given.
export function verdictText(pending: PendingRow[] | undefined, address: string): string | undefined {
  const row = pending?.find((p) => p.address === address);
  if (!row || !row.verdict || row.verdict === CLAIMED) return undefined;
  return row.reason ? `${row.verdict}: ${row.reason}` : row.verdict;
}

export function VerdictLine({ text, pl }: { text: string; pl?: string }) {
  return (
    <Text data-verdict fontSize="13px" color="text.dim" overflowWrap="anywhere" pl={pl}>
      {text}
    </Text>
  );
}
