import { useTranslation } from "react-i18next";
import type { JobPhase } from "../lib/jobs";
import { StatusBadge } from "./StatusBadge";

const TONES = { Pending: "neutral", Running: "progress", Succeeded: "success", Failed: "danger" } as const;

/** State of a backup or restore, always with icon and text. */
export function PhaseBadge({ phase }: { phase: JobPhase }) {
  const { t } = useTranslation();
  return <StatusBadge tone={TONES[phase]} label={t(`common.phase.${phase}`)} />;
}
