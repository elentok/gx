import {
  CardKindDesign,
  CardKindImplementation,
  EpicListFinished,
  SectionNeedsYou,
  type EpicList,
  type EpicListRow,
} from "../generated/server"
import { isCurrent } from "../epics/list"
import { sectionTitle } from "../epics/view"
import { slugOf } from "../slug"

export interface BlockerCandidate {
  /** The `-impl` epic's address: what a blocker entry holds. */
  address: string
  name: string
  /** The state the picker shows on the right. */
  state: string
}

const implOf = (row: EpicListRow): string | undefined => {
  if (row.card_kind === CardKindImplementation) return row.address
  if (row.card_kind === CardKindDesign) return row.addresses?.find((a) => a.endsWith("-impl"))
  return undefined
}

/**
 * The picker's sentence for a refused cycle. Both names come from the error: the blocker is the one
 * the server named and the chain starts at the epic being edited.
 */
export const cycleSentence = ({ blocker, chain }: { blocker?: string; chain?: string[] }) =>
  `Can’t add ${blocker}: it already waits on ${chain?.[0]}. That would be a cycle.`

/** The loop epics of this epic's project that it could wait on: not itself, not Merged. */
export function candidates(list: EpicList | null, address: string): BlockerCandidate[] {
  const project = address.slice(0, address.indexOf(":"))
  const out: BlockerCandidate[] = []
  for (const row of list?.epics ?? []) {
    const impl = implOf(row)
    if (!impl || row.project !== project || isCurrent(row, address)) continue
    if (row.state === EpicListFinished) continue
    const state =
      row.section === SectionNeedsYou ? (row.state_pill ?? "") : sectionTitle(row.section)
    out.push({ address: impl, name: slugOf(impl), state })
  }
  return out
}
