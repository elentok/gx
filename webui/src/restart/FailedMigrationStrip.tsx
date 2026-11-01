import { Box, Spinner, Text } from "@chakra-ui/react";
import { useState } from "react";
import { useConnectionContext, useConnectionState } from "../connection";
import { useDesignWrite } from "../design/SessionControls";
import { FixAgentDialog } from "../epic/FixAgentDialog";
import type { FixAgent } from "../epic/fixAgent";
import type { MigrationFailure } from "../generated/server";
import { Button } from "../ui/button/Button";
import { postRetryMigration, type RetryMigration } from "./api";

// The server's own prompt adds the harness preamble; this is what the person can copy.
const promptFor = (f: MigrationFailure) =>
  `The migration of project ${f.project} failed with: ${f.error}\nAfter fixing it, run \`${f.retry_command}\` to retry.`;

function FailureRow({
  failure,
  fixAgent,
  retry,
}: {
  failure: MigrationFailure;
  fixAgent?: FixAgent;
  retry: RetryMigration;
}) {
  const [fixing, setFixing] = useState(false);
  const { run, pending } = useDesignWrite();
  return (
    <Box role="status" display="flex" alignItems="center" justifyContent="space-between" gap="2" bg="surface.raised" borderWidth="1px" borderColor="border" borderRadius="md" px="3" py="2" mb="3">
      <Box minW="0">
        <Text fontWeight="600">{`${failure.project}: Migration blocked`}</Text>
        <Text overflowWrap="anywhere">{failure.error}</Text>
        <Box as="code" fontFamily="mono" fontSize="sm">
          {failure.retry_command}
        </Box>
      </Box>
      <Box display="flex" gap="2">
        <Button size="sm" disabled={pending} onClick={() => run(() => retry({ project: failure.project }))}>
          {pending && <Spinner size="sm" />}
          Retry migration
        </Button>
        <Button tone="running" size="sm" onClick={() => setFixing(true)}>
          Fix agent
        </Button>
      </Box>
      {fixing && (
        <FixAgentDialog
          prompt={promptFor(failure)}
          request={{ project: failure.project }}
          fixAgent={fixAgent}
          onClose={() => setFixing(false)}
        />
      )}
    </Box>
  );
}

/** One row per saved failed migration in the live snapshot; a row goes when the record clears. */
export function FailedMigrationStrip({
  fixAgent,
  retry = postRetryMigration,
}: {
  fixAgent?: FixAgent;
  retry?: RetryMigration;
}) {
  const { snapshot } = useConnectionState(useConnectionContext());
  return (
    <>
      {(snapshot?.migration_failures ?? []).map((f) => (
        <FailureRow key={f.project} failure={f} fixAgent={fixAgent} retry={retry} />
      ))}
    </>
  );
}
