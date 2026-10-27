import { Box, Text } from "@chakra-ui/react";
import { Button } from "../ui/button/Button";
import { suggestionCount, type ProjectChecks } from "./status";

const plural = (n: number, word: string) => `${n} ${word}${n === 1 ? "" : "s"}`;

/** Projects without check config. One project gets its own sentence; several get a list with a button each. */
export function MissingBanner({
  missing,
  onSetUp,
  onDismiss,
}: {
  missing: ProjectChecks[];
  onSetUp: (project: string) => void;
  onDismiss: () => void;
}) {
  if (missing.length === 0) return null;
  const one = missing.length === 1 ? missing[0] : undefined;
  const found = one ? suggestionCount(one) : 0;
  return (
    <Box
      role="region"
      aria-label="Missing check config"
      mt="4"
      display="flex"
      flexWrap="wrap"
      gap="3"
      justifyContent="space-between"
      alignItems={one ? "center" : "flex-start"}
      borderWidth="1px"
      borderColor="tone.waiting.fg"
      bg="tone.waiting.bg"
      borderRadius="md"
      px="14px"
      py="10px"
    >
      <Box>
        <Text>
          <Text as="b" color="tone.waiting.fg" fontWeight="600">
            {one ? `${one.project} has no check config.` : `${missing.length} projects have no check config.`}
          </Text>{" "}
          {`Agents run gx's built-in checks until you set ${one ? "it" : "them"} up.`}
          {one && found > 0 && ` gx found ${plural(found, "command")} to suggest.`}
        </Text>
        {!one && (
          <Box as="ul" m="0" mt="1.5" pl="18px">
            {missing.map((p) => (
              <Box as="li" key={p.project} display="flex" justifyContent="space-between" alignItems="center" mt="1.5">
                {p.project}
                <Button emphasis="main" tone="running" aria-label={`Set up checks for ${p.project}`} onClick={() => onSetUp(p.project)}>
                  Set up checks
                </Button>
              </Box>
            ))}
          </Box>
        )}
      </Box>
      <Box display="flex" gap="2">
        <Button emphasis="ghost" tone="muted" onClick={onDismiss}>
          Not now
        </Button>
        {one && (
          <Button emphasis="main" tone="running" onClick={() => onSetUp(one.project)}>
            Set up checks
          </Button>
        )}
      </Box>
    </Box>
  );
}
