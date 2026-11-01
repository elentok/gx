import { Box, Spinner, Text } from "@chakra-ui/react";
import { useState } from "react";
import {
  ActionChooseBase,
  ActionResume,
  ActionResumeOnWorker,
  ActionRetry,
  ActionStartOver,
  type TicketDetail,
} from "../generated/server";
import { Button } from "../ui/button/Button";
import { ExpandableTextField } from "../ui/expandable-text/ExpandableTextField";
import { Modal } from "../ui/modal/Modal";
import { useWrite, type Refusable } from "../useWrite";
import { ReasonStale } from "./answer";
import { isRecoverAction, postRecover, ReasonAgentWorking, type PostRecover, type RecoverBody } from "./recover";

const SUCCESS_TOASTS: Record<string, string> = { [ActionRetry]: "Retry started", [ActionResume]: "Resumed", [ActionResumeOnWorker]: "Resumed" };

// The web verb resume-on-worker is a resume whose next launch uses the worker tier.
const WORKER_TIER = "worker";

// A resume of a zero-commit pause with a live agent prompts it and leaves the pause in place.
const PROMPTED_TOAST = "Sent to the running agent. The ticket stays paused until it finishes.";

// The buttons for the recovery verbs the detail lists. The server decides which show and what they say;
// this holds no verb logic beyond where each one posts.
export function RecoveryActions({
  detail,
  post = postRecover,
  onDone,
  onStale,
}: {
  detail: TicketDetail;
  post?: PostRecover;
  /** Called after the server accepted a write. */
  onDone?: () => void;
  /** The pause changed under the person: refetch so the current one shows. */
  onStale?: () => void;
}) {
  const bases = detail.bases ?? [];
  const [base, setBase] = useState("");
  const [confirming, setConfirming] = useState(false);
  const { run, pending, refusal } = useWrite();
  const [seenState, setSeenState] = useState(detail.state);

  // A new pause clears the pick: it may list different blockers.
  if (seenState !== detail.state) {
    setSeenState(detail.state);
    setBase("");
  }

  const actions = detail.actions.filter(isRecoverAction);
  if (actions.length === 0) return null;

  const send = (action: string, extra: Partial<RecoverBody> = {}) =>
    run(
      async () => {
        const tier = action === ActionResumeOnWorker ? { tier: WORKER_TIER } : {};
        const res = await post(action, { address: detail.address, state: detail.state ?? "", ...tier, ...extra });
        if (res.refused && res.reason === ReasonStale) onStale?.();
        return res;
      },
      {
        success: (res) => {
          if ((res as Refusable & { data?: { status?: string } }).data?.status === "prompted") return { type: "success", title: PROMPTED_TOAST };
          return SUCCESS_TOASTS[action] ? { type: "success", title: SUCCESS_TOASTS[action]! } : undefined;
        },
        onDone: () => {
          setConfirming(false);
          onDone?.();
        },
      },
    );

  const ready = (action: string) => action !== ActionChooseBase || bases.length === 0 || base !== "";
  const stopAndRetry = refusal?.reason === ReasonAgentWorking;

  return (
    <Box
      position="sticky"
      bottom="0"
      bg="surface.raised"
      py="3"
      display="flex"
      flexDirection="column"
      gap="2"
      aria-label="Recovery actions"
      role="form"
    >
      {detail.recommendation && <Text whiteSpace="pre-wrap">{detail.recommendation.diagnosis}</Text>}
      {detail.model_ran && detail.model_expected && (
        <Text>
          Ran on {detail.model_ran}, expected {detail.model_expected}.
        </Text>
      )}
      {bases.length > 0 && actions.includes(ActionChooseBase) && (
        <>
          <Text>Choose the base branch:</Text>
          {bases.map((b) => (
            <Button
              key={b}
              emphasis={base === b ? "main" : "secondary"}
              aria-pressed={base === b}
              disabled={pending}
              onClick={() => setBase(b)}
              width="full"
              justifyContent="flex-start"
            >
              {b}
            </Button>
          ))}
        </>
      )}
      {stopAndRetry && (
        <Box display="flex" flexDirection="column" gap="2">
          <Text role="alert">{refusal?.message}</Text>
          <Button emphasis="secondary" disabled={pending} onClick={() => send(ActionRetry, { stop: true })}>
            Stop agent and retry
          </Button>
        </Box>
      )}
      <Box display="flex" gap="2" justifyContent="flex-end" flexWrap="wrap">
        {actions.map((action, i) => {
          const info = detail.action_info?.[action];
          return (
            <Button
              key={action}
              title={info?.tooltip}
              emphasis={i === 0 ? "main" : "secondary"}
              disabled={!ready(action) || pending}
              onClick={() => (action === ActionStartOver ? setConfirming(true) : send(action, base ? { base } : {}))}
            >
              {pending && <Spinner size="sm" />}
              {info?.label ?? action}
              {action === detail.recommendation?.verb && (
                <Text as="span" fontSize="xs" fontWeight="600" textTransform="uppercase">
                  Recommended
                </Text>
              )}
            </Button>
          );
        })}
      </Box>
      {confirming && (
        <StartOverDialog
          label={detail.action_info?.[ActionStartOver]?.label ?? "Start over"}
          tooltip={detail.action_info?.[ActionStartOver]?.tooltip}
          pending={pending}
          onConfirm={(reason) => send(ActionStartOver, { reason })}
          onClose={() => setConfirming(false)}
        />
      )}
    </Box>
  );
}

// Start over throws the work away, so it asks first. It never sends branch deletion: the server keeps a backup.
function StartOverDialog({
  label,
  tooltip,
  pending,
  onConfirm,
  onClose,
}: {
  label: string;
  tooltip?: string;
  pending: boolean;
  onConfirm: (reason: string) => void;
  onClose: () => void;
}) {
  const [reason, setReason] = useState("");
  return (
    <Modal label={label} onClose={onClose} p="4" w="md" gap="3">
      {tooltip && <Text>{tooltip}</Text>}
      <ExpandableTextField
        label="Reason"
        title="Start over reason"
        placeholder="Why start over?"
        value={reason}
        onChange={setReason}
        rows={3}
      />
      <Box display="flex" gap="2" justifyContent="flex-end">
        <Button emphasis="ghost" onClick={onClose}>
          Cancel
        </Button>
        <Button tone="deadlocked" disabled={pending || !reason.trim()} onClick={() => onConfirm(reason.trim())}>
          {pending && <Spinner size="sm" />}
          {label}
        </Button>
      </Box>
    </Modal>
  );
}
