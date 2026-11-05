import { Box, Text } from "@chakra-ui/react";
import { Fragment, type ReactNode } from "react";
import type { ModelLabel } from "../../modelName";

const NAMES: Record<string, string> = { claude: "Claude", codex: "Codex" };

function Pill({ children, title }: { children: ReactNode; title?: string }) {
  return (
    <Box
      as="span"
      title={title}
      display="inline-flex"
      alignItems="center"
      gap="5px"
      whiteSpace="nowrap"
      px="2"
      borderRadius="full"
      borderWidth="1px"
      borderColor="hairline"
      bg="surface.raised"
      color="text.dim"
      fontFamily="mono"
      fontSize="12px"
    >
      {children}
    </Box>
  );
}

/**
 * `model · tier · effort`. The model reads as its `modelName`, which already names the harness, so
 * the harness mark shows only when there is no model label (a queue note with a pinned harness).
 */
export function HarnessLabel({
  harness,
  tierLabel,
  model,
  effort,
  switched,
  note,
  plain,
}: {
  harness: string;
  tierLabel: string;
  model: ModelLabel;
  effort: string;
  switched: boolean;
  /** Server text for a queue row, as one more dim chip (`pinned`, `follows default`). */
  note?: string;
  /** One dim line of text joined by `·` instead of pills, for dense lists. */
  plain?: boolean;
}) {
  const tierTitle = tierLabel ? `${tierLabel} tier` : undefined;
  const showHarness = harness && !model;
  const modelText = [model, effort].filter(Boolean).join(" · ");
  if (plain) {
    const parts = [model, tierLabel, effort, note ?? ""].filter(Boolean);
    // One inline flow: it wraps between words (a real space), never inside the harness or a word.
    const words = parts.flatMap((part) => part.split(" · "));
    return (
      <Text as="span" fontFamily="mono" fontSize="12px" color="text.dim">
        {showHarness && (
          <Box as="span" display="inline-flex" alignItems="center" gap="5px" whiteSpace="nowrap" mr="2">
            <Box as="i" w="7px" h="7px" borderRadius="full" bg={`harness.${harness}`} />
            {NAMES[harness] ?? harness}
          </Box>
        )}
        {words.map((word, i, rest) => (
          <Fragment key={i}>
            {i > 0 && " "}
            <Box as="span" whiteSpace="nowrap">
              {word}
              {i < rest.length - 1 ? " ·" : ""}
            </Box>
          </Fragment>
        ))}
      </Text>
    );
  }
  return (
    <Box as="span" display="inline-flex" flexWrap="wrap" alignItems="center" gap="2">
      {showHarness && (
        <Pill>
          <Box as="i" w="7px" h="7px" borderRadius="full" bg={`harness.${harness}`} />
          {NAMES[harness] ?? harness}
        </Pill>
      )}
      {model && <Pill title={tierTitle}>{modelText}</Pill>}
      {tierLabel && <Pill title={tierTitle}>{tierLabel}</Pill>}
      {note && <Pill>{note}</Pill>}
      {switched && (
        <Text as="span" fontSize="sm" color="text.dim">
          switched by hand
        </Text>
      )}
    </Box>
  );
}
