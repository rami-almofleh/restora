import { CloudDownload, SkipForward } from "lucide-react";
import { useCallback, useState } from "react";
import { useTranslation } from "react-i18next";
import { Card } from "../../../components/Card";
import { formatBytes, formatDateTime } from "../../../lib/format";
import { isActive } from "../../../lib/jobs";
import type { StoredBackup } from "../../exports/api";
import { ExportRestoreDialog } from "../../exports/components/ExportRestoreDialog";
import { useStoredBackups } from "../../exports/use-exports";
import { RestoreItem } from "../../restores/components/RestoreItem";
import { useRestores, useStartRestore } from "../../restores/use-restores";

interface RestoreStepProps {
  /** The application the test run backed up. */
  application: string;
  onSkip: () => void;
  onChanged: () => void;
}

/**
 * Step 4: a test restore from the storage location. A backup that was never restored is a hope, not a backup.
 * The restore replaces the volumes of the application with the trial backup, made a moment ago, and says so first.
 */
export function RestoreStep({ application, onSkip, onChanged }: RestoreStepProps) {
  const { t, i18n } = useTranslation();
  const [confirming, setConfirming] = useState<StoredBackup | null>(null);
  const stored = useStoredBackups(application, 1);
  const restores = useRestores(application, 1);
  const reloadBoth = useCallback(() => {
    restores.reload();
    onChanged();
  }, [restores, onChanged]);
  const starter = useStartRestore(reloadBoth);

  const newest = stored.state.status === "ready" ? stored.state.data.items[0] : undefined;
  const items = restores.state.status === "ready" ? restores.state.data.items.slice(0, 2) : [];
  const running = items.some(isActive);
  const failedLast = items[0] && items[0].phase === "Failed";

  const close = () => {
    starter.reset();
    setConfirming(null);
  };
  const confirm = () => {
    if (confirming) {
      void starter.startFromExport(confirming.id).then((started) => {
        if (started) setConfirming(null);
      });
    }
  };

  return (
    <>
      <p className="setup-step__intro">{t("setup.restore.intro", { application })}</p>
      <p className="notice" role="note">
        {t("setup.restore.warning", { application })}
      </p>

      {newest ? (
        <Card title={t("setup.restore.backup")}>
          <p>
            <time dateTime={newest.createdAt}>{formatDateTime(newest.createdAt, i18n.language)}</time>
            {" · "}
            <bdi>{newest.location.name}</bdi>
            {" · "}
            {formatBytes(newest.sizeBytes, i18n.language)}
          </p>
        </Card>
      ) : null}

      <div className="setup-step__actions">
        <button type="button" className="button button--primary" disabled={!newest || running} onClick={() => newest && setConfirming(newest)}>
          <CloudDownload size={18} aria-hidden="true" />
          {running ? t("setup.restore.running") : failedLast ? t("setup.restore.again") : t("setup.restore.start")}
        </button>
        <button type="button" className="button button--secondary" onClick={onSkip} disabled={running}>
          <SkipForward size={18} aria-hidden="true" />
          {t("setup.restore.skip")}
        </button>
      </div>

      {items.length > 0 ? (
        <Card title={t("setup.restore.result")}>
          <ul className="job-list">
            {items.map((restore) => (
              <RestoreItem key={restore.id} restore={restore} />
            ))}
          </ul>
        </Card>
      ) : null}

      {confirming ? <ExportRestoreDialog backup={confirming} applicationMissing={false} state={starter.state} onConfirm={confirm} onClose={close} /> : null}
    </>
  );
}
