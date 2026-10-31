import { Box, Flex, Text } from "@chakra-ui/react";
import type { UsageSummary } from "../generated/server";
import { InfoTip } from "../ui/info-tip/InfoTip";
import { formatCost, sinceLabel } from "./format";

export const spendCaveat =
  "Estimated cost of Claude runs started by gx. Not the bill. Codex runs and Claude sessions outside gx are not counted.";

function SpendRow({ name, amount, note }: { name: string; amount: number; note?: string }) {
  return (
    <Box role="group" aria-label={name} px="3" py="1">
      <Flex justify="space-between" gap="2">
        <Text color="text.muted">{name}</Text>
        <Text fontSize="14px" fontWeight="semibold" color="text" style={{ fontVariantNumeric: "tabular-nums" }}>
          {formatCost(amount)}
        </Text>
      </Flex>
      {note && (
        <Text fontSize="xs" color="text.dim">
          {note}
        </Text>
      )}
    </Box>
  );
}

export function SpendBlock({ summary }: { summary: UsageSummary; now: number }) {
  const { today, since_sunday, since_sunday_short, since_first, since_first_short, first_day } = summary.spend;
  const note = (short?: boolean) => (short && first_day ? sinceLabel(first_day) : undefined);
  return (
    <>
      <Flex align="center" gap="1.5" px="3" pt="1.5" pb="0.5" fontSize="xs" textTransform="uppercase" letterSpacing="0.06em" color="text.faint">
        <span>gx spend (est.)</span>
        <InfoTip label="About gx spend" content={spendCaveat} />
      </Flex>
      <SpendRow name="Today" amount={today} />
      <SpendRow name="Since Sunday" amount={since_sunday} note={note(since_sunday_short)} />
      <SpendRow name="Since the 1st" amount={since_first} note={note(since_first_short)} />
    </>
  );
}
