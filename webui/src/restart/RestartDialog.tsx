import { Box, Spinner, Text } from "@chakra-ui/react";
import { useEffect, useRef, useState } from "react";
import type { RestartCheckReport } from "../generated/server";
import { Button } from "../ui/button/Button";
import { Modal } from "../ui/modal/Modal";
import { fetchRestartCheck, postRestart, type FetchRestartCheck, type PostRestart } from "./api";

export const RESTART_TIMEOUT_MS = 60_000;

interface Props {
  /** The live snapshot's instance id; a change after the click means the new server is up. */
  liveInstance?: string;
  check?: FetchRestartCheck;
  restart?: PostRestart;
  /** Injected so tests can allow exactly one reload. */
  reload?: () => void;
  onClose: () => void;
}

const failing = (r: RestartCheckReport) => r.projects.filter((p) => !p.ok);

/** Confirms a server restart: runs the check on mount, then sends the confirmed values. */
export function RestartDialog({ liveInstance, check = fetchRestartCheck, restart = postRestart, reload = () => window.location.reload(), onClose }: Props) {
  const [report, setReport] = useState<RestartCheckReport>();
  const [checkError, setCheckError] = useState<string>();
  const [notice, setNotice] = useState<string>();
  const [sending, setSending] = useState(false);
  const [restarting, setRestarting] = useState(false);
  const [timedOut, setTimedOut] = useState(false);
  const [log, setLog] = useState<string>();
  // The instance the click was made against; the reload fires when the live one differs.
  const clickedInstance = useRef<string | undefined>(undefined);

  useEffect(() => {
    let live = true;
    check().then(
      (r) => live && setReport(r),
      (e) => live && setCheckError(String(e?.message ?? e)),
    );
    return () => {
      live = false;
    };
  }, [check]);

  useEffect(() => {
    if (!restarting) return;
    const t = setTimeout(() => setTimedOut(true), RESTART_TIMEOUT_MS);
    return () => clearTimeout(t);
  }, [restarting]);

  const reloaded = useRef(false);
  useEffect(() => {
    if (!restarting || reloaded.current || !liveInstance || liveInstance === clickedInstance.current) return;
    reloaded.current = true;
    reload();
  }, [restarting, liveInstance, reload]);

  const begin = (instance: string, restartLog?: string) => {
    clickedInstance.current = instance;
    if (restartLog) setLog(restartLog);
    setRestarting(true);
  };

  const click = async () => {
    if (!report) return;
    setSending(true);
    setNotice(undefined);
    try {
      const res = await restart({ new_version: report.new_version ?? "", config_ok: report.config.ok, failing_projects: failing(report).map((p) => p.name) });
      if (res.started || res.restarting) {
        begin(report.instance, res.restart_log || report.restart_log);
      } else {
        if (res.report) setReport(res.report);
        setNotice(res.message);
      }
    } catch {
      // The server may have gone down before it answered.
      begin(report.instance);
    } finally {
      setSending(false);
    }
  };

  const ok = report?.status === "ok";
  const configBad = ok && !report.config.ok;
  const disabledError = report && (!ok ? { text: report.error || "The restart check failed.", hint: report.hint } : configBad ? { text: report.config.error || "The config is not valid.", hint: undefined } : undefined);
  const badProjects = report ? failing(report) : [];
  const anyway = ok && badProjects.length > 0;
  const logPath = log ?? report?.restart_log;

  return (
    <Modal label="Restart server" onClose={onClose} w="md" p="4" gap="3">
      <Text fontWeight="600">Restart server</Text>
      {restarting ? (
        <Box display="flex" flexDirection="column" gap="2" role="status">
          <Text>
            <Spinner size="sm" /> Restarting...
          </Text>
          {timedOut && (
            <Text>
              {`The server did not return. It may have failed to start. Check the restart log at ${logPath ?? "the restart log"}, or run \`gx server start\`. This page keeps checking.`}
            </Text>
          )}
        </Box>
      ) : !report && !checkError ? (
        <Text role="status">
          <Spinner size="sm" /> Checking...
        </Text>
      ) : checkError ? (
        <Text role="alert">{`The restart check failed: ${checkError}. Run \`gx server restart\` instead.`}</Text>
      ) : report ? (
        <Box display="flex" flexDirection="column" gap="2">
          <Text>{`Running version: ${report.running_version}`}</Text>
          {ok && <Text>{`Next version: ${report.new_version}`}</Text>}
          {ok && report.new_version === report.running_version && <Text>Versions match</Text>}
          {disabledError && (
            <Text role="alert">{`${disabledError.text}${disabledError.hint ? ` ${disabledError.hint}.` : " Run `gx server restart` instead."}`}</Text>
          )}
          {anyway && (
            <Box role="alert">
              <Text>A migration dry run failed for:</Text>
              {badProjects.map((p) => (
                <Text key={p.name}>{`${p.name}: ${p.error ?? "failed"}`}</Text>
              ))}
              <Text>After the restart the project is in Migration block. You can retry the migration from the strip.</Text>
            </Box>
          )}
          {report.agents > 0 && (
            <>
              <Text>{`${report.agents} ${report.agents === 1 ? "agent is" : "agents are"} running. Agents keep running through the restart.`}</Text>
              {report.projects.some((p) => p.pending > 0) && <Text>A migration runs while the agents work.</Text>}
            </>
          )}
          {notice && <Text role="alert">{notice}</Text>}
        </Box>
      ) : null}
      {!restarting && (
        <Box display="flex" gap="2" justifyContent="flex-end">
          <Button tone={anyway ? "deadlocked" : "running"} disabled={!report || !!disabledError || sending} onClick={click}>
            {sending && <Spinner size="sm" />}
            {anyway ? "Restart anyway" : "Restart"}
          </Button>
          <Button emphasis="ghost" onClick={onClose}>
            Cancel
          </Button>
        </Box>
      )}
    </Modal>
  );
}
