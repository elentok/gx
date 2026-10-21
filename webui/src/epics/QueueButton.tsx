import { Text } from "@chakra-ui/react";
import { useState } from "react";
import { Button } from "../ui/button/Button";
import { postQueueAdd } from "./queue";

export function QueueButton({ address, state, onQueued }: { address: string; state: string; onQueued: () => void }) {
  const [pending, setPending] = useState(false);
  const [refusal, setRefusal] = useState<string>();

  async function queue() {
    setPending(true);
    setRefusal(undefined);
    try {
      const res = await postQueueAdd({ address, state });
      if (res.refused) setRefusal(res.message || res.reason);
      else onQueued();
    } catch {
      setRefusal("Could not queue the epic");
    } finally {
      setPending(false);
    }
  }

  return (
    <>
      <Button tone="running" disabled={pending} onClick={queue}>
        {pending ? "Queueing…" : "Queue"}
      </Button>
      {refusal && (
        <Text as="span" role="alert" color="text.muted">
          {refusal}
        </Text>
      )}
    </>
  );
}
