import { Box, Text } from "@chakra-ui/react";
import { useRef, useState } from "react";
import { useConnectionContext } from "../connection";
import { TargetDone, type TargetResult } from "../generated/server";
import { isExited } from "../agents/sessionWrites";
import type { SessionInfo } from "../generated/server";
import { bulkHold, bulkMove, bulkRemove, bulkStop, bulkUnhold } from "../select/bulkWrites";
import { skipLines, visibleActions, type ActionView, type SelectAction } from "../select/selectModel";
import { Button } from "../ui/button/Button";
import { GxMenu } from "../ui/menu/Menu";
import { Modal } from "../ui/modal/Modal";
import { showToast } from "../ui/toaster";
import { WriteError } from "../useWrite";
import { useWorkspaces } from "../workspaces/useActiveWorkspace";
import { useSidebarSelect } from "./SidebarSelect";

const SHOWN: SelectAction[] = ["move", "hold", "unhold", "stop", "remove"];

/** Move, Hold and Take off hold in the select bar: each runs on the selected rows it fits. */
export function SidebarBulkActions() {
  const sel = useSidebarSelect();
  const { workspaces } = useWorkspaces(useConnectionContext());
  const [pending, setPending] = useState(false);
  const inFlight = useRef(false);
  const [confirm, setConfirm] = useState<{ keys: string[]; sessions: string[] }>();
  const [stopConfirm, setStopConfirm] = useState<{ view: ActionView; sessions: SessionInfo[] }>();
  const [removeConfirm, setRemoveConfirm] = useState<{ view: ActionView; sessions: SessionInfo[] }>();

  const views = visibleActions(sel.state, sel.rows).filter((v) => SHOWN.includes(v.action));
  if (views.length === 0 && !confirm && !stopConfirm && !removeConfirm) return null;

  const tip = (v: ActionView) =>
    [`Runs on ${v.fit.fits.map(sel.nameOf).join(", ")}`, ...skipLines(v.fit, sel.nameOf)].join("\n");

  /** Runs one write; reports refusals and the rows the action did not fit, and ends the mode when nothing is left. */
  async function run(v: ActionView, call: () => Promise<TargetResult[] | undefined>) {
    if (inFlight.current) return;
    inFlight.current = true;
    setPending(true);
    try {
      const results = await call();
      // undefined: the call stopped to ask a question; nothing ran yet.
      if (!results) return;
      for (const r of results) {
        if (r.status !== TargetDone) showToast({ type: "error", title: `${sel.nameOf(r.target)}: ${r.message || r.reason || "refused"}` });
      }
      sel.deselect(results.filter((r) => r.status === TargetDone).map((r) => r.target));
      if (v.fit.skipped.length > 0) showToast({ type: "info", title: "Not run on some rows", description: skipLines(v.fit, sel.nameOf).join("; ") });
      sel.afterRun();
    } catch (err) {
      showToast({ type: "error", title: err instanceof WriteError ? err.message : "Could not reach gx. Try again." });
    } finally {
      inFlight.current = false;
      setPending(false);
    }
  }

  // A dry run first: live sessions are the only thing worth a question.
  const hold = (v: ActionView) =>
    run(v, async () => {
      const keys = v.fit.fits;
      const dry = await bulkHold(keys, { dryRun: true });
      const sessions = dry.flatMap((r) => r.sessions ?? []);
      if (sessions.length === 0) return bulkHold(keys);
      setConfirm({ keys, sessions });
      return undefined;
    });

  const nameOfSession = (s: SessionInfo) => sel.nameOf(s.id);
  const sessionsOf =(keys: string[]) => keys.flatMap((k) => sel.rows.find((r) => r.key === k)?.session ?? []);

  // Stop asks once, listing the rows; Exited rows were already skipped "not live" by the fit.
  const stop = (v: ActionView) => setStopConfirm({ view: v, sessions: sessionsOf(v.fit.fits) });
  // Remove asks once and says which rows are live.
  const remove = (v: ActionView) => setRemoveConfirm({ view: v, sessions: sessionsOf(v.fit.fits) });

  const byAction = (a: SelectAction) => views.find((v) => v.action === a);
  const stopView = byAction("stop");
  const removeView = byAction("remove");
  const move = byAction("move");
  const holdView = byAction("hold");
  const unhold = byAction("unhold");

  return (
    <Box display="flex" flexWrap="wrap" gap="1.5">
      {move && (
        <GxMenu
          placement="top-start"
          trigger={
            <Button size="sm" emphasis="secondary" tone="running" title={tip(move)} disabled={pending}>
              {move.label}
            </Button>
          }
          items={workspaces.map((w) => ({
            value: `move-${w.name}`,
            label: w.name,
            disabled: pending,
            onSelect: () => void run(move, () => bulkMove(move.fit.fits, w.name)),
          }))}
        />
      )}
      {holdView && (
        <Button size="sm" emphasis="secondary" tone="paused" title={tip(holdView)} disabled={pending} onClick={() => void hold(holdView)}>
          {holdView.label}
        </Button>
      )}
      {unhold && (
        <Button size="sm" emphasis="secondary" tone="running" title={tip(unhold)} disabled={pending} onClick={() => void run(unhold, () => bulkUnhold(unhold.fit.fits))}>
          {unhold.label}
        </Button>
      )}
      {stopView && (
        <Button size="sm" emphasis="secondary" tone="paused" title={tip(stopView)} disabled={pending} onClick={() => stop(stopView)}>
          {stopView.label}
        </Button>
      )}
      {removeView && (
        <Button size="sm" emphasis="secondary" tone="paused" title={tip(removeView)} disabled={pending} onClick={() => remove(removeView)}>
          {removeView.label}
        </Button>
      )}
      {stopConfirm && (
        <Modal label="Stop selected" onClose={() => setStopConfirm(undefined)} maxW="sm">
          <Text>Stop {stopConfirm.sessions.map(nameOfSession).join(", ")}?</Text>
          <Box display="flex" gap="2" justifyContent="flex-end">
            <Button emphasis="secondary" onClick={() => setStopConfirm(undefined)} autoFocus>
              Cancel
            </Button>
            <Button
              tone="paused"
              onClick={() => {
                const { view, sessions } = stopConfirm;
                setStopConfirm(undefined);
                void run(view, () => bulkStop(sessions));
              }}
            >
              Stop
            </Button>
          </Box>
        </Modal>
      )}
      {removeConfirm && (
        <Modal label="Remove selected" onClose={() => setRemoveConfirm(undefined)} maxW="sm">
          <Text>
            Remove {removeConfirm.sessions.map(nameOfSession).join(", ")}? Their sessions are deleted.
            {removeConfirm.sessions.some((s) => !isExited(s)) &&
              ` Live and stopped first: ${removeConfirm.sessions.filter((s) => !isExited(s)).map(nameOfSession).join(", ")}.`}
          </Text>
          <Box display="flex" gap="2" justifyContent="flex-end">
            <Button emphasis="secondary" onClick={() => setRemoveConfirm(undefined)} autoFocus>
              Keep
            </Button>
            <Button
              tone="paused"
              onClick={() => {
                const { view, sessions } = removeConfirm;
                setRemoveConfirm(undefined);
                void run(view, () => bulkRemove(sessions));
              }}
            >
              Remove
            </Button>
          </Box>
        </Modal>
      )}
      {confirm && (
        <Modal label="Hold selected" onClose={() => setConfirm(undefined)} maxW="sm">
          <Text>Holding ends these live sessions: {confirm.sessions.join(", ")}.</Text>
          <Box display="flex" gap="2" justifyContent="flex-end">
            <Button emphasis="secondary" onClick={() => setConfirm(undefined)} autoFocus>
              Cancel
            </Button>
            <Button
              tone="paused"
              onClick={() => {
                const { keys } = confirm;
                setConfirm(undefined);
                const v = holdView ?? { action: "hold" as const, label: "Hold", fit: { fits: keys, skipped: [] } };
                void run(v, () => bulkHold(keys, { yes: true }));
              }}
            >
              Hold and end sessions
            </Button>
          </Box>
        </Modal>
      )}
    </Box>
  );
}
