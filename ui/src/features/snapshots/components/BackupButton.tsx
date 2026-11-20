import { LoaderCircle, ShieldCheck } from "lucide-react";
import { useTranslation } from "react-i18next";

interface BackupButtonProps {
  starting: boolean;
  /** Another backup of this application is still running. */
  blocked: boolean;
  /** Id of the element that explains why the button is disabled. */
  explanationId: string;
  onClick: () => void;
}

/** The main action of the application page. Fixed above the bottom bar on phones (see .action-bar). */
export function BackupButton({ starting, blocked, explanationId, onClick }: BackupButtonProps) {
  const { t } = useTranslation();
  const disabled = starting || blocked;
  return (
    <div className="action-bar">
      <button
        type="button"
        className="button button--primary action-bar__button"
        disabled={disabled}
        aria-describedby={blocked ? explanationId : undefined}
        onClick={onClick}
      >
        {starting ? <LoaderCircle size={18} className="spin" aria-hidden="true" /> : <ShieldCheck size={18} aria-hidden="true" />}
        {starting ? t("snapshots.backup.starting") : t("snapshots.backup.button")}
      </button>
    </div>
  );
}
