import { Box, Text } from "@chakra-ui/react";
import type { ReactNode } from "react";

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
 * `harness · tier · model effort`. Props are plain strings so any row can use it.
 * A tier with no model yet reads `<Tier> · CLI default`; nothing is shown for the tier and model
 * when all three are empty.
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
  model: string;
  effort: string;
  switched: boolean;
  /** Server text for a queue row, as one more dim chip (`pinned`, `follows default`). */
  note?: string;
  /** One dim line of text joined by `·` instead of pills, for dense lists. */
  plain?: boolean;
}) {
  const tierTitle = tierLabel ? `${tierLabel} tier` : undefined;
  const modelText = [model, effort].filter(Boolean).join(" · ");
  const known = tierLabel || model || effort;
  if (plain) {
    const parts = [
      known && model && tierLabel ? tierLabel : "",
      known && model ? modelText : "",
      known && !model ? [tierLabel, "CLI default", effort].filter(Boolean).join(" · ") : "",
      note ?? "",
    ].filter(Boolean);
    return (
      <Text as="span" display="inline-flex" alignItems="center" gap="2" fontFamily="mono" fontSize="12px" color="text.dim">
        {harness && (
          <Box as="span" display="inline-flex" alignItems="center" gap="5px">
            <Box as="i" w="7px" h="7px" borderRadius="full" bg={`harness.${harness}`} />
            {NAMES[harness] ?? harness}
          </Box>
        )}
        {parts.length > 0 && <span>{parts.join(" · ")}</span>}
      </Text>
    );
  }
  return (
    <Box as="span" display="inline-flex" flexWrap="wrap" alignItems="center" gap="2">
      {harness && (
        <Pill>
          <Box as="i" w="7px" h="7px" borderRadius="full" bg={`harness.${harness}`} />
          {NAMES[harness] ?? harness}
        </Pill>
      )}
      {known && model && tierLabel && <Pill title={tierTitle}>{tierLabel}</Pill>}
      {known && model && <Pill title={tierTitle}>{modelText}</Pill>}
      {known && !model && <Pill title={tierTitle}>{[tierLabel, "CLI default", effort].filter(Boolean).join(" · ")}</Pill>}
      {note && <Pill>{note}</Pill>}
      {switched && (
        <Text as="span" fontSize="sm" color="text.dim">
          switched by hand
        </Text>
      )}
    </Box>
  );
}
