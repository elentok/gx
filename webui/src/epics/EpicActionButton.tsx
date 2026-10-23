import { Text } from "@chakra-ui/react";
import { useState } from "react";
import { Button } from "../ui/button/Button";
import type { PostEpicWrite } from "./queue";

interface Props {
  address: string;
  state: string;
  post: PostEpicWrite;
  label: string;
  pendingLabel: string;
  /** The inline text for a request that failed before the server answered; the error message is appended. */
  failure: string;
  onDone: () => void;
}

/** A POST button for an epic write: it sends the state the view shows, and a refusal shows inline. */
export function EpicActionButton({ address, state, post, label, pendingLabel, failure, onDone }: Props) {
  const [pending, setPending] = useState(false);
  const [refusal, setRefusal] = useState<string>();

  async function run() {
    setPending(true);
    setRefusal(undefined);
    try {
      const res = await post({ address, state });
      if (res.refused) setRefusal(res.message || res.reason);
      else onDone();
    } catch (err) {
      setRefusal(`${failure}: ${err instanceof Error ? err.message : String(err)}`);
    } finally {
      setPending(false);
    }
  }

  return (
    <>
      <Button tone="running" disabled={pending} onClick={run}>
        {pending ? pendingLabel : label}
      </Button>
      {refusal && (
        <Text as="span" role="alert" color="text.muted">
          {refusal}
        </Text>
      )}
    </>
  );
}
