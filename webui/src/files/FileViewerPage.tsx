import { Box, chakra, Text } from "@chakra-ui/react"
import { useState } from "react"
import { useParams } from "react-router"
import { copyText } from "../clipboard"
import { formatAge } from "../epics/cardTime"
import { HostedFound, type HostedResolved } from "../generated/ops"
import { sizeText } from "../health/formatBytes"
import { DetailPage } from "../page/DetailPage"
import { Button } from "../ui/button/Button"
import { Spinner } from "../ui/spinner/Spinner"
import { showToast } from "../ui/toaster"
import { useFetched } from "../useFetched"
import { useFetchedReady } from "../useReady"
import { rawUrl } from "./rawUrl"
import { createResolver } from "./resolve"
import { ViewerBody } from "./viewerBody"
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

function Bar({ path, meta, found }: { path: string; meta: string; found: boolean }) {
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

function FoundView({ path, file }: { path: string; file: HostedResolved }) {
  const [pixels, setPixels] = useState("")
  const [imageFailed, setImageFailed] = useState(false)
  if (imageFailed) {
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
      <Bar path={path} meta={meta} found />
      <ViewerBody
        file={file}
        path={path}
        onImageLoad={(w, h) => setPixels(`${w}×${h}`)}
        onImageError={() => setImageFailed(true)}
      />
    </>
  )
}

export function FileViewerPage({
  fetchResolve = resolveOwnPath,
}: {
  fetchResolve?: FetchFileResolve
}) {
  // The router already decoded the splat once; it is the absolute path without its leading slash.
  const path = `/${useParams()["*"] ?? ""}`
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
            <FoundView key={path} path={path} file={data} />
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
