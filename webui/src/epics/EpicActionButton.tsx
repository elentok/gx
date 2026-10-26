import { Spinner } from "@chakra-ui/react";
import { Button } from "../ui/button/Button";
import type { Tone } from "../ui/tone";
import type { ToastSpec } from "../ui/toaster";
import { useWrite } from "../useWrite";
import type { EpicWriteResult, PostEpicWrite } from "./queue";

interface Props {
  address: string;
  state: string;
  post: PostEpicWrite;
  label: string;
  tone?: Tone;
  pendingLabel: string;
  /** A toast for a success the page does not show by itself. */
  success?: (res: EpicWriteResult) => ToastSpec | undefined;
  onDone: () => void;
}

/** A POST button for an epic write: it sends the state the view shows; the outcome shows in a toast. */
export function EpicActionButton({ address, state, post, label, tone = "running", pendingLabel, success, onDone }: Props) {
  const { run, pending } = useWrite();
  return (
    <Button tone={tone} disabled={pending} onClick={() => run(() => post({ address, state }), { onDone, success })}>
      {pending && <Spinner size="sm" />}
      {pending ? pendingLabel : label}
    </Button>
  );
}
