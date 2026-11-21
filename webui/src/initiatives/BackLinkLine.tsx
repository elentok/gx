import { Link, Text } from "@chakra-ui/react"
import type { BackLink } from "../generated/server"
import { initiativeHref } from "../hrefs"

/** "Milestone N of <Initiative>", shared by the epic card and the Design and epic headers. */
export function BackLinkLine({ link }: { link: BackLink }) {
  if (!link.milestone || !link.initiative || !link.initiative_address) return null
  return (
    <Text data-testid="back-link" fontSize="13px" color="text.dim">
      Milestone {link.milestone} of{" "}
      <Link href={initiativeHref(link.initiative_address)} color="tone.running.fg">
        {link.initiative}
      </Link>
    </Text>
  )
}
