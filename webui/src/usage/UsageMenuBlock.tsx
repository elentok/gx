import { Menu } from "@chakra-ui/react";
import type { Connection } from "../connection";
import { UsageModeLimits, UsageModeSpending } from "../generated/server";
import { LimitsBlock } from "./LimitsBlock";
import { SpendBlock } from "./SpendBlock";
import { useUsageSummary, type FetchSummary } from "./useUsageSummary";

// Renders nothing (no separators either) until a fetch succeeds, by spec: no spinner here.
export function UsageMenuBlock({ conn, fetcher }: { conn: Connection; fetcher?: FetchSummary }) {
  const state = useUsageSummary(conn, fetcher);
  if (!state) return null;
  const { summary, now } = state;
  const block =
    summary.mode === UsageModeLimits ? (
      <LimitsBlock summary={summary} now={now} />
    ) : summary.mode === UsageModeSpending ? (
      <SpendBlock summary={summary} now={now} />
    ) : null;
  if (!block) return null;
  return (
    <>
      <Menu.Separator />
      {block}
      <Menu.Separator />
    </>
  );
}
