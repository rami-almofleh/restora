import { useTranslation } from "react-i18next";
import { formatDateTime } from "../../../lib/format";
import type { RetentionPlan } from "../api";

/** How many deleted backups the list names. The rest is only counted. */
export const PLAN_LIST_LIMIT = 10;

/** The backups a retention would delete, newest first, with the reason for each. */
export function RetentionPlanList({ plan }: { plan: RetentionPlan }) {
  const { t, i18n } = useTranslation();
  const shown = plan.delete.slice(0, PLAN_LIST_LIMIT);
  const more = plan.delete.length - shown.length;

  return (
    <>
      <ul className="name-list">
        {shown.map((d) => (
          <li key={d.id}>
            <bdi>{d.application}</bdi> · {formatDateTime(d.createdAt, i18n.language)} · {t(`policies.retention.reasons.${d.reason}`, { defaultValue: d.reason })}
          </li>
        ))}
      </ul>
      {more > 0 ? <p className="muted">{t("policies.retention.more", { n: more })}</p> : null}
    </>
  );
}
