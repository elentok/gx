import { useEffect } from "react"
import { notificationToastEvent, useConnectionContext } from "./connection"
import { routeOpener } from "./pwa/notificationTap"
import { showToast } from "./ui/toaster"

const EVENT_TYPE = "notification-toast"

// Shows the server's Web toast notifications. The server stores none and the page does not replay
// events from an outage, so only a page that is open when one is sent shows it.
export function NotificationToast() {
  const conn = useConnectionContext()
  useEffect(() => {
    const off = conn.onEvent((_address, type, data) => {
      if (type !== EVENT_TYPE) return
      const { title, body, path } = notificationToastEvent(data)
      if (!title) return
      const open = routeOpener(path)
      showToast({
        type: "info",
        title,
        description: body,
        actions: open && [{ label: "Open", onClick: open }],
      })
    })
    return () => {
      off()
    }
  }, [conn])
  return null
}
