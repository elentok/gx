import { Text } from "@chakra-ui/react"
import type { HostedResolved } from "../generated/ops"

/** Maps the resolve `kind` to its body. Later tickets add one branch per kind. */
export function ViewerBody(_props: { file: HostedResolved }) {
  return (
    <Text p="5" textAlign="center" color="text.muted">
      No preview yet
    </Text>
  )
}
