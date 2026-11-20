import { Pause, Pencil, Play, PlayCircle, Trash2, TriangleAlert } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import { Card } from "../../../components/Card";
import { StatusBadge } from "../../../components/StatusBadge";
import { formatDateTimeIn } from "../../../lib/format";
import type { Policy } from "../api";
import { describeRetention, describeSchedule, describeSelector } from "../schedule";

interface PolicyCardProps {
  policy: Policy;
  /** An action on this policy is running. */
  busy: boolean;
  /** Titles of the storage locations by id, for the line about where backups are copied to. */
  locationNames: Record<string, string>;
  onRun: (policy: Policy) => void;
  onTogglePause: (policy: Policy) => void;
  onDelete: (policy: Policy) => void;
}

function StateBadge({ policy }: { policy: Policy }) {
  const { t } = useTranslation();
  if (policy.errorCode) return <StatusBadge tone="danger" label={t("policies.state.error")} />;
  if (policy.paused) return <StatusBadge tone="neutral" label={t("policies.state.paused")} />;
  return <StatusBadge tone="success" label={t("policies.state.active")} />;
}

/** One policy: what it protects, when it runs, what the last run did, and the actions. */
export function PolicyCard({ policy, busy, locationNames, onRun, onTogglePause, onDelete }: PolicyCardProps) {
  const { t, i18n } = useTranslation();
  const language = i18n.language;
  const zone = policy.schedule.timezone || "UTC";
  const last = policy.lastRun;

  return (
    <Card wide title={policy.name} aside={<StateBadge policy={policy} />}>
      {policy.errorCode ? (
        <div className="notice notice--danger" role="alert">
          <TriangleAlert size={18} aria-hidden="true" />
          <div>
            <p>{t(`policies.errors.${policy.errorCode}.text`, { defaultValue: t("policies.errors.unknown.text") })}</p>
            <p className="muted">{t(`policies.errors.${policy.errorCode}.hint`, { defaultValue: t("policies.errors.unknown.hint") })}</p>
            <details className="details">
              <summary>{t("common.technicalDetails")}</summary>
              <code>
                {policy.errorCode}: {policy.message}
              </code>
            </details>
          </div>
        </div>
      ) : null}

      <dl className="definition-list">
        <div className="definition-list__row">
          <dt className="definition-list__term">{t("policies.card.schedule")}</dt>
          <dd className="definition-list__value">
            <bdi>{describeSchedule(t, language, policy.schedule)}</bdi>
          </dd>
        </div>
        <div className="definition-list__row">
          <dt className="definition-list__term">{t("policies.card.applications")}</dt>
          <dd className="definition-list__value">
            <bdi>{describeSelector(t, policy)}</bdi>
          </dd>
        </div>
        <div className="definition-list__row">
          <dt className="definition-list__term">{t("policies.card.export")}</dt>
          <dd className="definition-list__value">
            {policy.exportTo.length === 0 ? (
              t("policies.card.exportNone")
            ) : (
              <bdi>{policy.exportTo.map((id) => locationNames[id] ?? id).join(", ")}</bdi>
            )}
          </dd>
        </div>
        <div className="definition-list__row">
          <dt className="definition-list__term">{t("policies.card.retention")}</dt>
          <dd className="definition-list__value">
            {describeRetention(t, language, policy.retention)}
            {policy.lastCleanup
              ? ` · ${t("policies.card.cleanup", { time: formatDateTimeIn(policy.lastCleanup.at, language, zone), n: policy.lastCleanup.deleted })}`
              : null}
          </dd>
        </div>
        <div className="definition-list__row">
          <dt className="definition-list__term">{t("policies.card.next")}</dt>
          <dd className="definition-list__value">
            {policy.nextRunAt ? formatDateTimeIn(policy.nextRunAt, language, zone) : policy.paused ? t("policies.card.pausedNext") : "–"}
          </dd>
        </div>
        <div className="definition-list__row">
          <dt className="definition-list__term">{t("policies.card.last")}</dt>
          <dd className="definition-list__value">
            {last
              ? `${formatDateTimeIn(last.at, language, zone)} · ${t("policies.card.lastResult", { started: last.snapshots.length, skipped: last.skipped.length })}`
              : t("policies.card.never")}
          </dd>
        </div>
      </dl>

      {policy.missing.length > 0 ? (
        <p className="notice" role="status">
          {t("policies.card.missing", { names: policy.missing.join(", ") })}
        </p>
      ) : null}

      {policy.selector.mode !== "applications" && policy.applications.length === 0 ? (
        <p className="notice" role="status">
          {t("policies.card.noMatch")}
        </p>
      ) : null}

      {last && last.skipped.length > 0 ? (
        <details className="details">
          <summary>{t("policies.card.skipped", { n: last.skipped.length })}</summary>
          <ul className="name-list">
            {last.skipped.map((s) => (
              <li key={s.application}>
                <bdi>{s.application}</bdi>: {t(`policies.skipped.${s.reason}`, { defaultValue: t("policies.skipped.request_failed") })}
              </li>
            ))}
          </ul>
        </details>
      ) : null}

      <div className="card__actions">
        <button type="button" className="button button--secondary" onClick={() => onRun(policy)} disabled={busy || policy.applications.length === 0}>
          <PlayCircle size={18} aria-hidden="true" />
          {t("policies.card.run")}
        </button>
        <button type="button" className="button button--secondary" onClick={() => onTogglePause(policy)} disabled={busy}>
          {policy.paused ? <Play size={18} aria-hidden="true" /> : <Pause size={18} aria-hidden="true" />}
          {policy.paused ? t("policies.card.resume") : t("policies.card.pause")}
        </button>
        <Link className="button button--secondary" to={`/policies/${encodeURIComponent(policy.id)}`}>
          <Pencil size={18} aria-hidden="true" />
          {t("policies.card.edit")}
        </Link>
        <button type="button" className="button button--secondary" onClick={() => onDelete(policy)} disabled={busy}>
          <Trash2 size={18} aria-hidden="true" />
          {t("policies.card.delete")}
        </button>
      </div>
    </Card>
  );
}

