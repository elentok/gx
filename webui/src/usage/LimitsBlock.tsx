import { Box, Flex, Text } from "@chakra-ui/react";
import type { LimitWindow, UsageSummary } from "../generated/server";
import { InfoTip } from "../ui/info-tip/InfoTip";
import { ProgressBar } from "../ui/progress-bar/ProgressBar";
import { exactTime, isPastReset, isStale, limitTone, resetsIn, seenAgo, wholePercent } from "./format";

function LimitRow({ name, win, now }: { name: string; win: LimitWindow; now: number }) {
  const pastReset = isPastReset(win.resets_at, now);
  const whole = wholePercent(win.used_percentage);
  const tone = limitTone(whole);
  const stale = !pastReset && isStale(win.received_at, now);
  return (
    <Box role="group" aria-label={name} px="3" py="1.5">
      <Text color="text.muted">{name}</Text>
      {pastReset ? (
        <Text fontSize="22px" lineHeight="1.3" fontStyle="italic" color="text.dim">
          waiting
        </Text>
      ) : (
        <>
          <Text
            data-tone={tone}
            fontSize="22px"
            lineHeight="1.3"
            fontWeight="semibold"
            letterSpacing="0.02em"
            color={tone === "running" ? "text" : `tone.${tone}.fg`}
            style={{ fontVariantNumeric: "tabular-nums" }}
          >
            {whole}%
          </Text>
          <Box mt="1.5">
            <ProgressBar thin value={whole} max={100} tone={tone} />
          </Box>
        </>
      )}
      <Flex justify="space-between" gap="2" mt="0.5" fontSize="xs" color="text.dim">
        {pastReset ? (
          <span>reset passed · waiting for a Claude session</span>
        ) : (
          <Flex align="center" gap="1.5">
            <span>{resetsIn(win.resets_at, now)}</span>
            <InfoTip label={`${name} reset time`} content={exactTime(win.resets_at)} />
          </Flex>
        )}
        {stale && (
          <Text as="span" color="text.faint">
            {seenAgo(win.received_at, now)}
          </Text>
        )}
      </Flex>
    </Box>
  );
}

export function LimitsBlock({ summary, now }: { summary: UsageSummary; now: number }) {
  const { five_hour, seven_day } = summary.limits;
  return (
    <>
      <Text px="3" pt="1.5" pb="0.5" fontSize="xs" textTransform="uppercase" letterSpacing="0.06em" color="text.faint">
        Claude limits
      </Text>
      {five_hour && <LimitRow name="5h limit" win={five_hour} now={now} />}
      {seven_day && <LimitRow name="Weekly limit" win={seven_day} now={now} />}
    </>
  );
}
