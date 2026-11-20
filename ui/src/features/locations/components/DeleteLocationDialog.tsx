import { LoaderCircle, TriangleAlert } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Dialog } from "../../../components/Dialog";
import type { ApiError } from "../../../lib/api";
import type { Location } from "../api";

interface DeleteLocationDialogProps {
  location: Location;
  deleting: boolean;
  error?: ApiError;
  onConfirm: () => void;
  onClose: () => void;
}

/** Says what deleting does. Cancel comes first and holds the focus. */
export function DeleteLocationDialog({ location, deleting, error, onConfirm, onClose }: DeleteLocationDialogProps) {
  const { t } = useTranslation();
  return (
    <Dialog
      title={t("locations.delete.title")}
      onClose={onClose}
      footer={
        <>
          <button type="button" className="button button--secondary" onClick={onClose}>
            {t("locations.delete.cancel")}
          </button>
          <button type="button" className="button button--danger" onClick={onConfirm} disabled={deleting}>
            {deleting ? <LoaderCircle size={18} className="spin" aria-hidden="true" /> : null}
            {t("locations.delete.confirm")}
          </button>
        </>
      }
    >
      <p>{t("locations.delete.intro", { name: location.name })}</p>
      <ul className="consequences">
        <li>{t("locations.delete.forgets")}</li>
        <li>{t("locations.delete.keeps")}</li>
        {location.exports > 0 ? <li>{t("locations.delete.exports", { n: location.exports })}</li> : null}
      </ul>
      {error ? (
        <div className="notice notice--danger" role="alert">
          <TriangleAlert size={18} aria-hidden="true" />
          <div>
            <p>{t(`locations.delete.errors.${error.code}`, { defaultValue: t("locations.delete.failed") })}</p>
            <p className="muted">{error.hint ?? error.message}</p>
          </div>
        </div>
      ) : null}
    </Dialog>
  );
}
