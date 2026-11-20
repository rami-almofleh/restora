import { useCallback, useState } from "react";
import { isActive } from "../../lib/jobs";
import { useRestores, useStartRestore } from "../restores/use-restores";
import { useBackup, useSnapshots } from "../snapshots/use-snapshots";

/** What is running on an application right now. Backups and restores never run at the same time. */
export type Busy = "snapshot" | "restore" | null;

/** Everything the application page needs to back up and restore, with one shared notion of "busy". */
export function useProtection(application: string) {
  const [snapshotPages, setSnapshotPages] = useState(1);
  const [restorePages, setRestorePages] = useState(1);
  const snapshots = useSnapshots(application, snapshotPages);
  const restores = useRestores(application, restorePages);
  const backup = useBackup(application, snapshots.reload);

  const reloadSnapshots = snapshots.reload;
  const reloadRestores = restores.reload;
  const reloadBoth = useCallback(() => {
    reloadSnapshots();
    reloadRestores();
  }, [reloadSnapshots, reloadRestores]);
  const restore = useStartRestore(reloadBoth);

  const snapshotList = snapshots.state.status === "ready" ? snapshots.state.data.items : [];
  const restoreList = restores.state.status === "ready" ? restores.state.data.items : [];
  const busy: Busy = restoreList.some(isActive) ? "restore" : snapshotList.some(isActive) ? "snapshot" : null;

  return {
    snapshots,
    restores,
    backup,
    restore,
    busy,
    snapshotList,
    loadMoreSnapshots: useCallback(() => setSnapshotPages((n) => n + 1), []),
    loadMoreRestores: useCallback(() => setRestorePages((n) => n + 1), []),
  };
}
