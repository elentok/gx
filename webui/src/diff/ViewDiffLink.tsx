import { diffHref } from "../hrefs"
import { Button } from "../ui/button/Button"
import type { DiffUrl } from "./diffUrl"

/** The project part of a `project:name` address. */
const projectOf = (address: string) => address.slice(0, address.indexOf(":"))

// A "View diff" link to the Diff page. The caller decides whether it shows, from the response's boolean.
export function ViewDiffLink({
  epic,
  base,
  tip,
  ticket,
}: {
  epic: string
  base?: string
  tip?: string
  ticket?: string
}) {
  const url: DiffUrl = {
    project: projectOf(epic),
    first: base ?? "",
    second: tip ?? "",
    epic,
    ticket,
  }
  return (
    <Button asChild emphasis="secondary" size="sm">
      <a href={diffHref(url)}>View diff</a>
    </Button>
  )
}
