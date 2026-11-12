import { useState } from "react"
import { useConnectionContext } from "../connection"
import { useDetail } from "../ticket/detail"
import { ImageStrip } from "./ImageStrip"
import { ImageViewer } from "./ImageViewer"
import { fetchMapImages, mapImageUrl, type FetchMapImages } from "./mapImages"

/** The read-only strip of the Design's images at the top of its chat; nothing when the map has none. */
export function MapImageStrip({
  address,
  fetcher = fetchMapImages,
}: {
  address: string
  fetcher?: FetchMapImages
}) {
  const { detail } = useDetail(address, useConnectionContext(), fetcher)
  const [viewing, setViewing] = useState<number>()
  const list = Array.isArray(detail) ? detail.filter((i) => i.id) : []
  if (list.length === 0) return null
  const images = list.map((i) => ({ id: i.id }))
  return (
    <>
      <ImageStrip
        readOnly
        images={list.map((i) => ({
          key: i.id!,
          state: "ready",
          url: mapImageUrl(address, i.id!),
          canRetry: false,
        }))}
        onOpen={setViewing}
      />
      {viewing !== undefined && (
        <ImageViewer
          images={images}
          source={(img) => mapImageUrl(address, img.id ?? "")}
          start={viewing}
          onClose={() => setViewing(undefined)}
        />
      )}
    </>
  )
}
