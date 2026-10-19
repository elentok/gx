import { useLiveConnection, type ConnectionDeps } from "./connection";
import { QueueScreen } from "./queue/QueueScreen";

export function App({ deps }: { deps?: ConnectionDeps }) {
  return <QueueScreen state={useLiveConnection(deps)} />;
}
