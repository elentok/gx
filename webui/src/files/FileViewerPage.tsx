import { Box, chakra, Text } from "@chakra-ui/react"
import { useEffect, useState } from "react"
import { useParams, useSearchParams } from "react-router"
import { copyText } from "../clipboard"
import { formatAge } from "../epics/cardTime"
import {
  HostedFound,
  HostedKindCode,
  HostedKindMarkdown,
  HostedKindMermaid,
  HostedKindText,
  type HostedResolved,
} from "../generated/ops"
import { sizeText } from "../health/formatBytes"
import { DetailPage } from "../page/DetailPage"
import { Button } from "../ui/button/Button"
import { Spinner } from "../ui/spinner/Spinner"
import { showToast } from "../ui/toaster"
import { useFetched } from "../useFetched"
import { useFetchedReady } from "../useReady"
import { rawUrl } from "./rawUrl"
import { type FetchFileText, fetchRawText } from "./rawText"
import { createResolver } from "./resolve"
import { previewNote, ViewerBody } from "./viewerBody"
import { viewerLink } from "./viewerUrl"

/** The verdict for one absolute path. Rejects when the call failed. */
export type FetchFileResolve = (path: string) => Promise<HostedResolved>

const resolveOwnPath: FetchFileResolve = async (path) => {
  const reply = (await createResolver().resolve([path])).get(path)
  if (!reply) throw new Error("resolve failed")
  return reply
}

const linkButtonProps = {
  display: "inline-flex",
  alignItems: "center",
  px: "3",
  h: "30px",
  fontSize: "12.5px",
  borderWidth: "1px",
  borderColor: "border",
  borderRadius: "8px",
  bg: "surface.raised",
  color: "text",
  textDecoration: "none",
} as const

function metaText(file: HostedResolved): string {
  const parts: string[] = []
  if (file.size !== undefined) parts.push(sizeText(file.size, "binary"))
  const modified = file.modified ? Date.parse(file.modified) : NaN
  if (!Number.isNaN(modified)) parts.push(`${formatAge(Date.now() - modified)} ago`)
  return parts.join(" · ")
}

async function copyViewerLink(path: string) {
  const ok = await copyText(viewerLink(path), null)
  showToast(
    ok ? { type: "info", title: "Link copied" } : { type: "error", title: "Could not copy" },
  )
}

function Bar({
  path,
  meta,
  found,
  source,
}: {
  path: string
  meta: string
  found: boolean
  /** Markdown only: the Source toggle. */
  source?: { on: boolean; toggle: () => void }
}) {
  return (
    <Box
      data-testid="viewer-bar"
      display="flex"
      flexWrap="wrap"
      alignItems="center"
      gap="10px"
      px="3"
      py="2"
      borderBottomWidth="1px"
      borderColor="border"
    >
      <Text flex="1" minW="140px" fontFamily="mono" fontSize="12.5px" overflowWrap="anywhere">
        {path}
      </Text>
      {meta && (
        <Text fontSize="12px" color="text.muted">
          {meta}
        </Text>
      )}
      {source && (
        <Button
          emphasis="secondary"
          size="sm"
          data-mock-part="source-toggle"
          aria-pressed={source.on}
          onClick={source.toggle}
        >
          Source
        </Button>
      )}
      {found && (
        <>
          <chakra.a {...linkButtonProps} href={rawUrl(path)} target="_blank" rel="noopener">
            Raw
          </chakra.a>
          <chakra.a {...linkButtonProps} href={rawUrl(path, { download: true })} download>
            Download
          </chakra.a>
        </>
      )}
      <Button emphasis="secondary" size="sm" onClick={() => void copyViewerLink(path)}>
        Copy link
      </Button>
    </Box>
  )
}

function NotFound() {
  return (
    <Box p="5" textAlign="center" color="text.muted">
      <Text as="b" display="block" mb="1" color="text">
        File not found
      </Text>
      It may have been deleted, moved or outside the known folders.
    </Box>
  )
}

function isTextKind(file: HostedResolved): boolean {
  const text =
    file.kind === HostedKindCode ||
    file.kind === HostedKindText ||
    file.kind === HostedKindMarkdown ||
    file.kind === HostedKindMermaid
  return text && !previewNote(file)
}

/** The file text of a code or text file, kept out of the page cache. */
function useFileText(path: string, wanted: boolean, fetchText: FetchFileText) {
  const [state, setState] = useState<{ text?: string; missing: boolean }>({ missing: false })
  useEffect(() => {
    if (!wanted) return
    let live = true
    fetchText(path).then(
      (text) => live && setState({ text, missing: text === undefined }),
      () => live && setState({ missing: true }),
    )
    return () => {
      live = false
    }
  }, [path, wanted, fetchText])
  return state
}

function FoundView({
  path,
  file,
  line,
  heading,
  fetchText,
}: {
  path: string
  file: HostedResolved
  line?: number
  heading?: string
  fetchText: FetchFileText
}) {
  const [pixels, setPixels] = useState("")
  const [imageFailed, setImageFailed] = useState(false)
  const [showSource, setShowSource] = useState(false)
  const body = useFileText(path, isTextKind(file), fetchText)
  if (imageFailed || body.missing) {
    return (
      <>
        <Bar path={path} meta="" found={false} />
        <NotFound />
      </>
    )
  }
  const meta = [metaText(file), pixels].filter(Boolean).join(" · ")
  return (
    <>
      <Bar
        path={path}
        meta={meta}
        found
        source={
          (file.kind === HostedKindMarkdown || file.kind === HostedKindMermaid) && isTextKind(file)
            ? { on: showSource, toggle: () => setShowSource((s) => !s) }
            : undefined
        }
      />
      <ViewerBody
        file={file}
        path={path}
        text={body.text}
        line={line}
        heading={heading}
        source={showSource}
        onImageLoad={(w, h) => setPixels(`${w}×${h}`)}
        onImageError={() => setImageFailed(true)}
      />
    </>
  )
}

export function FileViewerPage({
  fetchResolve = resolveOwnPath,
  fetchText = fetchRawText,
}: {
  fetchResolve?: FetchFileResolve
  fetchText?: FetchFileText
}) {
  // The router already decoded the splat once; it is the absolute path without its leading slash.
  const path = `/${useParams()["*"] ?? ""}`
  // Only the kinds that show lines read `line`; a bad value means no mark.
  const [params] = useSearchParams()
  const line = Number(params.get("line") ?? NaN)
  const heading = params.get("heading") ?? undefined
  const { data, failed, source, retry } = useFetched(
    "file-viewer",
    () => fetchResolve(path),
    undefined,
    {
      id: path,
    },
  )
  useFetchedReady({ failed, source })
  return (
    <DetailPage title={{ kind: "File", name: path.split("/").pop() }}>
      {failed ? (
        <Box mt="6" display="flex" alignItems="center" gap="3">
          <Text color="text.muted">Could not load file</Text>
          <Button emphasis="ghost" onClick={retry}>
            Retry
          </Button>
        </Box>
      ) : !data ? (
        <Spinner label="Loading file" />
      ) : (
        <Box
          borderWidth="1px"
          borderColor="border"
          borderRadius="10px"
          overflow="hidden"
          bg="surface"
        >
          {data.verdict === HostedFound ? (
            <FoundView
              key={path}
              path={path}
              file={data}
              line={line}
              heading={heading}
              fetchText={fetchText}
            />
          ) : (
            <>
              <Bar path={path} meta="" found={false} />
              <NotFound />
            </>
          )}
        </Box>
      )}
    </DetailPage>
  )
}
