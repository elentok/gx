import { Box, Link, Text } from "@chakra-ui/react"
import type { ReactNode } from "react"
import { useParams } from "react-router"
import { useConnectionContext } from "../connection"
import type {
  InitiativeGateNote,
  InitiativeMilestone,
  InitiativePage as InitiativeData,
  InitiativeUnparsed,
} from "../generated/server"
import { archiveHref, designHref, epicHref } from "../hrefs"
import { DetailPage } from "../page/DetailPage"
import { PageHeader } from "../page/PageHeader"
import { PageIcon } from "../page/PageIcon"
import { StateHeader } from "../page/StateHeader"
import { Button } from "../ui/button/Button"
import { Chip } from "../ui/chip/Chip"
import { EmptyState } from "../ui/empty-state/EmptyState"
import { FullScreenLoader } from "../ui/loader/FullScreenLoader"
import type { Tone } from "../ui/tone"
import { useFetched } from "../useFetched"
import { useFetchedReady } from "../useReady"
import { fetchInitiative, type FetchInitiative } from "./initiatives"

const INITIATIVE_STATE: Record<string, { label: string; tone: Tone }> = {
  active: { label: "Active", tone: "waiting" },
  stopped: { label: "Stopped", tone: "deadlocked" },
  finished: { label: "Finished", tone: "done" },
}

const MILESTONE_TONE: Record<string, Tone> = {
  sketch: "muted",
  designing: "running",
  implementing: "waiting",
  merged: "done",
  gated: "done",
  broken: "deadlocked",
}

/** Assumption, Deferred and gate-note states share one colour per word. */
const VALUE_TONE: Record<string, Tone> = {
  unknown: "muted",
  confirmed: "done",
  broken: "deadlocked",
  open: "waiting",
  taken: "done",
  dropped: "muted",
}

const headCell = {
  textAlign: "left",
  fontWeight: "500",
  fontSize: "12px",
  color: "text.muted",
  py: "1",
  px: "2",
  borderBottomWidth: "1px",
  borderColor: "hairline",
} as const

const bodyCell = {
  py: "1",
  px: "2",
  verticalAlign: "top",
  borderBottomWidth: "1px",
  borderColor: "hairline",
} as const

function StateText({ value }: { value: string }) {
  return (
    <Text as="span" color={`tone.${VALUE_TONE[value] ?? "muted"}.fg`}>
      {value}
    </Text>
  )
}

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <Box as="section" aria-label={title} mt="6">
      <Text
        as="h2"
        fontSize="15px"
        fontWeight="600"
        textTransform="uppercase"
        letterSpacing="0.04em"
        color="text.muted"
        mb="2"
      >
        {title}
      </Text>
      {children}
    </Box>
  )
}

function Empty({ children }: { children: ReactNode }) {
  return (
    <Text color="text.muted" fontStyle="italic">
      {children}
    </Text>
  )
}

/** Lines the server could not parse, verbatim, in the section they came from. */
function UnparsedLines({ lines }: { lines: InitiativeUnparsed[] }) {
  return (
    <>
      {lines.map((u) => (
        <Text key={u.raw} color="text.muted" fontFamily="mono" whiteSpace="pre-wrap" data-unparsed>
          {u.raw}
        </Text>
      ))}
    </>
  )
}

function MilestoneBlock({ m }: { m: InitiativeMilestone }) {
  const tone = MILESTONE_TONE[m.state] ?? "muted"
  const link = (href: string, text: string) => (
    <Link href={href} color="tone.running.fg" mr="3">
      {text}
    </Link>
  )
  return (
    <Box
      borderWidth="1px"
      borderColor="hairline"
      borderRadius="8px"
      px="3"
      py="2.5"
      mb="2"
      bg="surface.raised"
    >
      <Box display="flex" gap="2" alignItems="center" flexWrap="wrap">
        <Text as="span" fontWeight="600">
          {m.number}. {m.name}
        </Text>
        <Chip tone={tone} fontSize="12px">
          {m.state}
        </Chip>
        <Text as="span" color="text.muted">
          {m.goal}
        </Text>
      </Box>
      <Box>
        {m.broken ? (
          <Text as="span" color="text.muted">
            <Text as="span" color="tone.deadlocked.fg">
              broken link
            </Text>{" "}
            Design: {m.design}
          </Text>
        ) : !m.design ? (
          <Text as="span" color="text.muted">
            Sketch only: no Design yet
          </Text>
        ) : (
          <>
            {link(
              m.design_archived ? archiveHref(m.design_address) : designHref(m.design_address),
              `Design: ${m.design}`,
            )}
            {m.impl_exists &&
              link(
                m.impl_archived ? archiveHref(m.impl_address) : epicHref(m.impl_address),
                `Impl epic: ${m.design}-impl`,
              )}
          </>
        )}
      </Box>
    </Box>
  )
}

function GateNoteBlock({ g }: { g: InitiativeGateNote }) {
  const label = (text: string) => (
    <Text as="span" color="text.muted">
      {text}
    </Text>
  )
  return (
    <Box borderLeftWidth="3px" borderColor="tone.running.fg" px="3" py="1" mb="2.5">
      <Text>
        <b>{g.slug}</b> · outcome <b>{g.outcome}</b>
      </Text>
      {g.assumptions.map((a) => (
        <Text key={a.n}>
          A{a.n} · <StateText value={a.state} /> · {a.evidence}
        </Text>
      ))}
      <Text>
        {label("Taught:")} {g.taught.join("; ")}
      </Text>
      <Text>
        {label("Placements:")} {g.placed.join("; ")}
      </Text>
    </Box>
  )
}

