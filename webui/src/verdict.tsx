import { Box, Text } from "@chakra-ui/react";
import { postJson } from "./design/open";
import type { PendingRow, UsageOverrideRequest, UsageOverrideResult } from "./generated/server";
import { VerdictUsagePrefixClaude, VerdictUsagePrefixCodex } from "./generated/server";
import { Button } from "./ui/button/Button";
import { refusalText, useWrite } from "./useWrite";

// The one verdict that means the ticket is starting or running, so it shows nothing.
const CLAIMED = "claimed";

// "<verdict>: <reason>" for a queued, unclaimed ticket, else undefined. The words come from the server as given.
export function verdictText(pending: PendingRow[] | undefined, address: string): string | undefined {
  const row = pending?.find((p) => p.address === address);
  if (!row || !row.verdict || row.verdict === CLAIMED) return undefined;
  return row.reason ? `${row.verdict}: ${row.reason}` : row.verdict;
}

export type UsageHarness = "claude" | "codex";

export type OverrideUsage = (harness: UsageHarness) => Promise<UsageOverrideResult>;

const postUsageOverride: OverrideUsage = (harness) =>
  postJson<UsageOverrideResult>("/v1/usage/override")({ harness } satisfies UsageOverrideRequest);

// The harness whose usage gate holds the ticket, from the verdict's prefix; undefined for any other verdict.
function usageHarness(text: string): UsageHarness | undefined {
  if (text.startsWith(VerdictUsagePrefixClaude)) return "claude";
  if (text.startsWith(VerdictUsagePrefixCodex)) return "codex";
  return undefined;
}

// The queue-changed event the server publishes drops the verdict, so an accepted click needs no state here.
function UsageOverrideButton({ harness, override }: { harness: UsageHarness; override: OverrideUsage }) {
  const { run, pending, refusal } = useWrite({ toastRefusal: false });
  return (
    <>
      <Button emphasis="secondary" size="sm" disabled={pending} onClick={() => run(() => override(harness))}>
        Override
      </Button>
      {refusal && (
        <Text as="span" data-usage-refusal fontSize="13px" color="text.dim">
          {refusalText(refusal)}
        </Text>
      )}
    </>
  );
}

export function VerdictLine({
  text,
  pl,
  overrideUsage = postUsageOverride,
}: {
  text: string;
  pl?: string;
  overrideUsage?: OverrideUsage;
}) {
  const harness = usageHarness(text);
  return (
    <Box display="flex" flexWrap="wrap" alignItems="center" gap="2" pl={pl}>
      <Text data-verdict fontSize="13px" color="text.dim" overflowWrap="anywhere">
        {text}
      </Text>
      {harness && <UsageOverrideButton harness={harness} override={overrideUsage} />}
    </Box>
  );
}
