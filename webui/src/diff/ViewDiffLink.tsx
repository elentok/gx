import { diffHref } from "../hrefs"
import { Button } from "../ui/button/Button"
import { viewDiffUrl } from "./diffUrl"

// A "View diff" link to the Diff page. The caller decides whether it shows, from the response's boolean.
export function ViewDiffLink(props: {
  epic: string
  base?: string
  tip?: string
  ticket?: string
}) {
  return (
    <Button asChild emphasis="secondary" size="sm">
      <a href={diffHref(viewDiffUrl(props))}>View diff</a>
    </Button>
  )
}
