import { Database, Pencil, PlugZap, Server, Trash2 } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import { Card } from "../../../components/Card";
import type { Location } from "../api";
import { TestBadge } from "./TestBadge";
import { TestResult } from "./TestResult";

interface LocationCardProps {
  location: Location;
  /** An action on this location is running. */
  busy: boolean;
  onTest: (location: Location) => void;
  onDelete: (location: Location) => void;
}

/** One storage location: where it is, whether the keys are stored, how the last test went, and the actions. */
export function LocationCard({ location, busy, onTest, onDelete }: LocationCardProps) {
  const { t } = useTranslation();
  const { s3, nfs } = location;
  const Icon = location.type === "s3" ? Database : Server;

  return (
    <Card wide title={location.name} aside={<TestBadge state={location.test.state} />}>
      <dl className="definition-list">
        <div className="definition-list__row">
          <dt className="definition-list__term">{t("locations.card.type")}</dt>
          <dd className="definition-list__value">
            <span className="with-icon">
              <Icon size={16} aria-hidden="true" />
              {t(`locations.type.${location.type}`)}
            </span>
          </dd>
        </div>
        {s3 ? (
          <>
            <div className="definition-list__row">
              <dt className="definition-list__term">{t("locations.card.endpoint")}</dt>
              <dd className="definition-list__value">
                <bdi>{s3.endpoint}</bdi>
              </dd>
            </div>
            <div className="definition-list__row">
              <dt className="definition-list__term">{t("locations.card.bucket")}</dt>
              <dd className="definition-list__value">
                <bdi>{s3.prefix ? `${s3.bucket}/${s3.prefix}` : s3.bucket}</bdi>
              </dd>
            </div>
            <div className="definition-list__row">
              <dt className="definition-list__term">{t("locations.card.credentials")}</dt>
              <dd className="definition-list__value">{s3.credentialsSet ? t("locations.card.credentialsSet") : t("locations.card.credentialsMissing")}</dd>
            </div>
          </>
        ) : null}
        {nfs ? (
          <>
            <div className="definition-list__row">
              <dt className="definition-list__term">{t("locations.card.server")}</dt>
              <dd className="definition-list__value">
                <bdi>{nfs.server}</bdi>
              </dd>
            </div>
            <div className="definition-list__row">
              <dt className="definition-list__term">{t("locations.card.path")}</dt>
              <dd className="definition-list__value">
                <bdi>{nfs.path}</bdi>
              </dd>
            </div>
          </>
        ) : null}
        <div className="definition-list__row">
          <dt className="definition-list__term">{t("locations.card.exports")}</dt>
          <dd className="definition-list__value">{t("locations.card.exportCount", { n: location.exports })}</dd>
        </div>
      </dl>

      <TestResult test={location.test} />

      <div className="card__actions">
        <button type="button" className="button button--secondary" onClick={() => onTest(location)} disabled={busy || location.test.state === "pending"}>
          <PlugZap size={18} aria-hidden="true" />
          {t("locations.card.test")}
        </button>
        <Link className="button button--secondary" to={`/locations/${encodeURIComponent(location.id)}`}>
          <Pencil size={18} aria-hidden="true" />
          {t("locations.card.edit")}
        </Link>
        <button type="button" className="button button--secondary" onClick={() => onDelete(location)} disabled={busy}>
          <Trash2 size={18} aria-hidden="true" />
          {t("locations.card.delete")}
        </button>
      </div>
    </Card>
  );
}
