import { Text } from "@chakra-ui/react"
import { useParams } from "react-router"
import { DetailPage } from "../page/DetailPage"
import { PageHeader } from "../page/PageHeader"
import { PageIcon } from "../page/PageIcon"
import { useReady } from "../useReady"

/** A stub until the Initiative page itself lands: it shows the slug. */
export function InitiativePage() {
  const address = useParams()["*"] ?? ""
  useReady()
  return (
    <DetailPage title={{ kind: "Initiative", name: address.slice(address.indexOf(":") + 1) }}>
      <PageHeader
        title={address.slice(address.indexOf(":") + 1)}
        icon={<PageIcon kind="initiative" />}
      />
      <Text color="text.muted">{address}</Text>
    </DetailPage>
  )
}
