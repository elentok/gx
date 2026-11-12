import { footerHover } from "../shell/FooterButton";
import { Box, chakra, Popover, Portal } from "@chakra-ui/react";
import { useState } from "react";
import type { Connection } from "../connection";
import { useCloseSignal, useOnCloseSignal } from "../ui/closeSignal";
import { UsageModeLimits, UsageModeSpending, type UsageLimits } from "../generated/server";
import { barTone, isPastReset, wholePercent } from "./format";
import { LimitsBlock } from "./LimitsBlock";
import { SpendBlock } from "./SpendBlock";
import { useUsageSummary, type FetchSummary } from "./useUsageSummary";

const MIN_FILL = 8;

// A provider with no 5h window, or one whose window already reset, shows an empty bar.
export function barFill(limits: UsageLimits | undefined, now: number) {
  const win = limits?.five_hour;
  if (!win || isPastReset(win.resets_at, now)) return { percent: MIN_FILL, tone: barTone(0), whole: 0 };
  const whole = wholePercent(win.used_percentage);
  return { percent: Math.min(100, Math.max(MIN_FILL, whole)), tone: barTone(whole), whole };
}

function Bar({ letter, name, limits, now }: { letter: string; name: string; limits: UsageLimits | undefined; now: number }) {
  const { percent, tone, whole } = barFill(limits, now);
  return (
    <Box aria-hidden display="flex" flexDirection="column" alignItems="center" gap="1px" data-meter={letter} data-tone={tone} data-percent={whole} title={`${name} 5h limit`}>
      <Box w="5px" h="15px" borderRadius="2px" overflow="hidden" position="relative" bg="hairline">
        <Box data-part="fill" position="absolute" left="0" right="0" bottom="0" h={`${percent}%`} bg={`tone.${tone}.fg`} />
      </Box>
      <Box as="span" fontFamily="mono" fontWeight="600" fontSize="8px" lineHeight="1" color={`tone.${tone}.fg`}>
        {letter}
      </Box>
    </Box>
  );
}

// Renders nothing until a fetch succeeds, as the gear menu's block did: no spinner.
export function LimitsTray({ conn, fetcher }: { conn: Connection; fetcher?: FetchSummary }) {
  const state = useUsageSummary(conn, fetcher);
  const [open, setOpen] = useState(false);
  useOnCloseSignal(useCloseSignal(), () => setOpen(false));
  const summary = state?.summary;
  if (!state || !summary) return null;
  const body =
    summary.mode === UsageModeLimits ? (
      <LimitsBlock summary={summary} now={state.now} />
    ) : summary.mode === UsageModeSpending ? (
      <>
        <SpendBlock summary={summary} />
        <Box role="separator" h="1px" bg="border" my="1" />
        <LimitsBlock summary={summary} now={state.now} />
      </>
    ) : null;
  if (!body) return null;
  const label = "Usage limits";
  return (
    <Popover.Root open={open} onOpenChange={(e) => setOpen(e.open)} positioning={{ placement: "top-end" }} lazyMount unmountOnExit>
      <Popover.Trigger asChild>
        <chakra.button
          type="button"
          aria-label={label}
          title={label}
          h="30px"
          display="flex"
          alignItems="flex-start"
          gap="3px"
          pt="2px"
          px="6px"
          borderWidth="1px"
          borderColor="transparent"
          borderRadius="md"
          cursor="pointer"
          {...footerHover}
        >
          <Bar letter="C" name="Claude" limits={summary.limits} now={state.now} />
          <Bar letter="X" name="Codex" limits={summary.codex} now={state.now} />
        </chakra.button>
      </Popover.Trigger>
      <Portal>
        <Popover.Positioner>
          <Popover.Content role="dialog" aria-label={label} bg="surface.raised" borderWidth="1px" borderColor="border" borderRadius="8px" boxShadow="lg" color="text" w="280px" maxW="calc(100vw - 2rem)" py="1">
            {body}
          </Popover.Content>
        </Popover.Positioner>
      </Portal>
    </Popover.Root>
  );
}
