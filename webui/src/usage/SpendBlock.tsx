import { Box, Flex, Text } from "@chakra-ui/react";
import type { UsageSummary } from "../generated/server";
import { InfoTip } from "../ui/info-tip/InfoTip";
import { formatCost, sinceLabel } from "./format";

export const spendCaveat =
  "Estimated cost of Claude runs started by gx. Not the bill. Codex runs and Claude sessions outside gx are not counted.";

// The longer periods live in the info popover, so the menu shows one compact line.
function SpendRow({ name, amount, note }: { name: string; amount: number; note?: string }) {
  return (
    <Box role="group" aria-label={name}>
      <Flex justify="space-between" gap="3">
        <Text>{name}</Text>
        <Text style={{ fontVariantNumeric: "tabular-nums" }}>{formatCost(amount)}</Text>
      </Flex>
      {note && <Text color="text.dim">{note}</Text>}
    </Box>
  );
}

export function SpendBlock({ summary }: { summary: UsageSummary }) {
  const { today, since_sunday, since_sunday_short, since_first, since_first_short, first_day } = summary.spend;
  const note = (short?: boolean) => (short && first_day ? sinceLabel(first_day) : undefined);
  return (
    <Flex align="center" gap="1.5" px="3" py="1.5" fontSize="xs" color="text.muted">
      <Box role="group" aria-label="Today" style={{ fontVariantNumeric: "tabular-nums" }}>
        GX spend (est.) · Today {formatCost(today)}
      </Box>
      <InfoTip
        label="About gx spend"
        content={
          <>
            <Text>{spendCaveat}</Text>
            <SpendRow name="Since Sunday" amount={since_sunday} note={note(since_sunday_short)} />
            <SpendRow name="Since the 1st" amount={since_first} note={note(since_first_short)} />
          </>
        }
      />
    </Flex>
  );
}
