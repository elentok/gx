import { Box, Text } from "@chakra-ui/react";
import { Fragment } from "react";
import type { ModelLabel } from "../../modelName";

const NAMES: Record<string, string> = { claude: "Claude", codex: "Codex" };

/**
 * One dim line of `model · tier · effort`, for dense lists. The model reads as its `modelName`, which
 * already names the harness, so the harness mark shows only when there is no model label (a queue
 * note with a pinned harness).
 */
export function HarnessLabel({
  harness,
  tierLabel,
  model,
  effort,
  note,
}: {
  harness: string;
  tierLabel: string;
  model: ModelLabel;
  effort: string;
  /** Server text for a queue row (`pinned`, `follows default`). */
  note?: string;
}) {
  const showHarness = harness && !model;
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
