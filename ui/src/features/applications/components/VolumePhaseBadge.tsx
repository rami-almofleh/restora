import { useTranslation } from "react-i18next";
import { StatusBadge } from "../../../components/StatusBadge";

const TONES: Record<string, "success" | "warning" | "danger"> = {
  Bound: "success",
  Pending: "warning",
  Lost: "danger",
};

export function VolumePhaseBadge({ phase }: { phase: string }) {
  const { t } = useTranslation();
  const known = phase in TONES;
  return (
    <StatusBadge
      tone={known ? TONES[phase] : "neutral"}
      label={known ? t(`applications.detail.volumes.phase.${phase}`) : phase}
    />
  );
}
