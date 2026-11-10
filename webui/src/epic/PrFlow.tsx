import { Box, chakra, Text, type BoxProps } from "@chakra-ui/react";
import type { ReactNode } from "react";
import {
  CardActionCreatePR,
  CardActionOpenPR,
  CardActionTryAgain,
  PRActivityBad,
  PRActivityGood,
  PRActivityWarning,
  type CardAction,
  type EpicDetail,
  type PRActivityEntry,
} from "../generated/server";
import { EpicActionButton } from "../epics/EpicActionButton";
import { postEpicCreatePR, postEpicTryAgain } from "../epics/queue";
import { ticketHref } from "../hrefs";
import { Button } from "../ui/button/Button";

// A mono pill, as the mock draws the PR chip and the entry count.
function Chip({ children, ...rest }: { children: ReactNode } & Omit<BoxProps, "children">) {
  return (
    <Box as="span" fontFamily="mono" fontSize="12px" fontWeight="400" color="text.muted" borderWidth="1px" borderColor="border" bg="surface" borderRadius="full" px="10px" whiteSpace="nowrap" {...rest}>
      {children}
    </Box>
  );
}

const action = (detail: EpicDetail, kind: string): CardAction | undefined => detail.pr_actions?.find((a) => a.kind === kind);

function TryAgainButton({ detail, label, onChanged }: { detail: EpicDetail; label: string; onChanged: () => void }) {
  return (
    <EpicActionButton
      address={detail.address}
      state={detail.state ?? ""}
      post={postEpicTryAgain}
      label={label}
      tone="muted"
      pendingLabel="Trying again…"
      onDone={onChanged}
    />
  );
}

/** The PR chip, Open on GitHub, Try again and Create PR, in the order the server's pr_actions give them. */
export function PrActions({ detail, onChanged }: { detail: EpicDetail; onChanged: () => void }) {
  const tryAgain = action(detail, CardActionTryAgain);
  const create = action(detail, CardActionCreatePR);
  const open = action(detail, CardActionOpenPR);
  const link = open?.link ?? detail.pr?.url;
  return (
    <>
      {detail.pr && (
        <Chip data-testid="pr-chip" color={detail.pr.state === "merged" ? "tone.done.fg" : "tone.running.fg"}>
          {`PR ${detail.pr.chip}`}
        </Chip>
      )}
      {open && link && (
        <Button tone="muted" emphasis="secondary" onClick={() => window.open(link, "_blank", "noopener,noreferrer")}>
          {open.label}
        </Button>
      )}
      {tryAgain && <TryAgainButton detail={detail} label={tryAgain.label} onChanged={onChanged} />}
      {create && (
        <EpicActionButton
          address={detail.address}
          state={detail.state ?? ""}
          post={postEpicCreatePR}
          label={create.label}
          tone={detail.actions?.includes("merge") ? "muted" : "done"}
          pendingLabel="Creating PR…"
          onDone={onChanged}
        />
      )}
    </>
  );
}

/** The paused epic's notice text with its Try again; the same shape for "CI failing" and "PR text failed". */
export function PrNotice({ detail, wide, onChanged }: { detail: EpicDetail; wide: boolean; onChanged: () => void }) {
  if (!detail.pr_notice) return null;
  const tryAgain = action(detail, CardActionTryAgain);
  return (
    <Box
      role="region"
      aria-label="Paused"
      data-testid="pr-notice"
      mt="14px"
      bg="rgba(255,112,102,0.10)"
      borderRadius="10px"
      px="14px"
      py="10px"
      fontSize="14px"
      lineHeight="1.5"
      color="tone.paused.text"
    >
      <Text>{detail.pr_notice}</Text>
      {tryAgain && (
        <Box mt="2" css={wide ? undefined : { "& button": { height: "44px" } }}>
          <TryAgainButton detail={detail} label={tryAgain.label} onChanged={onChanged} />
        </Box>
      )}
    </Box>
  );
}

const DOT: Record<string, string> = {
  [PRActivityGood]: "tone.done.fg",
  [PRActivityWarning]: "tone.waiting.fg",
  [PRActivityBad]: "tone.paused.fg",
};

const clock = (iso: string) => {
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? "" : d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", hour12: false });
};

/** The PR activity panel, newest first as the server sends it. No entries, no panel. */
export function PrActivity({ entries }: { entries?: PRActivityEntry[] }) {
  if (!entries || entries.length === 0) return null;
  return (
    <Box
      as="section"
      aria-label="PR activity"
      data-testid="pr-activity"
      mt="16px"
      borderWidth="1px"
      borderColor="border"
      bg="surface"
      borderRadius="12px"
      px="16px"
      py="14px"
    >
      <Box as="h3" display="flex" alignItems="center" gap="10px" fontSize="17px" fontWeight="600" mb="2">
        PR activity <Chip>{entries.length === 1 ? "1 entry" : `${entries.length} entries`}</Chip>
      </Box>
      <Box as="ul" listStyleType="none" m="0" p="0" display="flex" flexDirection="column">
        {entries.map((e, i) => (
          <Box
            as="li"
            key={i}
            display="grid"
            gridTemplateColumns="64px 14px 1fr"
            gap="10px"
            py="6px"
            fontSize="14px"
            alignItems="start"
          >
            <chakra.time dateTime={e.time} fontFamily="mono" fontSize="12px" color="text.faint" pt="2px">
              {clock(e.time)}
            </chakra.time>
            <Box data-testid="pr-dot" data-kind={e.kind} w="10px" h="10px" borderRadius="full" mt="5px" bg={DOT[e.kind] ?? "text.muted"} />
            <span>
              {e.text}
              {e.ticket && (
                <>
                  {" · "}
                  <chakra.a href={ticketHref(e.ticket)} color="tone.running.fg" fontFamily="mono" fontSize="13px">
                    {`ticket ${e.ticket.split("/").pop()}`}
                  </chakra.a>
                </>
              )}
              {e.detail && (
                <Text as="span" display="block" color="text.dim" fontSize="13px">
                  {e.detail}
                </Text>
              )}
            </span>
          </Box>
        ))}
      </Box>
    </Box>
  );
}
