import { Box, Spinner, Text } from "@chakra-ui/react";
import { Button } from "../button/Button";
import { Strip } from "../strip/Strip";

// One low disk: its name, free space and a "Clean now" button. A held disk says new tickets wait.
export function DiskStrip({
  name,
  free,
  held,
  cleaning,
  pending,
  problem,
  onClean,
}: {
  name: string;
  /** Free space, already formatted. */
  free: string;
  held: boolean;
  /** A clean runs on this disk, started here or elsewhere. */
  cleaning: boolean;
  /** The clean call from this page is in flight. */
  pending: boolean;
  /** Why the last clean refused or failed. */
  problem?: string;
  onClean: () => void;
}) {
  const busy = cleaning || pending;
  return (
    <Strip tone={held ? "paused" : "waiting"}>
      <Box display="flex" alignItems="center" justifyContent="center" gap="3" flexWrap="wrap">
        <Text as="span">
          {held ? `${name} is low: ${free} free, new tickets held` : `${name} is low: ${free} free`}
        </Text>
        <Button size="sm" emphasis="secondary" disabled={busy} onClick={onClean}>
          {busy && <Spinner size="sm" />}
          Clean now
        </Button>
      </Box>
      {problem && <Text role="alert">{problem}</Text>}
    </Strip>
  );
}
