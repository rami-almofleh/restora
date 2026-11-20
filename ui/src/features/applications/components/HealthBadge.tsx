import { useTranslation } from "react-i18next";
import { StatusBadge } from "../../../components/StatusBadge";
import type { Health } from "../api";

const TONES = { running: "success", degraded: "warning", stopped: "neutral" } as const;

export function HealthBadge({ health }: { health: Health }) {
  const { t } = useTranslation();
  return <StatusBadge tone={TONES[health]} label={t(`applications.health.${health}`)} />;
}
