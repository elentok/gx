import { Menu } from "@chakra-ui/react";
import { useEffect } from "react";
import type { Connection } from "../connection";
import { UsageModeLimits, UsageModeSpending } from "../generated/server";
import { LimitsBlock } from "./LimitsBlock";
import { SpendBlock } from "./SpendBlock";
import { useUsageSummary, type FetchSummary } from "./useUsageSummary";

// Renders nothing (no separators either) until a fetch succeeds, by spec: no spinner here.
// onShownChange tells the menu whether a block rendered, so it only widens for one.
export function UsageMenuBlock({ conn, fetcher, onShownChange }: { conn: Connection; fetcher?: FetchSummary; onShownChange?: (shown: boolean) => void }) {
  const state = useUsageSummary(conn, fetcher);
  const summary = state?.summary;
  const block =
    summary?.mode === UsageModeLimits ? (
      <LimitsBlock summary={summary} now={state!.now} />
    ) : summary?.mode === UsageModeSpending ? (
      <SpendBlock summary={summary} />
    ) : null;
  const shown = block !== null;
  useEffect(() => {
    onShownChange?.(shown);
    return () => onShownChange?.(false);
  }, [shown, onShownChange]);
  if (!block) return null;
  return (
    <>
      <Menu.Separator />
      {block}
      <Menu.Separator />
    </>
  );
}
