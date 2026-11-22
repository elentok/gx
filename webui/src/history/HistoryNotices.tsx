import { Text } from "@chakra-ui/react"

/** The notices a History pick left, one line each, under a draft page's title row. */
export function HistoryNotices({ notices }: { notices: string[] }) {
  return notices.map((n) => (
    <Text key={n} fontSize="sm" color="text.dim" pt="2" flexShrink="0">
      {n}
    </Text>
  ))
}