function InitiativeBody({ data }: { data: InitiativeData }) {
  const state = INITIATIVE_STATE[data.state] ?? { label: data.state, tone: "muted" as Tone }
  const unparsed = (section: string) => data.unparsed.filter((u) => u.section === section)
  const paragraphs = data.destination.split(/\n\s*\n/).filter((p) => p.trim())
  return (
    <Box maxW="860px" mx="auto" w="100%">
      <PageHeader
        title={data.slug}
        icon={<PageIcon kind="initiative" />}
        actions={
          <Chip tone={state.tone} fontSize="12px">
            {state.label}
          </Chip>
        }
        subtitle={`${data.address.split(":")[0]} · initiative`}
      />
      <Section title="Destination">
        {paragraphs.length === 0 ? (
          <Empty>No Destination written yet.</Empty>
        ) : (
          paragraphs.map((p) => <Text key={p}>{p}</Text>)
        )}
      </Section>
      <Section title="Milestones">
        {data.milestones.length === 0 && unparsed("Milestones").length === 0 && (
          <Empty>No Milestones yet.</Empty>
        )}
        {data.milestones.map((m) => (
          <MilestoneBlock key={m.number} m={m} />
        ))}
        <UnparsedLines lines={unparsed("Milestones")} />
      </Section>
      <Section title="Assumptions">
        {data.assumptions.length === 0 && unparsed("Assumptions").length === 0 ? (
          <Empty>No Assumptions yet.</Empty>
        ) : (
          <>
            {data.assumptions.length > 0 && (
              <Box as="table" w="100%" borderCollapse="collapse">
                <Box as="thead">
                  <Box as="tr">
                    {["#", "Assumption", "Tested by", "State", "Evidence"].map((h) => (
                      <Box as="th" key={h} {...headCell}>
                        {h}
                      </Box>
                    ))}
                  </Box>
                </Box>
                <Box as="tbody">
                  {data.assumptions.map((a) => (
                    <Box as="tr" key={a.n}>
                      <Box as="td" {...bodyCell}>
                        A{a.n}
                      </Box>
                      <Box as="td" {...bodyCell}>
                        {a.text}
                      </Box>
                      <Box as="td" {...bodyCell}>
                        {a.tested_by}
                      </Box>
                      <Box as="td" {...bodyCell}>
                        <StateText value={a.state} />
                      </Box>
                      <Box as="td" {...bodyCell}>
                        {a.evidence}
                      </Box>
                    </Box>
                  ))}
                </Box>
              </Box>
            )}
            <UnparsedLines lines={unparsed("Assumptions")} />
          </>
        )}
      </Section>
      <Section title="Deferred">
        {data.deferred.length === 0 && unparsed("Deferred").length === 0 ? (
          <Empty>Nothing deferred.</Empty>
        ) : (
          <>
            {data.deferred.length > 0 && (
              <Box as="table" w="100%" borderCollapse="collapse">
                <Box as="thead">
                  <Box as="tr">
                    {["Item", "To", "Why", "From", "State"].map((h) => (
                      <Box as="th" key={h} {...headCell}>
                        {h}
                      </Box>
                    ))}
                  </Box>
                </Box>
                <Box as="tbody">
                  {data.deferred.map((d) => (
                    <Box as="tr" key={`${d.item}|${d.to}|${d.from}`}>
                      <Box as="td" {...bodyCell}>
                        {d.item}
                      </Box>
                      <Box as="td" {...bodyCell}>
                        {d.to}
                      </Box>
                      <Box as="td" {...bodyCell}>
                        {d.why}
                      </Box>
                      <Box as="td" {...bodyCell}>
                        {d.from}
                      </Box>
                      <Box as="td" {...bodyCell}>
                        <StateText value={d.state} />
                      </Box>
                    </Box>
                  ))}
                </Box>
              </Box>
            )}
            <UnparsedLines lines={unparsed("Deferred")} />
          </>
        )}
      </Section>
      <Section title="Gate notes">
        {data.gate_notes.length === 0 && unparsed("Gate notes").length === 0 ? (
          <Empty>
            No gate yet. The gate runs when the impl epic is merged: re-run{" "}
            <code>/gx-initiative {data.address}</code>.
          </Empty>
        ) : (
          <>
            {data.gate_notes.map((g) => (
              <GateNoteBlock key={g.slug} g={g} />
            ))}
            <UnparsedLines lines={unparsed("Gate notes")} />
          </>
        )}
      </Section>
    </Box>
  )
}

export function InitiativePage({
  fetchDetail = fetchInitiative,
}: {
  fetchDetail?: FetchInitiative
}) {
  const address = useParams()["*"] ?? ""
  const conn = useConnectionContext()
  const name = address.slice(address.indexOf(":") + 1)
  const { data, failed, source, retry } = useFetched(
    "initiative",
    () => fetchDetail(address),
    conn,
    { id: address },
  )
  useFetchedReady({ failed, source }, data?.kind === "not-found")

  return (
    <DetailPage title={{ kind: "Initiative", name }}>
      {failed ? (
        <>
          <StateHeader title={name} />
          <Box mt="6" display="flex" alignItems="center" gap="3">
            <Text color="text.muted">Could not load the initiative</Text>
            <Button emphasis="ghost" onClick={retry}>
              Retry
            </Button>
          </Box>
        </>
      ) : !data ? (
        <>
          <StateHeader title={name} />
          <FullScreenLoader label="Loading initiative" />
        </>
      ) : data.kind === "not-found" ? (
        <>
          <StateHeader title={name} />
          <EmptyState title="Initiative not found">{address} does not exist.</EmptyState>
        </>
      ) : (
        <InitiativeBody data={data.detail} />
      )}
    </DetailPage>
  )
}
