/** The text fields a New Design draft autosaves. */
export interface Fields {
  slug: string;
  idea: string;
  project: string;
}

export const hasText = (d: Partial<Fields>) => !!(d.slug?.trim() || d.idea?.trim());
