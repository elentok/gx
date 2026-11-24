import { createExternalStore } from "../shell/externalStore"
import type { Refusable } from "../useWrite"
import type { ConfirmContent, Group } from "./registry"

/** Where an action started: a button or menu item anchors its popover; a chord or palette opens a modal. */
export type SurfaceContext =
  | { mode: "anchor"; anchor: HTMLElement; page?: string }
  | { mode: "modal"; page?: string }

/** What the confirm shows: the consequence lines, the rows a bulk run lists, and the input it collects. */
export interface ConfirmBody extends ConfirmContent {
  /** A bulk run lists every row it runs on. */
  rows?: string[]
  /** Cautions shown apart from the lines, such as an epic that is not on trunk. */
  warnings?: string[]
  /** What the dry run answered (Archive: live sessions, `on_trunk`). */
  prepared?: unknown
}

/** One request to the confirm host. Running an action whose confirm applies renders nothing; it hands this over. */
export interface ConfirmRequest {
  actionId: string
  entityName: string
  /** The group sets the color; absent for a footer action. */
  group?: Group
  danger?: boolean
  confirmLabel: string
  dismissLabel: string
  title: string
  body: ConfirmBody
  surface: SurfaceContext
  /** Runs when the confirm opens (the Archive dry run); the confirm button stays off until it ends. */
  prepare?: () => Promise<ConfirmBody | PrepareRefusal>
  /** Set when the action removes the page the person is on: focus goes to the main heading of the page they land on. */
  removesOpenPage?: boolean
  /** Where focus goes when the anchor is gone; the host falls back to the page's ⋯ button. */
  focusAfter?: () => HTMLElement | null
  /** Runs the write. An action with an input gets the typed text. */
  onConfirm: (input?: string) => void
}

/** A dry run the server refused: the confirm closes and the reason is toasted. */
export interface PrepareRefusal extends Refusable {
  refused: true
}

const store = createExternalStore<ConfirmRequest | null>(null)

export const openConfirm = (request: ConfirmRequest) => store.set(request)
export const closeConfirm = () => store.set(null)
export const currentConfirm = store.get
export const useConfirmRequest = store.use
