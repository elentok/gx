/** The text a draft autosaves with its revision. */
export interface TextFields {
  idea: string
}

/** The slug and project, which have their own revision-free write. */
export interface TargetFields {
  slug: string
  project: string
}

export type Fields = TextFields & TargetFields

export const hasText = (d: Partial<Fields>) => !!(d.slug?.trim() || d.idea?.trim())
