import { useTranslation } from "react-i18next";
import { StatusBadge } from "../../../components/StatusBadge";
import type { TestState } from "../api";

const TONES = { pending: "progress", succeeded: "success", failed: "danger" } as const;

/** Result of the connection test, always with icon and text. */
export function TestBadge({ state }: { state: TestState }) {
  const { t } = useTranslation();
  return <StatusBadge tone={TONES[state]} label={t(`locations.test.${state}`)} />;
}
