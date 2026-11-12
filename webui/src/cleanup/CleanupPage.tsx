import { Box, chakra, Text } from "@chakra-ui/react"
import { useState, type ReactNode } from "react"
import { useSearchParams } from "react-router"
import { useConnectionContext } from "../connection"
import { useEpicList, type FetchEpicList } from "../epics/list"
import { DetailPage } from "../page/DetailPage"
import { PageHeader } from "../page/PageHeader"
import { PageIcon } from "../page/PageIcon"
import { SectionHeading } from "../page/SectionHeading"
import { Button } from "../ui/button/Button"
import { Modal } from "../ui/modal/Modal"
import { FullScreenLoader } from "../ui/loader/FullScreenLoader"
import { refusalText, useWrite } from "../useWrite"
import { sizeText } from "../health/formatBytes"
import {
  groupReport,
  postCleanup as defaultPost,
  useCleanup,
  type CleanupAction,
  type CleanupGroup,
  type FetchCleanup,
  type PostCleanup,
} from "./cleanup"

interface Acts {
  pending: boolean
  /** The refusal text kept on a row, by row key. */
  refused: Record<string, string>
  act: (key: string, action: CleanupAction, branch?: string, epic?: string) => void
  confirmUnmerged: (branch: string) => void
}

function UnmergedDialog({
  branch,
  onConfirm,
  onClose,
}: {
  branch: string
  onConfirm: () => void
  onClose: () => void
}) {
  return (
    <Modal label="Remove unmerged worktree" onClose={onClose} w="md" p="4" gap="3">
      <Text fontWeight="600">{`Remove the worktree of ${branch}?`}</Text>
      <Text>Ignored files such as node_modules and build output are lost. The branch stays.</Text>
      <Box display="flex" gap="2" justifyContent="flex-end">
        <Button tone="deadlocked" onClick={onConfirm}>
          Confirm remove
        </Button>
        <Button emphasis="ghost" onClick={onClose}>
          Cancel
        </Button>
      </Box>
    </Modal>
  )
}

function Row({
  rowKey,
  name,
  acts,
  children,
}: {
  rowKey: string
  name: string
  acts: Acts
  children?: ReactNode
}) {
  return (
    <Box as="li" display="flex" flexDirection="column" wordBreak="break-all">
      <Box display="flex" justifyContent="space-between" alignItems="center" gap="3">
        <Text>{name}</Text>
        <Box display="flex" alignItems="center" gap="3" flexShrink="0">
          {children}
        </Box>
      </Box>
      {acts.refused[rowKey] && (
        <Text role="alert" color="text.muted" fontSize="sm">
          {acts.refused[rowKey]}
        </Text>
      )}
    </Box>
  )
}

function Group({ group, acts }: { group: CleanupGroup; acts: Acts }) {
  const id = `cleanup-${group.id}`
  const agent = group.id === "agent"
  const button = (label: string, onClick: () => void) => (
    <Button emphasis="ghost" disabled={acts.pending} onClick={onClick}>
      {label}
    </Button>
  )
  return (
    <Box mt="6" role="region" aria-labelledby={id}>
      <SectionHeading id={id} tone={agent ? "waiting" : "running"}>
        {group.title}
      </SectionHeading>
      <Text color="text.muted" fontSize="sm" mb="2">
        {group.hint}
      </Text>
      {agent && button("Start cleanup agent", () => acts.act("agent", "agent"))}
      <Box as="ul" listStyleType="none" p="0" display="flex" flexDirection="column" gap="1">
        {group.worktrees.map((w) => (
          <Row key={w.branch} rowKey={w.branch} name={w.branch} acts={acts}>
            {w.size_bytes !== undefined && (
              <Text color="text.muted" data-testid="cleanup-size">
                {sizeText(w.size_bytes, "binary")}
              </Text>
            )}
            {group.id === "merged-worktrees" &&
              !w.dirty &&
              button("Remove", () => acts.act(w.branch, "remove-worktree", w.branch))}
            {group.id === "merged-branches" &&
              button("Delete branch", () => acts.act(w.branch, "delete-branch", w.branch))}
            {group.id === "unmerged-worktrees" &&
              button("Remove", () => acts.confirmUnmerged(w.branch))}
          </Row>
        ))}
        {group.epics.map((e) => (
          <Row key={`epic:${e.name}`} rowKey={`epic:${e.name}`} name={e.name} acts={acts}>
            {group.id === "done-epics" &&
              button("Archive", () =>
                acts.act(`epic:${e.name}`, "archive-epic", undefined, e.name),
              )}
          </Row>
        ))}
      </Box>
    </Box>
  )
}

export function CleanupPage({
  fetchCleanup,
  fetchEpics,
  postCleanup = defaultPost,
}: {
  fetchCleanup?: FetchCleanup
  fetchEpics?: FetchEpicList
  postCleanup?: PostCleanup
}) {
  const conn = useConnectionContext()
  const [params, setParams] = useSearchParams()
  const { list } = useEpicList(fetchEpics, conn)
  const projects = [...new Set(list?.epics.map((e) => e.project) ?? [])].sort()
  const project = params.get("project") ?? projects[0] ?? ""
  const { report, failed, retry } = useCleanup(project, fetchCleanup, conn)
  const groups = report ? groupReport(report) : []
  const { run, pending } = useWrite({ toastRefusal: false })
  const [refused, setRefused] = useState<Record<string, string>>({})
  const [confirming, setConfirming] = useState<string>()

  const act: Acts["act"] = (key, action, branch, epic) => {
    setConfirming(undefined)
    setRefused((r) => Object.fromEntries(Object.entries(r).filter(([k]) => k !== key)))
    void run(
      async () => {
        const res = await postCleanup(action, { project, branch, epic })
        if (res.refused) setRefused((r) => ({ ...r, [key]: refusalText(res) }))
        return res
      },
      { onDone: retry },
    )
  }
  const acts: Acts = { pending, refused, act, confirmUnmerged: setConfirming }

  return (
    <DetailPage title={{ kind: "Cleanup" }}>
      <PageHeader title="Cleanup" icon={<PageIcon kind="cleanup" />} subtitle={project} />
      <Box mt="4">
        <chakra.select
          aria-label="Project"
          value={project}
          onChange={(e) => setParams({ project: e.target.value })}
          fontSize="sm"
          px="2"
          py="1"
          borderWidth="1px"
          borderRadius="md"
          bg="transparent"
        >
          {projects.map((p) => (
            <option key={p} value={p}>
              {p}
            </option>
          ))}
        </chakra.select>
      </Box>
      {failed ? (
        <Box mt="6" display="flex" alignItems="center" gap="3">
          <Text color="text.muted">Could not load cleanup</Text>
          <Button emphasis="ghost" onClick={retry}>
            Retry
          </Button>
        </Box>
      ) : !report ? (
        <FullScreenLoader label="Loading cleanup" />
      ) : groups.length === 0 ? (
        <Text mt="6" color="text.muted">
          Nothing to clean up
        </Text>
      ) : (
        groups.map((g) => <Group key={g.id} group={g} acts={acts} />)
      )}
      {confirming && (
        <UnmergedDialog
          branch={confirming}
          onConfirm={() => act(confirming, "remove-unmerged-worktree", confirming)}
          onClose={() => setConfirming(undefined)}
        />
      )}
    </DetailPage>
  )
}
