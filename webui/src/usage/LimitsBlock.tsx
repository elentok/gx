import { Box, Flex, Text } from "@chakra-ui/react";
import { LimitChipDown, LimitChipUp, type UsageLimitWindow, type UsageLimits, type UsageSummary } from "../generated/server";
import { InfoTip } from "../ui/info-tip/InfoTip";
import { ProgressBar } from "../ui/progress-bar/ProgressBar";
import {
  chipText,
  chipTooltip,
  exactTime,
  forecastLine,
  isPastReset,
  isStale,
  limitTone,
  resetsIn,
  seenAgo,
  wholePercent,
} from "./format";

function LimitRow({ name, label, win, now }: { name: string; label: string; win: UsageLimitWindow; now: number }) {
  const pastReset = isPastReset(win.resets_at, now);
  const whole = wholePercent(win.used_percentage);
  const tone = limitTone(whole);
  const stale = !pastReset && isStale(win.received_at, now);
  const live = !pastReset && whole < 100;
  const chip = live ? win.chip : undefined;
  const forecast = live ? forecastLine(win.forecast) : undefined;
  const todaySoFar = live && (chip !== undefined || forecast !== undefined);
  const chipColor = chip?.direction === LimitChipUp ? "tone.waiting.fg" : chip?.direction === LimitChipDown ? "tone.done.fg" : "text.muted";
  return (
    <Box role="group" aria-label={label} px="3" py="1.5">
      <Flex align="center" gap="2" wrap="wrap">
        <Text color="text.muted">{name}</Text>
        {chip && <InfoTip variant="chip" chipColor={chipColor} label={`${label} burn`} content={chipTooltip(chip)}>{chipText(chip)}</InfoTip>}
      </Flex>
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
            <InfoTip label={`${label} reset time`} content={exactTime(win.resets_at)} />
          </Flex>
        )}
        {(stale || todaySoFar) && (
          <Text as="span" color="text.faint">
            {[todaySoFar && "today so far", stale && seenAgo(win.received_at, now)].filter(Boolean).join(" · ")}
          </Text>
        )}
      </Flex>
      {forecast && (
        <Text mt="1" fontSize="xs" color={forecast.warn ? "tone.waiting.fg" : "text.dim"}>
          {forecast.text}
        </Text>
      )}
    </Box>
  );
}

function Provider({ title, prefix, limits, now }: { title: string; prefix: string; limits: UsageLimits; now: number }) {
  const { five_hour, seven_day } = limits;
  return (
    <>
      <Text px="3" pt="1.5" pb="0.5" fontSize="xs" textTransform="uppercase" letterSpacing="0.06em" color="text.faint">
        {title}
      </Text>
      {five_hour && <LimitRow name="5h limit" label={`${prefix}5h limit`} win={five_hour} now={now} />}
      {seven_day && <LimitRow name="Weekly limit" label={`${prefix}Weekly limit`} win={seven_day} now={now} />}
    </>
  );
}

export function LimitsBlock({ summary, now }: { summary: UsageSummary; now: number }) {
  return (
    <>
      <Provider title="Claude limits" prefix="" limits={summary.limits} now={now} />
      {summary.codex && (
        <>
          <Box role="separator" h="1px" bg="border" my="1" />
          <Provider title="Codex limits" prefix="Codex " limits={summary.codex} now={now} />
        </>
      )}
    </>
  );
}
