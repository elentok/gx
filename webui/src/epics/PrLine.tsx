import { Box, Link, Text } from "@chakra-ui/react";
import { CardActionOpenPR, CardActionOpenTicket, EpicListFixingCI, type EpicListRow } from "../generated/server";
import { epicHref, ticketHref } from "../hrefs";
import type { Tone } from "../ui/tone";

function checksTone(checks: string): Tone | undefined {
  if (checks.includes("failed")) return "paused";
  if (checks.includes("running")) return "waiting";
  if (checks.includes("passed")) return "done";
  return undefined;
}

function Chip({ color, children, ...rest }: { color?: string; children: string; "data-testid"?: string }) {
  return (
    <Box
      as="span"
      fontFamily="mono"
      fontSize="12px"
      borderWidth="1px"
      borderColor="hairline"
      bg="surface"
      borderRadius="full"
      px="10px"
      py="1px"
      color={color ?? "text.dim"}
      whiteSpace="nowrap"
      {...rest}
    >
      {children}
    </Box>
  );
}

/** The PR line of a card that has a pull request: number chip, check summary and one link. */
export function PrLine({ row }: { row: EpicListRow }) {
  const pr = row.pr;
  if (!pr) return null;
  const tone = pr.checks ? checksTone(pr.checks) : undefined;
  // While a fix runs the link opens its ticket; the row does not name the ticket, so an Open ticket action decides.
  const fix = row.state === EpicListFixingCI;
  const fixTicket = row.actions?.find((a) => a.kind === CardActionOpenTicket)?.target;
  const github = row.actions?.find((a) => a.kind === CardActionOpenPR)?.link ?? pr.url;
  const linkProps = { fontFamily: "mono", fontSize: "13px", color: "tone.running.fg", textDecoration: "none" } as const;
  return (
    <Box data-testid="pr-line" display="flex" flexWrap="wrap" alignItems="center" gap="2" fontSize="14px">
      <Chip color="kind.design.fg" data-testid="pr-number">
        {`#${pr.number}`}
      </Chip>
      {pr.checks && (
        <Chip color={tone && `tone.${tone}.fg`} data-testid="pr-checks">
          {pr.checks}
        </Chip>
      )}
      {fix ? (
        <Link {...linkProps} data-testid="pr-link" href={fixTicket ? ticketHref(fixTicket) : epicHref(row.addresses?.[0] ?? row.address)}>
          {fixTicket ? `Fix ticket ${fixTicket.split("/").pop()}` : "Fix ticket"}
        </Link>
      ) : (
        github && (
          <Link {...linkProps} data-testid="pr-link" href={github} target="_blank" rel="noreferrer">
            Open on GitHub
          </Link>
        )
      )}
      {pr.unreachable && (
        <Text as="span" data-testid="pr-unreachable" color="tone.waiting.fg" fontSize="13px">
          GitHub unreachable
        </Text>
      )}
    </Box>
  );
}
