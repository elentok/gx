import { PageHeader } from "./PageHeader"

/** The header for a page state that has none of its own (loading, error, empty): the burger plus a title. */
export function StateHeader({ title }: { title: string }) {
  return <PageHeader title={title} />
}
